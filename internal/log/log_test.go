package log

import (
	"testing"

	"github.com/shoenig/test/must"
	"go.uber.org/zap/zapcore"

	"github.com/rasorp/smuggle/internal/config"
	"github.com/rasorp/smuggle/internal/helper"
)

func TestNew(t *testing.T) {
	testCases := []struct {
		name        string
		cfg         *config.LogConfig
		expectError bool
		validate    func(t *testing.T, logger *Logger)
	}{
		{
			name: "default config with console encoding",
			cfg: &config.LogConfig{
				Level:            "info",
				JSON:             helper.PointerOf(false),
				IncludeLine:      helper.PointerOf(false),
				EnableStacktrace: helper.PointerOf(false),
			},
			expectError: false,
			validate: func(t *testing.T, logger *Logger) {
				must.NotNil(t, logger)
				core := logger.Core()
				must.True(t, core.Enabled(zapcore.InfoLevel))
				must.False(t, core.Enabled(zapcore.DebugLevel))
			},
		},
		{
			name: "json encoding enabled",
			cfg: &config.LogConfig{
				Level:            "info",
				JSON:             helper.PointerOf(true),
				IncludeLine:      helper.PointerOf(false),
				EnableStacktrace: helper.PointerOf(false),
			},
			expectError: false,
			validate: func(t *testing.T, logger *Logger) {
				must.NotNil(t, logger)
			},
		},
		{
			name: "debug level",
			cfg: &config.LogConfig{
				Level:            "debug",
				JSON:             helper.PointerOf(false),
				IncludeLine:      helper.PointerOf(false),
				EnableStacktrace: helper.PointerOf(false),
			},
			expectError: false,
			validate: func(t *testing.T, logger *Logger) {
				must.NotNil(t, logger)
				core := logger.Core()
				must.True(t, core.Enabled(zapcore.DebugLevel))
				must.True(t, core.Enabled(zapcore.InfoLevel))
			},
		},
		{
			name: "warn level",
			cfg: &config.LogConfig{
				Level:            "warn",
				JSON:             helper.PointerOf(false),
				IncludeLine:      helper.PointerOf(false),
				EnableStacktrace: helper.PointerOf(false),
			},
			expectError: false,
			validate: func(t *testing.T, logger *Logger) {
				must.NotNil(t, logger)
				core := logger.Core()
				must.True(t, core.Enabled(zapcore.WarnLevel))
				must.False(t, core.Enabled(zapcore.InfoLevel))
				must.False(t, core.Enabled(zapcore.DebugLevel))
			},
		},
		{
			name: "error level",
			cfg: &config.LogConfig{
				Level:            "error",
				JSON:             helper.PointerOf(false),
				IncludeLine:      helper.PointerOf(false),
				EnableStacktrace: helper.PointerOf(false),
			},
			expectError: false,
			validate: func(t *testing.T, logger *Logger) {
				must.NotNil(t, logger)
				core := logger.Core()
				must.True(t, core.Enabled(zapcore.ErrorLevel))
				must.False(t, core.Enabled(zapcore.WarnLevel))
				must.False(t, core.Enabled(zapcore.InfoLevel))
			},
		},
		{
			name: "invalid log level returns error",
			cfg: &config.LogConfig{
				Level:            "invalid-level",
				JSON:             helper.PointerOf(false),
				IncludeLine:      helper.PointerOf(false),
				EnableStacktrace: helper.PointerOf(false),
			},
			expectError: true,
			validate:    nil,
		},
		{
			name: "include line enabled",
			cfg: &config.LogConfig{
				Level:            "info",
				JSON:             helper.PointerOf(false),
				IncludeLine:      helper.PointerOf(true),
				EnableStacktrace: helper.PointerOf(false),
			},
			expectError: false,
			validate: func(t *testing.T, logger *Logger) {
				must.NotNil(t, logger)
			},
		},
		{
			name: "json with include line",
			cfg: &config.LogConfig{
				Level:            "info",
				JSON:             helper.PointerOf(true),
				IncludeLine:      helper.PointerOf(true),
				EnableStacktrace: helper.PointerOf(false),
			},
			expectError: false,
			validate: func(t *testing.T, logger *Logger) {
				must.NotNil(t, logger)
			},
		},
		{
			name: "all levels work correctly",
			cfg: &config.LogConfig{
				Level:            "dpanic",
				JSON:             helper.PointerOf(false),
				IncludeLine:      helper.PointerOf(false),
				EnableStacktrace: helper.PointerOf(false),
			},
			expectError: false,
			validate: func(t *testing.T, logger *Logger) {
				must.NotNil(t, logger)
				core := logger.Core()
				must.True(t, core.Enabled(zapcore.DPanicLevel))
				must.False(t, core.Enabled(zapcore.ErrorLevel))
			},
		},
		{
			name: "panic level",
			cfg: &config.LogConfig{
				Level:            "panic",
				JSON:             helper.PointerOf(false),
				IncludeLine:      helper.PointerOf(false),
				EnableStacktrace: helper.PointerOf(false),
			},
			expectError: false,
			validate: func(t *testing.T, logger *Logger) {
				must.NotNil(t, logger)
				core := logger.Core()
				must.True(t, core.Enabled(zapcore.PanicLevel))
				must.False(t, core.Enabled(zapcore.DPanicLevel))
			},
		},
		{
			name: "fatal level",
			cfg: &config.LogConfig{
				Level:            "fatal",
				JSON:             helper.PointerOf(false),
				IncludeLine:      helper.PointerOf(false),
				EnableStacktrace: helper.PointerOf(false),
			},
			expectError: false,
			validate: func(t *testing.T, logger *Logger) {
				must.NotNil(t, logger)
				core := logger.Core()
				must.True(t, core.Enabled(zapcore.FatalLevel))
				must.False(t, core.Enabled(zapcore.PanicLevel))
			},
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			logger, err := New(tt.cfg)

			if tt.expectError {
				must.Error(t, err)
				must.Nil(t, logger)
			} else {
				must.NoError(t, err)
				must.NotNil(t, logger)

				if tt.validate != nil {
					tt.validate(t, logger)
				}

				// Ensure the logger can be used without panicking.
				logger.Info("test log message")
			}
		})
	}
}
