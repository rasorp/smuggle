package wireguard

import (
	"fmt"

	"github.com/containernetworking/plugins/pkg/utils/sysctl"
)

// configureSysctl applies the kernel parameters required for WireGuard overlay
// networking. The kernel computes the effective rp_filter as
// max(conf/all, conf/<iface>), so both the overlay interface and the physical
// host interface must be set explicitly.
func (p *Provider) configureSysctl(ifName, hostIfaceName string) error {

	// Enable IPv4 forwarding so the host can route between the overlay and
	// any other interfaces.
	if _, err := sysctl.Sysctl("net/ipv4/ip_forward", "1"); err != nil {
		return fmt.Errorf("failed to enable ipv4 forwarding: %w", err)
	}

	// Disable rp_filter on the WireGuard interface. Overlay traffic arrives
	// decapsulated so the reverse path check would otherwise drop packets
	// whose return route resolves to a different interface.
	if _, err := sysctl.Sysctl(fmt.Sprintf("net/ipv4/conf/%s/rp_filter", ifName), "0"); err != nil {
		return fmt.Errorf("failed to disable rp_filter for %s: %w", ifName, err)
	}

	// Disable rp_filter on the physical host interface explicitly. Without
	// this, Cloud and modern Linux distributions default to rp_filter=1 on
	// each interface, overriding the conf/all setting.
	if _, err := sysctl.Sysctl(fmt.Sprintf("net/ipv4/conf/%s/rp_filter", hostIfaceName), "0"); err != nil {
		return fmt.Errorf("failed to disable rp_filter for host interface %s: %w", hostIfaceName, err)
	}

	// Set the global baseline so any newly created interfaces inherit a
	// permissive default.
	if _, err := sysctl.Sysctl("net/ipv4/conf/all/rp_filter", "0"); err != nil {
		return fmt.Errorf("failed to disable rp_filter globally: %w", err)
	}

	return nil
}
