package types

// Firewall defines the interface for managing firewall rules. Implementations
// of this interface are responsible for setting up the necessary rules to
// allow traffic forwarding and masquerading for networks and subnets.
type Firewall interface {

	// EnsureIsolation ensures that all networks in the provided list are
	// isolated from each other by adding REJECT rules for cross-network
	// traffic. This prevents containers on different networks from
	// communicating with each other.
	EnsureIsolation([]*Network) error

	// SetupForwardRules sets up firewall forwarding rules for the provided
	// network. This is used to allow traffic to be forwarded between subnets on
	// the network.
	SetupForwardRules(*Network) error

	// SetupMasqRules sets up firewall masquerading rules for the provided
	// network and subnet. This is used to enable NAT for traffic leaving the
	// subnet to external destinations.
	SetupMasqRules(*Network, *Subnet) error
}
