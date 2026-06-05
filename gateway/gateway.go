package main

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	"github.com/coreos/go-iptables/iptables"
	"github.com/vishvananda/netlink"
	"go.uber.org/zap"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// Gateway manages the customer-facing WireGuard interface (wg-gw0), the
// in-memory peer registry, and the iptables rules that forward and masquerade
// customer traffic into the Smuggle overlay.
type Gateway struct {
	cfg      *Config
	logger   *zap.Logger
	registry *PeerRegistry

	// publicKey is the base64-encoded WireGuard public key for wg-gw0. It is
	// set during Start and returned to customers in the API response.
	publicKey string

	ipt          *iptables.IPTables
	tunnelSubnet *net.IPNet
	overlayCIDR  *net.IPNet
}

// NewGateway validates the CIDRs in cfg and initialises the Gateway. It does
// not yet touch the network — call Start for that.
func NewGateway(cfg *Config, logger *zap.Logger) (*Gateway, error) {
	_, tunnelSubnet, err := net.ParseCIDR(cfg.TunnelSubnet)
	if err != nil {
		return nil, fmt.Errorf("failed to parse tunnel subnet: %w", err)
	}

	_, overlayCIDR, err := net.ParseCIDR(cfg.OverlayCIDR)
	if err != nil {
		return nil, fmt.Errorf("failed to parse overlay CIDR: %w", err)
	}

	ipt, err := iptables.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialise iptables: %w", err)
	}

	return &Gateway{
		cfg:          cfg,
		logger:       logger.Named("gateway"),
		registry:     NewPeerRegistry(tunnelSubnet),
		ipt:          ipt,
		tunnelSubnet: tunnelSubnet,
		overlayCIDR:  overlayCIDR,
	}, nil
}

// Start brings up the WireGuard interface, assigns the gateway IP, loads or
// generates the keypair, enables forwarding, and installs the base iptables
// rules. It must be called before the HTTP server accepts requests.
func (g *Gateway) Start() error {
	if err := g.ensureLink(); err != nil {
		return err
	}
	if err := g.assignGatewayIP(); err != nil {
		return err
	}
	pubKey, err := g.configureWireGuard()
	if err != nil {
		return err
	}
	g.publicKey = pubKey

	if err := g.bringUp(); err != nil {
		return err
	}
	if err := g.enableForwarding(); err != nil {
		return err
	}
	if err := g.setupBaseIPTables(); err != nil {
		return err
	}

	g.logger.Info("gateway started",
		zap.String("wg_iface", g.cfg.WGIface),
		zap.String("public_key", g.publicKey),
		zap.String("endpoint", fmt.Sprintf("%s:%d", g.cfg.EndpointIP, g.cfg.WGPort)),
		zap.String("tunnel_subnet", g.cfg.TunnelSubnet),
		zap.String("overlay_cidr", g.cfg.OverlayCIDR),
	)
	return nil
}

// Stop removes all registered peers, tears down the base iptables rules, and
// deletes the WireGuard interface. Errors from individual peer removals are
// logged but do not abort the shutdown sequence.
func (g *Gateway) Stop() error {
	g.logger.Info("stopping gateway")

	for _, peer := range g.registry.All() {
		if err := g.removePeer(peer.PublicKey, peer.TunnelIP); err != nil {
			g.logger.Warn("failed to remove peer during shutdown",
				zap.String("public_key", peer.PublicKey),
				zap.Error(err),
			)
		}
	}

	g.cleanupBaseIPTables()

	link, err := netlink.LinkByName(g.cfg.WGIface)
	if err != nil {
		if !errors.Is(err, syscall.ENODEV) {
			g.logger.Warn("could not find WireGuard interface for deletion",
				zap.String("iface", g.cfg.WGIface),
				zap.Error(err),
			)
		}
		return nil
	}
	if err := netlink.LinkDel(link); err != nil {
		g.logger.Warn("failed to delete WireGuard interface",
			zap.String("iface", g.cfg.WGIface),
			zap.Error(err),
		)
	}

	g.logger.Info("gateway stopped")
	return nil
}

