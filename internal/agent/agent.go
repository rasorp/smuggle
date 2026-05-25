package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/hashicorp/nomad/api"
	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/config"
	"github.com/rasorp/smuggle/internal/helper/retry"
	"github.com/rasorp/smuggle/internal/http"
	"github.com/rasorp/smuggle/internal/log"
	"github.com/rasorp/smuggle/internal/version"
)

// Agent holds the long-running lifecycle logic common to both the server and
// client commands.
type Agent struct {
	logger     *zap.Logger
	httpServer *http.Server
	start      func() error
	stop       func() error
	reload     func()
}

// AgentReq is the configuration used to construct a new Agent.
type AgentReq struct {
	Logger     *zap.Logger
	HTTPConfig *config.HTTPConfig

	// Start is an optional function called by Agent.Start before the HTTP
	// server is started.
	Start func() error

	// Stop is called by Agent.Stop after the HTTP server has been shut down.
	Stop func() error

	// Reload is an optional function called when SIGHUP is received. It
	// should trigger a non-blocking reload of dynamic configuration.
	Reload func()
}

// New constructs an Agent from the provided AgentReq. If HTTP is enabled in
// the configuration, the HTTP server is set up and will be started/stopped as
// part of the agent lifecycle.
func New(req *AgentReq) (*Agent, error) {
	if req.Logger == nil {
		return nil, errors.New("logger is required")
	}

	a := &Agent{
		logger: req.Logger.Named(log.ComponentNameAgent),
		start:  req.Start,
		stop:   req.Stop,
		reload: req.Reload,
	}

	if req.HTTPConfig != nil && req.HTTPConfig.Enabled != nil && *req.HTTPConfig.Enabled {
		a.httpServer = http.New(req.HTTPConfig, req.Logger)
	}

	return a, nil
}

// Start runs the optional start hook, then brings up the HTTP server (if
// configured), and finally logs the startup banner.
func (a *Agent) Start() error {
	if a.start != nil {
		if err := a.start(); err != nil {
			return fmt.Errorf("failed to start agent: %w", err)
		}
	}

	if a.httpServer != nil {
		if err := a.httpServer.Start(); err != nil {
			return fmt.Errorf("failed to start HTTP server: %w", err)
		}
	}

	a.logger.Info("started",
		zap.String("version", version.Get()),
		zap.String("build_commit", version.BuildCommit),
		zap.String("build_time", version.BuildTime),
	)

	return nil
}

// Stop shuts down the HTTP server (if present) and then calls the stop
// function supplied at construction time.
func (a *Agent) Stop() error {

	// If the HTTP server is enabled, we attempt to gracefully shutdown with a
	// timeout context. If the shutdown fails, we log the error but continue with
	// the rest of the shutdown process to avoid leaving the agent in a broken
	// state.
	//
	// In the future, we may want to consider more robust error handling here,
	// such as retrying the shutdown or escalating the error. For now, the HTTP
	// server only exposes a health check endpoint and does not manage critical
	// resources, so we prioritize ensuring the rest of the shutdown process
	// completes.
	if a.httpServer != nil {

		timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := a.httpServer.Shutdown(timeoutCtx); err != nil {
			a.logger.Error("failed to gracefully shutdown HTTP server", zap.Error(err))
		}
	}

	if a.stop != nil {
		if err := a.stop(); err != nil {
			return fmt.Errorf("failed to stop agent: %w", err)
		}
	}

	return nil
}

// WaitForSignal blocks until SIGTERM, SIGINT, or SIGHUP is received. On SIGHUP
// it logs that configuration reload is not yet implemented. On all other
// signals it calls Stop and returns.
func (a *Agent) WaitForSignal() {
	signalCh := make(chan os.Signal, 3)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	a.logger.Info("signal notification handler running")

	for {
		sig := <-signalCh
		a.logger.Info("received notification signal", zap.String("signal", sig.String()))

		switch sig {
		case syscall.SIGHUP:
			a.logger.Info("SIGHUP received, reloading agent")
			if a.reload != nil {
				a.reload()
			}
		default:
			a.logger.Info("shutting down")
			if err := a.Stop(); err != nil {
				a.logger.Error("failed to gracefully shutdown", zap.Error(err))
			} else {
				a.logger.Info("successfully shutdown")
			}
			return
		}
	}
}

// generateNomadClient creates a Nomad API client from the provided
// configuration and verifies connectivity by pinging the API with retries.
func generateNomadClient(cfg *config.NomadConfig, logger *zap.Logger) (*api.Client, error) {

	nomadClient, err := config.NomadClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Nomad client: %w", err)
	}

	// Verify connectivity to the Nomad API with retries, as the agent may be
	// starting up before the API is available. We use the Leader endpoint as a
	// simple and lightweight.
	if err := retry.Retry(func() error {
		_, err := nomadClient.Status().Leader()
		if err != nil {
			logger.Warn("failed to ping the Nomad API", zap.Error(err))
		}
		return err
	}); err != nil {
		return nil, fmt.Errorf("failed to connect to Nomad: %w", err)
	}

	return nomadClient, nil
}
