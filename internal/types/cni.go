package types

type CNIStore interface {
	Set(*CNIConfig) error
}

// CNIConfig represents a CNI (Container Network Interface) configuration.
type CNIConfig struct {
	Name   string         `json:"name"`
	Bridge string         `json:"bridge,omitempty"`
	MTU    int            `json:"mtu"`
	IPMasq bool           `json:"ipmasq"`
	IPv4   *IPv4CNIConfig `json:"ipv4"`
}

// IPv4CNIConfig represents IPv4-specific CNI configuration.
type IPv4CNIConfig struct {
	Network string `json:"network"`
	Subnet  string `json:"subnet"`
	Gateway string `json:"gateway,omitempty"`
}

// GenerateCNIConfig creates a CNI configuration from network and subnet configurations.
func GenerateCNIConfig(network *Network, subnet *Subnet) *CNIConfig {
	return &CNIConfig{
		Name:   network.Name,
		Bridge: network.Name + "brd0",
		MTU:    subnet.MTU,
		IPMasq: !*network.IPMasq,
		IPv4: &IPv4CNIConfig{
			Network: network.IPv4.Network.String(),
			Subnet:  subnet.IPv4Network.NextAddr().String(),
			Gateway: subnet.IPv4Network.NextAddr().IP.String(),
		},
	}
}
