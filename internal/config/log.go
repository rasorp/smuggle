package config

import (
	"github.com/urfave/cli/v3"
	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/helper"
)

const (
	logLevalFlag            = "log-level"
	logJSONFlag             = "log-json"
	logIncludeLineFlag      = "log-include-line"
	logEnableStacktraceFlag = "log-enable-stacktrace"
)

type LogConfig struct {
	Level            string `hcl:"level,optional" json:"level"`
	JSON             *bool  `hcl:"json,optional" json:"json"`
	IncludeLine      *bool  `hcl:"include_line,optional" json:"include_line"`
	EnableStacktrace *bool  `hcl:"enable_stacktrace,optional" json:"enable_stacktrace"`
}

func DefaultLogConfig() *LogConfig {
	return &LogConfig{
		Level:            zap.InfoLevel.String(),
		JSON:             helper.PointerOf(false),
		IncludeLine:      helper.PointerOf(false),
		EnableStacktrace: helper.PointerOf(false),
	}
}

func (l *LogConfig) Merge(other *LogConfig) *LogConfig {
	if l == nil {
		return other
	}
	if other == nil {
		return l
	}

	result := *l

	if other.Level != "" {
		result.Level = other.Level
	}
	if other.JSON != nil {
		result.JSON = other.JSON
	}
	if other.IncludeLine != nil {
		result.IncludeLine = other.IncludeLine
	}
	if other.EnableStacktrace != nil {
		result.EnableStacktrace = other.EnableStacktrace
	}

	return &result
}

func (l *LogConfig) Validate() []error {
	var errs []error

	if _, err := zap.ParseAtomicLevel(l.Level); err != nil {
		errs = append(errs, err)
	}

	return errs
}

func LogConfigCommandFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			HideDefault: true,
			Name:        logLevalFlag,
			Usage:       "The threshold level for logging",
			Sources:     cli.EnvVars("SMUGGLE_LOG_LEVEL"),
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
}

func LogConfigFromCommand(cmd *cli.Command) *LogConfig {
	return &LogConfig{
		Level: cmd.String(logLevalFlag),
		JSON: func() *bool {
			if cmd.IsSet(logJSONFlag) {
				val := cmd.Bool(logJSONFlag)
				return &val
			}
			return nil
		}(),
		IncludeLine: func() *bool {
			if cmd.IsSet(logIncludeLineFlag) {
				val := cmd.Bool(logIncludeLineFlag)
				return &val
			}
			return nil
		}(),
		EnableStacktrace: func() *bool {
			if cmd.IsSet(logEnableStacktraceFlag) {
				val := cmd.Bool(logEnableStacktraceFlag)
				return &val
			}
			return nil
		}(),
	}
}
