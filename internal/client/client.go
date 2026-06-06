package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	set "github.com/hashicorp/go-set/v3"
	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/client/cni"
	"github.com/rasorp/smuggle/internal/config"
	"github.com/rasorp/smuggle/internal/helper/file"
	"github.com/rasorp/smuggle/internal/log"
	"github.com/rasorp/smuggle/internal/network"
	"github.com/rasorp/smuggle/internal/types"
)

const (
	// clientIDFileName is the name of the file that stores the client ID within
	// the data directory.
	clientIDFileName = "id"

	// networksDirName is the name of the subdirectory within the data
	// directory that holds per-network state files. Each network is stored as
	// <data_dir>/networks/<name>.json, which makes individual additions and
	// removals atomic without rewriting the whole list.
	networksDirName = "networks"

	// networkReconcilerInterval is the period between scheduled reconciliation
	// runs. A SIGHUP triggers an immediate run in addition to the ticker.
	networkReconcilerInterval = 30 * time.Second
)

type Client struct {
	cfg *config.ClientConfig

	logger *zap.Logger

	// id is the unique identifier for this client instance. It is persisted
	// to disk in the data directory, so it remains consistent across restarts
	// and changes to the host.
	id atomic.Value

	// server is the remote Smuggle server the client communicates with to read
	// and write subnet state and watch for changes.
	server Server

	// cniStore writes and deletes CNI configuration files on disk in response
	// to subnet additions or deletions.
	cniStore *cni.Store

	networkManager *network.Manager

	// mu protects networks, subnets, subnetStopChs, watcherStopChs, and networkWGs.
	mu sync.RWMutex

	// networks tracks the networks that this Smuggle client is aware of and
	// should configure on the host.
	networks []*types.Network

	// subnets tracks the subnets assigned to this client on each configured
	// network. The set is keyed by the network name, so there can only be one
	// subnet per network.
	subnets *set.HashSet[*types.Subnet, string]

	// subnetStopChs holds per-network stop channels for heartbeat goroutines.
	// Closing a channel causes the corresponding heartbeat goroutine to exit
	// without waiting for the global shutdown.
	subnetStopChs map[string]chan struct{}

	// watcherStopChs holds per-network stop channels for subnet-watcher
	// goroutines. Closing a channel causes that watcher to exit independently
	// of the global shutdown.
	watcherStopChs map[string]chan struct{}

	// networkWGs holds a per-network WaitGroup that is incremented by each
	// goroutine launched for that network (heartbeat, watcher) and decremented
	// when they exit. teardownNetwork waits on the group before destroying
	// network state, ensuring no goroutine accesses a network after it has been
	// torn down.
	networkWGs map[string]*sync.WaitGroup

	// reconcileTriggerCh accepts a single token to trigger an immediate
	// reconcileNetworks() call from outside the reconciler goroutine (e.g. on
	// SIGHUP). The channel is buffered with capacity 1 so the sender never
	// blocks.
	reconcileTriggerCh chan struct{}

	// shutdownCh is used to signal to all client processes that the agent is
	// shutting down. All long-running processes should monitor this channel and
	// use the shutdownGroup wait group to ensure the agent does not exit before
	// they have completed.
	shutdownCh chan struct{}

	// shutdownGroup tracks all long-running client processes (e.g. heartbeats,
	// watchers, reconciler) to ensure they have all exited before Stop()
	//  returns.
	shutdownGroup sync.WaitGroup
}

type ClientReq struct {
	Config   *config.ClientConfig
	Logger   *zap.Logger
	Server   Server
	CNIStore *cni.Store
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
		subnets:            set.NewHashSet[*types.Subnet, string](0),
		subnetStopChs:      make(map[string]chan struct{}),
		watcherStopChs:     make(map[string]chan struct{}),
		networkWGs:         make(map[string]*sync.WaitGroup),
		reconcileTriggerCh: make(chan struct{}, 1),
		server:             req.Server,
		cniStore:           req.CNIStore,
		networkManager:     netManager,
		shutdownCh:         make(chan struct{}),
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
	c.startNetworkReconciler()

	return nil
}

