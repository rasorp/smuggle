package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/urfave/cli/v3"

	"github.com/rasorp/smuggle/internal/helper"
)

const (
	dataDirFlag          = "data-dir"
	disableIPMasqFlag    = "disable-ipmasq"
	networkInterfaceFlag = "network-interface"
	serversFlag          = "servers"

	// DefaultCNIDir is the directory where Smuggle writes CNI configuration
	// files when no other path is provided.
	DefaultCNIDir = "/opt/smuggle/config"
)

type ClientConfig struct {

	// DataDir is the directory where client related data will be stored
	// including generated CNI configuration options and the Smuggle agent ID.
	DataDir string `hcl:"data_dir,optional" json:"data_dir"`

	// DisableIPMasq disables IP masquerading for the client networks which is
	// used for routing taffic from the container to the internet.
	DisableIPMasq *bool `hcl:"disable_ipmasq,optional" json:"disable_ipmasq"`

	// NetworkInterface specifies the network interface to use for client
	// networking. If not specified, the default interface will be identified
	// and used.
	NetworkInterface string `hcl:"network_interface,optional" json:"network_interface"`

	// Servers holds the addresses of Smuggle servers the client connects to
	// via RPC for all subnet and network operations.
	Servers *ServersConfig `hcl:"servers,block" json:"servers"`
}

func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		DataDir:          "/var/lib/smuggle/client",
		DisableIPMasq:    helper.PointerOf(false),
		NetworkInterface: "",
		Servers:          DefaultServersConfig(),
	}
}

func (c *ClientConfig) Merge(other *ClientConfig) *ClientConfig {
	if c == nil {
		return other
	}
	if other == nil {
		return c
	}

	result := *c

	if other.DataDir != "" {
		result.DataDir = other.DataDir
	}
	if other.DisableIPMasq != nil {
		result.DisableIPMasq = other.DisableIPMasq
	}
	if other.NetworkInterface != "" {
		result.NetworkInterface = other.NetworkInterface
	}
	if other.Servers != nil {
		result.Servers = result.Servers.Merge(other.Servers)
	}

	return &result
}

func (c *ClientConfig) Validate() []error {
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
	flags := ServersConfigCommandFlags()
	flags = append(flags, []cli.Flag{
		&cli.StringFlag{
			HideDefault: true,
			Name:        dataDirFlag,
			Usage:       "The data directory for the Smuggle client",
			Sources:     cli.EnvVars("SMUGGLE_DATA_DIR"),
		},
		&cli.BoolFlag{
			HideDefault: true,
			Name:        disableIPMasqFlag,
			Usage:       "Disable IP masquerading for client networks",
			Sources:     cli.EnvVars("SMUGGLE_DISABLE_IPMASQ"),
		},
		&cli.StringFlag{
			HideDefault: true,
			Name:        networkInterfaceFlag,
			Usage:       "The network interface to use for client networking",
			Sources:     cli.EnvVars("SMUGGLE_NETWORK_INTERFACE"),
		},
	}...)
	return flags
}

func ClientConfigFromCommand(c *cli.Command) *ClientConfig {
	cfg := &ClientConfig{
		DataDir:          c.String(dataDirFlag),
		NetworkInterface: c.String(networkInterfaceFlag),
		Servers:          ServersConfigFromCommand(c),
	}

	if c.IsSet(disableIPMasqFlag) {
		cfg.DisableIPMasq = helper.PointerOf(c.Bool(disableIPMasqFlag))
	}

	return cfg
}

// ClientAgentConfig is the top-level configuration for the Smuggle client
// agent, combining the client sub-config with shared infrastructure config.
type ClientAgentConfig struct {
	Client *ClientConfig `hcl:"client,block" json:"client"`
	HTTP   *HTTPConfig   `hcl:"http,block" json:"http"`
	Log    *LogConfig    `hcl:"log,block" json:"log"`
}

func DefaultClientAgentConfig() *ClientAgentConfig {
	return &ClientAgentConfig{
		Client: DefaultClientConfig(),
		HTTP:   DefaultHTTPConfig(),
		Log:    DefaultLogConfig(),
	}
}

func (c *ClientAgentConfig) Merge(other *ClientAgentConfig) *ClientAgentConfig {
	if c == nil {
		return other
	}
	if other == nil {
		return c
	}

	result := *c

	result.Client = result.Client.Merge(other.Client)
	result.HTTP = result.HTTP.Merge(other.HTTP)
	result.Log = result.Log.Merge(other.Log)

	return &result
}

