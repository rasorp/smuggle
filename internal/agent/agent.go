package agent

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/agent/client"
	"github.com/rasorp/smuggle/internal/agent/server"
	"github.com/rasorp/smuggle/internal/config"
	"github.com/rasorp/smuggle/internal/http"
	"github.com/rasorp/smuggle/internal/log"
	"github.com/rasorp/smuggle/internal/store/file"
	"github.com/rasorp/smuggle/internal/store/nvar"
	"github.com/rasorp/smuggle/internal/types"
	"github.com/rasorp/smuggle/internal/version"
)

// Agent coordinates reading subnet configurations from storage
// and writing CNI configurations to disk.
type Agent struct {
	cfg    *config.AgentConfig
	logger *zap.Logger

	httpServer *http.Server

	client *client.Client
	server *server.Server
}

// NewAgent creates a new Agent with the provided stores.
func New(cfg *config.AgentConfig) (*Agent, error) {

	logger, err := log.New(cfg.Log)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	a := Agent{
		cfg:    cfg,
		logger: logger.Named(log.ComponentNameAgent),
	}

	if cfg.HTTP != nil && cfg.HTTP.Enabled != nil && *cfg.HTTP.Enabled {
		a.httpServer = http.New(cfg.HTTP, logger)
	}

	if cfg.Client.IsEnabled() {
		if err := a.setupClient(); err != nil {
			return nil, fmt.Errorf("failed to setup client: %w", err)
		}
	}
	if cfg.Server.IsEnabled() {
		if err := a.setupServer(); err != nil {
			return nil, fmt.Errorf("failed to setup server: %w", err)
		}
	}

	return &a, nil
}

func (a *Agent) setupClient() error {

	store, err := a.setupStore()
	if err != nil {
		return fmt.Errorf("failed to setup store: %w", err)
	}

	clientReq := &client.ClientReq{
		Config:   a.cfg.Client,
		CNIStore: file.NewCNIStore("/opt/smuggle/config"),
		Logger:   a.logger,
		Store:    store,
	}

	cl, err := client.New(clientReq)
	if err != nil {
		return err
	}

	a.client = cl
	return nil
}

func (a *Agent) setupServer() error {

	store, err := a.setupStore()
	if err != nil {
		return fmt.Errorf("failed to setup store: %w", err)
	}

	serverReq := &server.ServerReq{
		Config: a.cfg.Server,
		Logger: a.logger,
		Store:  store,
	}

	server, err := server.New(serverReq)
	if err != nil {
		return err
	}

	a.server = server
	return nil
}

func (a *Agent) setupStore() (types.Store, error) {

	nomadClient, err := a.setupNomadClient()
	if err != nil {
		return nil, err
	}

	switch a.cfg.Store.Backend {
	case "nvar":
		return nvar.New(nomadClient, a.cfg.Store.NVar.Path), nil
	default:
		return nil, fmt.Errorf("unsupported store backend: %q", a.cfg.Store.Backend)
	}
}

func (a *Agent) Start() error {

	if a.client != nil {
		if err := a.client.Start(); err != nil {
			return fmt.Errorf("failed to start client: %w", err)
		}
	}
	if a.server != nil {
		if err := a.server.Start(); err != nil {
			return fmt.Errorf("failed to start server: %w", err)
		}
	}
	if a.httpServer != nil {
		if err := a.httpServer.Start(); err != nil {
			return fmt.Errorf("failed to start HTTP server: %w", err)
		}
	}

	// Log startup information as it's useful for debugging and general
	// operational visibility.
	a.logger.Info("started agent",
		zap.String("version", version.Get()),
		zap.String("build_commit", version.BuildCommit),
		zap.String("build_time", version.BuildTime),
	)

	return nil
}

func (a *Agent) Stop() error {
	if a.httpServer != nil {
		if err := a.httpServer.Shutdown(context.Background()); err != nil {
			a.logger.Error("failed to gracefully shutdown HTTP server", zap.Error(err))
		}
	}
	if a.client != nil {
		if err := a.client.Stop(); err != nil {
			a.logger.Error("failed to gracefully shutdown client", zap.Error(err))
		}
	}
	if a.server != nil {
		if err := a.server.Stop(); err != nil {
			a.logger.Error("failed to gracefully shutdown server", zap.Error(err))
		}
	}
	return nil
}

func (a *Agent) WaitForSignal() {

	signalCh := make(chan os.Signal, 3)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)

	a.logger.Info("signal notification handler running")

	// Wait to receive a signal. This blocks until we are notified.
	for {

		sig := <-signalCh
		a.logger.Info("received notification signal", zap.String("signal", sig.String()))

		switch sig {
		case syscall.SIGHUP:
			a.logger.Info("SIGHUP received, configuration reload not yet implemented")
		default:
			a.logger.Info("shutting down agent")
			if err := a.Stop(); err != nil {
				a.logger.Error("failed to gracefully shutdown agent", zap.Error(err))
			} else {
				a.logger.Info("successfully shutdown agent")
			}
			return
		}
	}
}
