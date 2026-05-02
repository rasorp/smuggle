package config

import (
	"errors"
	"path/filepath"

	"github.com/urfave/cli/v3"
	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/helper"
)

const (
	logLevalFlag            = "log-level"
	logJSONFlag             = "log-json"
	logFileFlag             = "log-file"
	logIncludeLineFlag      = "log-include-line"
	logEnableStacktraceFlag = "log-enable-stacktrace"
)

type LogConfig struct {
	Level            string `hcl:"level,optional" json:"level"`
	File             string `hcl:"file,optional" json:"file"`
	JSON             *bool  `hcl:"json,optional" json:"json"`
	IncludeLine      *bool  `hcl:"include_line,optional" json:"include_line"`
	EnableStacktrace *bool  `hcl:"enable_stacktrace,optional" json:"enable_stacktrace"`
}

func DefaultLogConfig() *LogConfig {
	return &LogConfig{
		Level:            zap.InfoLevel.String(),
		File:             "",
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
	if other.File != "" {
		result.File = other.File
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
	if l.File != "" {
		if !filepath.IsAbs(l.File) {
			errs = append(errs, errors.New("log file path must be absolute"))
		}
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
}

func LogConfigFromCommand(cmd *cli.Command) *LogConfig {
	return &LogConfig{
		Level: cmd.String(logLevalFlag),
		File:  cmd.String(logFileFlag),
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
