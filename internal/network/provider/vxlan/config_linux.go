package vxlan

import (
	"go.uber.org/zap"
)

// Config represents the configuration options for a VXLAN network provider. Not
// all fields can be set by the user; some are populated by the provider during
// setup.
type Config struct {
	// VNI is the VXLAN Network Identifier. This defaults to 1, but should be
	// set when multiple networks will be assigned to the same set of hosts.
	VNI int `json:"vni"`

	// Port is the UDP port used for VXLAN encapsulation. This defaults to
	// 4789 which is the IANA assigned port for VXLAN.
	Port int `json:"port"`

	// MTU is the Maximum Transmission Unit for the VXLAN interface. This is
	// typically set to the host interface MTU minus the VXLAN overhead of 50
	// bytes.
	MTU int `json:"mtu"`

	// VtepMAC is the MAC address of the local VXLAN interface that was created
	// for the subnet.
	VtepMAC string `json:"vtep_mac"`
}

// loggingPairs returns a set of zap fields representing the VXLAN configuration
// for logging purposes.
func (c *Config) loggingPairs() []zap.Field {
	return []zap.Field{
		zap.Int("vni", c.VNI),
		zap.Int("port", c.Port),
		zap.Int("mtu", c.MTU),
		zap.String("vtep_mac", c.VtepMAC),
	}
}
