package types

import (
	"encoding/json"
	"net"
	"time"

	"go.uber.org/zap"
)

// Subnet represents a subnet allocation for a specific client in the network
// fabric. Each client receives one subnet per network they join and can have
// multiple subnets if they are part of multiple networks.
type Subnet struct {

	// ClientID is the unique identifier of the client that owns this subnet.
	ClientID string `json:"client_id"`

	// NetworkName is the name of the network this subnet belongs to and is
	// declared by the network config object.
	NetworkName string `json:"network_name"`

	// Provider is the name of the network provider used to create and manage
	// this subnet. This currently only supports "vxlan".
	Provider string `json:"provider"`

	// HostIPv4 is the IPv4 address of the host interface on which this subnet
	// is configured. This is used by the network provider to set up the overlay
	// network.
	HostIPv4 *net.IP `json:"host_ipv4"`

	// Config is the provider-specific configuration for this subnet. This is
	// a JSON-encoded object that contains settings required by the network
	// provider to set up the subnet and should be opaque to the core system.
	Config json.RawMessage `json:"config"`

	// Expiration is the time when this subnet allocation expires. Clients must
	// refresh their subnet via heartbeat before this time to maintain their
	// allocation.
	Expiration time.Time `json:"expiration"`

	// Expired indicates whether the subnet has expired based on the current
	// time and the Expiration field. This field is used by the server to mark
	// subnets as expired and ready for reaping.
	Expired bool `json:"expired"`

	// IPv4Network is the IPv4 subnet allocated to this client within the
	// network fabric.
	IPv4Network *IPv4Net `json:"ipv4_network"`

	// MTU is the maximum transmission unit size for this subnet, which is
	// typically derived from the host interface's MTU minus any overhead for
	// the network provider.
	MTU int `json:"mtu"`
}

// Copy creates a deep copy of the Subnet, so it can be modified without
// affecting the original.
func (s *Subnet) Copy() *Subnet {
	if s == nil {
		return nil
	}

	copy := *s

	if s.HostIPv4 != nil {
		ipCopy := *s.HostIPv4
		copy.HostIPv4 = &ipCopy
	}

	if s.IPv4Network != nil {
		ipv4Copy := *s.IPv4Network
		copy.IPv4Network = &ipv4Copy
	}

	return &copy
}

// InterfaceName returns the name of the network interface that is used for
// this subnet. Networks are expected to have a single interface per host, so
// this with a static suffix of "0" is sufficient to be unique.
func (s *Subnet) InterfaceName() string { return s.NetworkName + "0" }

// LoggingPairs returns a set of zap fields for logging this subnet.
func (s *Subnet) LoggingPairs() []zap.Field {
	fields := []zap.Field{
		zap.String("client_id", s.ClientID),
		zap.String("network_name", s.NetworkName),
		zap.String("provider", s.Provider),
	}

	if s.HostIPv4 != nil {
		fields = append(fields, zap.String("host_ipv4", s.HostIPv4.String()))
	}
	if s.IPv4Network != nil {
		fields = append(fields, zap.String("ipv4_network", s.IPv4Network.String()))
	}

	return fields
}

// DefaultSubnetTTL is the default time-to-live for subnets. Subnets must be
// refreshed via heartbeat before this TTL expires, or they will be considered
// expired and ready for reaping.
var DefaultSubnetTTL = 24 * time.Hour