// addPeer adds a WireGuard peer entry for publicKey with tunnelIP/32 as its
// AllowedIPs, then installs a per-peer MASQUERADE rule so traffic from
// tunnelIP is rewritten to the gateway's overlay IP before entering smuggle0.
func (g *Gateway) addPeer(publicKey string, tunnelIP net.IP) error {
	pubKey, err := wgtypes.ParseKey(publicKey)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	wgc, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("failed to create wgctrl client: %w", err)
	}
	defer wgc.Close()

	if err := wgc.ConfigureDevice(g.cfg.WGIface, wgtypes.Config{
		Peers: []wgtypes.PeerConfig{
			{
				PublicKey: pubKey,
				AllowedIPs: []net.IPNet{
					{IP: cloneIP(tunnelIP), Mask: net.CIDRMask(32, 32)},
				},
				ReplaceAllowedIPs: true,
			},
		},
	}); err != nil {
		return fmt.Errorf("failed to add WireGuard peer: %w", err)
	}

	spec := g.masqSpec(tunnelIP)
	exists, err := g.ipt.Exists("nat", "POSTROUTING", spec...)
	if err != nil {
		return fmt.Errorf("failed to check MASQUERADE rule: %w", err)
	}
	if !exists {
		if err := g.ipt.Append("nat", "POSTROUTING", spec...); err != nil {
			return fmt.Errorf("failed to add MASQUERADE rule: %w", err)
		}
	}

	g.logger.Info("added WireGuard peer",
		zap.String("public_key", publicKey),
		zap.String("tunnel_ip", tunnelIP.String()),
	)
	return nil
}

// removePeer removes the WireGuard peer entry for publicKey and deletes its
// MASQUERADE rule. A missing iptables rule is logged but not treated as an
// error so that shutdown is as clean as possible.
func (g *Gateway) removePeer(publicKey string, tunnelIP net.IP) error {
	pubKey, err := wgtypes.ParseKey(publicKey)
	if err != nil {
		return fmt.Errorf("failed to parse public key: %w", err)
	}

	wgc, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("failed to create wgctrl client: %w", err)
	}
	defer wgc.Close()

	if err := wgc.ConfigureDevice(g.cfg.WGIface, wgtypes.Config{
		Peers: []wgtypes.PeerConfig{
			{PublicKey: pubKey, Remove: true},
		},
	}); err != nil {
		return fmt.Errorf("failed to remove WireGuard peer: %w", err)
	}

	spec := g.masqSpec(tunnelIP)
	if err := g.ipt.DeleteIfExists("nat", "POSTROUTING", spec...); err != nil {
		g.logger.Warn("failed to delete MASQUERADE rule",
			zap.String("tunnel_ip", tunnelIP.String()),
			zap.Error(err),
		)
	}

	g.logger.Info("removed WireGuard peer",
		zap.String("public_key", publicKey),
		zap.String("tunnel_ip", tunnelIP.String()),
	)
	return nil
}

// ── private helpers ───────────────────────────────────────────────────────────

func (g *Gateway) ensureLink() error {
	la := netlink.NewLinkAttrs()
	la.Name = g.cfg.WGIface

	wg := &netlink.GenericLink{
		LinkAttrs: la,
		LinkType:  "wireguard",
	}

	if err := netlink.LinkAdd(wg); err != nil {
		if !errors.Is(err, syscall.EEXIST) {
			return fmt.Errorf("failed to create WireGuard interface %s: %w", g.cfg.WGIface, err)
		}

		existing, err := netlink.LinkByName(g.cfg.WGIface)
		if err != nil {
			return fmt.Errorf("failed to look up existing interface %q: %w", g.cfg.WGIface, err)
		}
		if existing.Type() != "wireguard" {
			return fmt.Errorf("interface %q already exists but is type %q, not wireguard",
				g.cfg.WGIface, existing.Type())
		}

		g.logger.Debug("WireGuard interface already exists, reusing",
			zap.String("iface", g.cfg.WGIface))
	}
	return nil
}

func (g *Gateway) assignGatewayIP() error {
	link, err := netlink.LinkByName(g.cfg.WGIface)
	if err != nil {
		return fmt.Errorf("failed to find interface %q: %w", g.cfg.WGIface, err)
	}

	// The gateway takes the first host address in the tunnel subnet.
	gatewayIP := make(net.IP, 4)
	copy(gatewayIP, g.tunnelSubnet.IP.To4())
	incrementIP(gatewayIP)

	addr := &netlink.Addr{
		IPNet: &net.IPNet{
			IP:   gatewayIP,
			Mask: g.tunnelSubnet.Mask,
		},
	}
	if err := netlink.AddrAdd(link, addr); err != nil && !errors.Is(err, syscall.EEXIST) {
		return fmt.Errorf("failed to assign gateway IP to %s: %w", g.cfg.WGIface, err)
	}

	g.logger.Debug("assigned gateway tunnel IP",
		zap.String("ip", gatewayIP.String()),
		zap.String("iface", g.cfg.WGIface),
	)
	return nil
}

