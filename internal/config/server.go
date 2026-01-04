package config

import (
	"time"

	"github.com/urfave/cli/v3"

	"github.com/rasorp/smuggle/internal/helper"
)

const (
	serverEnabledFlag         = "server-enabled"
	serverReaperIntervalFlag  = "server-reaper-interval"
	serverReaperThresholdFlag = "server-reaper-threshold"
)

type ServerConfig struct {
	Enabled *bool `hcl:"enabled,optional" json:"enabled"`

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
		Enabled: helper.PointerOf(false),
		Reaper: &ReaperConfig{
			Interval:  5 * time.Minute,
			Threshold: 5 * time.Minute,
		},
	}
}

func (s *ServerConfig) IsEnabled() bool { return s != nil && s.Enabled != nil && *s.Enabled }

func (s *ServerConfig) Merge(other *ServerConfig) *ServerConfig {
	if s == nil {
		return other
	}
	if other == nil {
		return s
	}

	if other.Enabled != nil {
		s.Enabled = other.Enabled
	}
	if other.Reaper != nil {
		if s.Reaper == nil {
			s.Reaper = &ReaperConfig{}
		}
		if other.Reaper.Interval != 0 {
			s.Reaper.Interval = other.Reaper.Interval
		}
		if other.Reaper.Threshold != 0 {
			s.Reaper.Threshold = other.Reaper.Threshold
		}
	}

	return s
}

func (s *ServerConfig) Validate() []error {

	if !s.IsEnabled() {
		return nil
	}

	var errs []error
	return errs
}

func ServerConfigCommandFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{
			HideDefault: true,
			Name:        serverEnabledFlag,
			Usage:       "Enable the Smuggle server functionality",
			Sources:     cli.EnvVars("SMUGGLE_SERVER_ENABLED"),
		},
		&cli.DurationFlag{
			HideDefault: true,
			Name:        serverReaperIntervalFlag,
			Usage:       "Interval between runs of the server reaper",
			Sources:     cli.EnvVars("SMUGGLE_SERVER_REAPER_INTERVAL"),
		},
		&cli.DurationFlag{
			HideDefault: true,
			Name:        serverReaperThresholdFlag,
			Usage:       "Duration after which inactive clients are reaped",
			Sources:     cli.EnvVars("SMUGGLE_SERVER_REAPER_THRESHOLD"),
		},
	}
}

func ServerConfigFromCommand(cmd *cli.Command) *ServerConfig {
	return &ServerConfig{
		Enabled: func() *bool {
			if cmd.IsSet(serverEnabledFlag) {
				val := cmd.Bool(serverEnabledFlag)
				return &val
			}
			return nil
		}(),
		Reaper: &ReaperConfig{
			Interval:  cmd.Duration(serverReaperIntervalFlag),
			Threshold: cmd.Duration(serverReaperThresholdFlag),
		},
	}
}
