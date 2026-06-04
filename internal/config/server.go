package config

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/urfave/cli/v3"
)

const (
	reaperIntervalFlag  = "reaper-interval"
	reaperThresholdFlag = "reaper-threshold"
)

type ServerConfig struct {
	RPC    *RPCConfig    `hcl:"rpc,block" json:"rpc"`
	Reaper *ReaperConfig `hcl:"reaper,block" json:"reaper"`
}

type ReaperConfig struct {
	IntervalHCL string `hcl:"interval,optional" json:"interval"`
	Interval    time.Duration

	ThresholdHCL string `hcl:"threshold,optional" json:"threshold"`
	Threshold    time.Duration
}

func (r *ReaperConfig) Parse() error {
	if r == nil {
		return nil
	}

	if r.IntervalHCL != "" {
		d, err := time.ParseDuration(r.IntervalHCL)
		if err != nil {
			return err
		}
		r.Interval = d
	}

	if r.ThresholdHCL != "" {
		d, err := time.ParseDuration(r.ThresholdHCL)
		if err != nil {
			return err
		}
		r.Threshold = d
	}

	return nil
}

func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		RPC: DefaultRPCConfig(),
		Reaper: &ReaperConfig{
			Interval:  5 * time.Minute,
			Threshold: 5 * time.Minute,
		},
	}
}

func (s *ServerConfig) Merge(other *ServerConfig) *ServerConfig {
	if s == nil {
		return other
	}
	if other == nil {
		return s
	}

	result := *s

	if other.RPC != nil {
		result.RPC = result.RPC.Merge(other.RPC)
	}

	if other.Reaper != nil {
		if result.Reaper == nil {
			result.Reaper = &ReaperConfig{}
		} else {
			// Deep-copy the nested pointer so we never write back through the
			// receiver's Reaper field.
			reaperCopy := *result.Reaper
			result.Reaper = &reaperCopy
		}
		if other.Reaper.Interval != 0 {
			result.Reaper.Interval = other.Reaper.Interval
		}
		if other.Reaper.Threshold != 0 {
			result.Reaper.Threshold = other.Reaper.Threshold
		}
	}

	return &result
}

func (s *ServerConfig) Validate() []error {
	var errs []error
	if s.RPC != nil {
		errs = append(errs, s.RPC.Validate()...)
	}
	return errs
}

func ServerConfigCommandFlags() []cli.Flag {
	flags := RPCConfigCommandFlags()
	flags = append(flags, []cli.Flag{
		&cli.DurationFlag{
			HideDefault: true,
			Name:        reaperIntervalFlag,
			Usage:       "Interval between runs of the server reaper",
			Sources:     cli.EnvVars("SMUGGLE_REAPER_INTERVAL"),
		},
		&cli.DurationFlag{
			HideDefault: true,
			Name:        reaperThresholdFlag,
			Usage:       "Duration after which inactive clients are reaped",
			Sources:     cli.EnvVars("SMUGGLE_REAPER_THRESHOLD"),
		},
	}...)
	return flags
}

func ServerConfigFromCommand(cmd *cli.Command) *ServerConfig {
	return &ServerConfig{
		RPC: RPCConfigFromCommand(cmd),
		Reaper: &ReaperConfig{
			Interval:  cmd.Duration(reaperIntervalFlag),
			Threshold: cmd.Duration(reaperThresholdFlag),
		},
	}
}

// ServerAgentConfig is the top-level configuration for the Smuggle server
// agent, combining the server sub-config with shared infrastructure config.
type ServerAgentConfig struct {
	HTTP   *HTTPConfig   `hcl:"http,block" json:"http"`
	Log    *LogConfig    `hcl:"log,block" json:"log"`
	Nomad  *NomadConfig  `hcl:"nomad,block" json:"nomad"`
	Server *ServerConfig `hcl:"server,block" json:"server"`
	Store  *StoreConfig  `hcl:"store,block" json:"store"`
}

func DefaultServerAgentConfig() *ServerAgentConfig {
	return &ServerAgentConfig{
		HTTP:   DefaultHTTPConfig(),
		Log:    DefaultLogConfig(),
		Nomad:  DefaultNomadConfig(),
		Server: DefaultServerConfig(),
		Store:  DefaultStoreConfig(),
	}
}

