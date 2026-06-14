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
	ProviderName = "vxlan"

	// defaultVNI is the default VXLAN Network Identifier used if none is
	// specified by the operator.
	defaultVNI = 1

	// defaultPort is the default UDP port used for VXLAN traffic.
	defaultPort = 4789

	// vxlanEncapuslationOverhead is the overhead in bytes introduced by VXLAN
	// encapsulation. This is used to adjust the MTU of the VXLAN interface to
	// avoid fragmentation.
	vxlanEncapuslationOverhead = 50

	// smuggleRoutingTableID is the custom routing table ID managed by smuggle
	// for policy-based routing. Traffic originating from local container
	// subnets uses this table to force return paths back through the VXLAN
	// tunnel.
	smuggleRoutingTableID = 100

	// smuggleRulePriority is the ip-rule priority for smuggle policy rules.
	// Lower numbers take precedence; 100 places these rules above the main
	// table (253) but below local (0) and any operator-defined rules above 100.
	smuggleRulePriority = 100
)

type Provider struct {
	logger *zap.Logger
}

func New(logger *zap.Logger) types.NetworkProvider {
	return &Provider{
		logger: logger.Named(ProviderName),
	}
}

func (p *Provider) Name() string { return ProviderName }

func (p *Provider) SetLocal(
	req *types.NetworkProviderSetReq,
) (*types.NetworkProviderSetResp, error) {

	opCfg := OperatorConfig{
		VNI:  defaultVNI,
		Port: defaultPort,
	}

	if req.Client.Config != nil {
		if err := json.Unmarshal(req.Client.Config, &opCfg); err != nil {
			return nil, err
		}
	}

	// Build the peer config combining operator settings with derived state.
	peerCfg := &PeerConfig{
		VNI:  opCfg.VNI,
		Port: opCfg.Port,
		MTU:  req.HostInterface.MTU - vxlanEncapuslationOverhead,
	}

	vxlanLink, err := p.createIPv4(req.Client, peerCfg, req.HostInterface.Index, req.HostInterface.Name)
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
	peerCfg.VtepMAC = vxlanLink.Attrs().HardwareAddr.String()
	if peerCfg.VtepMAC == "" {
		return nil, errors.New("VXLAN interface MAC address is empty")
	}

	marshaledCfg, err := json.Marshal(peerCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal vxlan config: %v", err)
	}

	p.logger.Info("setup local VXLAN interface", peerCfg.loggingPairs()...)

	// Create a copy of the subnet to avoid mutating the request object and
	// ensure we don't accidentally modify the caller's data.
	respSubnet := req.Client.Copy()
	respSubnet.Config = marshaledCfg

	return &types.NetworkProviderSetResp{Network: respSubnet}, nil
}

