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
	// should configure on the host. This is used to compare against updates
	// from the store to determine when networks are added or removed. All
	// access to this slice must be synchronized using the networksLock mutex.
	networks     []*types.Network
	networksLock sync.RWMutex

	subnetWatchers     map[string]*subnetWatcher
	subnetWatchersLock sync.Mutex

	// subnetHeartbeaters tracks the currently running subnet heartbeaters. It
	// is keyed by subnet ID and all access must be synchronized using the
	// subnetHeartbeatersLock mutex.
	subnetHeartbeaters     map[string]*heartbeater
	subnetHeartbeatersLock sync.Mutex

	//
	reloadCh chan struct{}

	// shutdownCh is used to signal to all client processes that the agent is
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
		cfg:                req.Config,
		logger:             req.Logger.Named(log.ComponentNameClient),
		networks:           []*types.Network{},
		store:              req.Store,
		cniStore:           req.CNIStore,
		networkManager:     netManager,
		reloadCh:           make(chan struct{}, 1),
		shutdownCh:         make(chan struct{}),
		subnetWatchers:     make(map[string]*subnetWatcher),
		subnetHeartbeaters: make(map[string]*heartbeater),
	}, nil
}

// Reload signals the client to immediately re-read the network list from the
// store. If a reload is already pending, the signal is dropped to avoid
// queuing up redundant work.
func (c *Client) Reload() {
	select {
	case c.reloadCh <- struct{}{}:
	default:
		// If the channel is full, do not block. This means a reload is already
		// pending, so we do not need to send another signal.
	}
}

func (c *Client) Start() error {

	if err := c.generateID(); err != nil {
		return fmt.Errorf("failed to get client ID: %w", err)
	}

	// Perform the initial network setup synchronously so that all local subnets
	// and remote subnet routes are in place before Start returns. The
	// background monitor handles periodic refreshes and SIGHUP reloads after
	// this point.
	c.triggerNetworksRead()

	go c.monitorNetworks()

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

func (c *Client) shutdownGroupDecrement() { c.shutdownGroup.Done() }

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
