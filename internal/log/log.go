package log

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/lumberjack.v2"

	"github.com/rasorp/smuggle/internal/config"
)

const (
	ComponentNameAgent    = "agent"
	ComponentNameServer   = "server"
	ComponentNameClient   = "client"
	ComponentNameHTTP     = "http"
	ComponentNameNetwork  = "network"
	ComponentNameIptables = "iptables"
)

// Logger is an alias for zap.Logger which simplifies imports as all log
// users just need to import this package.
type Logger = zap.Logger

// New constructs and returns a new Logger based on the provided configuration
// that is ready to use.
func New(cfg *config.LogConfig) (*Logger, error) {

	lvl, err := zap.ParseAtomicLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	var encoder zapcore.Encoder

	encCfg := zap.NewProductionEncoderConfig()
	encCfg.NameKey = "component"
	encCfg.TimeKey = "timestamp"

	if *cfg.JSON {
		encCfg.EncodeTime = zapcore.RFC3339NanoTimeEncoder
		encoder = zapcore.NewJSONEncoder(encCfg)
	} else {
		encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		encoder = zapcore.NewConsoleEncoder(encCfg)
	}

	// Always write to stdout.
	writeSyncer := zapcore.AddSync(os.Stdout)

	// If a log file is configured, add a sync that is wrapped around lumberjack
	// for log rotation. Building log rotation into the logger means operators
	// don't have to set this up separately, thus lowering the barrier to entry.
	if cfg.File != "" {
		fileWriter := zapcore.AddSync(&lumberjack.Logger{
			Filename:   cfg.File,
			MaxSize:    100,
			MaxBackups: 5,
			MaxAge:     30,
			Compress:   true,
		})
		writeSyncer = zapcore.NewMultiWriteSyncer(writeSyncer, fileWriter)
	}

	core := zapcore.NewCore(encoder, writeSyncer, lvl)

	opts := []zap.Option{
		zap.WithCaller(*cfg.IncludeLine),
	}
	if *cfg.EnableStacktrace {
		opts = append(opts, zap.AddStacktrace(zapcore.ErrorLevel))
	}

	return zap.New(core, opts...), nil
}
