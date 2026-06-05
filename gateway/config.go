package main

import (
	"errors"
	"fmt"
	"net"
)

// Config holds all configuration for the standalone gateway process.
type Config struct {
	// OverlayCIDR is the full Smuggle overlay CIDR (e.g. 10.100.0.0/16).
	OverlayCIDR string

	// TunnelSubnet is the subnet from which customer tunnel IPs are drawn
	// (e.g. 10.200.0.0/24). The first host address is reserved for the
	// gateway interface itself.
	TunnelSubnet string

	// EndpointIP is the IP address of this node that customers will use as
	// their WireGuard endpoint. This may be a public IP or a private IP in a
	// VPC peering setup — whatever is routable from the customer's network.
	EndpointIP string

	// OverlayIface is the name of the existing Smuggle overlay interface that
	// customer traffic is forwarded into (default: smuggle0).
	OverlayIface string

	// WGIface is the name of the WireGuard interface created for customer
	// tunnels (default: wg-gw0). This is kept separate from the Smuggle
	// overlay interface so the two are never confused.
	WGIface string

	// WGPort is the UDP port the customer WireGuard interface listens on.
	WGPort int

	// WGKeyFile is the path used to persist and reload the gateway's WireGuard
	// private key across restarts. /tmp is fine for dev use.
	WGKeyFile string

	// HTTPAddr is the address the HTTP API listens on (e.g. 0.0.0.0:9091).
	HTTPAddr string
}

// DefaultConfig returns a Config with sensible defaults for every optional
// field. Required fields are left as zero values and caught by Validate.
func DefaultConfig() *Config {
	return &Config{
		OverlayIface: "smuggle0",
		WGIface:      "wg-gw0",
		WGPort:       51820,
		WGKeyFile:    "/tmp/wg-gw0.key",
		HTTPAddr:     "0.0.0.0:9091",
	}
}

// Validate returns a slice of all configuration errors found. An empty slice
// means the config is usable.
func (c *Config) Validate() []error {
	var errs []error

	if c.OverlayCIDR == "" {
		errs = append(errs, errors.New("--overlay-cidr is required"))
	} else if _, _, err := net.ParseCIDR(c.OverlayCIDR); err != nil {
		errs = append(errs, fmt.Errorf("--overlay-cidr is not a valid CIDR: %w", err))
	}

	if c.TunnelSubnet == "" {
		errs = append(errs, errors.New("--tunnel-subnet is required"))
	} else {
		_, ipNet, err := net.ParseCIDR(c.TunnelSubnet)
		if err != nil {
			errs = append(errs, fmt.Errorf("--tunnel-subnet is not a valid CIDR: %w", err))
		} else {
			ones, bits := ipNet.Mask.Size()
			// We need: network addr + gateway IP + at least 1 customer IP + broadcast
			// That requires at least 4 addresses, so at least 2 host bits.
			if bits-ones < 2 {
				errs = append(errs, errors.New("--tunnel-subnet must have room for at least one customer IP (minimum /30)"))
			}
		}
	}

	if c.EndpointIP == "" {
		errs = append(errs, errors.New("--endpoint-ip is required"))
	} else if net.ParseIP(c.EndpointIP) == nil {
		errs = append(errs, fmt.Errorf("--endpoint-ip %q is not a valid IP address", c.EndpointIP))
	}

	if c.OverlayIface == "" {
		errs = append(errs, errors.New("--overlay-iface must not be empty"))
	}

	if c.WGIface == "" {
		errs = append(errs, errors.New("--wg-iface must not be empty"))
	}

	if c.WGPort <= 0 || c.WGPort > 65535 {
		errs = append(errs, errors.New("--wg-port must be between 1 and 65535"))
	}

	if c.WGKeyFile == "" {
		errs = append(errs, errors.New("--wg-key-file must not be empty"))
	}

	if c.HTTPAddr == "" {
		errs = append(errs, errors.New("--http-addr must not be empty"))
	}

	return errs
}
