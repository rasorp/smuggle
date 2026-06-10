package wireguard

import (
	"errors"
	"fmt"

	"go.uber.org/zap"
)

const (
	ProviderName = "wireguard"

	// encapsulationOverhead is the overhead in bytes introduced by WireGuard
	// tunnelling over IPv4 UDP.
	//   20  outer IPv4 header
	//    8  outer UDP header
	//   16  WireGuard transport header (type + receiver index + counter)
	//   16  Poly1305 authentication tag
	// ----
	//   60  total
	encapsulationOverhead = 60
)

// OperatorConfig holds the fields an operator may supply in the network's
// provider config block.
type OperatorConfig struct {

	// ListenPort is the UDP port that this host's WireGuard interface listens
	// on. If left to its default value of 0, the kernel will assign a free
	// ephemeral port.
	ListenPort int `json:"listen_port"`

	// PersistentKeepalive is the interval in seconds at which WireGuard will
	// send keepalive packets to the peer. It is required for peers behind NAT
	// to maintain the UDP mapping. A value of 0 disables keepalives.
	PersistentKeepalive int `json:"persistent_keepalive"`
}

// Validate checks that the operator-supplied configuration is valid.
func (o *OperatorConfig) Validate() error {
	if o.ListenPort < 0 || o.ListenPort > 65535 {
		return fmt.Errorf("wireguard listen_port %d out of range [0, 65535]", o.ListenPort)
	}
	if o.PersistentKeepalive < 0 {
		return errors.New("wireguard persistent_keepalive must not be negative")
	}
	return nil
}

// PeerConfig is the provider-generated state stored in the subnet config and
// distributed to remote peers via the remote store. All fields are {re-}computed
// by the provider; none are directly operator-supplied.
type PeerConfig struct {

	// ListenPort is the resolved UDP port.
	ListenPort int `json:"listen_port"`

	// PublicKey is the base64-encoded WireGuard public key for this host's
	// interface. It is derived from the private key and distributed to peers,
	// so they can configure their peer entries.
	PublicKey string `json:"public_key"`
}

// loggingPairs returns a set of zap fields representing the WireGuard
// peer configuration for logging purposes.
func (c *PeerConfig) loggingPairs() []zap.Field {
	return []zap.Field{
		zap.Int("listen_port", c.ListenPort),
		zap.String("public_key", c.PublicKey),
	}
}
