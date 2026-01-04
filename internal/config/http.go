package config

import (
	"errors"
	"strings"

	"github.com/urfave/cli/v3"
	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/helper"
)

const (
	httpEnabledFlag        = "http-enabled"
	httpAddressFlag        = "http-address"
	httpPortFlag           = "http-port"
	httpAccessLogLevelFlag = "http-access-log-level"
	httpEnableDebugFlag    = "http-debug-enabled"
)

type HTTPConfig struct {
	Enabled        *bool  `hcl:"enabled,optional" json:"enabled"`
	DebugEnabled   *bool  `hcl:"debug_enabled,optional" json:"debug_enabled"`
	Address        string `hcl:"address" json:"address"`
	AccessLogLevel string `hcl:"access_log_level" json:"access_log_level"`
	Port           uint   `hcl:"port" json:"port"`
}

func DefaultHTTPConfig() *HTTPConfig {
	return &HTTPConfig{
		Enabled:        helper.PointerOf(true),
		DebugEnabled:   helper.PointerOf(false),
		Address:        "localhost",
		AccessLogLevel: zap.DebugLevel.String(),
		Port:           9090,
	}
}

func (h *HTTPConfig) IsEnabled() bool {
	return h != nil && h.Enabled != nil && *h.Enabled
}

func (h *HTTPConfig) IsDebugEnabled() bool {
	return h != nil && h.DebugEnabled != nil && *h.DebugEnabled
}

func (h *HTTPConfig) Merge(other *HTTPConfig) *HTTPConfig {
	if h == nil {
		return other
	}
	if other == nil {
		return h
	}

	newCfg := *h

	if other.Enabled != nil {
		newCfg.Enabled = other.Enabled
	}
	if other.Address != "" {
		newCfg.Address = other.Address
	}
	if other.AccessLogLevel != "" {
		newCfg.AccessLogLevel = other.AccessLogLevel
	}
	if other.Port != 0 {
		newCfg.Port = other.Port
	}
	if other.DebugEnabled != nil {
		newCfg.DebugEnabled = other.DebugEnabled
	}

	return &newCfg
}

func (h *HTTPConfig) Validate() []error {

	var errs []error

	if !h.IsEnabled() && h.IsDebugEnabled() {
		errs = append(errs, errors.New("http debug enabled cannot be true when http enabled is false"))
	}
	if _, err := zap.ParseAtomicLevel(strings.ToLower(h.AccessLogLevel)); err != nil {
		errs = append(errs, err)
	}
	if h.Port == 0 || h.Port > 65535 {
		errs = append(errs, errors.New("http port must be between 1 and 65535"))
	}

	return errs
}

func HTTPConfigCommandFlags() []cli.Flag {
	return []cli.Flag{
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
}

func HTTPConfigFromCommand(cmd *cli.Command) *HTTPConfig {
	return &HTTPConfig{
		Enabled: func() *bool {
			if cmd.IsSet(httpEnabledFlag) {
				val := cmd.Bool(httpEnabledFlag)
				return &val
			}
			return nil
		}(),
		Address:        cmd.String(httpAddressFlag),
		Port:           cmd.Uint(httpPortFlag),
		AccessLogLevel: cmd.String(httpAccessLogLevelFlag),
		DebugEnabled: func() *bool {
			if cmd.IsSet(httpEnableDebugFlag) {
				val := cmd.Bool(httpEnableDebugFlag)
				return &val
			}
			return nil
		}(),
	}
}
