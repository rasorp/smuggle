package file

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/shoenig/test/must"

	"github.com/rasorp/smuggle/internal/types"
)

func Test_NewCNIStore(t *testing.T) {
	storePath := "/tmp/cni-store"
	store := NewCNIStore(storePath)

	cniStore, ok := store.(*CNIStore)
	must.True(t, ok)
	must.Eq(t, storePath, cniStore.path)
}

func TestCNIStore_Set(t *testing.T) {
	testCases := []struct {
		name                  string
		storePath             string
		config                *types.CNIConfig
		setupFunc             func(t *testing.T, storePath string)
		validateFunc          func(t *testing.T, storePath string, cfg *types.CNIConfig)
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
			config: &types.CNIConfig{
				Name:   "test-network",
				MTU:    1450,
				IPMasq: true,
				IPv4: &types.IPv4CNIConfig{
					Network: "10.0.0.0/16",
					Subnet:  "10.0.1.1/24",
					Gateway: "10.0.1.1",
				},
			},
			validateFunc: func(t *testing.T, storePath string, cfg *types.CNIConfig) {

				// Ensure the file exists on the filesystem.
				filePath := filepath.Join(storePath, cfg.Name+".conf")
				must.FileExists(t, filePath)

				// Read the file and ensure the contents match the config we
				// passed.
				data, err := os.ReadFile(filePath)
				must.NoError(t, err)

				var readCfg types.CNIConfig
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
			config: &types.CNIConfig{
				Name:   "nested-network",
				MTU:    1500,
				IPMasq: false,
				IPv4: &types.IPv4CNIConfig{
					Network: "172.16.0.0/12",
					Subnet:  "172.16.1.1/24",
				},
			},
			validateFunc: func(t *testing.T, storePath string, cfg *types.CNIConfig) {
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
			config: &types.CNIConfig{
				Name:   "overwrite-network",
				MTU:    1400,
				IPMasq: true,
				IPv4: &types.IPv4CNIConfig{
					Network: "192.168.0.0/16",
					Subnet:  "192.168.1.1/24",
					Gateway: "192.168.1.1",
				},
			},
			setupFunc: func(t *testing.T, storePath string) {
				// Create an existing file with different content
				oldCfg := &types.CNIConfig{
					Name:   "overwrite-network",
					MTU:    9000,
					IPMasq: false,
					IPv4: &types.IPv4CNIConfig{
						Network: "10.0.0.0/8",
						Subnet:  "10.0.0.1/24",
					},
				}
				data, err := json.Marshal(oldCfg)
				must.NoError(t, err)

				filePath := filepath.Join(storePath, oldCfg.Name+".conf")
				must.NoError(t, os.WriteFile(filePath, data, 0644))
			},
			validateFunc: func(t *testing.T, storePath string, cfg *types.CNIConfig) {
				filePath := filepath.Join(storePath, cfg.Name+".conf")
				data, err := os.ReadFile(filePath)
				must.NoError(t, err)

				var readCfg types.CNIConfig
				must.NoError(t, json.Unmarshal(data, &readCfg))

				// Verify new config, not old config
				must.Eq(t, 1400, readCfg.MTU)
				must.Eq(t, "192.168.0.0/16", readCfg.IPv4.Network)
			},
		},
		{
			name:      "multiple configs",
			storePath: t.TempDir(),
			config: &types.CNIConfig{
				Name:   "network-2",
				MTU:    1450,
				IPMasq: true,
				IPv4: &types.IPv4CNIConfig{
					Network: "10.1.0.0/16",
					Subnet:  "10.1.1.1/24",
				},
			},
			setupFunc: func(t *testing.T, storePath string) {
				// Create first network config
				cfg1 := &types.CNIConfig{
					Name:   "network-1",
					MTU:    1500,
					IPMasq: false,
					IPv4: &types.IPv4CNIConfig{
						Network: "10.0.0.0/16",
						Subnet:  "10.0.1.1/24",
					},
				}
				store := NewCNIStore(storePath)
				must.NoError(t, store.Set(cfg1))
			},
			validateFunc: func(t *testing.T, storePath string, cfg *types.CNIConfig) {
				// Verify both files exist
				must.FileExists(t, filepath.Join(storePath, "network-1.conf"))
				must.FileExists(t, filepath.Join(storePath, "network-2.conf"))
			},
		},
		{
			name:      "read-only directory",
			storePath: filepath.Join(t.TempDir(), "readonly"),
			config: &types.CNIConfig{
				Name:   "readonly-test",
				MTU:    1450,
				IPMasq: true,
				IPv4: &types.IPv4CNIConfig{
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
			err := NewCNIStore(tc.storePath).Set(tc.config)

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
