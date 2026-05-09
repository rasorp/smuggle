package types

import (
	"testing"

	"github.com/shoenig/test/must"
)

// validNetwork returns a fully-populated Network that passes Validate, used as
// the starting point for individual test cases that mutate one field at a time.
func validNetwork() *Network {
	return &Network{
		Name: "test",
		IPv4: &IPv4Config{
			Network: &IPv4Net{IP: 167772160, Size: 24},
			Size:    24,
		},
		Provider: &ProviderConfig{Name: "vxlan"},
	}
}

func TestNetwork_Validate(t *testing.T) {
	testCases := []struct {
		name          string
		inputNetwork  *Network
		expectedError string
	}{
		{
			name:          "name empty",
			inputNetwork:  func() *Network { n := validNetwork(); n.Name = ""; return n }(),
			expectedError: "network name must not be empty",
		},
		{
			name:         "name exactly max length",
			inputNetwork: func() *Network { n := validNetwork(); n.Name = "abcdefghijk"; return n }(),
		},
		{
			name:          "name one over max length",
			inputNetwork:  func() *Network { n := validNetwork(); n.Name = "abcdefghijkl"; return n }(),
			expectedError: `network name "abcdefghijkl" is too long: maximum 11 characters allowed`,
		},
		{
			name:          "name far over max length",
			inputNetwork:  func() *Network { n := validNetwork(); n.Name = "verylongnetworkname"; return n }(),
			expectedError: `network name "verylongnetworkname" is too long: maximum 11 characters allowed`,
		},
		{
			name:          "name starts with digit",
			inputNetwork:  func() *Network { n := validNetwork(); n.Name = "1network"; return n }(),
			expectedError: `network name "1network" is invalid: regex ^[a-z][a-z0-9_-]*$`,
		},
		{
			name:          "name contains uppercase",
			inputNetwork:  func() *Network { n := validNetwork(); n.Name = "Network"; return n }(),
			expectedError: `network name "Network" is invalid: regex ^[a-z][a-z0-9_-]*$`,
		},
		{
			name:          "name contains space",
			inputNetwork:  func() *Network { n := validNetwork(); n.Name = "net work"; return n }(),
			expectedError: `network name "net work" is invalid: regex ^[a-z][a-z0-9_-]*$`,
		},
		{
			name:          "name contains slash",
			inputNetwork:  func() *Network { n := validNetwork(); n.Name = "net/work"; return n }(),
			expectedError: `network name "net/work" is invalid: regex ^[a-z][a-z0-9_-]*$`,
		},
		{
			name:         "name with hyphen",
			inputNetwork: func() *Network { n := validNetwork(); n.Name = "net-a"; return n }(),
		},
		{
			name:         "name with underscore",
			inputNetwork: func() *Network { n := validNetwork(); n.Name = "net_a"; return n }(),
		},
		{
			name:         "name with trailing digit",
			inputNetwork: func() *Network { n := validNetwork(); n.Name = "net1"; return n }(),
		},
		{
			name:         "name single char",
			inputNetwork: func() *Network { n := validNetwork(); n.Name = "a"; return n }(),
		},
		{
			name:          "ipv4 nil",
			inputNetwork:  func() *Network { n := validNetwork(); n.IPv4 = nil; return n }(),
			expectedError: "network configuration is empty",
		},
		{
			name:          "ipv4 network nil",
			inputNetwork:  func() *Network { n := validNetwork(); n.IPv4.Network = nil; return n }(),
			expectedError: "IPv4 network configuration is missing",
		},
		{
			name:          "ipv4 size zero",
			inputNetwork:  func() *Network { n := validNetwork(); n.IPv4.Size = 0; return n }(),
			expectedError: "IPv4 subnet size must be between 1 and 32",
		},
		{
			name:          "ipv4 size above 32",
			inputNetwork:  func() *Network { n := validNetwork(); n.IPv4.Size = 33; return n }(),
			expectedError: "IPv4 subnet size must be between 1 and 32",
		},
		{
			name:         "ipv4 size 1 boundary",
			inputNetwork: func() *Network { n := validNetwork(); n.IPv4.Size = 1; return n }(),
		},
		{
			name:         "ipv4 size 32 boundary",
			inputNetwork: func() *Network { n := validNetwork(); n.IPv4.Size = 32; return n }(),
		},
		{
			name:         "ipv4 min empty skips check",
			inputNetwork: func() *Network { n := validNetwork(); n.IPv4.Min = EmptyIPv4Addr; return n }(),
		},
		{
			name: "ipv4 min within range",
			inputNetwork: func() *Network {
				n := validNetwork()
				n.IPv4.Min = 167772161 // 10.0.0.1
				return n
			}(),
		},
		{
			name: "ipv4 min below network",
			inputNetwork: func() *Network {
				n := validNetwork()
				n.IPv4.Min = 167772159 // 9.255.255.255
				return n
			}(),
			expectedError: "IPv4 minimum address is out of network range",
		},
		{
			name: "ipv4 min above network",
			inputNetwork: func() *Network {
				n := validNetwork()
				n.IPv4.Min = 167772416 // 10.0.1.0
				return n
			}(),
			expectedError: "IPv4 minimum address is out of network range",
		},
		{
			name:          "provider nil",
			inputNetwork:  func() *Network { n := validNetwork(); n.Provider = nil; return n }(),
			expectedError: "network provider configuration is missing",
		},
		{
			name:          "provider unsupported",
			inputNetwork:  func() *Network { n := validNetwork(); n.Provider = &ProviderConfig{Name: "bridge"}; return n }(),
			expectedError: `unsupported network provider: "bridge"`,
		},
		{
			name:         "provider vxlan",
			inputNetwork: validNetwork(),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.inputNetwork.Validate()
			if tc.expectedError == "" {
				must.NoError(t, err)
			} else {
				must.EqError(t, err, tc.expectedError)
			}
		})
	}
}
