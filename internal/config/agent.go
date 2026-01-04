package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/urfave/cli/v3"
)

type AgentConfig struct {
	Client *ClientConfig `hcl:"client,block" json:"client"`
	HTTP   *HTTPConfig   `hcl:"http,block" json:"http"`
	Log    *LogConfig    `hcl:"log,block" json:"log"`
	Nomad  *NomadConfig  `hcl:"nomad,block" json:"nomad"`
	Server *ServerConfig `hcl:"server,block" json:"server"`
	Store  *StoreConfig  `hcl:"store,block" json:"store"`
}

func (a *AgentConfig) Merge(other *AgentConfig) *AgentConfig {
	if a == nil {
		return other
	}
	if other == nil {
		return a
	}

	result := *a

	result.Client = result.Client.Merge(other.Client)
	result.HTTP = result.HTTP.Merge(other.HTTP)
	result.Log = result.Log.Merge(other.Log)
	result.Nomad = result.Nomad.Merge(other.Nomad)
	result.Server = result.Server.Merge(other.Server)
	result.Store = result.Store.Merge(other.Store)

	return &result
}

func (a *AgentConfig) Validate() []error {
	var errs []error

	// An agent must be configured in either client or server mode, but not
	// both. It would be possible to run an agent in both modes, but it does not
	// make sense in practice.
	if a.Client.IsEnabled() && a.Server.IsEnabled() {
		errs = append(errs, errors.New("agent cannot be configured as both client and server"))
	}
	if !a.Client.IsEnabled() && !a.Server.IsEnabled() {
		errs = append(errs, errors.New("agent must be configured as either client or server"))
	}

	errs = append(errs, a.Client.Validate()...)
	errs = append(errs, a.HTTP.Validate()...)
	errs = append(errs, a.Log.Validate()...)
	errs = append(errs, a.Nomad.Validate()...)
	errs = append(errs, a.Server.Validate()...)
	errs = append(errs, a.Store.Validate()...)

	return errs
}

// DefaultConfig returns the default configuration for the agent.
func DefaultAgentConfig() *AgentConfig {
	return &AgentConfig{
		Client: DefaultClientConfig(),
		HTTP:   DefaultHTTPConfig(),
		Log:    DefaultLogConfig(),
		Nomad:  DefaultNomadConfig(),
		Server: DefaultServerConfig(),
		Store:  DefaultStoreConfig(),
	}
}

func AgentConfigCommandFlags() []cli.Flag {
	flags := []cli.Flag{
		&cli.StringSliceFlag{
			Name:  "config",
			Usage: "The path to a configuration file to load",
		},
	}
	flags = append(flags, ClientConfigCommandFlags()...)
	flags = append(flags, HTTPConfigCommandFlags()...)
	flags = append(flags, LogConfigCommandFlags()...)
	flags = append(flags, NomadConfigCommandFlags()...)
	flags = append(flags, ServerConfigCommandFlags()...)
	flags = append(flags, StoreConfigCommandFlags()...)
	return flags
}

func AgentConfigFromCommand(cmd *cli.Command) *AgentConfig {
	return &AgentConfig{
		Client: ClientConfigFromCommand(cmd),
		HTTP:   HTTPConfigFromCommand(cmd),
		Log:    LogConfigFromCommand(cmd),
		Nomad:  NomadConfigFromCommand(cmd),
		Server: ServerConfigFromCommand(cmd),
		Store:  StoreConfigFromCommand(cmd),
	}
}

// AgentConfigFromFiles loads and merges configuration passed via the config CLI
// flag.
func AgentConfigFromFiles(cmd *cli.Command) (*AgentConfig, error) {
	var cfg *AgentConfig

	for _, file := range cmd.StringArgs("config") {
		fileCfg, err := parseAgentConfgigFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to parse config file %q: %w", file, err)
		}
		cfg = cfg.Merge(fileCfg)
	}

	return cfg, nil
}

func parseAgentConfgigFile(path string) (*AgentConfig, error) {

	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	switch filepath.Ext(absPath) {
	case ".json":
		return parseJSONConfigFile(absPath)
	case ".hcl":
		return parseHCLConfigFile(absPath)
	default:
		return nil, fmt.Errorf("unsupported config file format: %q", filepath.Ext(absPath))
	}
}

func parseHCLConfigFile(path string) (*AgentConfig, error) {
	parser := hclparse.NewParser()

	f, diags := parser.ParseHCLFile(path)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL file: %w", diags)
	}

	var resp AgentConfig
	diags = gohcl.DecodeBody(f.Body, nil, &resp)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode HCL: %w", diags)
	}

	// Parse duration strings into time.Duration values
	if resp.Server != nil && resp.Server.Reaper != nil {
		if err := resp.Server.Reaper.Parse(); err != nil {
			return nil, fmt.Errorf("failed to parse server config: %w", err)
		}
	}

	return &resp, nil
}

func parseJSONConfigFile(path string) (*AgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON file: %w", err)
	}

	parser := hclparse.NewParser()
	f, diags := parser.ParseJSON(data, path)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse JSON file: %w", diags)
	}

	var resp AgentConfig
	diags = gohcl.DecodeBody(f.Body, nil, &resp)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode JSON: %w", diags)
	}

	// Parse duration strings into time.Duration values
	if resp.Server != nil && resp.Server.Reaper != nil {
		if err := resp.Server.Reaper.Parse(); err != nil {
			return nil, fmt.Errorf("failed to parse server config: %w", err)
		}
	}

	return &resp, nil
}
