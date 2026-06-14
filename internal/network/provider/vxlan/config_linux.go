package vxlan

import (
	"go.uber.org/zap"
)

// OperatorConfig holds the fields an operator may supply in the network's
// provider config block.
type OperatorConfig struct {

	// VNI is the VXLAN Network Identifier. This defaults to 1, but should be
	// set when multiple networks will be assigned to the same set of hosts.
	VNI int `json:"vni"`

	// Port is the UDP port used for VXLAN encapsulation. This defaults to
	// 4789 which is the IANA assigned port for VXLAN.
	Port int `json:"port"`
}

// PeerConfig is the provider-generated state stored in Subnet.Config and
// distributed to remote peers via the subnet store. It contains operator
// settings like VNI and Port alongside provider-derived fields such as MTU and
// the VtepMAC.
type PeerConfig struct {
	// VNI is the VXLAN Network Identifier.
	VNI int `json:"vni"`

	// Port is the UDP port used for VXLAN encapsulation.
	Port int `json:"port"`

	// MTU is the Maximum Transmission Unit for the VXLAN interface, derived
	// from the host interface MTU minus the VXLAN encapsulation overhead.
	MTU int `json:"mtu"`

	// VtepMAC is the MAC address of the local VXLAN interface that was created
	// for the subnet.
	VtepMAC string `json:"vtep_mac"`
}

// loggingPairs returns a set of zap fields representing the VXLAN peer
// configuration for logging purposes.
func (c *PeerConfig) loggingPairs() []zap.Field {
	return []zap.Field{
		zap.Int("vni", c.VNI),
		zap.Int("port", c.Port),
		zap.Int("mtu", c.MTU),
		zap.String("vtep_mac", c.VtepMAC),
	}
}