func (c *Client) Stop() error {
	c.logger.Info("stopping client processes")

	close(c.shutdownCh)

	// In order to avoid blocking forever if the shutdown groups do not
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

// Reload is used by the agent when a SIGHUP is received. It triggers an
// immediate run of the network reconciler.
func (c *Client) Reload() {
	select {
	case c.reconcileTriggerCh <- struct{}{}:
	default:
		// A trigger is already queued; the reconciler will handle it shortly.
	}
}

func (c *Client) Init() error {

	// Read all network configurations from the store that we are able to see
	// and therefore should configure on this host.
	listResp, err := c.server.ListNetworks(nil)
	if err != nil {
		return fmt.Errorf("failed to get network configs: %w", err)
	}

	if len(listResp.Networks) == 0 {
		c.logger.Warn("no network configurations found; reconciler will retry when networks appear")
	}

	for _, networkConfig := range listResp.Networks {
		if _, err := c.initNetwork(networkConfig); err != nil {
			return fmt.Errorf("failed to initialize network %q: %w", networkConfig.Name, err)
		}
	}

	if err := c.networkManager.Firewall.EnsureIsolation(c.networks); err != nil {
		return fmt.Errorf("failed to ensure network isolation: %w", err)
	}

	return nil
}

// initNetwork performs the full per-network setup: it validates the config,
// acquires or generates a subnet, configures the network interface and
// firewall, updates the in-memory state, and persists the network list to
// disk. It returns the assigned subnet so the caller can start the heartbeat
// goroutine.
func (c *Client) initNetwork(networkConfig *types.Network) (*types.Subnet, error) {

	// Validate the network configuration.
	if err := networkConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid network: %w", err)
	}

	clientSubnetResp, err := c.server.GetSubnet(&types.StoreGetSubnetReq{
		ID:          c.getID(),
		NetworkName: networkConfig.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get client subnet config: %w", err)
	}

	// Perform the canonicalization, so we have all fields set correctly
	// set. It would be possible to write this back to the data store, but
	// seeing as this happens on the client, if more than one started at the
	// same time, they would all race to write it back.
	networkConfig.Canonicalize()

	subnet := clientSubnetResp.Subnet

	if subnet == nil {

		subnetListReq := types.StoreListSubnetsReq{Network: networkConfig.Name}

		subnetListResp, err := c.server.ListSubnets(&subnetListReq)
		if err != nil {
			return nil, fmt.Errorf("failed to list existing client subnets: %w", err)
		}

		subnet, err = c.networkManager.GenerateIPv4Subnet(c.getID(), networkConfig, subnetListResp.Subnets)
		if err != nil {
			return nil, fmt.Errorf("failed to generate IPv4 subnet: %w", err)
		}
	}

	c.logger.Info("initializing local host subnet", networkConfig.LoggingPairs()...)

	if err := c.initSubnet(networkConfig, subnet); err != nil {
		return nil, fmt.Errorf("failed to initialize subnet: %w", err)
	}

	if networkConfig.IPMasq != nil && *networkConfig.IPMasq {
		if err := c.networkManager.Firewall.SetupMasqRules(networkConfig, subnet); err != nil {
			return nil, fmt.Errorf("failed to set up firewall masquerade rules: %w", err)
		}
	}

	if err := c.networkManager.Firewall.SetupForwardRules(networkConfig); err != nil {
		return nil, fmt.Errorf("failed to set up firewall forward rules: %w", err)
	}

	// Update in-memory state under the write lock.
	c.mu.Lock()
	c.networks = append(c.networks, networkConfig)
	c.subnets.Insert(subnet)
	c.mu.Unlock()

	// Failing to persist the network to disk is not fatal. The reconciler will
	// detect the discrepancy on the next run and retry the disk write without
	// re-running initialization.
	if err := c.saveNetworkToDisk(networkConfig); err != nil {
		c.logger.Error(
			"failed to persist network to disk",
			append(networkConfig.LoggingPairs(), zap.Error(err))...,
		)
	}

	c.logger.Info("successfully initialized local host subnet", subnet.LoggingPairs()...)
	return subnet, nil
}

// teardownNetwork is the inverse of initNetwork. It stops the heartbeat and
// watcher goroutines for the network, removes firewall rules, deletes the
// network interface, removes the CNI config, and updates in-memory and on-disk
// state.
func (c *Client) teardownNetwork(networkConfig *types.Network) {

	logPairs := networkConfig.LoggingPairs()
	c.logger.Info("tearing down network", logPairs...)

	// 1. Find the subnet for this network to use in the steps below.
	var subnet *types.Subnet
	c.mu.RLock()
	for s := range c.subnets.Items() {
		if s.NetworkName == networkConfig.Name {
			subnet = s
			break
		}
	}
	c.mu.RUnlock()

	// 2. Stop the heartbeat goroutine for this network's subnet.
	// 3. Stop the subnet watcher goroutine for this network.
	c.mu.Lock()
	if stopCh, ok := c.subnetStopChs[networkConfig.Name]; ok {
		close(stopCh)
		delete(c.subnetStopChs, networkConfig.Name)
	}
	if stopCh, ok := c.watcherStopChs[networkConfig.Name]; ok {
		close(stopCh)
		delete(c.watcherStopChs, networkConfig.Name)
	}
	wg, hasWG := c.networkWGs[networkConfig.Name]
	delete(c.networkWGs, networkConfig.Name)
	c.mu.Unlock()

	// Wait for the heartbeat and watcher goroutines to exit before destroying
	// network state. This prevents them from accessing an interface or subnet
	// that has already been removed.
	if hasWG {
		wg.Wait()
	}

	// 4. Remove firewall masquerade and forward rules for the network.
	if subnet != nil && networkConfig.IPMasq != nil && *networkConfig.IPMasq {
		if err := c.networkManager.Firewall.TeardownMasqRules(networkConfig, subnet); err != nil {
			c.logger.Warn("failed to tear down masquerade rules",
				append(logPairs, zap.Error(err))...)
		}
	}
	if err := c.networkManager.Firewall.TeardownForwardRules(networkConfig); err != nil {
		c.logger.Warn("failed to tear down forward rules",
			append(logPairs, zap.Error(err))...)
	}

	// 5. Delete the network interface from the host.
	if subnet != nil {
		if _, err := c.networkManager.DeleteLocal(&types.NetworkProviderDeleteLocalReq{
			Subnet: subnet,
		}); err != nil {
			c.logger.Warn("failed to delete local network interface",
				append(logPairs, zap.Error(err))...)
		}
	}

	// 6. Delete the CNI config file for the network.
	if err := c.cniStore.Delete(networkConfig.Name); err != nil {
		c.logger.Warn("failed to delete CNI config", append(logPairs, zap.Error(err))...)
	}

	// 7. Remove the network and its subnet from in-memory state.
	// 8. Remove the network's state file from disk.
	c.mu.Lock()
	newNetworks := make([]*types.Network, 0, len(c.networks))
	for _, n := range c.networks {
		if n.Name != networkConfig.Name {
			newNetworks = append(newNetworks, n)
		}
	}
	c.networks = newNetworks

	c.subnets.Remove(&types.Subnet{NetworkName: networkConfig.Name})
	c.mu.Unlock()

	if err := c.removeNetworkFromDisk(networkConfig.Name); err != nil {
		c.logger.Error("failed to remove network from filesystem after teardown",
			append(logPairs, zap.Error(err))...)
	}

	c.logger.Info("successfully tore down network", logPairs...)
}

// startNetworkReconciler launches the background goroutine that periodically
// diffs server state against local disk state and calls initNetwork /
// teardownNetwork as needed. It also reacts to reconcileTriggerCh for
// immediate runs (e.g. on SIGHUP).
func (c *Client) startNetworkReconciler() {
	c.shutdownGroup.Go(func() {

		ticker := time.NewTicker(networkReconcilerInterval)
		defer ticker.Stop()

		c.logger.Info("starting network reconciler",
			zap.String("interval", networkReconcilerInterval.String()))

		for {
			select {
			case <-c.shutdownCh:
				c.logger.Info("shutting down network reconciler")
				return

			case <-c.reconcileTriggerCh:
				c.reconcileNetworks()
				// Reset the ticker so the next scheduled run is a full
				// interval away from this manual trigger.
				ticker.Reset(networkReconcilerInterval)

			case <-ticker.C:
				c.reconcileNetworks()
				ticker.Reset(networkReconcilerInterval)
			}
		}
	})
}

// reconcileNetworks diffs the server's current network list against the
// networks persisted on disk and calls initNetwork / teardownNetwork for each
// addition / deletion it detects. It re-runs EnsureIsolation when any change
// is made.
func (c *Client) reconcileNetworks() {
	c.logger.Debug("running network reconciliation")

	serverResp, err := c.server.ListNetworks(nil)
	if err != nil {
		c.logger.Error("failed to list networks during reconciliation", zap.Error(err))
		return
	}

	localNetworks, err := c.loadNetworksFromDisk()
	if err != nil {
		c.logger.Error("failed to load local networks during reconciliation", zap.Error(err))
		return
	}

	serverSet := make(map[string]*types.Network, len(serverResp.Networks))
	for _, n := range serverResp.Networks {
		serverSet[n.Name] = n
	}

	localSet := make(map[string]*types.Network, len(localNetworks))
	for _, n := range localNetworks {
		localSet[n.Name] = n
	}

	var changed bool

	// Build the set of networks already configured in memory. This guards
	// against a subtle duplicate-initialization bug: if initNetwork succeeds
	// but the subsequent saveNetworkToDisk fails, the network is present in
	// c.networks but absent from disk. On the next reconciler tick it would
	// appear in server\local and trigger a second initNetwork call, appending
	// duplicate entries to c.networks and c.subnets and spawning extra
	// goroutines. When the network is already in memory we retry only the disk
	// write, skipping full re-initialization.
	//
	// reconcileNetworks is the only writer of c.networks after Start() returns,
	// so no lock is needed here.
	memSet := make(map[string]*types.Network, len(c.networks))
	for _, n := range c.networks {
		memSet[n.Name] = n
	}

	// server \ local → networks to add.
	for name, networkConfig := range serverSet {
		if _, ok := localSet[name]; ok {
			continue
		}

		// Already running in memory but missing from disk: a previous disk
		// write failed. Retry using the canonical in-memory config rather than
		// re-running full initialization.
		if memNetwork, ok := memSet[name]; ok {
			if err := c.saveNetworkToDisk(memNetwork); err != nil {
				c.logger.Error("failed to persist network to disk on retry",
					append(memNetwork.LoggingPairs(), zap.Error(err))...)
			}
			continue
		}

		c.logger.Info("reconciler detected new network, initializing",
			networkConfig.LoggingPairs()...)

		subnet, err := c.initNetwork(networkConfig)
		if err != nil {
			c.logger.Error("failed to initialize new network during reconciliation",
				append(networkConfig.LoggingPairs(), zap.Error(err))...)
			continue
		}

		wg := c.networkWGFor(networkConfig.Name)
		if err := c.startNetworkWatcher(networkConfig, wg); err != nil {
			c.logger.Error("failed to start watcher for new network",
				append(networkConfig.LoggingPairs(), zap.Error(err))...)
		}

		wg.Add(1)
		stopCh := c.newSubnetStopCh(networkConfig.Name)
		go c.startSubnetHeartbeat(subnet, stopCh, wg)

		changed = true
	}

	// local \ server → networks to remove.
	for name, networkConfig := range localSet {
		if _, ok := serverSet[name]; ok {
			continue
		}

		c.logger.Info("reconciler detected removed network, tearing down",
			networkConfig.LoggingPairs()...)

		c.teardownNetwork(networkConfig)

		changed = true
	}

	// mem \ (local ∪ server) → initialized in memory but never reached disk
	// and since deleted from the server. Without this pass the heartbeat and
	// watcher goroutines would run indefinitely for a network that no longer
	// exists. Networks that are still on the server are left alone (handled by
	// the add pass above); networks that are on disk are already covered by the
	// local \ server pass above.
	for name, memNetwork := range memSet {
		if _, ok := serverSet[name]; ok {
			continue
		}
		if _, ok := localSet[name]; ok {
			continue
		}

		c.logger.Info("reconciler detected orphaned in-memory network, tearing down",
			memNetwork.LoggingPairs()...)

		c.teardownNetwork(memNetwork)

		changed = true
	}

	if changed {
		if err := c.networkManager.Firewall.EnsureIsolation(c.networks); err != nil {
			c.logger.Error("failed to ensure network isolation after reconciliation",
				zap.Error(err))
		}
	}

	c.logger.Info("network reconciliation complete")
}

// localSubnets returns a snapshot of the current subnets slice under a read
// lock. Callers should use this instead of accessing the subnets object
// directly, so we avoid access data races.
func (c *Client) localSubnets() []*types.Subnet {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.subnets.Slice()
}

// newSubnetStopCh creates a fresh stop channel for the heartbeat goroutine of
// the named network.
func (c *Client) newSubnetStopCh(networkName string) chan struct{} {
	stopCh := make(chan struct{})
	c.mu.Lock()
	c.subnetStopChs[networkName] = stopCh
	c.mu.Unlock()
	return stopCh
}

// networkWGFor returns the WaitGroup for the named network, creating one if
// it does not already exist.
func (c *Client) networkWGFor(name string) *sync.WaitGroup {
	c.mu.Lock()
	defer c.mu.Unlock()
	wg, ok := c.networkWGs[name]
	if !ok {
		wg = &sync.WaitGroup{}
		c.networkWGs[name] = wg
	}
	return wg
}

// newWatcherStopCh creates a fresh stop channel for the subnet-watcher
// goroutine of the named network.
func (c *Client) newWatcherStopCh(networkName string) chan struct{} {
	stopCh := make(chan struct{})
	c.mu.Lock()
	c.watcherStopChs[networkName] = stopCh
	c.mu.Unlock()
	return stopCh
}

func (c *Client) initSubnet(netCfg *types.Network, cfg *types.Subnet) error {

	providerResp, err := c.networkManager.SetLocal(&types.NetworkProviderSetReq{Client: cfg})
	if err != nil {
		return fmt.Errorf("failed to set up local subnet: %w", err)
	}

	if _, err := c.server.SetSubnet(&types.StoreSetSubnetReq{
		Subnet: providerResp.Network,
	}); err != nil {
		return fmt.Errorf("failed to store client subnet: %w", err)
	}

	if err := c.cniStore.Set(cni.GenerateCNIConfig(netCfg, cfg)); err != nil {
		return fmt.Errorf("failed to write CNI config: %w", err)
	}

	return nil
}

// networksDirPath returns the absolute path of the directory that holds network
// state files within the client data directory.
func (c *Client) networksDirPath() string { return filepath.Join(c.cfg.DataDir, networksDirName) }

// networkFilePath returns the absolute path of the state file for a single
// named network.
func (c *Client) networkFilePath(name string) string {
	return filepath.Join(c.networksDirPath(), name+".json")
}

// loadNetworksFromDisk reads each per-network state file from the networks
// subdirectory and returns them as a slice. It returns nil (no error) when the
// directory does not exist yet.
func (c *Client) loadNetworksFromDisk() ([]*types.Network, error) {
	dir := c.networksDirPath()

	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read networks directory: %w", err)
	}

	var networks []*types.Network

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("failed to read network file %q: %w", entry.Name(), err)
		}

		var network types.Network
		if err := json.Unmarshal(data, &network); err != nil {
			// A single corrupt file should not prevent all other networks from
			// being reconciled. Log the problem and skip it; the operator can
			// remove or repair the file manually.
			c.logger.Warn("skipping unreadable network state file",
				zap.String("file", entry.Name()),
				zap.Error(err),
			)
			continue
		}

		networks = append(networks, &network)
	}

	return networks, nil
}

