package types

// Firewall defines the interface for managing firewall rules. Implementations
// of this interface are responsible for setting up the necessary rules to
// allow traffic forwarding and masquerading for networks and subnets.
type Firewall interface {

	// CreateIsolation ensures that all networks in the provided list are
	// isolated from each other by adding REJECT rules for cross-network
	// traffic. This prevents containers on different networks from
	// communicating with each other.
	CreateIsolation([]*Network) error

	// DeleteIsolation removes the isolation rules for the deleted networks,
	// while keeping the rules for the existing networks intact. It allows the
	// agent to clean up any rules that are no longer needed after a network is
	// deleted.
	DeleteIsolation(exist, deleted []*Network) error

	// CreateForwardRules sets up firewall forwarding rules for the provided
	// network. This is used to allow traffic to be forwarded between subnets on
	// the network.
	CreateForwardRules(network *Network) error

	// DeleteForwardRules deletes firewall forwarding rules for the provided
	// network. This is used to clean up rules when a network is deleted from
	// the store.
	DeleteForwardRules(network *Network) error

	// CreateMasqRules sets up firewall masquerading rules for the provided
	// network and subnet. This is used to enable NAT for traffic leaving the
	// subnet to external destinations.
	CreateMasqRules(network *Network, subnet *Subnet) error

	// DeleteMasqRules deletes firewall masquerading rules for the provided
	// network and subnet. This is used to clean up rules when a subnet is
	// deleted from the store.
	DeleteMasqRules(network *Network, subnet *Subnet) error
}
