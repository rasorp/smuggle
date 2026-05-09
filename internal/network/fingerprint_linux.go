//go:build linux

package network

import (
	"errors"
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
)

// getDefaultInterface discovers the default network interface by asking the
// kernel's routing table which interface it would use to reach an arbitrary
// external destination. This is a pure local netlink query — no packets are
// sent and no external IP is contacted, so the call succeeds regardless of
// egress firewall rules.
func getDefaultInterface() (*net.Interface, error) {

	// Any routable unicast IP works here; the kernel returns the route it would
	// use without sending any traffic. We pick 1.2.3.4 as a well-known,
	// documentation-range address that will never match a local route.
	routes, err := netlink.RouteGet(net.ParseIP("1.2.3.4"))
	if err != nil {
		return nil, fmt.Errorf("failed to get default route: %w", err)
	}
	if len(routes) == 0 {
		return nil, errors.New("no routes found")
	}

	iface, err := net.InterfaceByIndex(routes[0].LinkIndex)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup interface with index %d: %w",
			routes[0].LinkIndex, err)
	}

	return iface, nil
}
