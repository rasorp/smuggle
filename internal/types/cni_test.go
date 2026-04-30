package types

import (
	"net"
	"testing"

	"github.com/shoenig/test/must"

	"github.com/rasorp/smuggle/internal/helper"
)

func TestGenerateCNIConfig(t *testing.T) {

	// Use a test helper that parses a CIDR string into an *IPv4Net, to keep
	// test data declarations concise.
	mustIPv4Net := func(cidr string) *IPv4Net {
		_, ipNet, err := net.ParseCIDR(cidr)
		must.NoError(t, err)
		n := fromIPNet(ipNet)
		return &n
	}

	testCases := []struct {
		name     string
		network  *Network
		subnet   *Subnet
		expected *CNIConfig
	}{
		{
			name: "ipmasq enabled produces correct config",
			network: &Network{
				Name:   "smuggle",
				IPMasq: helper.PointerOf(true),
				IPv4: &IPv4Config{
					Network: mustIPv4Net("10.0.0.0/16"),
				},
			},
			subnet: &Subnet{
				IPv4Network: mustIPv4Net("10.0.1.0/24"),
				MTU:         1450,
			},
			expected: &CNIConfig{
				Name:   "smuggle",
				Bridge: "smugglebrd0",
				MTU:    1450,
				IPMasq: true,
				IPv4: &IPv4CNIConfig{
					Network: "10.0.0.0/16",
					Subnet:  "10.0.1.1/24",
					Gateway: "10.0.1.1",
				},
			},
		},
		{
			name: "ipmasq disabled is correctly dereferenced",
			network: &Network{
				Name:   "smuggle",
				IPMasq: helper.PointerOf(false),
				IPv4: &IPv4Config{
					Network: mustIPv4Net("10.0.0.0/16"),
				},
			},
			subnet: &Subnet{
				IPv4Network: mustIPv4Net("10.0.1.0/24"),
				MTU:         1450,
			},
			expected: &CNIConfig{
				Name:   "smuggle",
				Bridge: "smugglebrd0",
				MTU:    1450,
				IPMasq: false,
				IPv4: &IPv4CNIConfig{
					Network: "10.0.0.0/16",
					Subnet:  "10.0.1.1/24",
					Gateway: "10.0.1.1",
				},
			},
		},
		{
			name: "bridge name is always network name suffixed with brd0",
			network: &Network{
				Name:   "prod",
				IPMasq: helper.PointerOf(true),
				IPv4: &IPv4Config{
					Network: mustIPv4Net("192.168.0.0/20"),
				},
			},
			subnet: &Subnet{
				IPv4Network: mustIPv4Net("192.168.4.0/24"),
				MTU:         1500,
			},
			expected: &CNIConfig{
				Name:   "prod",
				Bridge: "prodbrd0",
				MTU:    1500,
				IPMasq: true,
				IPv4: &IPv4CNIConfig{
					Network: "192.168.0.0/20",
					Subnet:  "192.168.4.1/24",
					Gateway: "192.168.4.1",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := GenerateCNIConfig(tc.network, tc.subnet)
			must.Eq(t, tc.expected, result)
		})
	}
}
