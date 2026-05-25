package agent

import (
	"fmt"

	"github.com/rasorp/smuggle/internal/client"
	"github.com/rasorp/smuggle/internal/config"
	"github.com/rasorp/smuggle/internal/log"
	"github.com/rasorp/smuggle/internal/store/file"
)

// NewClientAgent constructs and returns an Agent configured to run the Smuggle
// client. It sets up logging, a Nomad client, the backing store, the CNI store,
// and the client itself, then wires everything into an Agent whose
// Start/Stop/WaitForSignal methods drive the lifecycle.
func NewClientAgent(cfg *config.ClientAgentConfig) (*Agent, error) {
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

	cl, err := client.New(&client.ClientReq{
		Config:   cfg.Client,
		Logger:   logger,
		Store:    store,
		CNIStore: file.NewCNIStore("/opt/smuggle/config"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return New(&AgentReq{
		Logger:     logger,
		HTTPConfig: cfg.HTTP,
		Start:      cl.Start,
		Stop:       cl.Stop,
		Reload:     cl.Reload,
	})
}