func (c *ClientAgentConfig) Validate() []error {
	var errs []error

	errs = append(errs, c.Client.Validate()...)
	errs = append(errs, c.HTTP.Validate()...)
	errs = append(errs, c.Log.Validate()...)

	return errs
}

func ClientAgentConfigCommandFlags() []cli.Flag {
	flags := []cli.Flag{
		&cli.StringSliceFlag{
			Name:  "config",
			Usage: "The path to a configuration file to load",
		},
	}
	flags = append(flags, ClientConfigCommandFlags()...)
	flags = append(flags, HTTPConfigCommandFlags()...)
	flags = append(flags, LogConfigCommandFlags()...)
	return flags
}

func ClientAgentConfigFromCommand(cmd *cli.Command) *ClientAgentConfig {
	return &ClientAgentConfig{
		Client: ClientConfigFromCommand(cmd),
		HTTP:   HTTPConfigFromCommand(cmd),
		Log:    LogConfigFromCommand(cmd),
	}
}

func ClientAgentConfigFromFiles(cmd *cli.Command) (*ClientAgentConfig, error) {
	var cfg *ClientAgentConfig

	for _, file := range cmd.StringSlice("config") {
		fileCfg, err := parseClientAgentConfigFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to parse config file %q: %w", file, err)
		}
		cfg = cfg.Merge(fileCfg)
	}

	return cfg, nil
}

func parseClientAgentConfigFile(path string) (*ClientAgentConfig, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	switch filepath.Ext(absPath) {
	case ".json":
		return parseClientAgentConfigJSONFile(absPath)
	case ".hcl":
		return parseClientAgentConfigHCLFile(absPath)
	default:
		return nil, fmt.Errorf("unsupported config file format: %q", filepath.Ext(absPath))
	}
}

func parseClientAgentConfigHCLFile(path string) (*ClientAgentConfig, error) {
	parser := hclparse.NewParser()

	f, diags := parser.ParseHCLFile(path)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL file: %w", diags)
	}

	var resp ClientAgentConfig
	diags = gohcl.DecodeBody(f.Body, nil, &resp)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode HCL: %w", diags)
	}

	return &resp, nil
}

func parseClientAgentConfigJSONFile(path string) (*ClientAgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON file: %w", err)
	}

	parser := hclparse.NewParser()
	f, diags := parser.ParseJSON(data, path)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse JSON file: %w", diags)
	}

	var resp ClientAgentConfig
	diags = gohcl.DecodeBody(f.Body, nil, &resp)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode JSON: %w", diags)
	}

	return &resp, nil
}

// ServersConfig holds the addresses of Smuggle servers that a client agent
// connects to for all store operations. It is kept as a dedicated struct to
// allow future expansion (e.g. TLS settings, per-server weights) without
// breaking the existing configuration schema.
type ServersConfig struct {
	// Addresses is the list of server RPC addresses in "host:port" form.
	Addresses []string `hcl:"addresses,optional" json:"addresses"`
}

// DefaultServersConfig returns an empty ServersConfig.
func DefaultServersConfig() *ServersConfig {
	return &ServersConfig{
		Addresses: []string{
			"localhost:8081",
		},
	}
}

// IsConfigured returns true when at least one server address has been set.
func (s *ServersConfig) IsConfigured() bool {
	return s != nil && len(s.Addresses) > 0
}

func (s *ServersConfig) Merge(other *ServersConfig) *ServersConfig {
	if s == nil {
		return other
	}
	if other == nil {
		return s
	}
	result := *s
	if len(other.Addresses) > 0 {
		result.Addresses = other.Addresses
	}
	return &result
}

func (s *ServersConfig) Validate() []error {
	return nil
}

func ServersConfigCommandFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringSliceFlag{
			HideDefault: true,
			Name:        serversFlag,
			Usage:       "Smuggle server addresses (host:port) to connect to for store operations",
			Sources:     cli.EnvVars("SMUGGLE_SERVERS"),
		},
	}
}

func ServersConfigFromCommand(cmd *cli.Command) *ServersConfig {
	return &ServersConfig{
		Addresses: cmd.StringSlice(serversFlag),
	}
}
