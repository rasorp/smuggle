package cni

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rasorp/smuggle/internal/types"
)

// Store implements the CNIStore interface by writing CNI configurations to a
// file on disk using atomic write operations.
type Store struct {
	path string
}

// Config represents a CNI (Container Network Interface) configuration.
type Config struct {
	Name   string      `json:"name"`
	Bridge string      `json:"bridge,omitempty"`
	MTU    int         `json:"mtu"`
	IPMasq bool        `json:"ipmasq"`
	IPv4   *IPv4Config `json:"ipv4"`
}

// IPv4Config represents IPv4-specific CNI configuration.
type IPv4Config struct {
	Network string `json:"network"`
	Subnet  string `json:"subnet"`
	Gateway string `json:"gateway,omitempty"`
}

// GenerateCNIConfig creates a CNI configuration from network and subnet
// configurations.
func GenerateCNIConfig(network *types.Network, subnet *types.Subnet) *Config {
	return &Config{
		Name:   network.Name,
		Bridge: network.Name + "brd0",
		MTU:    subnet.MTU,
		IPMasq: *network.IPMasq,
		IPv4: &IPv4Config{
			Network: network.IPv4.Network.String(),
			Subnet:  subnet.IPv4Network.NextAddr().String(),
			Gateway: subnet.IPv4Network.NextAddr().IP.String(),
		},
	}
}

// NewCNIFileStore creates a new CNIFileStore that will write to the specified
// file path. The path can be absolute or relative, and parent directories will
// be created if they don't exist.
func NewStore(path string) *Store {
	return &Store{
		path: path,
	}
}

// Set writes the CNI configuration to the configured file path atomically.
// The write is atomic by first writing to a temporary file in the same directory,
// then renaming it to the target file. This ensures that readers never see a
// partially written file.
func (s *Store) Set(cfg *Config) error {
	if cfg == nil {
		return errors.New("CNI config cannot be nil")
	}

	// Ensure the parent directory exists
	if err := os.MkdirAll(s.path, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Marshal the configuration to JSON
	data, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("failed to marshal CNI config: %w", err)
	}

	// Create a temporary file in the same directory as the target file
	// This ensures the temp file is on the same filesystem, which is required
	// for atomic rename operations
	tempFile, err := os.CreateTemp(s.path, ".cni-*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temporary file: %w", err)
	}
	tempPath := tempFile.Name()

	// Clean up the temp file if we fail before the rename
	defer func() {
		if tempFile != nil {
			_ = tempFile.Close()
			_ = os.Remove(tempPath)
		}
	}()

	// Write the data to the temporary file
	if _, err := tempFile.Write(data); err != nil {
		return fmt.Errorf("failed to write to temporary file: %w", err)
	}

	// Sync to ensure data is written to disk
	if err := tempFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync temporary file: %w", err)
	}

	// Close the temporary file before renaming
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("failed to close temporary file: %w", err)
	}
	tempFile = nil // Prevent deferred cleanup from trying to close again

	// Guard against path traversal; cfg.Name must be a plain filename with no
	// directory components. The name is based on the network name which is
	// validated on read, but we add this extra check here to be safe since
	// we're using it as a filename.
	if cfg.Name != filepath.Base(cfg.Name) {
		return fmt.Errorf("invalid config name %q: path traversal detected", cfg.Name)
	}

	filename := filepath.Join(s.path, cfg.Name+".conf")

	// Atomically rename the temporary file to the target path
	// On Unix systems, this is atomic even if the target file exists
	if err := os.Rename(tempPath, filename); err != nil {
		return fmt.Errorf("failed to rename temporary file: %w", err)
	}

	return nil
}
