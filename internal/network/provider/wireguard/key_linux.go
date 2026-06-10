package wireguard

import (
	"fmt"
	"os"
	"path/filepath"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

// loadOrGenerateKey loads the WireGuard private key from path. If the file does
// not exist a new key is generated and written to disk. The on-disk format is
// the standard base64 WireGuard key string, matching the output of `wg genkey`,
// so keys can be inspected or replaced with standard tooling if needed.
func loadOrGenerateKey(path string) (wgtypes.Key, error) {

	// Try and read the file from disk. If this succeeds, parse and return the
	// key or an error.
	data, err := os.ReadFile(path)
	if err == nil {
		key, err := wgtypes.ParseKey(string(data))
		if err != nil {
			return wgtypes.Key{}, fmt.Errorf("failed to parse key: %w", err)
		}
		return key, nil
	}

	// If the error is something other than "file does not exist", return it as
	// it's a sign of an known state and we should not attempt to generate a new
	// key.
	if !os.IsNotExist(err) {
		return wgtypes.Key{}, fmt.Errorf("failed to read key file: %w", err)
	}

	// Reaching this point means the file does not exist, so we need to generate
	// a new key and write it to disk.
	privateKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return wgtypes.Key{}, fmt.Errorf("failed to generate WireGuard private key: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return wgtypes.Key{}, fmt.Errorf("failed to create key directory: %w", err)
	}

	if err := os.WriteFile(path, []byte(privateKey.String()), 0600); err != nil {
		return wgtypes.Key{}, fmt.Errorf("failed to write WireGuard private key: %w", err)
	}

	return privateKey, nil
}
