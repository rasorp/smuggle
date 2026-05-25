package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shoenig/test/must"
	"github.com/urfave/cli/v3"

	"github.com/rasorp/smuggle/internal/helper"
)

func TestReaperConfig_Parse(t *testing.T) {
	testCases := []struct {
		name          string
		config        *ReaperConfig
		expected      *ReaperConfig
		expectedError bool
	}{
		{
			name:          "nil config",
			config:        nil,
			expected:      nil,
			expectedError: false,
		},
		{
			name:          "empty config",
			config:        &ReaperConfig{},
			expected:      &ReaperConfig{},
			expectedError: false,
		},
		{
			name:          "valid interval only",
			config:        &ReaperConfig{IntervalHCL: "10m"},
			expected:      &ReaperConfig{IntervalHCL: "10m", Interval: 10 * time.Minute},
			expectedError: false,
		},
		{
			name:          "valid threshold only",
			config:        &ReaperConfig{ThresholdHCL: "30s"},
			expected:      &ReaperConfig{ThresholdHCL: "30s", Threshold: 30 * time.Second},
			expectedError: false,
		},
		{
			name:          "both valid",
			config:        &ReaperConfig{IntervalHCL: "5m", ThresholdHCL: "1h"},
			expected:      &ReaperConfig{IntervalHCL: "5m", Interval: 5 * time.Minute, ThresholdHCL: "1h", Threshold: time.Hour},
			expectedError: false,
		},
		{
			name:          "invalid interval",
			config:        &ReaperConfig{IntervalHCL: "not-a-duration"},
			expectedError: true,
		},
		{
			name:          "invalid threshold",
			config:        &ReaperConfig{ThresholdHCL: "not-a-duration"},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.config.Parse()
			if tc.expectedError {
				must.Error(t, err)
			} else {
				must.NoError(t, err)
				must.Eq(t, tc.expected, tc.config)
			}
		})
	}
}

func Test_DefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()

	must.NotNil(t, cfg)
	must.NotNil(t, cfg.Reaper)
	must.Eq(t, 5*time.Minute, cfg.Reaper.Interval)
	must.Eq(t, 5*time.Minute, cfg.Reaper.Threshold)
}

func TestServerConfig_Merge(t *testing.T) {
	testCases := []struct {
		name     string
		base     *ServerConfig
		other    *ServerConfig
		expected *ServerConfig
	}{
		{
			name:     "both nil",
			base:     nil,
			other:    nil,
			expected: nil,
		},
		{
			name:     "base nil",
			base:     nil,
			other:    DefaultServerConfig(),
			expected: DefaultServerConfig(),
		},
		{
			name:     "other nil",
			base:     DefaultServerConfig(),
			other:    nil,
			expected: DefaultServerConfig(),
		},
		{
			name: "both set - other overrides",
			base: &ServerConfig{
				Reaper: &ReaperConfig{
					Interval:  1 * time.Minute,
					Threshold: 2 * time.Minute,
				},
			},
			other: &ServerConfig{
				Reaper: &ReaperConfig{
					Interval:  10 * time.Minute,
					Threshold: 15 * time.Minute,
				},
			},
			expected: &ServerConfig{
				Reaper: &ReaperConfig{
					Interval:  10 * time.Minute,
					Threshold: 15 * time.Minute,
				},
			},
		},
		{
			name: "partial reaper override",
			base: &ServerConfig{
				Reaper: &ReaperConfig{
					Interval:  5 * time.Minute,
					Threshold: 5 * time.Minute,
				},
			},
			other: &ServerConfig{
				Reaper: &ReaperConfig{
					Interval: 10 * time.Minute,
				},
			},
			expected: &ServerConfig{
				Reaper: &ReaperConfig{
					Interval:  10 * time.Minute,
					Threshold: 5 * time.Minute,
				},
			},
		},
		{
			name: "reaper nil in base",
			base: &ServerConfig{},
			other: &ServerConfig{
				Reaper: &ReaperConfig{
					Interval:  3 * time.Minute,
					Threshold: 4 * time.Minute,
				},
			},
			expected: &ServerConfig{
				Reaper: &ReaperConfig{
					Interval:  3 * time.Minute,
					Threshold: 4 * time.Minute,
				},
			},
		},
		{
			name: "zero duration doesn't override",
			base: &ServerConfig{
				Reaper: &ReaperConfig{
					Interval:  5 * time.Minute,
					Threshold: 5 * time.Minute,
				},
			},
			other: &ServerConfig{
				Reaper: &ReaperConfig{
					Interval:  0,
					Threshold: 0,
				},
			},
			expected: &ServerConfig{
				Reaper: &ReaperConfig{
					Interval:  5 * time.Minute,
					Threshold: 5 * time.Minute,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.base.Merge(tc.other)
			must.Eq(t, tc.expected, result)
		})
	}
}

func TestServerConfig_Validate(t *testing.T) {
	testCases := []struct {
		name          string
		config        *ServerConfig
		expectedError bool
	}{
		{
			name:          "valid config",
			config:        DefaultServerConfig(),
			expectedError: false,
		},
		{
			name: "valid config with reaper",
			config: &ServerConfig{
				Reaper: &ReaperConfig{
					Interval:  10 * time.Minute,
					Threshold: 15 * time.Minute,
				},
			},
			expectedError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := tc.config.Validate()
			if tc.expectedError {
				must.Greater(t, 0, len(errs), must.Sprintf("errs: %v", errs))
			} else {
				must.Len(t, 0, errs, must.Sprintf("errs: %v", errs))
			}
		})
	}
}

