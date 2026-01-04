package log

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/rasorp/smuggle/internal/config"
)

const (
	ComponentNameAgent    = "agent"
	ComponentNameServer   = "server"
	ComponentNameClient   = "client"
	ComponentNameHTTP     = "http"
	ComponentNameNetwork  = "network"
	ComponentNameIptables = "store"
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

	enc := "console"
	ts := zapcore.ISO8601TimeEncoder

	if *cfg.JSON {
		enc = "json"
		ts = zapcore.RFC3339NanoTimeEncoder
	}

	baseCfg := zap.NewProductionConfig()
	baseCfg.DisableStacktrace = true
	baseCfg.Level = lvl
	baseCfg.Encoding = enc
	baseCfg.DisableCaller = !*cfg.IncludeLine
	baseCfg.EncoderConfig.NameKey = "component"
	baseCfg.EncoderConfig.TimeKey = "timestamp"
	baseCfg.EncoderConfig.EncodeTime = ts

	return baseCfg.Build()
}
