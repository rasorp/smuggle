package cni

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	helperfile "github.com/rasorp/smuggle/internal/helper/file"
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

	// Guard against path traversal; cfg.Name must be a plain filename with no
	// directory components.
	if cfg.Name != filepath.Base(cfg.Name) {
		return fmt.Errorf("invalid config name %q: path traversal detected", cfg.Name)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal CNI config: %w", err)
	}

	if err := helperfile.AtomicWrite(filepath.Join(s.path, cfg.Name+".conf"), data, 0644); err != nil {
		return fmt.Errorf("failed to write CNI config: %w", err)
	}

	return nil
}

// Delete removes the CNI configuration file for the named network. It is a
// no-op if the file does not exist.
func (s *Store) Delete(name string) error {
	if name != filepath.Base(name) {
		return fmt.Errorf("invalid config name %q: path traversal detected", name)
	}
	return helperfile.Delete(filepath.Join(s.path, name+".conf"))
}
