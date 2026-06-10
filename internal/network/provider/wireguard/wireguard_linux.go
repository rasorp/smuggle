package wireguard

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/vishvananda/netlink"
	"go.uber.org/zap"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"github.com/rasorp/smuggle/internal/types"
)

// Provider implements types.NetworkProvider using WireGuard as the overlay
// transport. Each network gets one WireGuard interface per host (named
// <network>0). Peers are added/removed via wgctrl as remote subnets are
// learnt from the store.
type Provider struct {

	// dataDir is the path where per-network private key files are persisted.
	dataDir string

	logger *zap.Logger

	// networkConfigs stores the network operator configurations keyed by
	// network name. The configs are used when processing remote subnet updates
	// to determine network wide peer configuration parameters like persistent
	// keepalive.
	networkConfigs     map[string]*OperatorConfig
	networkConfigsLock sync.RWMutex
}

func New(logger *zap.Logger, dataDir string) types.NetworkProvider {
	return &Provider{
		dataDir:        dataDir,
		logger:         logger.Named(ProviderName),
		networkConfigs: make(map[string]*OperatorConfig),
	}
}

func (p *Provider) Name() string { return ProviderName }

// SetLocal creates or reconciles the local WireGuard interface for the given
// subnet, loads or generates the host's private key, and publishes the derived
// public key in the returned subnet config so remote peers can configure their
// peer entries via SetRemote.
func (p *Provider) SetLocal(
	req *types.NetworkProviderSetReq,
) (*types.NetworkProviderSetResp, error) {

	var opCfg OperatorConfig

	if req.Client.Config != nil {
		if err := json.Unmarshal(req.Client.Config, &opCfg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal WireGuard config: %w", err)
		}
	}

	if err := opCfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid WireGuard config: %w", err)
	}

	// Always recompute MTU from the current host interface. The stored config
	// may contain a stale value from a previous run where the host MTU was
	// different.
	mtu := req.HostInterface.MTU - encapsulationOverhead

	// The network name is used to generate the WireGuard interface name and is
	// validated when the client reads the network, so we do not need to check it
	// here.
	ifName := req.Client.InterfaceName()

	if err := p.ensureLink(ifName, mtu); err != nil {
		return nil, err
	}

	// Load the existing private key from disk, or generate and persist a new
	// one. A stable key is critical; if the key changed across restarts, all
	// remote peers would hold a stale public key and handshakes would silently
	// fail until they received the updated subnet config from the store.
	keyPath := filepath.Join(p.dataDir, "wireguard", req.Client.NetworkName+".key")

	privateKey, err := loadOrGenerateKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load WireGuard key: %w", err)
	}

	listenPort := opCfg.ListenPort

	wgc, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create WireGuard client: %w", err)
	}
	defer func() { _ = wgc.Close() }()

	// Configure the WireGuard device with the private key and requested listen
	// port. It would be possible to fallback to ephemeral port assignment if
	// the requested port is unavailable, but the operator would have requested
	// this port for a reason, so it's better to fail loudly and let them fix
	// the config rather than continue with a configuration they didn't ask for.
	if err := wgc.ConfigureDevice(ifName, wgtypes.Config{
		PrivateKey: &privateKey,
		ListenPort: &listenPort,
	}); err != nil {
		return nil, fmt.Errorf("failed to configure WireGuard device: %w", err)
	}

	link, err := netlink.LinkByName(ifName)
	if err != nil {
		return nil, fmt.Errorf("failed to find WireGuard link: %w", err)
	}

	if err := netlink.LinkSetUp(link); err != nil {
		return nil, fmt.Errorf("failed to bring link up: %w", err)
	}

	// Read back the device state to get the actual listen port. When the
	// operator set value is 0, the kernel assigns a free ephemeral port. This
	// must happen AFTER LinkSetUp: WireGuard only binds its UDP socket when
	// the interface transitions to up, so reading the port before that always
	// returns 0, causing every remote peer to configure endpoint :0.
	dev, err := wgc.Device(ifName)
	if err != nil {
		return nil, fmt.Errorf("failed to read WireGuard device state for %s: %w", ifName, err)
	}

	// Assign the local overlay gateway IP as a /32 to the WireGuard interface.
	// Remote peers set their allowed IPs to the full local subnet, so any
	// source within that range, including this /32, is accepted through the
	// tunnel.
	gatewayAddr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   req.Client.IPv4Network.NextAddr().IP.ToNetIP().To4(),
			Mask: net.CIDRMask(32, 32),
		},
	}
	if err := netlink.AddrAdd(link, gatewayAddr); err != nil && !errors.Is(err, syscall.EEXIST) {
		return nil, fmt.Errorf("failed to assign gateway IP to WireGuard interface: %w", err)
	}

	if err := p.configureSysctl(ifName, req.HostInterface.Name); err != nil {
		return nil, fmt.Errorf("failed to configure sysctl for WireGuard interface: %w", err)
	}

	// Build the peer config.
	peerCfg := PeerConfig{
		ListenPort: dev.ListenPort,
		PublicKey:  privateKey.PublicKey().String(),
	}

	marshaledCfg, err := json.Marshal(peerCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal wireguard peer config: %w", err)
	}

	p.logger.Info("successfully set up local WireGuard interface", peerCfg.loggingPairs()...)

	// Store the operator config for this network so we can apply it when
	// processing remote subnet updates.
	p.setNetworkConfig(req.Client.NetworkName, &opCfg)

	respSubnet := req.Client.Copy()
	respSubnet.Config = marshaledCfg

	return &types.NetworkProviderSetResp{Network: respSubnet}, nil
}

