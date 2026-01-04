package client

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/config"
	"github.com/rasorp/smuggle/internal/log"
	"github.com/rasorp/smuggle/internal/network"
	"github.com/rasorp/smuggle/internal/types"
)

const (
	// clientIDFileName is the name of the file that stores the client ID within
	// the data directory.
	clientIDFileName = "id"
)

type Client struct {
	cfg *config.ClientConfig

	logger *zap.Logger

	// id is the unique identifier for this client instance. It is persisted
	// to disk in the data directory, so it remains consistent across restarts
	// and changes to the host.
	id atomic.Value

	// store is used to persist client state information and receive updates
	// about other subnets in the Smuggle network.
	store types.Store

	//
	cniStore types.CNIStore

	networkManager *network.Manager

	// networks tracks the networks that this Smuggle client is aware of and
	// should configure on the host.
	networks []*types.Network

	//
	subnets []*types.Subnet

	// shtutdownCh is used to signal to all client processes that the agent is
	// shutting down. All long-running processes should monitor this channel and
	// use the shutdownGroup wait group to ensure the agent does not exit before
	// they have completed.
	shutdownCh    chan struct{}
	shutdownGroup sync.WaitGroup
}

type ClientReq struct {
	Config   *config.ClientConfig
	Logger   *zap.Logger
	Store    types.Store
	CNIStore types.CNIStore
}

func New(req *ClientReq) (*Client, error) {

	netManager, err := network.NewManager(req.Logger, req.Config.NetworkInterface)
	if err != nil {
		return nil, fmt.Errorf("failed to create network manager: %w", err)
	}

	return &Client{
		cfg:            req.Config,
		logger:         req.Logger.Named(log.ComponentNameClient),
		networks:       []*types.Network{},
		store:          req.Store,
		cniStore:       req.CNIStore,
		networkManager: netManager,
		shutdownCh:     make(chan struct{}),
	}, nil
}

func (c *Client) Start() error {

	if err := c.generateID(); err != nil {
		return fmt.Errorf("failed to get client ID: %w", err)
	}

	if err := c.Init(); err != nil {
		return fmt.Errorf("failed to initialize client: %w", err)
	}

	if err := c.startSubnetUpdateHandler(); err != nil {
		return fmt.Errorf("failed to start remote subnet handler: %w", err)
	}

	c.startHeartbeaters()

	return nil
}

func (c *Client) Stop() error {
	c.logger.Info("stopping client processes")

	close(c.shutdownCh)

	// In order to avoid blocking forever is the shutdown groups do not
	// terminate correctly, we use a timer to enforce a timeout. In order to do
	// this, we use a channel that will unblock once the wait group is done.
	waitFinishedCh := make(chan struct{})

	go func() {
		c.shutdownGroup.Wait()
		close(waitFinishedCh)
	}()

	t := time.NewTimer(10 * time.Second)
	defer t.Stop()

	// Wait for either the wait group to finish or the timer to expire.
	// Returning an error provides operator feedback that something is not right
	// during shutdown.
	select {
	case <-t.C:
		return errors.New("timeout waiting for shutdown")
	case <-waitFinishedCh:
	}
	return nil
}

