package log

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/shoenig/test/must"
	"go.uber.org/zap/zapcore"

	"github.com/rasorp/smuggle/internal/config"
	"github.com/rasorp/smuggle/internal/helper"
)

func TestComponentName(t *testing.T) {
	testCases := []struct {
		constant string
		expected string
	}{
		{ComponentNameAgent, "agent"},
		{ComponentNameServer, "server"},
		{ComponentNameClient, "client"},
		{ComponentNameHTTP, "http"},
		{ComponentNameNetwork, "network"},
		{ComponentNameIptables, "iptables"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {

			must.Eq(t, tc.expected, tc.constant)

			// Verify the constant works correctly as a zap logger name.
			base, err := New(&config.LogConfig{
				Level:            "info",
				JSON:             helper.PointerOf(false),
				IncludeLine:      helper.PointerOf(false),
				EnableStacktrace: helper.PointerOf(false),
			})

			must.NoError(t, err)
			must.StrContains(t, base.Named(tc.constant).Name(), tc.expected)
		})
	}
}

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
			name: "file logging enabled",
			cfg: &config.LogConfig{
				Level:            "info",
				File:             filepath.Join(t.TempDir(), "smuggle.log"),
				JSON:             helper.PointerOf(false),
				IncludeLine:      helper.PointerOf(false),
				EnableStacktrace: helper.PointerOf(false),
			},
			expectError: false,
			validate: func(t *testing.T, logger *Logger) {
				must.NotNil(t, logger)
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

func TestNew_fileLogging(t *testing.T) {
	const logMsg = "test file and stdout log entry"

	logFile := filepath.Join(t.TempDir(), "smuggle.log")

	cfg := &config.LogConfig{
		Level:            "info",
		File:             logFile,
		JSON:             helper.PointerOf(false),
		IncludeLine:      helper.PointerOf(false),
		EnableStacktrace: helper.PointerOf(false),
	}

	// Redirect os.Stdout to a pipe before calling New so that the
	// WriteSyncer inside the logger captures the pipe end, not the
	// original terminal.
	r, w, err := os.Pipe()
	must.NoError(t, err)

	origStdout := os.Stdout
	os.Stdout = w

	logger, err := New(cfg)
	must.NoError(t, err)
	must.NotNil(t, logger)

	logger.Info(logMsg)
	_ = logger.Sync()

	// Restore stdout and close the write end so the reader reaches EOF.
	os.Stdout = origStdout
	must.NoError(t, w.Close())

	// Drain the pipe into a buffer.
	var stdoutBuf bytes.Buffer
	_, err = stdoutBuf.ReadFrom(r)
	must.NoError(t, err)

	// Verify the message reached stdout.
	must.StrContains(t, stdoutBuf.String(), logMsg)

	// Verify the message was also written to the log file on disk.
	fileContent, err := os.ReadFile(logFile)
	must.NoError(t, err)
	must.StrContains(t, string(fileContent), logMsg)
}
