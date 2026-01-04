package config

import (
	"testing"
	"time"

	"github.com/shoenig/test/must"
	"github.com/urfave/cli/v3"

	"github.com/rasorp/smuggle/internal/helper"
)

func Test_DefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()

	must.NotNil(t, cfg)
	must.False(t, *cfg.Enabled)
	must.NotNil(t, cfg.Reaper)
	must.Eq(t, 5*time.Minute, cfg.Reaper.Interval)
	must.Eq(t, 5*time.Minute, cfg.Reaper.Threshold)
}

func TestServerConfig_IsEnabled(t *testing.T) {
	testCases := []struct {
		name     string
		config   *ServerConfig
		expected bool
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: false,
		},
		{
			name:     "enabled nil",
			config:   &ServerConfig{Enabled: nil},
			expected: false,
		},
		{
			name:     "enabled false",
			config:   &ServerConfig{Enabled: helper.PointerOf(false)},
			expected: false,
		},
		{
			name:     "enabled true",
			config:   &ServerConfig{Enabled: helper.PointerOf(true)},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			must.Eq(t, tc.expected, tc.config.IsEnabled())
		})
	}
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
				Enabled: helper.PointerOf(false),
				Reaper: &ReaperConfig{
					Interval:  1 * time.Minute,
					Threshold: 2 * time.Minute,
				},
			},
			other: &ServerConfig{
				Enabled: helper.PointerOf(true),
				Reaper: &ReaperConfig{
					Interval:  10 * time.Minute,
					Threshold: 15 * time.Minute,
				},
			},
			expected: &ServerConfig{
				Enabled: helper.PointerOf(true),
				Reaper: &ReaperConfig{
					Interval:  10 * time.Minute,
					Threshold: 15 * time.Minute,
				},
			},
		},
		{
			name: "partial reaper override",
			base: &ServerConfig{
				Enabled: helper.PointerOf(true),
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
				Enabled: helper.PointerOf(true),
				Reaper: &ReaperConfig{
					Interval:  10 * time.Minute,
					Threshold: 5 * time.Minute,
				},
			},
		},
		{
			name: "reaper nil in base",
			base: &ServerConfig{
				Enabled: helper.PointerOf(true),
			},
			other: &ServerConfig{
				Reaper: &ReaperConfig{
					Interval:  3 * time.Minute,
					Threshold: 4 * time.Minute,
				},
			},
			expected: &ServerConfig{
				Enabled: helper.PointerOf(true),
				Reaper: &ReaperConfig{
					Interval:  3 * time.Minute,
					Threshold: 4 * time.Minute,
				},
			},
		},
		{
			name: "zero duration doesn't override",
			base: &ServerConfig{
				Enabled: helper.PointerOf(true),
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
				Enabled: helper.PointerOf(true),
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
			name: "valid enabled config",
			config: &ServerConfig{
				Enabled: helper.PointerOf(true),
				Reaper: &ReaperConfig{
					Interval:  10 * time.Minute,
					Threshold: 15 * time.Minute,
				},
			},
			expectedError: false,
		},
		{
			name: "server disabled",
			config: &ServerConfig{
				Enabled: helper.PointerOf(false),
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
				must.NoError(t, cmd.Set(serverEnabledFlag, "true"))
				must.NoError(t, cmd.Set(serverReaperIntervalFlag, "10m"))
				must.NoError(t, cmd.Set(serverReaperThresholdFlag, "15m"))
			},
			expected: &ServerConfig{
				Enabled: helper.PointerOf(true),
				Reaper: &ReaperConfig{
					Interval:  10 * time.Minute,
					Threshold: 15 * time.Minute,
				},
			},
		},
		{
			name: "partial flags set",
			setFlags: func(cmd *cli.Command) {
				must.NoError(t, cmd.Set(serverEnabledFlag, "true"))
			},
			expected: &ServerConfig{
				Enabled: helper.PointerOf(true),
				Reaper:  &ReaperConfig{},
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
