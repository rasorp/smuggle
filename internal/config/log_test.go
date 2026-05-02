package config

import (
	"testing"

	"github.com/shoenig/test/must"
	"github.com/urfave/cli/v3"

	"github.com/rasorp/smuggle/internal/helper"
)

func Test_DefaultLogConfig(t *testing.T) {
	cfg := DefaultLogConfig()

	must.NotNil(t, cfg)
	must.Eq(t, "info", cfg.Level)
	must.Eq(t, "", cfg.File)
	must.False(t, *cfg.JSON)
	must.False(t, *cfg.IncludeLine)
	must.False(t, *cfg.EnableStacktrace)
}

func TestLogConfig_Merge(t *testing.T) {
	testCases := []struct {
		name     string
		base     *LogConfig
		other    *LogConfig
		expected *LogConfig
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
			other:    DefaultLogConfig(),
			expected: DefaultLogConfig(),
		},
		{
			name:     "other nil",
			base:     DefaultLogConfig(),
			other:    nil,
			expected: DefaultLogConfig(),
		},
		{
			name: "both set",
			base: &LogConfig{
				Level:            "debug",
				File:             "/var/log/smuggle-base.log",
				JSON:             helper.PointerOf(true),
				IncludeLine:      helper.PointerOf(true),
				EnableStacktrace: helper.PointerOf(true),
			},
			other: &LogConfig{
				Level:            "warn",
				File:             "/var/log/smuggle-other.log",
				JSON:             helper.PointerOf(false),
				IncludeLine:      helper.PointerOf(false),
				EnableStacktrace: helper.PointerOf(false),
			},
			expected: &LogConfig{
				Level:            "warn",
				File:             "/var/log/smuggle-other.log",
				JSON:             helper.PointerOf(false),
				IncludeLine:      helper.PointerOf(false),
				EnableStacktrace: helper.PointerOf(false),
			},
		},
		{
			name: "partial override",
			base: &LogConfig{
				Level:            "debug",
				File:             "/var/log/smuggle.log",
				JSON:             helper.PointerOf(true),
				IncludeLine:      helper.PointerOf(true),
				EnableStacktrace: helper.PointerOf(true),
			},
			other: &LogConfig{
				Level: "error",
			},
			expected: &LogConfig{
				Level:            "error",
				File:             "/var/log/smuggle.log",
				JSON:             helper.PointerOf(true),
				IncludeLine:      helper.PointerOf(true),
				EnableStacktrace: helper.PointerOf(true),
			},
		},
		{
			name: "file override",
			base: &LogConfig{
				Level: "info",
				File:  "/var/log/smuggle-base.log",
			},
			other: &LogConfig{
				File: "/var/log/smuggle-override.log",
			},
			expected: &LogConfig{
				Level: "info",
				File:  "/var/log/smuggle-override.log",
			},
		},
		{
			name: "file not overridden when other file empty",
			base: &LogConfig{
				Level: "info",
				File:  "/var/log/smuggle.log",
			},
			other: &LogConfig{
				Level: "warn",
			},
			expected: &LogConfig{
				Level: "warn",
				File:  "/var/log/smuggle.log",
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

func TestLogConfig_Validate(t *testing.T) {
	testCases := []struct {
		name          string
		config        *LogConfig
		expectedError bool
	}{
		{
			name:          "info",
			config:        DefaultLogConfig(),
			expectedError: false,
		},
		{
			name: "debug",
			config: &LogConfig{
				Level:            "debug",
				JSON:             helper.PointerOf(false),
				IncludeLine:      helper.PointerOf(true),
				EnableStacktrace: helper.PointerOf(false),
			},
			expectedError: false,
		},
		{
			name: "warn",
			config: &LogConfig{
				Level: "warn",
			},
			expectedError: false,
		},
		{
			name: "error",
			config: &LogConfig{
				Level: "error",
			},
			expectedError: false,
		},
		{
			name: "invalid log level",
			config: &LogConfig{
				Level: "invalid-level",
			},
			expectedError: true,
		},
		{
			name: "valid absolute log file path",
			config: &LogConfig{
				Level: "info",
				File:  "/var/log/smuggle.log",
			},
			expectedError: false,
		},
		{
			name: "invalid relative log file path",
			config: &LogConfig{
				Level: "info",
				File:  "relative/smuggle.log",
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

func Test_LogConfigCommandFlags(t *testing.T) {
	expectedFlags := []cli.Flag{
		&cli.StringFlag{
			HideDefault: true,
			Name:        logLevalFlag,
			Usage:       "The threshold level for logging",
			Sources:     cli.EnvVars("SMUGGLE_LOG_LEVEL"),
		},
		&cli.StringFlag{
			HideDefault: true,
			Name:        logFileFlag,
			Usage:       "The file to write logs to",
			Sources:     cli.EnvVars("SMUGGLE_LOG_FILE"),
		},
		&cli.BoolFlag{
			HideDefault: true,
			Name:        logJSONFlag,
			Usage:       "If the output should be in JSON format",
			Sources:     cli.EnvVars("SMUGGLE_LOG_JSON"),
		},
		&cli.BoolFlag{
			HideDefault: true,
			Name:        logIncludeLineFlag,
			Usage:       "Include file and line information in each log line",
			Sources:     cli.EnvVars("SMUGGLE_LOG_INCLUDE_LINE"),
		},
		&cli.BoolFlag{
			HideDefault: true,
			Name:        logEnableStacktraceFlag,
			Usage:       "Enable stacktrace capturing for error level logs",
			Sources:     cli.EnvVars("SMUGGLE_LOG_ENABLE_STACKTRACE"),
		},
	}
	must.Eq(t, expectedFlags, LogConfigCommandFlags())
}

func Test_LogConfigFromCommand(t *testing.T) {
	testCases := []struct {
		name     string
		setFlags func(*cli.Command)
		expected *LogConfig
	}{
		{
			name:     "no flags",
			setFlags: func(_ *cli.Command) {},
			expected: &LogConfig{},
		},
		{
			name: "all flags set",
			setFlags: func(cmd *cli.Command) {
				must.NoError(t, cmd.Set(logLevalFlag, "debug"))
				must.NoError(t, cmd.Set(logFileFlag, "/var/log/smuggle.log"))
				must.NoError(t, cmd.Set(logJSONFlag, "true"))
				must.NoError(t, cmd.Set(logIncludeLineFlag, "true"))
				must.NoError(t, cmd.Set(logEnableStacktraceFlag, "true"))
			},
			expected: &LogConfig{
				Level:            "debug",
				File:             "/var/log/smuggle.log",
				JSON:             helper.PointerOf(true),
				IncludeLine:      helper.PointerOf(true),
				EnableStacktrace: helper.PointerOf(true),
			},
		},
		{
			name: "partial flags set",
			setFlags: func(cmd *cli.Command) {
				must.NoError(t, cmd.Set(logLevalFlag, "warn"))
				must.NoError(t, cmd.Set(logJSONFlag, "true"))
			},
			expected: &LogConfig{
				Level: "warn",
				JSON:  helper.PointerOf(true),
			},
		},
		{
			name: "only file flag set",
			setFlags: func(cmd *cli.Command) {
				must.NoError(t, cmd.Set(logFileFlag, "/var/log/smuggle.log"))
			},
			expected: &LogConfig{
				File: "/var/log/smuggle.log",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmdApp := &cli.Command{Flags: LogConfigCommandFlags()}
			tc.setFlags(cmdApp)
			must.Eq(t, tc.expected, LogConfigFromCommand(cmdApp))
		})
	}
}