func (s *ServerAgentConfig) Merge(other *ServerAgentConfig) *ServerAgentConfig {
	if s == nil {
		return other
	}
	if other == nil {
		return s
	}

	result := *s

	result.HTTP = result.HTTP.Merge(other.HTTP)
	result.Log = result.Log.Merge(other.Log)
	result.Nomad = result.Nomad.Merge(other.Nomad)
	result.Server = result.Server.Merge(other.Server)
	result.Store = result.Store.Merge(other.Store)

	return &result
}

func (s *ServerAgentConfig) Validate() []error {
	var errs []error

	errs = append(errs, s.HTTP.Validate()...)
	errs = append(errs, s.Log.Validate()...)
	errs = append(errs, s.Nomad.Validate()...)
	errs = append(errs, s.Server.Validate()...)
	errs = append(errs, s.Store.Validate()...)

	return errs
}

func ServerAgentConfigCommandFlags() []cli.Flag {
	flags := []cli.Flag{
		&cli.StringSliceFlag{
			Name:  "config",
			Usage: "The path to a configuration file to load",
		},
	}
	flags = append(flags, HTTPConfigCommandFlags()...)
	flags = append(flags, LogConfigCommandFlags()...)
	flags = append(flags, NomadConfigCommandFlags()...)
	flags = append(flags, ServerConfigCommandFlags()...)
	flags = append(flags, StoreConfigCommandFlags()...)
	return flags
}

func ServerAgentConfigFromCommand(cmd *cli.Command) *ServerAgentConfig {
	return &ServerAgentConfig{
		HTTP:   HTTPConfigFromCommand(cmd),
		Log:    LogConfigFromCommand(cmd),
		Nomad:  NomadConfigFromCommand(cmd),
		Server: ServerConfigFromCommand(cmd),
		Store:  StoreConfigFromCommand(cmd),
	}
}

func ServerAgentConfigFromFiles(cmd *cli.Command) (*ServerAgentConfig, error) {
	var cfg *ServerAgentConfig

	for _, file := range cmd.StringSlice("config") {
		fileCfg, err := parseServerAgentConfigFile(file)
		if err != nil {
			return nil, fmt.Errorf("failed to parse config file %q: %w", file, err)
		}
		cfg = cfg.Merge(fileCfg)
	}

	return cfg, nil
}

func parseServerAgentConfigFile(path string) (*ServerAgentConfig, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	switch filepath.Ext(absPath) {
	case ".json":
		return parseServerAgentConfigJSONFile(absPath)
	case ".hcl":
		return parseServerAgentConfigHCLFile(absPath)
	default:
		return nil, fmt.Errorf("unsupported config file format: %q", filepath.Ext(absPath))
	}
}

func parseServerAgentConfigHCLFile(path string) (*ServerAgentConfig, error) {
	parser := hclparse.NewParser()

	f, diags := parser.ParseHCLFile(path)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse HCL file: %w", diags)
	}

	var resp ServerAgentConfig
	diags = gohcl.DecodeBody(f.Body, nil, &resp)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode HCL: %w", diags)
	}

	if resp.Server != nil {
		if resp.Server.Reaper != nil {
			if err := resp.Server.Reaper.Parse(); err != nil {
				return nil, fmt.Errorf("failed to parse server reaper config: %w", err)
			}
		}
	}

	return &resp, nil
}

func parseServerAgentConfigJSONFile(path string) (*ServerAgentConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read JSON file: %w", err)
	}

	parser := hclparse.NewParser()
	f, diags := parser.ParseJSON(data, path)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to parse JSON file: %w", diags)
	}

	var resp ServerAgentConfig
	diags = gohcl.DecodeBody(f.Body, nil, &resp)
	if diags.HasErrors() {
		return nil, fmt.Errorf("failed to decode JSON: %w", diags)
	}

	if resp.Server != nil {
		if resp.Server.Reaper != nil {
			if err := resp.Server.Reaper.Parse(); err != nil {
				return nil, fmt.Errorf("failed to parse server reaper config: %w", err)
			}
		}
	}

	return &resp, nil
}