// SetRemote adds a WireGuard peer entry for the remote host and installs a
// route to its overlay subnet. Unlike VXLAN, WireGuard handles encapsulation
// entirely in the kernel once the peer is configured, so there is no FDB, ARP,
// or policy routing required.
func (p *Provider) SetRemote(
	req *types.NetworkProviderSetRemoteReq,
) (*types.NetworkProviderSetRemoteResp, error) {

	var cfg PeerConfig
	if req.Subnet.Config != nil {
		if err := json.Unmarshal(req.Subnet.Config, &cfg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal WireGuard peer config: %w", err)
		}
	}

	pubKey, err := wgtypes.ParseKey(cfg.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse remote public key: %w", err)
	}

	if req.Subnet.HostIPv4 == nil {
		return nil, errors.New("remote subnet has no host IPv4 address")
	}

	endpoint := &net.UDPAddr{
		IP:   *req.Subnet.HostIPv4,
		Port: cfg.ListenPort,
	}

	allowedIP := *req.Subnet.IPv4Network.ToIPNet()

	peer := wgtypes.PeerConfig{
		PublicKey:         pubKey,
		Endpoint:          endpoint,
		AllowedIPs:        []net.IPNet{allowedIP},
		ReplaceAllowedIPs: true,
	}

	// Apply the operator-configured persistent keepalive if set. This is
	// required for peers behind NAT to maintain the UDP mapping, so it's
	// important to apply it if the operator requested it.
	if opCfg := p.getNetworkConfig(req.Subnet.NetworkName); opCfg != nil && opCfg.PersistentKeepalive > 0 {
		ka := time.Duration(opCfg.PersistentKeepalive) * time.Second
		peer.PersistentKeepaliveInterval = &ka
	}

	ifName := req.Subnet.InterfaceName()

	wgc, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create WireGuard client: %w", err)
	}
	defer func() { _ = wgc.Close() }()

	if err := wgc.ConfigureDevice(ifName, wgtypes.Config{
		Peers: []wgtypes.PeerConfig{peer},
	}); err != nil {
		return nil, fmt.Errorf("failed to add WireGuard peer: %w", err)
	}

	// Add a kernel route to the remote overlay subnet via the WireGuard
	// interface. SCOPE_LINK tells the kernel the destination is directly
	// reachable on this interface. WireGuard handles all encapsulation
	// internally once the peer entry is in place. Source address selection for
	// host-originated traffic is covered by the /32 gateway IP assigned to the
	// interface.
	link, err := netlink.LinkByName(ifName)
	if err != nil {
		return nil, fmt.Errorf("failed to find WireGuard link: %w", err)
	}

	route := &netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       req.Subnet.IPv4Network.ToIPNet(),
		Scope:     netlink.SCOPE_LINK,
	}

	if err := netlink.RouteReplace(route); err != nil {
		return nil, fmt.Errorf("failed to add remote subnet route: %w", err)
	}

	p.logger.Info("successfully configured WireGuard peer",
		zap.String("public_key", cfg.PublicKey),
		zap.String("endpoint", endpoint.String()),
		zap.String("allowed_ips", allowedIP.String()),
	)

	return &types.NetworkProviderSetRemoteResp{}, nil
}

