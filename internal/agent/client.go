package agent

import (
	"fmt"

	"github.com/rasorp/smuggle/internal/client"
	"github.com/rasorp/smuggle/internal/client/cni"
	"github.com/rasorp/smuggle/internal/config"
	"github.com/rasorp/smuggle/internal/log"
	"github.com/rasorp/smuggle/internal/rpc"
)

// NewClientAgent constructs and returns an Agent configured to run the Smuggle
// client. It sets up logging, an RPC client connected to the configured
// servers, the CNI store, and the client itself, then wires everything into an
// Agent whose Start/Stop/WaitForSignal methods drive the lifecycle.
func NewClientAgent(cfg *config.ClientAgentConfig) (*Agent, error) {
	logger, err := log.New(cfg.Log)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}

	rpcClient, err := rpc.NewClient(cfg.Client.Servers.Addresses, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to create RPC client: %w", err)
	}

	cl, err := client.New(&client.ClientReq{
		Config:   cfg.Client,
		Logger:   logger,
		Server:   rpcClient,
		CNIStore: cni.NewStore(config.DefaultCNIDir),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return New(&AgentReq{
		Logger:     logger,
		HTTPConfig: cfg.HTTP,
		Start:      cl.Start,
		Stop:       cl.Stop,
		SIGHUP:     cl.Reload,
	})
}
