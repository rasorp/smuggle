package rpc

import (
	"fmt"
	"time"

	"go.uber.org/zap"
)

// loggedNetworkHandler wraps NetworkHandler and emits a structured log line
// for every RPC call, recording the service, method, latency, and any error.
type loggedNetworkHandler struct {
	inner    *NetworkHandler
	logger   *zap.Logger
	loggerfn func(string, ...zap.Field)
}

func newLoggedNetworkHandler(h *NetworkHandler, logger *zap.Logger, accessLevel string) *loggedNetworkHandler {
	return &loggedNetworkHandler{
		inner:    h,
		logger:   logger,
		loggerfn: rpcLogFn(logger, accessLevel),
	}
}

func (h *loggedNetworkHandler) List(args *NetworkListArgs, reply *NetworkListReply) error {
	start := time.Now()
	err := h.inner.List(args, reply)
	logRPCCall(h.logger, h.loggerfn, NetworkService, "List", start, err)
	return err
}

// loggedSubnetHandler wraps SubnetHandler and emits a structured log line
// for every RPC call, recording the service, method, latency, and any error.
type loggedSubnetHandler struct {
	inner    *SubnetHandler
	logger   *zap.Logger
	loggerfn func(string, ...zap.Field)
}

func newLoggedSubnetHandler(h *SubnetHandler, logger *zap.Logger, accessLevel string) *loggedSubnetHandler {
	return &loggedSubnetHandler{
		inner:    h,
		logger:   logger,
		loggerfn: rpcLogFn(logger, accessLevel),
	}
}

func (h *loggedSubnetHandler) List(args *SubnetListArgs, reply *SubnetListReply) error {
	start := time.Now()
	err := h.inner.List(args, reply)
	logRPCCall(h.logger, h.loggerfn, SubnetService, "List", start, err)
	return err
}

func (h *loggedSubnetHandler) Get(args *SubnetGetArgs, reply *SubnetGetReply) error {
	start := time.Now()
	err := h.inner.Get(args, reply)
	logRPCCall(h.logger, h.loggerfn, SubnetService, "Get", start, err)
	return err
}

func (h *loggedSubnetHandler) Set(args *SubnetSetArgs, reply *SubnetSetReply) error {
	start := time.Now()
	err := h.inner.Set(args, reply)
	logRPCCall(h.logger, h.loggerfn, SubnetService, "Set", start, err)
	return err
}

func (h *loggedSubnetHandler) Delete(args *SubnetDeleteArgs, reply *SubnetDeleteReply) error {
	start := time.Now()
	err := h.inner.Delete(args, reply)
	logRPCCall(h.logger, h.loggerfn, SubnetService, "Delete", start, err)
	return err
}

func (h *loggedSubnetHandler) Watch(args *SubnetWatchArgs, reply *SubnetWatchReply) error {
	start := time.Now()
	err := h.inner.Watch(args, reply)
	logRPCCall(h.logger, h.loggerfn, SubnetService, "Watch", start, err)
	return err
}

// logRPCCall emits a single structured log line for a completed RPC call.
// Successful calls are logged via loggerfn at the configured access log level;
// failures are always logged at error level.
func logRPCCall(
	logger *zap.Logger,
	loggerfn func(string, ...zap.Field),
	service, method string,
	start time.Time,
	err error,
) {
	fields := []zap.Field{
		zap.String("service", service),
		zap.String("method", method),
		zap.Int64("latency_ns", time.Since(start).Nanoseconds()),
	}
	if err != nil {
		logger.Error("failed to handle RPC call", append(fields, zap.Error(err))...)
		return
	}
	loggerfn("successfully handled RPC call", fields...)
}

// rpcLogFn resolves the access log level string to the corresponding logger
// method. It panics on an unrecognised level, matching the behaviour of the
// HTTP logger middleware as it's unexpected behaviour as we validate the level
// on server start.
func rpcLogFn(logger *zap.Logger, accessLevel string) func(string, ...zap.Field) {
	switch accessLevel {
	case zap.DebugLevel.String():
		return logger.Debug
	case zap.InfoLevel.String():
		return logger.Info
	case zap.WarnLevel.String():
		return logger.Warn
	case zap.ErrorLevel.String():
		return logger.Error
	default:
		panic(fmt.Sprintf("unsupported RPC access log level: %q", accessLevel))
	}
}