// saveNetworkToDisk atomically writes a single network's state to its own file
// under the networks subdirectory.
func (c *Client) saveNetworkToDisk(network *types.Network) error {
	data, err := json.MarshalIndent(network, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal network: %w", err)
	}
	return file.AtomicWrite(c.networkFilePath(network.Name), data, 0600)
}

// removeNetworkFromDisk deletes the state file for the named network.
func (c *Client) removeNetworkFromDisk(name string) error {
	return file.Delete(c.networkFilePath(name))
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

	// If we failed to read the file for any reason other than it not existing,
	// return an error rather than trying to continue. This prevents us from
	// taking action in an environment where we are unsure if we have a stable
	// ID.
	if !errors.Is(err, os.ErrNotExist) && err != nil {
		return fmt.Errorf("failed to read client ID file: %w", err)
	}

	// Generate a new UUID.
	newID := uuid.New().String()
	c.logger.Info("generated new client ID", zap.String("id", newID))

	if err := file.AtomicWrite(idFilePath, []byte(newID), 0600); err != nil {
		return fmt.Errorf("failed to write client ID to file: %w", err)
	}

	// Store the new ID once we have successfully written it to disk, so we
	// never have an in-memory ID that is not persisted. While the client will
	// exit if the file cannot be written, this future proofs against any
	// routines that might be trying to read this value and taking action.
	c.id.Store(newID)

	return nil
}

// getID is a helper function to retrieve the client ID from the atomic value as
// a string.
func (c *Client) getID() string { return c.id.Load().(string) }
