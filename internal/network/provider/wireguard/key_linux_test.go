package wireguard

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/shoenig/test/must"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func Test_loadOrGenerateKey(t *testing.T) {

	// Pre-generate a known valid key used by cases that need a pre-existing
	// key file on disk.
	existingKey, err := wgtypes.GeneratePrivateKey()
	must.NoError(t, err)

	testCases := []struct {
		name          string
		setupFunc     func(t *testing.T) string
		validateFunc  func(t *testing.T, path string, key wgtypes.Key)
		errorContains string
	}{
		{
			name: "existing file with valid key",
			setupFunc: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "privatekey")
				must.NoError(t, os.WriteFile(path, []byte(existingKey.String()), 0600))
				return path
			},
			validateFunc: func(t *testing.T, _ string, key wgtypes.Key) {
				must.EqOp(t, existingKey, key)
			},
		},
		{
			name: "existing file with invalid content",
			setupFunc: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "privatekey")
				must.NoError(t, os.WriteFile(path, []byte("not-a-valid-wireguard-key"), 0600))
				return path
			},
			errorContains: "failed to parse key",
		},
		{
			name: "no existing file",
			setupFunc: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "privatekey")
			},
			validateFunc: func(t *testing.T, path string, key wgtypes.Key) {
				// Returned key must not be the zero value.
				must.NotEqOp(t, wgtypes.Key{}, key)

				// The key must have been written to disk.
				data, readErr := os.ReadFile(path)
				must.NoError(t, readErr)

				// The on-disk representation must round-trip to the same key.
				persisted, parseErr := wgtypes.ParseKey(string(data))
				must.NoError(t, parseErr)
				must.EqOp(t, key, persisted)
			},
		},
		{
			name: "no existing file missing parent dir",
			setupFunc: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "a", "b", "c", "privatekey")
			},
			validateFunc: func(t *testing.T, path string, key wgtypes.Key) {
				must.NotEqOp(t, wgtypes.Key{}, key)

				data, readErr := os.ReadFile(path)
				must.NoError(t, readErr)

				persisted, parseErr := wgtypes.ParseKey(string(data))
				must.NoError(t, parseErr)
				must.EqOp(t, key, persisted)
			},
		},
		{
			name: "file read error",
			setupFunc: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), "privatekey")
				must.NoError(t, os.MkdirAll(path, 0700))
				return path
			},
			errorContains: "failed to read key file",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			path := tc.setupFunc(t)

			key, err := loadOrGenerateKey(path)

			if tc.errorContains != "" {
				must.ErrorContains(t, err, tc.errorContains)
				return
			}

			must.NoError(t, err)
			if tc.validateFunc != nil {
				tc.validateFunc(t, path, key)
			}
		})
	}
}