// DeleteRemote removes the WireGuard peer entry for the remote host and tears
// down its overlay route. Peer removal is keyed on the public key, so no
// interface-level teardown is needed.
func (p *Provider) DeleteRemote(
	req *types.NetworkProviderDeleteRemoteReq,
) (*types.NetworkProviderDeleteRemoteResp, error) {

	var cfg PeerConfig
	if req.Subnet.Config != nil {
		if err := json.Unmarshal(req.Subnet.Config, &cfg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal WireGuard peer config: %w", err)
		}
	}

	pubKey, err := wgtypes.ParseKey(cfg.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse remote public key: %w", err)
	}

	ifName := req.Subnet.InterfaceName()

	wgc, err := wgctrl.New()
	if err != nil {
		return nil, fmt.Errorf("failed to create WireGuard client: %w", err)
	}
	defer func() { _ = wgc.Close() }()

	if err := wgc.ConfigureDevice(ifName, wgtypes.Config{
		Peers: []wgtypes.PeerConfig{{
			PublicKey: pubKey,
			Remove:    true,
		}},
	}); err != nil {
		return nil, fmt.Errorf("failed to remove WireGuard peer: %w", err)
	}

	link, err := netlink.LinkByName(ifName)
	if err != nil {
		return nil, fmt.Errorf("failed to find WireGuard link: %w", err)
	}

	route := &netlink.Route{
		LinkIndex: link.Attrs().Index,
		Dst:       req.Subnet.IPv4Network.ToIPNet(),
		Scope:     netlink.SCOPE_LINK,
	}

	if err := netlink.RouteDel(route); err != nil && !errors.Is(err, syscall.ESRCH) {
		return nil, fmt.Errorf("failed to delete remote subnet route: %w", err)
	}

	p.logger.Info("successfully removed WireGuard peer", zap.String("public_key", cfg.PublicKey))

	return &types.NetworkProviderDeleteRemoteResp{}, nil
}

// ensureLink creates the WireGuard netlink interface with the given name and
// MTU. If an interface with the same name already exists and is a WireGuard
// device, the call is a no-op. If the existing interface is of a different
// type, an error is returned rather than silently replacing it.
func (p *Provider) ensureLink(name string, mtu int) error {

	la := netlink.NewLinkAttrs()
	la.Name = name
	la.MTU = mtu

	wg := &netlink.GenericLink{
		LinkAttrs: la,
		LinkType:  "wireguard",
	}

	if err := netlink.LinkAdd(wg); err != nil {
		if !errors.Is(err, syscall.EEXIST) {
			return fmt.Errorf("failed to create WireGuard interface: %w", err)
		}

		existing, err := netlink.LinkByName(name)
		if err != nil {
			return fmt.Errorf("failed to find existing link: %w", err)
		}

		if existing.Type() != "wireguard" {
			return fmt.Errorf("link exists but is type %q, not wireguard", existing.Type())
		}

		// Reconcile MTU in case the host interface MTU changed since the link
		// was originally created.
		if err := netlink.LinkSetMTU(existing, mtu); err != nil {
			return fmt.Errorf("failed to update MTU on existing WireGuard interface: %w", err)
		}

		p.logger.Debug("WireGuard interface already exists, reusing", zap.String("name", name))
	}

	return nil
}

// setNetworkConfig stores the operator configuration for a network in memory,
// keyed by network name.
func (p *Provider) setNetworkConfig(networkName string, cfg *OperatorConfig) {
	p.networkConfigsLock.Lock()
	defer p.networkConfigsLock.Unlock()
	p.networkConfigs[networkName] = cfg
}

// getNetworkConfig retrieves the operator configuration for a network from
// memory. It is the callers responsibility to check the returned object, which
// may be nil if no config has been stored for this network.
func (p *Provider) getNetworkConfig(networkName string) *OperatorConfig {
	p.networkConfigsLock.RLock()
	defer p.networkConfigsLock.RUnlock()
	return p.networkConfigs[networkName]
}
