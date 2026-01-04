package vxlan

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"syscall"

	"github.com/vishvananda/netlink"
	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/helper/retry"
	"github.com/rasorp/smuggle/internal/types"
)

const (
	providerName = "vxlan"

	// defaultVNI is the default VXLAN Network Identifier used if none is
	// specified by the operator.
	defaultVNI = 1

	// defaultPort is the default UDP port used for VXLAN traffic.
	defaultPort = 4789

	// vxlanEncapuslationOverhead is the overhead in bytes introduced by VXLAN
	// encapsulation. This is used to adjust the MTU of the VXLAN interface to
	// avoid fragmentation.
	vxlanEncapuslationOverhead = 50
)

type Provider struct {
	logger *zap.Logger
}

func New(logger *zap.Logger) types.NetworkProvider {
	return &Provider{
		logger: logger.Named(providerName),
	}
}

func (p *Provider) Name() string { return providerName }

func (p *Provider) SetLocal(
	req *types.NetworkProviderSetReq,
) (*types.NetworkProviderSetResp, error) {

	cfg := Config{
		VNI:  defaultVNI,
		Port: defaultPort,
		MTU:  req.HostInteface.MTU - vxlanEncapuslationOverhead,
	}

	if req.Client.Config != nil {
		if err := json.Unmarshal(req.Client.Config, &cfg); err != nil {
			return nil, err
		}
	}

	vxlanLink, err := p.createIPv4(req.Client, &cfg, req.HostInteface.Index)
	if err != nil {
		return nil, err
	}

	// Store the VXLAN interface's MAC address in the config. This will be used
	// by remote hosts to set up FDB and ARP entries for this subnet.
	//
	// TODO(jrasell): There is currently a race condition in the netlink library
	// that can result in the MAC address being incorrect immediately after
	// creation. We may need to implement a retry here to ensure we get the
	// correct address. https://github.com/vishvananda/netlink/issues/993
	cfg.VtepMAC = vxlanLink.Attrs().HardwareAddr.String()
	if cfg.VtepMAC == "" {
		return nil, errors.New("VXLAN interface MAC address is empty")
	}

	marshaledCfg, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal vxlan config: %v", err)
	}

	p.logger.Info("setup local VXLAN interface", cfg.loggingPairs()...)

	// Create a copy of the subnet to avoid mutating the request object and
	// ensure we don't accidentally modify the caller's data.
	respSubnet := req.Client.Copy()
	respSubnet.Config = marshaledCfg

	return &types.NetworkProviderSetResp{Network: respSubnet}, nil
}

