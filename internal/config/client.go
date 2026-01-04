package config

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"

	"github.com/urfave/cli/v3"

	"github.com/rasorp/smuggle/internal/helper"
)

const (
	clientEnabledFlag          = "client-enabled"
	clientDataDirFlag          = "client-data-dir"
	clientDisableIPMasqFlag    = "client-disable-ipmasq"
	clientNetworkInterfaceFlag = "client-network-interface"
)

type ClientConfig struct {

	// Enabled indicates whether the client functionality is enabled.
	Enabled *bool `hcl:"enabled,optional" json:"enabled"`

	// DataDir is the directory where client related data will be stored
	// including generated CNI configuration options and the Smuggle agent ID.
	DataDir string `hcl:"data_dir,optional" json:"data_dir"`

	// DisableIPMasq disables IP masquerading for the client networks which is
	// used for routing taffic from the container to the internet.
	DisableIPMasq bool `hcl:"disable_ipmasq,optional" json:"disable_ipmasq"`

	// NetworkInterface specifies the network interface to use for client
	// networking. If not specified, the default interface will be identified
	// and used.
	NetworkInterface string `hcl:"network_interface,optional" json:"network_interface"`
}

func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		Enabled:          helper.PointerOf(false),
		DataDir:          "/var/lib/smuggle/client",
		DisableIPMasq:    false,
		NetworkInterface: "",
	}
}

func (c *ClientConfig) IsEnabled() bool { return c != nil && c.Enabled != nil && *c.Enabled }

func (c *ClientConfig) Merge(other *ClientConfig) *ClientConfig {
	if c == nil {
		return other
	}
	if other == nil {
		return c
	}

	result := *c

	if other.Enabled != nil {
		result.Enabled = other.Enabled
	}
	if other.DataDir != "" {
		result.DataDir = other.DataDir
	}
	if other.DisableIPMasq {
		result.DisableIPMasq = other.DisableIPMasq
	}
	if other.NetworkInterface != "" {
		result.NetworkInterface = other.NetworkInterface
	}

	return &result
}

func (c *ClientConfig) Validate() []error {

	if !c.IsEnabled() {
		return nil
	}

	var errs []error

	if runtime.GOOS != "linux" {
		errs = append(errs, fmt.Errorf("client functionality not supported on %q", runtime.GOOS))
	}
	if !filepath.IsAbs(c.DataDir) || c.DataDir == "" {
		errs = append(errs, errors.New("client data directory must be an absolute path"))
	}

	return errs
}

func ClientConfigCommandFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			HideDefault: true,
			Name:        clientEnabledFlag,
			Usage:       "Enable or disable the Smuggle client functionality",
			Sources:     cli.EnvVars("SMUGGLE_CLIENT_ENABLED"),
		},
		&cli.StringFlag{
			HideDefault: true,
			Name:        clientDataDirFlag,
			Usage:       "The data directory for the Smuggle client",
			Sources:     cli.EnvVars("SMUGGLE_CLIENT_DATA_DIR"),
		},
		&cli.BoolFlag{
			HideDefault: true,
			Name:        clientDisableIPMasqFlag,
			Usage:       "Disable IP masquerading for client networks",
			Sources:     cli.EnvVars("SMUGGLE_CLIENT_DISABLE_IPMASQ"),
		},
		&cli.StringFlag{
			HideDefault: true,
			Name:        clientNetworkInterfaceFlag,
			Usage:       "The network interface to use for client networking",
			Sources:     cli.EnvVars("SMUGGLE_CLIENT_NETWORK_INTERFACE"),
		},
	}
}

func ClientConfigFromCommand(c *cli.Command) *ClientConfig {
	cfg := &ClientConfig{
		DataDir:          c.String(clientDataDirFlag),
		NetworkInterface: c.String(clientNetworkInterfaceFlag),
	}

	if c.IsSet(clientEnabledFlag) {
		cfg.Enabled = helper.PointerOf(c.Bool(clientEnabledFlag))
	}
	if c.IsSet(clientDisableIPMasqFlag) {
		cfg.DisableIPMasq = c.Bool(clientDisableIPMasqFlag)
	}

	return cfg
}
