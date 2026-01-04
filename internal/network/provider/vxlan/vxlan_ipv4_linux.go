package vxlan

import (
	"fmt"

	"github.com/containernetworking/plugins/pkg/utils/sysctl"
	"github.com/vishvananda/netlink"

	"github.com/rasorp/smuggle/internal/types"
)

func (p *Provider) createIPv4(
	cfg *types.Subnet,
	providerCfg *Config,
	vtepDevIndex int,
) (*netlink.Vxlan, error) {

	intfName := cfg.InterfaceName()

	vxlanLink := &netlink.Vxlan{
		LinkAttrs: netlink.LinkAttrs{
			Name: intfName,
			MTU:  providerCfg.MTU,
		},
		VxlanId:      providerCfg.VNI,
		VtepDevIndex: vtepDevIndex,
		SrcAddr:      *cfg.HostIPv4,
		Port:         providerCfg.Port,
	}

	vxlanLink, err := p.ensureLink(vxlanLink)
	if err != nil {
		return nil, err
	}

	_, _ = sysctl.Sysctl(fmt.Sprintf("net/ipv6/conf/%s/accept_ra", intfName), "0")

	if err := netlink.LinkSetUp(vxlanLink); err != nil {
		return nil, fmt.Errorf("failed to configure interface state: %w", err)
	}

	// Enable IPv4 forwarding to allow routing between interfaces
	if _, err := sysctl.Sysctl("net/ipv4/ip_forward", "1"); err != nil {
		return nil, fmt.Errorf("failed to enable ipv4 forwarding: %w", err)
	}

	// Disable reverse path filtering for the VXLAN interface to allow asymmetric routing
	// This is necessary for overlay networks where return traffic may come via different paths
	if _, err := sysctl.Sysctl(fmt.Sprintf("net/ipv4/conf/%s/rp_filter", intfName), "0"); err != nil {
		return nil, fmt.Errorf("failed to disable rp_filter for %s: %w", intfName, err)
	}

	// Also disable rp_filter on all interfaces to ensure forwarding works
	if _, err := sysctl.Sysctl("net/ipv4/conf/all/rp_filter", "0"); err != nil {
		return nil, fmt.Errorf("failed to disable rp_filter for all interfaces: %w", err)
	}

	return vxlanLink, nil
}
