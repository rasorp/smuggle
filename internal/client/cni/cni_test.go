package cni

import (
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/shoenig/test/must"

	"github.com/rasorp/smuggle/internal/helper"
	"github.com/rasorp/smuggle/internal/types"
)

func TestGenerateCNIConfig(t *testing.T) {

	// Use a test helper that parses a CIDR string into an *IPv4Net, to keep
	// test data declarations concise.
	mustIPv4Net := func(cidr string) *types.IPv4Net {
		_, ipNet, err := net.ParseCIDR(cidr)
		must.NoError(t, err)
		n := types.FromIPNet(ipNet)
		return &n
	}

	testCases := []struct {
		name     string
		network  *types.Network
		subnet   *types.Subnet
		expected *Config
	}{
		{
			name: "ipmasq enabled produces correct config",
			network: &types.Network{
				Name:   "smuggle",
				IPMasq: helper.PointerOf(true),
				IPv4: &types.IPv4Config{
					Network: mustIPv4Net("10.0.0.0/16"),
				},
			},
			subnet: &types.Subnet{
				IPv4Network: mustIPv4Net("10.0.1.0/24"),
				MTU:         1450,
			},
			expected: &Config{
				Name:   "smuggle",
				Bridge: "smugglebrd0",
				MTU:    1450,
				IPMasq: true,
				IPv4: &IPv4Config{
					Network: "10.0.0.0/16",
					Subnet:  "10.0.1.1/24",
					Gateway: "10.0.1.1",
				},
			},
		},
		{
			name: "ipmasq disabled is correctly dereferenced",
			network: &types.Network{
				Name:   "smuggle",
				IPMasq: helper.PointerOf(false),
				IPv4: &types.IPv4Config{
					Network: mustIPv4Net("10.0.0.0/16"),
				},
			},
			subnet: &types.Subnet{
				IPv4Network: mustIPv4Net("10.0.1.0/24"),
				MTU:         1450,
			},
			expected: &Config{
				Name:   "smuggle",
				Bridge: "smugglebrd0",
				MTU:    1450,
				IPMasq: false,
				IPv4: &IPv4Config{
					Network: "10.0.0.0/16",
					Subnet:  "10.0.1.1/24",
					Gateway: "10.0.1.1",
				},
			},
		},
		{
			name: "bridge name is always network name suffixed with brd0",
			network: &types.Network{
				Name:   "prod",
				IPMasq: helper.PointerOf(true),
				IPv4: &types.IPv4Config{
					Network: mustIPv4Net("192.168.0.0/20"),
				},
			},
			subnet: &types.Subnet{
				IPv4Network: mustIPv4Net("192.168.4.0/24"),
				MTU:         1500,
			},
			expected: &Config{
				Name:   "prod",
				Bridge: "prodbrd0",
				MTU:    1500,
				IPMasq: true,
				IPv4: &IPv4Config{
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

func Test_NewCNIStore(t *testing.T) {
	storePath := "/tmp/cni-store"
	store := NewStore(storePath)
	must.Eq(t, storePath, store.path)
}

func TestCNIStore_Set(t *testing.T) {
	testCases := []struct {
		name                  string
		storePath             string
		config                *Config
		setupFunc             func(t *testing.T, storePath string)
		validateFunc          func(t *testing.T, storePath string, cfg *Config)
		expectedErrorContains string
	}{
		{
			name:                  "nil config",
			storePath:             t.TempDir(),
			config:                nil,
			expectedErrorContains: "CNI config cannot be nil",
		},
		{
			name:      "valid config",
			storePath: t.TempDir(),
			config: &Config{
				Name:   "test-network",
				MTU:    1450,
				IPMasq: true,
				IPv4: &IPv4Config{
					Network: "10.0.0.0/16",
					Subnet:  "10.0.1.1/24",
					Gateway: "10.0.1.1",
				},
			},
			validateFunc: func(t *testing.T, storePath string, cfg *Config) {

				// Ensure the file exists on the filesystem.
				filePath := filepath.Join(storePath, cfg.Name+".conf")
				must.FileExists(t, filePath)

				// Read the file and ensure the contents match the config we
				// passed.
				data, err := os.ReadFile(filePath)
				must.NoError(t, err)

				var readCfg Config
				must.NoError(t, json.Unmarshal(data, &readCfg))
				must.Eq(t, cfg, &readCfg)

				// Ensure the permissions are as expected.
				info, err := os.Stat(filePath)
				must.NoError(t, err)
				must.False(t, info.IsDir())
			},
		},
		{
			name:      "creates parent directory",
			storePath: filepath.Join(t.TempDir(), "nested", "deeply", "path"),
			config: &Config{
				Name:   "nested-network",
				MTU:    1500,
				IPMasq: false,
				IPv4: &IPv4Config{
					Network: "172.16.0.0/12",
					Subnet:  "172.16.1.1/24",
				},
			},
			validateFunc: func(t *testing.T, storePath string, cfg *Config) {
				filePath := filepath.Join(storePath, cfg.Name+".conf")
				must.FileExists(t, filePath)

				// Verify all parent directories were created
				dirInfo, err := os.Stat(storePath)
				must.NoError(t, err)
				must.True(t, dirInfo.IsDir())
			},
		},
		{
			name:      "existing file",
			storePath: t.TempDir(),
			config: &Config{
				Name:   "overwrite-network",
				MTU:    1400,
				IPMasq: true,
				IPv4: &IPv4Config{
					Network: "192.168.0.0/16",
					Subnet:  "192.168.1.1/24",
					Gateway: "192.168.1.1",
				},
			},
			setupFunc: func(t *testing.T, storePath string) {
				// Create an existing file with different content
				oldCfg := &Config{
					Name:   "overwrite-network",
					MTU:    9000,
					IPMasq: false,
					IPv4: &IPv4Config{
						Network: "10.0.0.0/8",
						Subnet:  "10.0.0.1/24",
					},
				}
				data, err := json.Marshal(oldCfg)
				must.NoError(t, err)

				filePath := filepath.Join(storePath, oldCfg.Name+".conf")
				must.NoError(t, os.WriteFile(filePath, data, 0644))
			},
			validateFunc: func(t *testing.T, storePath string, cfg *Config) {
				filePath := filepath.Join(storePath, cfg.Name+".conf")
				data, err := os.ReadFile(filePath)
				must.NoError(t, err)

				var readCfg Config
				must.NoError(t, json.Unmarshal(data, &readCfg))

				// Verify new config, not old config
				must.Eq(t, 1400, readCfg.MTU)
				must.Eq(t, "192.168.0.0/16", readCfg.IPv4.Network)
			},
		},
		{
			name:      "multiple configs",
			storePath: t.TempDir(),
			config: &Config{
				Name:   "network-2",
				MTU:    1450,
				IPMasq: true,
				IPv4: &IPv4Config{
					Network: "10.1.0.0/16",
					Subnet:  "10.1.1.1/24",
				},
			},
			setupFunc: func(t *testing.T, storePath string) {
				// Create first network config
				cfg1 := &Config{
					Name:   "network-1",
					MTU:    1500,
					IPMasq: false,
					IPv4: &IPv4Config{
						Network: "10.0.0.0/16",
						Subnet:  "10.0.1.1/24",
					},
				}
				store := NewStore(storePath)
				must.NoError(t, store.Set(cfg1))
			},
			validateFunc: func(t *testing.T, storePath string, cfg *Config) {
				// Verify both files exist
				must.FileExists(t, filepath.Join(storePath, "network-1.conf"))
				must.FileExists(t, filepath.Join(storePath, "network-2.conf"))
			},
		},
		{
			name:      "path traversal via dotdot segments",
			storePath: t.TempDir(),
			config: &Config{
				Name: "../../etc/cron.d/evil",
				MTU:  1450,
				IPv4: &IPv4Config{
					Network: "10.0.0.0/16",
					Subnet:  "10.0.1.1/24",
				},
			},
			expectedErrorContains: "path traversal detected",
		},
		{
			name:      "path traversal via absolute path in name",
			storePath: t.TempDir(),
			config: &Config{
				Name: "/etc/cron.d/evil",
				MTU:  1450,
				IPv4: &IPv4Config{
					Network: "10.0.0.0/16",
					Subnet:  "10.0.1.1/24",
				},
			},
			expectedErrorContains: "path traversal detected",
		},
		{
			name:      "read-only directory",
			storePath: filepath.Join(t.TempDir(), "readonly"),
			config: &Config{
				Name:   "readonly-test",
				MTU:    1450,
				IPMasq: true,
				IPv4: &IPv4Config{
					Network: "10.0.0.0/16",
					Subnet:  "10.0.1.1/24",
				},
			},
			setupFunc: func(t *testing.T, storePath string) {
				// Create directory and make it read-only
				must.NoError(t, os.MkdirAll(storePath, 0755))
				must.NoError(t, os.Chmod(storePath, 0444))

				// Cleanup: restore permissions after test
				t.Cleanup(func() {
					_ = os.Chmod(storePath, 0755)
				})
			},
			expectedErrorContains: "failed to create temporary file",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			// If the test supplied a setup function, run it now before
			// executing the store function.
			if tc.setupFunc != nil {
				tc.setupFunc(t, tc.storePath)
			}

			// Execute the store function with the passed config.
			err := NewStore(tc.storePath).Set(tc.config)

			// Perform the test assertions.
			if tc.expectedErrorContains != "" {
				must.ErrorContains(t, err, tc.expectedErrorContains)
			} else {
				must.NoError(t, err)
				if tc.validateFunc != nil {
					tc.validateFunc(t, tc.storePath, tc.config)
				}
			}
		})
	}
}