func Test_ServerConfigCommandFlags(t *testing.T) {
	expectedFlags := []cli.Flag{
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
	}
	must.Eq(t, expectedFlags, ServerConfigCommandFlags())
}

func Test_ServerConfigFromCommand(t *testing.T) {
	testCases := []struct {
		name     string
		setFlags func(*cli.Command)
		expected *ServerConfig
	}{
		{
			name:     "no flags",
			setFlags: func(_ *cli.Command) {},
			expected: &ServerConfig{
				Reaper: &ReaperConfig{},
			},
		},
		{
			name: "all flags set",
			setFlags: func(cmd *cli.Command) {
				must.NoError(t, cmd.Set(reaperIntervalFlag, "10m"))
				must.NoError(t, cmd.Set(reaperThresholdFlag, "15m"))
			},
			expected: &ServerConfig{
				Reaper: &ReaperConfig{
					Interval:  10 * time.Minute,
					Threshold: 15 * time.Minute,
				},
			},
		},
		{
			name: "partial flags set",
			setFlags: func(cmd *cli.Command) {
				must.NoError(t, cmd.Set(reaperIntervalFlag, "10m"))
			},
			expected: &ServerConfig{
				Reaper: &ReaperConfig{
					Interval: 10 * time.Minute,
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmdApp := &cli.Command{Flags: ServerConfigCommandFlags()}
			tc.setFlags(cmdApp)
			must.Eq(t, tc.expected, ServerConfigFromCommand(cmdApp))
		})
	}
}
func Test_DefaultServerAgentConfig(t *testing.T) {
	cfg := DefaultServerAgentConfig()

	must.NotNil(t, cfg)
	must.Eq(t, DefaultHTTPConfig(), cfg.HTTP)
	must.Eq(t, DefaultLogConfig(), cfg.Log)
	must.Eq(t, DefaultNomadConfig(), cfg.Nomad)
	must.Eq(t, DefaultServerConfig(), cfg.Server)
	must.Eq(t, DefaultStoreConfig(), cfg.Store)
}

func TestServerAgentConfig_Merge(t *testing.T) {
	testCases := []struct {
		name     string
		base     *ServerAgentConfig
		other    *ServerAgentConfig
		expected *ServerAgentConfig
	}{
		{
			name:     "both nil",
			base:     nil,
			other:    nil,
			expected: nil,
		},
		{
			name:     "base nil",
			base:     nil,
			other:    DefaultServerAgentConfig(),
			expected: DefaultServerAgentConfig(),
		},
		{
			name:     "other nil",
			base:     DefaultServerAgentConfig(),
			other:    nil,
			expected: DefaultServerAgentConfig(),
		},
		{
			name: "other overrides base",
			base: &ServerAgentConfig{
				HTTP:  &HTTPConfig{Port: 9090},
				Log:   &LogConfig{Level: "info"},
				Nomad: &NomadConfig{Address: "http://localhost:4646"},
				Server: &ServerConfig{
					Reaper: &ReaperConfig{
						Interval:  5 * time.Minute,
						Threshold: 5 * time.Minute,
					},
				},
				Store: &StoreConfig{Backend: "nvar", NVar: &StoreNVarConfig{Path: "smuggle/"}},
			},
			other: &ServerAgentConfig{
				HTTP:  &HTTPConfig{Port: 8080},
				Log:   &LogConfig{Level: "debug"},
				Nomad: &NomadConfig{Address: "http://nomad.example.com:4646"},
				Server: &ServerConfig{
					Reaper: &ReaperConfig{
						Interval: 10 * time.Minute,
					},
				},
				Store: &StoreConfig{NVar: &StoreNVarConfig{Path: "platform/"}},
			},
			expected: &ServerAgentConfig{
				HTTP:  &HTTPConfig{Port: 8080},
				Log:   &LogConfig{Level: "debug"},
				Nomad: &NomadConfig{Address: "http://nomad.example.com:4646"},
				Server: &ServerConfig{
					Reaper: &ReaperConfig{
						Interval:  10 * time.Minute,
						Threshold: 5 * time.Minute,
					},
				},
				Store: &StoreConfig{Backend: "nvar", NVar: &StoreNVarConfig{Path: "platform/"}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.base.Merge(tc.other)
			must.Eq(t, tc.expected, result)
		})
	}
}

func TestServerAgentConfig_Validate(t *testing.T) {
	testCases := []struct {
		name          string
		config        *ServerAgentConfig
		expectedError bool
	}{
		{
			name:          "valid default config",
			config:        DefaultServerAgentConfig(),
			expectedError: false,
		},
		{
			name: "invalid http config",
			config: &ServerAgentConfig{
				HTTP: &HTTPConfig{
					Enabled:        helper.PointerOf(false),
					DebugEnabled:   helper.PointerOf(true),
					Address:        "localhost",
					AccessLogLevel: "debug",
					Port:           9090,
				},
				Log:    DefaultLogConfig(),
				Nomad:  DefaultNomadConfig(),
				Server: DefaultServerConfig(),
				Store:  DefaultStoreConfig(),
			},
			expectedError: true,
		},
		{
			name: "invalid log config",
			config: &ServerAgentConfig{
				HTTP:   DefaultHTTPConfig(),
				Log:    &LogConfig{Level: "invalid-level"},
				Nomad:  DefaultNomadConfig(),
				Server: DefaultServerConfig(),
				Store:  DefaultStoreConfig(),
			},
			expectedError: true,
		},
		{
			name: "invalid store config",
			config: &ServerAgentConfig{
				HTTP:   DefaultHTTPConfig(),
				Log:    DefaultLogConfig(),
				Nomad:  DefaultNomadConfig(),
				Server: DefaultServerConfig(),
				Store:  &StoreConfig{Backend: "etcd"},
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := tc.config.Validate()
			if tc.expectedError {
				must.Greater(t, 0, len(errs), must.Sprintf("errs: %v", errs))
			} else {
				must.Len(t, 0, errs, must.Sprintf("errs: %v", errs))
			}
		})
	}
}

func Test_ServerAgentConfigCommandFlags(t *testing.T) {
	expected := []cli.Flag{
		&cli.StringSliceFlag{
			Name:  "config",
			Usage: "The path to a configuration file to load",
		},
	}
	expected = append(expected, HTTPConfigCommandFlags()...)
	expected = append(expected, LogConfigCommandFlags()...)
	expected = append(expected, NomadConfigCommandFlags()...)
	expected = append(expected, ServerConfigCommandFlags()...)
	expected = append(expected, StoreConfigCommandFlags()...)

	must.Eq(t, expected, ServerAgentConfigCommandFlags())
}

func Test_ServerAgentConfigFromCommand(t *testing.T) {
	testCases := []struct {
		name     string
		setFlags func(*cli.Command)
		expected *ServerAgentConfig
	}{
		{
			name:     "no flags",
			setFlags: func(_ *cli.Command) {},
			expected: &ServerAgentConfig{
				HTTP:   &HTTPConfig{},
				Log:    &LogConfig{},
				Nomad:  &NomadConfig{},
				Server: &ServerConfig{Reaper: &ReaperConfig{}},
				Store:  &StoreConfig{NVar: &StoreNVarConfig{}},
			},
		},
		{
			name: "one flag per sub-config",
			setFlags: func(cmd *cli.Command) {
				must.NoError(t, cmd.Set(httpPortFlag, "8080"))
				must.NoError(t, cmd.Set(logLevalFlag, "debug"))
				must.NoError(t, cmd.Set(nomadAddrFlag, "http://nomad.example.com:4646"))
				must.NoError(t, cmd.Set(reaperIntervalFlag, "10m"))
				must.NoError(t, cmd.Set(storeBackendFlag, "nvar"))
			},
			expected: &ServerAgentConfig{
				HTTP:   &HTTPConfig{Port: 8080},
				Log:    &LogConfig{Level: "debug"},
				Nomad:  &NomadConfig{Address: "http://nomad.example.com:4646"},
				Server: &ServerConfig{Reaper: &ReaperConfig{Interval: 10 * time.Minute}},
				Store:  &StoreConfig{Backend: "nvar", NVar: &StoreNVarConfig{}},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmdApp := &cli.Command{Flags: ServerAgentConfigCommandFlags()}
			tc.setFlags(cmdApp)
			must.Eq(t, tc.expected, ServerAgentConfigFromCommand(cmdApp))
		})
	}
}

func Test_ServerAgentConfigFromFiles(t *testing.T) {
	validHCL := `
log {
  level = "debug"
}
server {
  reaper {
    interval  = "10m"
    threshold = "1h"
  }
}
store {
  backend = "nvar"
  nvar {
    path = "smuggle/"
  }
}
`

	validJSON := `{
  "log": { "level": "warn" },
  "store": { "backend": "nvar", "nvar": { "path": "smuggle/" } }
}`

	secondHCL := `
log {
  level = "warn"
}
nomad {
  address = "http://nomad.example.com:4646"
}
`

	invalidDurationHCL := `
server {
  reaper {
    interval = "not-a-duration"
  }
}
`

	testCases := []struct {
		name          string
		setFlags      func(*cli.Command, string)
		expected      *ServerAgentConfig
		expectedError bool
	}{
		{
			name:     "no config files",
			setFlags: func(_ *cli.Command, _ string) {},
			expected: nil,
		},
		{
			name: "valid hcl file",
			setFlags: func(cmd *cli.Command, dir string) {
				f := filepath.Join(dir, "config.hcl")
				must.NoError(t, os.WriteFile(f, []byte(validHCL), 0600))
				must.NoError(t, cmd.Set("config", f))
			},
			expected: &ServerAgentConfig{
				Log: &LogConfig{Level: "debug"},
				Server: &ServerConfig{
					Reaper: &ReaperConfig{
						IntervalHCL:  "10m",
						Interval:     10 * time.Minute,
						ThresholdHCL: "1h",
						Threshold:    time.Hour,
					},
				},
				Store: &StoreConfig{
					Backend: "nvar",
					NVar:    &StoreNVarConfig{Path: "smuggle/"},
				},
			},
		},
		{
			name: "valid json file",
			setFlags: func(cmd *cli.Command, dir string) {
				f := filepath.Join(dir, "config.json")
				must.NoError(t, os.WriteFile(f, []byte(validJSON), 0600))
				must.NoError(t, cmd.Set("config", f))
			},
			expected: &ServerAgentConfig{
				Log: &LogConfig{Level: "warn"},
				Store: &StoreConfig{
					Backend: "nvar",
					NVar:    &StoreNVarConfig{Path: "smuggle/"},
				},
			},
		},
		{
			name: "multiple files merged",
			setFlags: func(cmd *cli.Command, dir string) {
				f1 := filepath.Join(dir, "config1.hcl")
				must.NoError(t, os.WriteFile(f1, []byte(validHCL), 0600))
				must.NoError(t, cmd.Set("config", f1))

				f2 := filepath.Join(dir, "config2.hcl")
				must.NoError(t, os.WriteFile(f2, []byte(secondHCL), 0600))
				must.NoError(t, cmd.Set("config", f2))
			},
			expected: &ServerAgentConfig{
				Log:   &LogConfig{Level: "warn"},
				Nomad: &NomadConfig{Address: "http://nomad.example.com:4646"},
				Server: &ServerConfig{
					Reaper: &ReaperConfig{
						IntervalHCL:  "10m",
						Interval:     10 * time.Minute,
						ThresholdHCL: "1h",
						Threshold:    time.Hour,
					},
				},
				Store: &StoreConfig{
					Backend: "nvar",
					NVar:    &StoreNVarConfig{Path: "smuggle/"},
				},
			},
		},
		{
			name: "non-existent file",
			setFlags: func(cmd *cli.Command, dir string) {
				must.NoError(t, cmd.Set("config", filepath.Join(dir, "does-not-exist.hcl")))
			},
			expectedError: true,
		},
		{
			name: "unsupported file extension",
			setFlags: func(cmd *cli.Command, dir string) {
				f := filepath.Join(dir, "config.toml")
				must.NoError(t, os.WriteFile(f, []byte{}, 0600))
				must.NoError(t, cmd.Set("config", f))
			},
			expectedError: true,
		},
		{
			name: "invalid duration in hcl file",
			setFlags: func(cmd *cli.Command, dir string) {
				f := filepath.Join(dir, "config.hcl")
				must.NoError(t, os.WriteFile(f, []byte(invalidDurationHCL), 0600))
				must.NoError(t, cmd.Set("config", f))
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmdApp := &cli.Command{Flags: ServerAgentConfigCommandFlags()}
			tc.setFlags(cmdApp, t.TempDir())

			result, err := ServerAgentConfigFromFiles(cmdApp)
			if tc.expectedError {
				must.Error(t, err)
			} else {
				must.NoError(t, err)
				must.Eq(t, tc.expected, result)
			}
		})
	}
}
