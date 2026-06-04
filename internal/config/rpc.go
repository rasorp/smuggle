package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/urfave/cli/v3"
	"go.uber.org/zap"
)

const (
	rpcAccessLogLevelFlag = "rpc-access-log-level"
	rpcBindAddressFlag    = "rpc-bind-address"
	rpcBindPortFlag       = "rpc-bind-port"
	rpcWriteRateFlag      = "rpc-write-rate"
	rpcWriteBurstFlag     = "rpc-write-burst"
)

// RPCConfig controls the server-side RPC listener and write-through behaviour.
type RPCConfig struct {

	// AccessLogLevel controls the log level used for successful RPC call
	// entries.
	AccessLogLevel string `hcl:"access_log_level,optional" json:"access_log_level"`

	// BindAddress is the address the RPC server binds to.
	BindAddress string `hcl:"bind_address,optional" json:"bind_address"`

	// BindPort is the port the RPC server listens on.
	BindPort uint `hcl:"bind_port,optional" json:"bind_port"`

	// WriteRateLimit is the maximum number of writes per second forwarded to
	// the backing store. Zero means unlimited.
	WriteRateLimit int `hcl:"write_rate_limit,optional" json:"write_rate_limit"`

	// WriteBurst is the token-bucket burst size for the write rate limiter.
	WriteBurst int `hcl:"write_burst,optional" json:"write_burst"`
}

// DefaultRPCConfig returns production-ready defaults.
func DefaultRPCConfig() *RPCConfig {
	return &RPCConfig{
		AccessLogLevel: zap.DebugLevel.String(),
		BindAddress:    "localhost",
		BindPort:       8081,
		WriteRateLimit: 20,
		WriteBurst:     5,
	}
}

// Addr returns the "host:port" string for the RPC listener.
func (r *RPCConfig) Addr() string { return fmt.Sprintf("%s:%d", r.BindAddress, r.BindPort) }

func (r *RPCConfig) Merge(other *RPCConfig) *RPCConfig {
	if r == nil {
		return other
	}
	if other == nil {
		return r
	}
	result := *r
	if other.AccessLogLevel != "" {
		result.AccessLogLevel = other.AccessLogLevel
	}
	if other.BindAddress != "" {
		result.BindAddress = other.BindAddress
	}
	if other.BindPort != 0 {
		result.BindPort = other.BindPort
	}
	if other.WriteRateLimit != 0 {
		result.WriteRateLimit = other.WriteRateLimit
	}
	if other.WriteBurst != 0 {
		result.WriteBurst = other.WriteBurst
	}
	return &result
}

func (r *RPCConfig) Validate() []error {
	var errs []error
	if _, err := zap.ParseAtomicLevel(strings.ToLower(r.AccessLogLevel)); err != nil {
		errs = append(errs, err)
	}
	if r.BindPort == 0 || r.BindPort > 65535 {
		errs = append(errs, errors.New("rpc port must be between 1 and 65535"))
	}
	return errs
}

func RPCConfigCommandFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			HideDefault: true,
			Name:        rpcAccessLogLevelFlag,
			Usage:       "The access log level for the RPC server (debug, info, warn, error)",
			Sources:     cli.EnvVars("SMUGGLE_RPC_ACCESS_LOG_LEVEL"),
		},
		&cli.StringFlag{
			HideDefault: true,
			Name:        rpcBindAddressFlag,
			Usage:       "The address to bind the RPC server to",
			Sources:     cli.EnvVars("SMUGGLE_RPC_BIND_ADDRESS"),
		},
		&cli.UintFlag{
			HideDefault: true,
			Name:        rpcBindPortFlag,
			Usage:       "The port to bind the RPC server to",
			Sources:     cli.EnvVars("SMUGGLE_RPC_BIND_PORT"),
		},
		&cli.IntFlag{
			HideDefault: true,
			Name:        rpcWriteRateFlag,
			Usage:       "Maximum writes per second forwarded to the backing store (0 = unlimited)",
			Sources:     cli.EnvVars("SMUGGLE_RPC_WRITE_RATE"),
		},
		&cli.IntFlag{
			HideDefault: true,
			Name:        rpcWriteBurstFlag,
			Usage:       "Burst size for the RPC write rate limiter",
			Sources:     cli.EnvVars("SMUGGLE_RPC_WRITE_BURST"),
		},
	}
}

func RPCConfigFromCommand(cmd *cli.Command) *RPCConfig {
	return &RPCConfig{
		AccessLogLevel: cmd.String(rpcAccessLogLevelFlag),
		BindAddress:    cmd.String(rpcBindAddressFlag),
		BindPort:       cmd.Uint(rpcBindPortFlag),
		WriteRateLimit: cmd.Int(rpcWriteRateFlag),
		WriteBurst:     cmd.Int(rpcWriteBurstFlag),
	}
}
