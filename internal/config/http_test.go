package config

import (
	"testing"

	"github.com/shoenig/test/must"
	"github.com/urfave/cli/v3"

	"github.com/rasorp/smuggle/internal/helper"
)

func Test_DefaultHTTPConfig(t *testing.T) {
	cfg := DefaultHTTPConfig()

	must.Eq(t, true, cfg.IsEnabled())
	must.Eq(t, false, cfg.IsDebugEnabled())
	must.Eq(t, "localhost", cfg.Address)
	must.Eq(t, "debug", cfg.AccessLogLevel)
	must.Eq(t, uint(9090), cfg.Port)
}

func TestHTTPConfig_IsEnabled(t *testing.T) {
	testCases := []struct {
		name     string
		config   *HTTPConfig
		expected bool
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: false,
		},
		{
			name:     "enabled true",
			config:   &HTTPConfig{Enabled: helper.PointerOf(true)},
			expected: true,
		},
		{
			name:     "enabled false",
			config:   &HTTPConfig{Enabled: helper.PointerOf(false)},
			expected: false,
		},
		{
			name:     "enabled nil",
			config:   &HTTPConfig{Enabled: nil},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			must.Eq(t, tc.expected, tc.config.IsEnabled())
		})
	}
}

func TestHTTPConfig_IsDebugEnabled(t *testing.T) {
	testCases := []struct {
		name     string
		config   *HTTPConfig
		expected bool
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: false,
		},
		{
			name:     "debug enabled true",
			config:   &HTTPConfig{DebugEnabled: helper.PointerOf(true)},
			expected: true,
		},
		{
			name:     "debug enabled false",
			config:   &HTTPConfig{DebugEnabled: helper.PointerOf(false)},
			expected: false,
		},
		{
			name:     "debug enabled nil",
			config:   &HTTPConfig{DebugEnabled: nil},
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			must.Eq(t, tc.expected, tc.config.IsDebugEnabled())
		})
	}
}

func TestHTTPConfig_Merge(t *testing.T) {
	testCases := []struct {
		name     string
		base     *HTTPConfig
		other    *HTTPConfig
		expected *HTTPConfig
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
			other:    DefaultHTTPConfig(),
			expected: DefaultHTTPConfig(),
		},
		{
			name:     "other nil",
			base:     DefaultHTTPConfig(),
			other:    nil,
			expected: DefaultHTTPConfig(),
		},
		{
			name: "both set",
			base: &HTTPConfig{
				Enabled:        helper.PointerOf(true),
				DebugEnabled:   helper.PointerOf(false),
				Address:        "localhost",
				AccessLogLevel: "info",
				Port:           8080,
			},
			other: &HTTPConfig{
				Enabled:        helper.PointerOf(false),
				DebugEnabled:   helper.PointerOf(true),
				Address:        "",
				AccessLogLevel: "debug",
				Port:           9090,
			},
			expected: &HTTPConfig{
				Enabled:        helper.PointerOf(false),
				DebugEnabled:   helper.PointerOf(true),
				Address:        "localhost",
				AccessLogLevel: "debug",
				Port:           9090,
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

func TestHTTPConfig_Validate(t *testing.T) {
	testCases := []struct {
		name          string
		config        *HTTPConfig
		expectedError bool
	}{
		{
			name:          "valid config",
			config:        DefaultHTTPConfig(),
			expectedError: false,
		},
		{
			name: "http disabled with debug enabled",
			config: &HTTPConfig{
				Enabled:        helper.PointerOf(false),
				DebugEnabled:   helper.PointerOf(true),
				Address:        "localhost",
				AccessLogLevel: "debug",
				Port:           9090,
			},
			expectedError: true,
		},
		{
			name: "invalid access log level",
			config: &HTTPConfig{
				Enabled:        helper.PointerOf(true),
				DebugEnabled:   helper.PointerOf(false),
				Address:        "localhost",
				AccessLogLevel: "invalid-level",
				Port:           9090,
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

func Test_HTTPConfigCommandFlags(t *testing.T) {
	expectedFlags := []cli.Flag{
		&cli.BoolFlag{
			HideDefault: true,
			Name:        httpEnabledFlag,
			Usage:       "Enable the HTTP server",
			Sources:     cli.EnvVars("SMUGGLE_HTTP_ENABLED"),
		},
		&cli.StringFlag{
			HideDefault: true,
			Name:        httpAddressFlag,
			Usage:       "The address to bind the HTTP server to",
			Sources:     cli.EnvVars("SMUGGLE_HTTP_ADDRESS"),
		},
		&cli.UintFlag{
			HideDefault: true,
			Name:        httpPortFlag,
			Usage:       "The port to bind the HTTP server to",
			Sources:     cli.EnvVars("SMUGGLE_HTTP_PORT"),
		},
		&cli.StringFlag{
			HideDefault: true,
			Name:        httpAccessLogLevelFlag,
			Usage:       "The access log level for the HTTP server (debug, info, warn, error)",
			Sources:     cli.EnvVars("SMUGGLE_HTTP_ACCESS_LOG_LEVEL"),
		},
		&cli.BoolFlag{
			HideDefault: true,
			Name:        httpEnableDebugFlag,
			Usage:       "Enable debug endpoints on the HTTP server",
			Sources:     cli.EnvVars("SMUGGLE_HTTP_ENABLE_DEBUG"),
		},
	}
	must.Eq(t, expectedFlags, HTTPConfigCommandFlags())
}

func Test_HTTPConfigFromCommand(t *testing.T) {
	testCases := []struct {
		name     string
		setFlags func(*cli.Command)
		expected *HTTPConfig
	}{
		{
			name:     "no flags",
			setFlags: func(*cli.Command) {},
			expected: &HTTPConfig{},
		},
		{
			name: "all flags set",
			setFlags: func(cmd *cli.Command) {
				must.NoError(t, cmd.Set(httpEnabledFlag, "true"))
				must.NoError(t, cmd.Set(httpAddressFlag, "192.168.1.131"))
				must.NoError(t, cmd.Set(httpPortFlag, "8080"))
				must.NoError(t, cmd.Set(httpAccessLogLevelFlag, "warn"))
				must.NoError(t, cmd.Set(httpEnableDebugFlag, "true"))
			},
			expected: &HTTPConfig{
				Enabled:        helper.PointerOf(true),
				Address:        "192.168.1.131",
				Port:           8080,
				AccessLogLevel: "warn",
				DebugEnabled:   helper.PointerOf(true),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmdApp := &cli.Command{Flags: HTTPConfigCommandFlags()}
			tc.setFlags(cmdApp)
			must.Eq(t, tc.expected, HTTPConfigFromCommand(cmdApp))
		})
	}
}
