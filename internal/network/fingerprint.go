package network

import (
	"errors"
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

// getDefaultInterface discovers the default network interface by finding
// the interface used for the default route.
func getDefaultInterface() (*net.Interface, error) {
	// Get all network interfaces
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list interfaces: %w", err)
	}

	// Try to find the default interface by attempting to connect to a public IP
	// This doesn't actually send data, just determines which interface would be used
	conn, err := net.Dial("udp", "8.8.8.8:80")
	if err != nil {
		// If we can't dial out, fall back to finding the first non-loopback interface
		return getFirstNonLoopbackInterface(interfaces)
	}
	defer func() { _ = conn.Close() }()

	localAddr := conn.LocalAddr().(*net.UDPAddr)

	// Find the interface that has this local address
	for _, iface := range interfaces {
		// Skip down interfaces
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		// Skip loopback
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}

			if ipNet.IP.Equal(localAddr.IP) {
				return &iface, nil
			}
		}
	}

	// If we couldn't find the interface by address, fall back
	return getFirstNonLoopbackInterface(interfaces)
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