func (g *Gateway) configureWireGuard() (string, error) {
	var privateKey wgtypes.Key

	data, err := os.ReadFile(g.cfg.WGKeyFile)
	if err != nil {
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("failed to read WireGuard key file %s: %w", g.cfg.WGKeyFile, err)
		}

		privateKey, err = wgtypes.GeneratePrivateKey()
		if err != nil {
			return "", fmt.Errorf("failed to generate WireGuard private key: %w", err)
		}
		if err := os.WriteFile(g.cfg.WGKeyFile, []byte(privateKey.String()), 0600); err != nil {
			return "", fmt.Errorf("failed to write WireGuard key file %s: %w", g.cfg.WGKeyFile, err)
		}
		g.logger.Info("generated new WireGuard keypair", zap.String("key_file", g.cfg.WGKeyFile))
	} else {
		privateKey, err = wgtypes.ParseKey(string(data))
		if err != nil {
			return "", fmt.Errorf("failed to parse WireGuard private key from %s: %w", g.cfg.WGKeyFile, err)
		}
		g.logger.Info("loaded existing WireGuard keypair", zap.String("key_file", g.cfg.WGKeyFile))
	}

	wgc, err := wgctrl.New()
	if err != nil {
		return "", fmt.Errorf("failed to create wgctrl client: %w", err)
	}
	defer wgc.Close()

	listenPort := g.cfg.WGPort
	if err := wgc.ConfigureDevice(g.cfg.WGIface, wgtypes.Config{
		PrivateKey: &privateKey,
		ListenPort: &listenPort,
	}); err != nil {
		return "", fmt.Errorf("failed to configure WireGuard device %s: %w", g.cfg.WGIface, err)
	}

	return privateKey.PublicKey().String(), nil
}

func (g *Gateway) bringUp() error {
	link, err := netlink.LinkByName(g.cfg.WGIface)
	if err != nil {
		return fmt.Errorf("failed to find interface %q: %w", g.cfg.WGIface, err)
	}
	if err := netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("failed to bring up %s: %w", g.cfg.WGIface, err)
	}
	return nil
}

func (g *Gateway) enableForwarding() error {
	const path = "/proc/sys/net/ipv4/ip_forward"
	if err := os.WriteFile(path, []byte("1"), 0644); err != nil {
		return fmt.Errorf("failed to enable IP forwarding via %s: %w", path, err)
	}
	return nil
}

func (g *Gateway) setupBaseIPTables() error {
	type rule struct {
		table string
		chain string
		spec  []string
	}

	rules := []rule{
		{
			table: "filter",
			chain: "FORWARD",
			spec:  []string{"-i", g.cfg.WGIface, "-o", g.cfg.OverlayIface, "-j", "ACCEPT"},
		},
		{
			table: "filter",
			chain: "FORWARD",
			spec:  []string{"-i", g.cfg.OverlayIface, "-o", g.cfg.WGIface, "-j", "ACCEPT"},
		},
		{
			table: "filter",
			chain: "FORWARD",
			spec:  []string{"-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
		},
	}

	for _, r := range rules {
		exists, err := g.ipt.Exists(r.table, r.chain, r.spec...)
		if err != nil {
			return fmt.Errorf("failed to check iptables rule: %w", err)
		}
		if !exists {
			if err := g.ipt.Append(r.table, r.chain, r.spec...); err != nil {
				return fmt.Errorf("failed to install iptables rule: %w", err)
			}
		}
	}
	return nil
}

func (g *Gateway) cleanupBaseIPTables() {
	type rule struct {
		table string
		chain string
		spec  []string
	}

	rules := []rule{
		{
			table: "filter",
			chain: "FORWARD",
			spec:  []string{"-i", g.cfg.WGIface, "-o", g.cfg.OverlayIface, "-j", "ACCEPT"},
		},
		{
			table: "filter",
			chain: "FORWARD",
			spec:  []string{"-i", g.cfg.OverlayIface, "-o", g.cfg.WGIface, "-j", "ACCEPT"},
		},
		{
			table: "filter",
			chain: "FORWARD",
			spec:  []string{"-m", "conntrack", "--ctstate", "RELATED,ESTABLISHED", "-j", "ACCEPT"},
		},
	}

	for _, r := range rules {
		if err := g.ipt.DeleteIfExists(r.table, r.chain, r.spec...); err != nil {
			g.logger.Warn("failed to remove base iptables rule", zap.Error(err))
		}
	}
}

// masqSpec builds the iptables spec for the per-peer MASQUERADE rule.
func (g *Gateway) masqSpec(tunnelIP net.IP) []string {
	return []string{
		"-s", tunnelIP.String() + "/32",
		"-o", g.cfg.OverlayIface,
		"-j", "MASQUERADE",
	}
}
