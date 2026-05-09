package network

import (
	"fmt"
	"net"
)

type networkFingerprint struct {
	ifaceName string
	iface     *net.Interface
	ipv4Addr  net.IP
}

// Fingerprint discovers network interface information and populates an ExternalInterface.
// If interfaceName is empty, it will attempt to discover the default network interface.
// If interfaceName is provided, it will lookup that specific interface.
func fingerprint(interfaceName string) (*networkFingerprint, error) {
	var iface *net.Interface
	var err error

	if interfaceName == "" {
		// Discover the default interface
		iface, err = getDefaultInterface()
		if err != nil {
			return nil, fmt.Errorf("failed to get default interface: %w", err)
		}
	} else {
		// Lookup the named interface
		iface, err = net.InterfaceByName(interfaceName)
		if err != nil {
			return nil, fmt.Errorf("failed to lookup interface: %w", err)
		}
	}

	// Get addresses for the interface
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, fmt.Errorf("failed to get addresses for interface: %w", err)
	}

	extIface := &networkFingerprint{
		iface:     iface,
		ifaceName: iface.Name,
	}

	// Parse addresses to find IPv4 and IPv6
	for _, addr := range addrs {
		ipNet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}

		ip := ipNet.IP

		// Skip loopback addresses
		if ip.IsLoopback() {
			continue
		}

		if ip.To4() != nil {
			if extIface.ipv4Addr == nil {
				extIface.ipv4Addr = ip
			}
		}
	}

	// Validate that we found at least one address
	if extIface.ipv4Addr == nil {
		return nil, fmt.Errorf("no valid addresses found on interface %s", iface.Name)
	}

	return extIface, nil
}
