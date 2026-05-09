//go:build !linux

package network

import (
	"errors"
	"net"
)

// getDefaultInterface is a non-Linux stub. On Linux the implementation uses
// netlink.RouteGet for a kernel routing-table query with no external I/O.
// Here we fall back to scanning interfaces, which is the best we can do on
// platforms that don't support netlink. In practice this path is unreachable
// at runtime because NewManager returns an error on non-Linux systems.
func getDefaultInterface() (*net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	return getFirstNonLoopbackInterface(ifaces)
}

// getFirstNonLoopbackInterface returns the first non-loopback interface that is up
// and has an assigned IP address.
func getFirstNonLoopbackInterface(interfaces []net.Interface) (*net.Interface, error) {
	for _, iface := range interfaces {
		// Skip down interfaces
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		// Skip loopback
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		// Check if it has addresses
		addrs, err := iface.Addrs()
		if err != nil || len(addrs) == 0 {
			continue
		}

		// Check for at least one non-loopback IP
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			if !ipNet.IP.IsLoopback() {
				return &iface, nil
			}
		}
	}

	return nil, errors.New("no suitable network interface found")
}