func (p *Provider) DeleteRemote(
	req *types.NetworkProviderDeleteRemoteReq,
) (*types.NetworkProviderDeleteRemoteResp, error) {

	// Parse the provider config to get the VNI and VTEP MAC address.
	var cfg Config

	if req.Subnet.Config != nil {
		if err := json.Unmarshal(req.Subnet.Config, &cfg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %w", err)
		}
	} else {
		cfg.VNI = defaultVNI
	}

	// Get the VXLAN link by name
	name := req.Subnet.InterfaceName()

	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("failed to find VXLAN link: %w", err)
	}

	vxlan, ok := link.(*netlink.Vxlan)
	if !ok {
		return nil, fmt.Errorf("link %q is not a VXLAN interface", name)
	}

	// Get the remote VTEP MAC address for the subnet config.
	hwAddr, err := net.ParseMAC(cfg.VtepMAC)
	if err != nil {
		return nil, fmt.Errorf("failed to parse MAC address: %w", err)
	}

	gatewayIP := req.Subnet.IPv4Network.NextAddr().IP.ToNetIP()
	route := &netlink.Route{
		LinkIndex: vxlan.Index,
		Dst:       req.Subnet.IPv4Network.ToIPNet(),
		Gw:        gatewayIP,
		Scope:     netlink.SCOPE_UNIVERSE,
	}
	route.SetFlag(syscall.RTNH_F_ONLINK)

	if err := retry.Retry(func() error {
		err := netlink.RouteDel(route)
		if err != nil {
			p.logger.Warn("failed to delete link route", zap.Error(err))
			return err
		}
		return err
	}); err != nil {
		return nil, err
	}

	arpEntry := netlink.Neigh{
		LinkIndex:    vxlan.Index,
		Family:       syscall.AF_INET,
		State:        netlink.NUD_PERMANENT,
		IP:           gatewayIP,
		HardwareAddr: hwAddr,
	}

	if err := retry.Retry(func() error {
		if err := netlink.NeighDel(&arpEntry); err != nil {
			p.logger.Warn("failed to delete ARP entry", zap.Error(err))
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}

	fdbEntry := netlink.Neigh{
		LinkIndex:    vxlan.Index,
		Family:       syscall.AF_BRIDGE,
		State:        netlink.NUD_PERMANENT,
		Flags:        netlink.NTF_SELF,
		IP:           *req.Subnet.HostIPv4,
		HardwareAddr: hwAddr,
	}

	if err := retry.Retry(func() error {
		err := netlink.NeighDel(&fdbEntry)
		if err != nil {
			p.logger.Warn("failed to delete FDB entry", zap.Error(err))
			return err
		}
		return err
	}); err != nil {
		return nil, err
	}

	return &types.NetworkProviderDeleteRemoteResp{}, nil
}

func (p *Provider) SetRemote(
	req *types.NetworkProviderSetRemoteReq,
) (*types.NetworkProviderSetRemoteResp, error) {

	// Parse the provider config to get the VNI.
	var cfg Config
	if req.Subnet.Config != nil {
		if err := json.Unmarshal(req.Subnet.Config, &cfg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal vxlan config: %w", err)
		}
	} else {
		cfg.VNI = 1
	}

	// Pull the VXLAN link by name, so we can add route, FDB and ARP entries to
	// it.
	name := req.Subnet.InterfaceName()

	link, err := netlink.LinkByName(name)
	if err != nil {
		return nil, fmt.Errorf("failed to find vxlan link %s: %w", name, err)
	}

	vxlan, ok := link.(*netlink.Vxlan)
	if !ok {
		return nil, fmt.Errorf("link %s is not a vxlan interface", name)
	}

	// Get the remote VTEP MAC address from the subnet config. This is the
	// actual MAC address of the remote host's VXLAN interface.
	hwAddr, err := net.ParseMAC(cfg.VtepMAC)
	if err != nil {
		return nil, fmt.Errorf("failed to parse VTEP MAC address: %w", err)
	}

	// Add FDB entry that maps the MAC address to the remote VTEP IP. This tells
	// the VXLAN interface where to send packets destined for this MAC.
	fdbEntry := netlink.Neigh{
		LinkIndex:    vxlan.Index,
		Family:       syscall.AF_BRIDGE,
		State:        netlink.NUD_PERMANENT,
		Flags:        netlink.NTF_SELF,
		IP:           *req.Subnet.HostIPv4,
		HardwareAddr: hwAddr,
	}

	if err := retry.Retry(func() error {
		err := netlink.NeighSet(&fdbEntry)
		if err != nil {
			p.logger.Warn("failed to add FDB entry", zap.Error(err))
			return err
		}
		return err
	}); err != nil {
		return nil, err
	}

	// The gateway IP is the first usable IP of the remote subnet and used in a
	// couple of places below, so extract it here.
	gatewayIP := req.Subnet.IPv4Network.NextAddr().IP.ToNetIP()

	// Add a neighbor ARP entry that maps the remote gateway IP to the remote
	// VTEP MAC. Gateway is the first usable IP of the remote subnet. This tells
	// the kernel the MAC address to use when sending to the gateway IP.
	arpEntry := netlink.Neigh{
		LinkIndex:    vxlan.Index,
		Family:       syscall.AF_INET,
		State:        netlink.NUD_PERMANENT,
		IP:           gatewayIP,
		HardwareAddr: hwAddr,
	}

	if err := retry.Retry(func() error {
		err := netlink.NeighSet(&arpEntry)
		if err != nil {
			p.logger.Warn("failed to add ARP entry", zap.Error(err))
			return err
		}
		return err
	}); err != nil {
		return nil, err
	}

	// Add a route to the remote subnet via the VXLAN interface. The ONLINK flag
	// tells the kernel to treat this gateway as directly reachable on this
	// interface.
	route := &netlink.Route{
		LinkIndex: vxlan.Index,
		Dst:       req.Subnet.IPv4Network.ToIPNet(),
		Gw:        gatewayIP,
		Flags:     syscall.RTNH_F_ONLINK,
		Scope:     netlink.SCOPE_UNIVERSE,
	}

	if err := retry.Retry(func() error {
		err := netlink.RouteReplace(route)
		if err != nil {
			p.logger.Warn("failed to add route", zap.Error(err))
			return err
		}
		return err
	}); err != nil {
		return nil, err
	}

	return &types.NetworkProviderSetRemoteResp{}, nil
}

func (p *Provider) ensureLink(vxlan *netlink.Vxlan) (*netlink.Vxlan, error) {

	// Try to create the VXLAN link and correctly handle the case where it
	// already exists.
	if err := netlink.LinkAdd(vxlan); err != nil {
		if errors.Is(err, syscall.EEXIST) {
			existing, err := netlink.LinkByName(vxlan.Name)
			if err != nil {
				return nil, err
			}

			// If existing link matches desired config, the VXLAN is already set
			// up.
			if eq := vxlansEqual(vxlan, existing); eq {
				return existing.(*netlink.Vxlan), nil
			}

			// Attempt to replace existing link by deleting and recreating it.
			// This will briefly disrupt any traffic using the existing VXLAN
			// interface.
			p.logger.Warn("recreating existing VXLAN interface with updated configuration",
				zap.String("name", vxlan.Name),
			)

			if err = netlink.LinkDel(existing); err != nil {
				return nil, fmt.Errorf("failed to delete VXLAN interface: %w", err)
			}

			if err = netlink.LinkAdd(vxlan); err != nil {
				return nil, fmt.Errorf("failed to create VXLAN interface: %w", err)
			}
		} else {
			return nil, err
		}
	}

	// Retrieve the link to get its attributes, so we can perform checks.
	link, err := netlink.LinkByIndex(vxlan.Index)
	if err != nil {
		return nil, fmt.Errorf("failed to locate VXLAN device with index %v", vxlan.Index)
	}

	// Ensure the link is a VXLAN device.
	var ok bool

	if vxlan, ok = link.(*netlink.Vxlan); !ok {
		return nil, fmt.Errorf("vxlan device with index %v is not VXLAN", link.Attrs().Index)
	}

	return vxlan, nil
}