func (p *Provider) DeleteRemote(
	req *types.NetworkProviderDeleteRemoteReq,
) (*types.NetworkProviderDeleteRemoteResp, error) {

	// Parse the provider peer config to get the VNI and VTEP MAC address.
	var cfg PeerConfig

	if req.Subnet.Config != nil {
		if err := json.Unmarshal(req.Subnet.Config, &cfg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal peer config: %w", err)
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

	// Remove the neighbor entry for the remote host's physical IP on the VXLAN
	// interface that was added for policy routing.
	hostARPEntry := netlink.Neigh{
		LinkIndex: vxlan.Index,
		Family:    syscall.AF_INET,
		State:     netlink.NUD_PERMANENT,
		IP:        *req.Subnet.HostIPv4,
	}
	if err := retry.Retry(func() error {
		if err := netlink.NeighDel(&hostARPEntry); err != nil {
			if errors.Is(err, syscall.ENOENT) {
				return nil
			}
			p.logger.Warn("failed to delete host ARP entry", zap.Error(err))
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// Remove the host route from the smuggle routing table. The scope must
	// match what was set in SetRemote (SCOPE_LINK); the kernel's
	// fib_table_delete treats scope=0 (UNIVERSE) as an exact filter, not a
	// wildcard, so omitting it causes ESRCH even when the route is present.
	hostRoute := &netlink.Route{
		LinkIndex: vxlan.Index,
		Dst: &net.IPNet{
			IP:   *req.Subnet.HostIPv4,
			Mask: net.CIDRMask(32, 32),
		},
		Scope: netlink.SCOPE_LINK,
		Table: smuggleRoutingTableID,
	}
	if err := retry.Retry(func() error {
		if err := netlink.RouteDel(hostRoute); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return nil
			}
			p.logger.Warn("failed to delete policy host route", zap.Error(err))
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// Only remove the ip rules when no host routes remain in the smuggle
	// routing table. The rules (src localSubnet → table 100) are shared by
	// all remote hosts; removing them while any remote still has a /32 host
	// route in table 100 would break policy routing for those remotes.
	remainingRoutes, err := netlink.RouteListFiltered(syscall.AF_INET, &netlink.Route{
		Table: smuggleRoutingTableID,
	}, netlink.RT_FILTER_TABLE)
	if err != nil {
		return nil, fmt.Errorf("failed to list routes in smuggle routing table: %w", err)
	}

	if len(remainingRoutes) == 0 {
		for _, localSubnet := range req.LocalSubnets {
			if localSubnet.IPv4Network == nil {
				continue
			}
			rule := netlink.NewRule()
			rule.Src = localSubnet.IPv4Network.ToIPNet()
			rule.Table = smuggleRoutingTableID
			rule.Priority = smuggleRulePriority

			if err := netlink.RuleDel(rule); err != nil && !errors.Is(err, syscall.ENOENT) {
				return nil, fmt.Errorf("failed to delete policy rule for subnet %s: %w",
					localSubnet.IPv4Network, err)
			}
			p.logger.Debug("removed policy routing rule",
				zap.String("src_subnet", localSubnet.IPv4Network.String()),
				zap.Int("table", smuggleRoutingTableID),
			)
		}
	}

	return &types.NetworkProviderDeleteRemoteResp{}, nil
}

func (p *Provider) SetRemote(
	req *types.NetworkProviderSetRemoteReq,
) (*types.NetworkProviderSetRemoteResp, error) {

	// Parse the provider peer config to get the VNI and VTEP MAC.
	var cfg PeerConfig

	if req.Subnet.Config != nil {
		if err := json.Unmarshal(req.Subnet.Config, &cfg); err != nil {
			return nil, fmt.Errorf("failed to unmarshal vxlan peer config: %w", err)
		}
	} else {
		cfg.VNI = defaultVNI
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

	// Add a neighbor entry mapping the remote host's physical IP to the VTEP
	// MAC on the VXLAN interface. This allows the host-route below to resolve
	// L2 without broadcasting on the overlay.
	hostARPEntry := netlink.Neigh{
		LinkIndex:    vxlan.Index,
		Family:       syscall.AF_INET,
		State:        netlink.NUD_PERMANENT,
		IP:           *req.Subnet.HostIPv4,
		HardwareAddr: hwAddr,
	}
	if err := retry.Retry(func() error {
		if err := netlink.NeighSet(&hostARPEntry); err != nil {
			p.logger.Warn("failed to add host ARP entry", zap.Error(err))
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// Add a host route for the remote physical IP into the smuggle routing
	// table, directing it through the VXLAN interface so the outer UDP/VXLAN
	// packet uses a symmetric path and AWS security groups track it correctly.
	hostRoute := &netlink.Route{
		LinkIndex: vxlan.Index,
		Dst: &net.IPNet{
			IP:   *req.Subnet.HostIPv4,
			Mask: net.CIDRMask(32, 32),
		},
		Scope: netlink.SCOPE_LINK,
		Table: smuggleRoutingTableID,
	}
	if err := retry.Retry(func() error {
		if err := netlink.RouteReplace(hostRoute); err != nil {
			p.logger.Warn("failed to add policy host route", zap.Error(err))
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// For each local subnet, install an ip rule that steers traffic originating
	// from that subnet into the smuggle routing table. This ensures replies
	// from local containers to a remote host's physical IP are routed via the
	// VXLAN interface, so the return path is symmetric.
	for _, localSubnet := range req.LocalSubnets {
		if localSubnet.IPv4Network == nil {
			continue
		}
		rule := netlink.NewRule()
		rule.Src = localSubnet.IPv4Network.ToIPNet()
		rule.Table = smuggleRoutingTableID
		rule.Priority = smuggleRulePriority

		if err := netlink.RuleAdd(rule); err != nil && !errors.Is(err, syscall.EEXIST) {
			return nil, fmt.Errorf("failed to add policy rule for subnet %s: %w",
				localSubnet.IPv4Network, err)
		}
		p.logger.Debug("successfully added policy routing rule",
			zap.String("src_subnet", localSubnet.IPv4Network.String()),
			zap.Int("table", smuggleRoutingTableID),
		)
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
