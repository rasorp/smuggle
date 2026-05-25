package types

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/helper"
)

// Network represents a network configuration defining the address space
// and behavior for a Smuggle network.
type Network struct {
	Name     string          `json:"name"`
	IPMasq   *bool           `json:"ipmasq"`
	IPv4     *IPv4Config     `json:"ipv4"`
	Provider *ProviderConfig `json:"provider"`
}

// IPv4Config defines the IPv4 address space configuration for a network.
type IPv4Config struct {
	Network *IPv4Net `json:"network"`
	Min     IPv4Addr `json:"min"`
	Max     IPv4Addr `json:"max"`
	Size    uint     `json:"size"`
}

// ProviderConfig specifies which network provider implementation to use.
type ProviderConfig struct {

	// Name is the name of the network provider and must correspond to a
	// registered provider.
	Name string `json:"name"`

	// Config contains provider-specific configuration data in raw JSON format
	// which is opaque to the core Smuggle system and passed directly to the
	// network provider implementation.
	Config json.RawMessage `json:"config,omitempty"`
}

// Canonicalize fills in default values for unset fields in the network
// configuration.
func (n *Network) Canonicalize() {
	if n.IPv4 != nil {
		if n.IPv4.Min == EmptyIPv4Addr {
			n.IPv4.Min = n.IPv4.Network.IP + IPv4Addr(1<<(32-n.IPv4.Size))
		}
		if n.IPv4.Max == EmptyIPv4Addr {
			n.IPv4.Max = n.IPv4.Network.NextNetwork().IP - IPv4Addr(1<<(32-n.IPv4.Size)) - 1
		}
	}

	if n.IPMasq == nil {
		n.IPMasq = helper.PointerOf(true)
	}
}

// Equals compares two network configurations for equality. The name field is
// intentionally excluded from the comparison as networks are identified by
// this field.
func (n *Network) Equals(other *Network) bool {

	if n == nil && other == nil {
		return true
	}
	if n == nil || other == nil {
		return false
	}

	if (n.IPMasq == nil) != (other.IPMasq == nil) {
		return false
	}
	if n.IPMasq != nil && other.IPMasq != nil && *n.IPMasq != *other.IPMasq {
		return false
	}

	// Compare IPv4 configuration.
	if (n.IPv4 == nil) != (other.IPv4 == nil) {
		return false
	}
	if n.IPv4 != nil && other.IPv4 != nil {
		if (n.IPv4.Network == nil) != (other.IPv4.Network == nil) {
			return false
		}
		if n.IPv4.Network != nil && other.IPv4.Network != nil {
			if n.IPv4.Network.IP != other.IPv4.Network.IP ||
				n.IPv4.Network.Size != other.IPv4.Network.Size {
				return false
			}
		}
		if n.IPv4.Min != other.IPv4.Min {
			return false
		}
		if n.IPv4.Max != other.IPv4.Max {
			return false
		}
		if n.IPv4.Size != other.IPv4.Size {
			return false
		}
	}

	// Compare Provider configuration.
	if (n.Provider == nil) != (other.Provider == nil) {
		return false
	}
	if n.Provider != nil && other.Provider != nil {
		if n.Provider.Name != other.Provider.Name {
			return false
		}
		if !bytes.Equal(n.Provider.Config, other.Provider.Config) {
			return false
		}
	}

	return true
}

// InterfaceName returns the name of the network interface that is used for
// this network. Networks are expected to have a single interface per host, so
// this with a static suffix of "0" is sufficient to be unique.
func (n *Network) InterfaceName() string { return n.Name + "0" }

// BridgeInterfaceName returns the name of the bridge interface that containers
// connect to for this network.
func (n *Network) BridgeInterfaceName() string { return n.Name + "brd0" }

// LoggingPairs returns a set of zap fields for logging this network.
func (n *Network) LoggingPairs() []zap.Field {
	f := []zap.Field{
		zap.String("network_name", n.Name),
	}

	if n.IPv4 != nil {
		f = append(
			f,
			zap.String("ipv4_network", n.IPv4.Network.String()),
			zap.Uint("ipv4_size", n.IPv4.Network.Size),
		)
	}

	if n.Provider != nil {
		f = append(f, zap.String("provider", n.Provider.Name))
	}

	return f
}

// networkNameRe is the set of characters permitted in a network name. Names
// must start with a lowercase letter and may only contain lowercase letters,
// digits, hyphens, and underscores.
var networkNameRe = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)

// maxNetworkNameLen is the maximum number of characters allowed in a network
// name. The binding constraint is BridgeInterfaceName(), which appends "brd0"
// (4 chars) to the name. Linux caps interface names at 15 characters, so the
// name itself must be no longer than 11 characters.
const maxNetworkNameLen = 11

// Validate performs validation on the network configuration to ensure it is
// usable.
func (n *Network) Validate() error {

	// Validate the network name before it is used to derive Linux interface
	// names and iptables rule specs.
	if n.Name == "" {
		return errors.New("network name must not be empty")
	}
	if len(n.Name) > maxNetworkNameLen {
		return fmt.Errorf("network name %q is too long: maximum %d characters allowed",
			n.Name, maxNetworkNameLen)
	}
	if !networkNameRe.MatchString(n.Name) {
		return fmt.Errorf("network name %q is invalid: regex %s", n.Name, networkNameRe.String())
	}

	// Validation for the IPv4 configuration.
	if n.IPv4 == nil {
		return errors.New("network configuration is empty")
	}
	if n.IPv4.Network == nil {
		return errors.New("IPv4 network configuration is missing")
	}
	if n.IPv4.Size == 0 || n.IPv4.Size > 32 {
		return errors.New("IPv4 subnet size must be between 1 and 32")
	}
	if n.IPv4.Min != EmptyIPv4Addr && (n.IPv4.Min < n.IPv4.Network.IP || n.IPv4.Min > n.IPv4.Network.NextNetwork().IP-1) {
		return errors.New("IPv4 minimum address is out of network range")
	}

	// Validation for the network provider configuration.
	if n.Provider == nil {
		return errors.New("network provider configuration is missing")
	}
	if n.Provider.Name != "vxlan" {
		return fmt.Errorf("unsupported network provider: %q", n.Provider.Name)
	}

	return nil
}
