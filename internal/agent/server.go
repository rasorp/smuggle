package agent

import (
	"fmt"

	"github.com/hashicorp/nomad/api"

	"github.com/rasorp/smuggle/internal/config"
	"github.com/rasorp/smuggle/internal/log"
	"github.com/rasorp/smuggle/internal/server"
	"github.com/rasorp/smuggle/internal/store/nvar"
	"github.com/rasorp/smuggle/internal/types"
)

// NewServerAgent constructs and returns an Agent configured to run the Smuggle
// server. It sets up logging, a Nomad client, the backing store, and the server
// itself, then wires everything into an Agent whose Start/Stop/WaitForSignal
// methods drive the lifecycle.
func NewServerAgent(cfg *config.ServerAgentConfig) (*Agent, error) {
	logger, err := log.New(cfg.Log)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	nomadClient, err := generateNomadClient(cfg.Nomad, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create Nomad client: %w", err)
	}

	store, err := setupStore(cfg.Store, nomadClient)
	if err != nil {
		return nil, fmt.Errorf("failed to setup store: %w", err)
	}

	srv, err := server.New(&server.ServerReq{
		Config: cfg.Server,
		Logger: logger,
		Store:  store,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	return New(&AgentReq{
		Logger:     logger,
		HTTPConfig: cfg.HTTP,
		Start:      srv.Start,
		Stop:       srv.Stop,
	})
}

// setupStore initialises the backing store based on the provided StoreConfig.
func setupStore(cfg *config.StoreConfig, nomadClient *api.Client) (types.Store, error) {
	switch cfg.Backend {
	case "nvar":
		return nvar.New(nomadClient, cfg.NVar.Path), nil
	default:
		return nil, fmt.Errorf("unsupported store backend: %q", cfg.Backend)
	}
}