func (c *Client) Init() error {

	// Read all network configurations from the store that we are able to see
	// and therefore should configure on this host.
	listResp, err := c.store.ListNetworks(nil)
	if err != nil {
		return fmt.Errorf("failed to get network configs: %w", err)
	}

	if len(listResp.Networks) == 0 {
		return errors.New("no networks configurations found")
	}

	for _, networkConfig := range listResp.Networks {

		// Validate the network configuration.
		if err := networkConfig.Validate(); err != nil {
			return fmt.Errorf("invalid network: %w", err)
		}

		c.networks = append(c.networks, networkConfig)

		clientSubnetResp, err := c.store.GetSubnet(&types.StoreGetSubnetReq{
			ID:          c.id.Load().(string),
			NetworkName: networkConfig.Name,
		})
		if err != nil {
			return fmt.Errorf("failed to get client subnet config: %w", err)
		}

		// Perform the canonicalization, so we have all fields set correctly
		// set. It would be possible to write this back to the data store, but
		// seeing as this happens on the client, if more than one started at the
		// same time, they would all race to write it back.
		networkConfig.Canonicalize()

		subnet := clientSubnetResp.Subnet

		if subnet == nil {

			//
			subnetListReq := types.StoreListSubnetsReq{Network: networkConfig.Name}

			subnetListResp, err := c.store.ListSubnets(&subnetListReq)
			if err != nil {
				return fmt.Errorf("failed to list existing client subnets: %w", err)
			}

			subnet, err = c.networkManager.GenerateIPv4Subnet(c.getID(), networkConfig, subnetListResp.Subnets)
			if err != nil {
				return fmt.Errorf("failed to generate IPv4 subnet: %w", err)
			}
		}

		c.logger.Info("initializing local host subnet", networkConfig.LoggingPairs()...)

		if err := c.initSubnet(networkConfig, subnet); err != nil {
			return fmt.Errorf("failed to initialize subnet: %w", err)
		}

		c.subnets = append(c.subnets, subnet)

		if networkConfig.IPMasq != nil && *networkConfig.IPMasq {
			if err := c.networkManager.Firewall.SetupMasqRules(networkConfig, subnet); err != nil {
				return fmt.Errorf("failed to set up firewall masquerade rules: %w", err)
			}
		}

		if err := c.networkManager.Firewall.SetupForwardRules(networkConfig); err != nil {
			return fmt.Errorf("failed to set up firewall forward rules: %w", err)
		}

		c.logger.Info("successfully initialized local host subnet", subnet.LoggingPairs()...)
	}

	if err := c.networkManager.Firewall.EnsureIsolation(c.networks); err != nil {
		return fmt.Errorf("failed to ensure network isolation: %w", err)
	}

	return nil
}

func (c *Client) initSubnet(netCfg *types.Network, cfg *types.Subnet) error {

	providerResp, err := c.networkManager.SetLocal(&types.NetworkProviderSetReq{Client: cfg})
	if err != nil {
		return fmt.Errorf("failed to set up local subnet: %w", err)
	}

	if _, err := c.store.SetSubnet(&types.StoreSetSubnetReq{
		Subnet: providerResp.Network,
	}); err != nil {
		return fmt.Errorf("failed to store client subnet: %w", err)
	}

	if err := c.cniStore.Set(types.GenerateCNIConfig(netCfg, cfg)); err != nil {
		return fmt.Errorf("failed to write CNI config: %w", err)
	}

	return nil
}

// generateID attempts to read the client ID from disk. If the file does not exist,
// it generates a new UUID, saves it to disk, and returns it.
func (c *Client) generateID() error {
	idFilePath := filepath.Join(c.cfg.DataDir, clientIDFileName)

	// Try to read existing ID from file
	data, err := os.ReadFile(idFilePath)
	if err == nil {
		id := string(data)
		if id != "" {
			c.id.Store(id)
			return nil
		}
	}

	// File doesn't exist or is empty - generate new UUID
	if !os.IsNotExist(err) && err != nil {
		// Some other error occurred (not just file not found)
		return fmt.Errorf("failed to read client ID file: %w", err)
	}

	// Ensure data directory exists
	if err := os.MkdirAll(c.cfg.DataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Generate new UUID
	newID := uuid.New().String()

	// Write the new ID to file
	if err := os.WriteFile(idFilePath, []byte(newID), 0600); err != nil {
		return fmt.Errorf("failed to write client ID to file: %w", err)
	}

	c.id.Store(newID)

	return nil
}

// getID is a helper function to retrieve the client ID from the atomic value as
// a string.
func (c *Client) getID() string { return c.id.Load().(string) }
