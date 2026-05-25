package client

import (
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/types"
)

func (c *Client) monitorNetworks() {
	c.shutdownGroup.Add(1)
	defer c.shutdownGroup.Done()

	// The ticker duration is currently hardcoded to 10 minutes, which is long
	// enough to avoid excessive load on the store, but short enough to ensure
	// that network changes are picked up in a timely manner.
	//
	// If operators require more immediate updates, they can issue a SIGHUP to
	// the agent process to trigger an immediate refresh of the network list.
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			c.triggerNetworksRead()
		case <-c.reloadCh:
			c.triggerNetworksRead()
			ticker.Reset(10 * time.Minute)
		case <-c.shutdownCh:
			c.logger.Info("shutting down network monitor")
			return
		}
	}
}

// triggerNetworksRead fetches the list of networks from the store and
// processes any additions, updates, or deletions.
func (c *Client) triggerNetworksRead() {
	listResp, err := c.store.ListNetworks(nil)
	if err != nil {
		c.logger.Error("failed to list networks", zap.Error(err))
	} else {
		c.logger.Info("successfully listed networks", zap.Int("network_num", len(listResp.Networks)))
		c.handleNetworkMonitorTrigger(listResp.Networks)
		c.logger.Info("network reload complete")
	}
}

func (c *Client) handleNetworkMonitorTrigger(nets []*types.Network) {

	// Lock the client's network list for the duration of this function, as we
	// will be modifying it based on the list of networks from the store. While
	// it's a coarse lock, it simplifies the logic and is acceptable given that
	// network updates are expected to be infrequent and the operations
	// performed here are not expected to be long-running.
	c.networksLock.Lock()
	defer c.networksLock.Unlock()

	// Build maps for easier comparison.
	currentNetworks := make(map[string]*types.Network, len(c.networks))
	for _, network := range c.networks {
		currentNetworks[network.Name] = network
	}

	// Iterate through the array of networks from the store. Validate and
	// canonicalize each one here, so we don't have to worry about it later.
	//
	// Invalid networks are logged and skipped. This protects us from a bad
	// update performing an incorrect reconfiguration of a network on the host.
	newNetworks := make(map[string]*types.Network, len(nets))

	for _, network := range nets {
		if err := network.Validate(); err != nil {
			c.logger.Error("invalid network configuration",
				zap.String("network_name", network.Name),
				zap.Error(err),
			)
		} else {
			network.Canonicalize()
			newNetworks[network.Name] = network
		}
	}

	var networkAdditions, networkUpdates, networksToDelete []*types.Network

	// Identify additions and updates.
	for name, newNet := range newNetworks {
		if currentNet, exists := currentNetworks[name]; !exists {
			networkAdditions = append(networkAdditions, newNet)
		} else if !currentNet.Equals(newNet) {
			networkUpdates = append(networkUpdates, newNet)
		}
	}

	// Identify deletions.
	deleteSet := make(map[string]struct{}, len(networksToDelete))
	for name, network := range currentNetworks {
		if _, exists := newNetworks[name]; !exists {
			networksToDelete = append(networksToDelete, network)
			deleteSet[name] = struct{}{}
		}
	}

	// Track successful operations for firewall updates.
	successfulAdditions := make([]*types.Network, 0, len(networkAdditions))
	successfulDeletions := make([]*types.Network, 0, len(networksToDelete))

	// Handle network deletions first to free up resources.
	for _, network := range networksToDelete {
		c.deleteNetwork(network)
		successfulDeletions = append(successfulDeletions, network)
	}

	// Remove deleted networks from slice using map for O(1) lookup.
	if len(deleteSet) > 0 {
		updatedNetworks := make([]*types.Network, 0, len(c.networks)-len(deleteSet))
		for _, network := range c.networks {
			if _, isDeleted := deleteSet[network.Name]; !isDeleted {
				updatedNetworks = append(updatedNetworks, network)
			}
		}
		c.networks = updatedNetworks

		// Delete isolation rules for removed networks.
		if err := c.networkManager.Firewall.DeleteIsolation(
			c.networks, successfulDeletions,
		); err != nil {
			c.logger.Error("failed to delete network isolation", zap.Error(err))
		}
	}

	// Handle network updates (delete then re-add).
	for _, network := range networkUpdates {
		c.logger.Info("detected network configuration change, updating", network.LoggingPairs()...)
		// Remove old version from the slice.
		for i, n := range c.networks {
			if n.Name == network.Name {
				c.networks = append(c.networks[:i], c.networks[i+1:]...)
				break
			}
		}
		c.deleteNetwork(network)
		c.addNetwork(network)
	}

	// Handle network additions.
	for _, network := range networkAdditions {
		c.addNetwork(network)
		successfulAdditions = append(successfulAdditions, network)
	}

	// Update isolation if we had any additions or updates.
	if len(successfulAdditions) > 0 || len(networkUpdates) > 0 {
		if err := c.networkManager.Firewall.CreateIsolation(c.networks); err != nil {
			c.logger.Error("failed to ensure network isolation", zap.Error(err))
		}
	}
}

// addNetwork sets up a new network on the client, including subnet
// initialization, firewall configuration, and starting the heartbeating
// and subnet watching processes.
func (c *Client) addNetwork(network *types.Network) {

	subnet, err := c.setupNetwork(network)
	if err != nil {
		c.logger.Error("failed to setup new network",
			append(network.LoggingPairs(), zap.Error(err))...,
		)
	} else {

		// Explicitly set up routing for all remote subnets that already exist in
		// the store. This is necessary to avoid a race on startup where the watch
		// goroutine delivers initial state asynchronously, but traffic may be
		// attempted before it has had a chance to run.
		c.syncRemoteSubnets(network, subnet)

		// Setup and start the heartbeater for the new subnet. Once started,
		// store it in the client's map of subnet heartbeaters, so it can be
		// managed when needed.
		hb := &heartbeater{
			logger:     c.logger,
			store:      c.store,
			subnet:     subnet,
			shutdownCh: c.shutdownCh,
			stopCh:     make(chan struct{}),
			stopWGFn:   c.shutdownGroupDecrement,
		}

		c.shutdownGroup.Add(1)
		go hb.start()

		c.subnetHeartbeatersLock.Lock()
		c.subnetHeartbeaters[network.Name] = hb
		c.subnetHeartbeatersLock.Unlock()

		// Setup and start the subnet watcher for the new network. Once started,
		// store it in the client's map of subnet watchers, so it can be managed
		// when needed.
		sw := &subnetWatcher{
			logger:         c.logger,
			store:          c.store,
			cID:            c.getID(),
			network:        network,
			networkManager: c.networkManager,
			localSubnet:    subnet,
			shutdownCh:     c.shutdownCh,
			stopCh:         make(chan struct{}),
			stopWGFn:       c.shutdownGroupDecrement,
		}

		c.shutdownGroup.Add(1)
		sw.start()

		c.subnetWatchersLock.Lock()
		c.subnetWatchers[network.Name] = sw
		c.subnetWatchersLock.Unlock()

		c.networks = append(c.networks, network)

		c.logger.Info("successfully set up new network", network.LoggingPairs()...)
	}
}

// syncRemoteSubnets lists all currently known remote subnets for the given
// network and synchronously installs their routes on this host. This is called
// during network initialisation to avoid a startup race: the watch goroutine
// delivers existing state asynchronously, so without this explicit sync there
// is a window between Start returning and routes being in place.
func (c *Client) syncRemoteSubnets(network *types.Network, localSubnet *types.Subnet) {

	listResp, err := c.store.ListSubnets(&types.StoreListSubnetsReq{Network: network.Name})
	if err != nil {
		c.logger.Error("failed to list remote subnets for initial sync",
			append(network.LoggingPairs(), zap.Error(err))...,
		)
		return
	}

	var localSubnets []*types.Subnet
	if localSubnet != nil {
		localSubnets = []*types.Subnet{localSubnet}
	}

	clientID := c.getID()

	for _, subnet := range listResp.Subnets {
		if subnet.ClientID == clientID || subnet.Expired {
			continue
		}

		c.logger.Debug("setting up remote subnet during network initialization",
			subnet.LoggingPairs()...)

		_, err := c.networkManager.SetRemote(&types.NetworkProviderSetRemoteReq{
			Subnet:       subnet,
			LocalSubnets: localSubnets,
		})
		if err != nil {
			c.logger.Error("failed to set up remote subnet during initialization",
				append(subnet.LoggingPairs(), zap.Error(err))...,
			)
		}
	}
}

// setupNetwork performs the complete setup for a new network, including subnet
// initialization and firewall configuration.
func (c *Client) setupNetwork(networkConfig *types.Network) (*types.Subnet, error) {

	clientSubnetResp, err := c.store.GetSubnet(&types.StoreGetSubnetReq{
		ID:          c.id.Load().(string),
		NetworkName: networkConfig.Name,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get client subnet config: %w", err)
	}

	subnet := clientSubnetResp.Subnet

	// If there is no record of a subnet for this client, we need to generate
	// a new one. Otherwise, we can use the existing one.
	if subnet == nil {

		// We need to list all existing subnets for this network, so we can
		// avoid IP conflicts when generating a new subnet.
		subnetListResp, err := c.store.ListSubnets(
			&types.StoreListSubnetsReq{
				Network: networkConfig.Name,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to list existing client subnets: %w", err)
		}

		subnet, err = c.networkManager.GenerateIPv4Subnet(c.getID(), networkConfig, subnetListResp.Subnets)
		if err != nil {
			return nil, fmt.Errorf("failed to generate IPv4 subnet: %w", err)
		}
	}

	c.logger.Info("initializing local host subnet", networkConfig.LoggingPairs()...)

	if err := c.initNetworkSubnet(networkConfig, subnet); err != nil {
		return nil, fmt.Errorf("failed to initialize subnet: %w", err)
	}

	if networkConfig.IPMasq != nil && *networkConfig.IPMasq {
		if err := c.networkManager.Firewall.CreateMasqRules(networkConfig, subnet); err != nil {
			return nil, fmt.Errorf("failed to set up firewall masquerade rules: %w", err)
		}
	}

	if err := c.networkManager.Firewall.CreateForwardRules(networkConfig); err != nil {
		return nil, fmt.Errorf("failed to set up firewall forward rules: %w", err)
	}

	c.logger.Info("successfully initialized local host subnet", subnet.LoggingPairs()...)
	return subnet, nil
}

func (c *Client) initNetworkSubnet(netCfg *types.Network, cfg *types.Subnet) error {

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

func (c *Client) deleteNetwork(network *types.Network) {

	networkPairs := network.LoggingPairs()

	clientSubnetResp, err := c.store.GetSubnet(&types.StoreGetSubnetReq{
		ID:          c.id.Load().(string),
		NetworkName: network.Name,
	})

	switch err {
	case nil:

		// If there is no subnet for this network, we can skip the resource
		// cleanup as there won't be any resources to clean up. This can happen
		// if the network was added but the subnet initialization failed, so we
		// never got to the point of creating the subnet record in the store.
		if clientSubnetResp.Subnet == nil {
			c.logger.Warn("no subnet found for network, skipping resource cleanup", networkPairs...)
			break
		}
		subnetPairs := clientSubnetResp.Subnet.LoggingPairs()
		c.logger.Debug("deleting local host subnet", subnetPairs...)

		// Remove firewall rules first, so that any traffic to/from the
		// subnet is blocked before we remove the network interfaces.
		if network.IPMasq != nil && *network.IPMasq {
			if err := c.networkManager.Firewall.DeleteMasqRules(
				network,
				clientSubnetResp.Subnet,
			); err != nil {
				c.logger.Error("failed to remove firewall masquerade rules",
					append(subnetPairs, zap.Error(err))...,
				)
			}
		}

		if err := c.networkManager.Firewall.DeleteForwardRules(network); err != nil {
			c.logger.Error("failed to remove firewall forward rules",
				append(subnetPairs, zap.Error(err))...,
			)
		}

		// Delete the local subnet networking.
		if _, err := c.networkManager.DeleteLocal(&types.NetworkProviderDeleteLocalReq{
			Subnet: clientSubnetResp.Subnet,
		}); err != nil {
			c.logger.Error("failed to delete local subnet networking",
				append(subnetPairs, zap.Error(err))...,
			)
		}

	default:
		c.logger.Error("failed to get subnet", append(networkPairs, zap.Error(err))...)
	}

	// The CNI configuration file deletion does not require having the subnet
	// information, so we can try and delete it. The function does not return an
	// error if the file does not exist, so this is safe to do and tidy from an
	// operational perspective.
	if err := c.cniStore.Delete(network.Name); err != nil {
		c.logger.Error("failed to delete CNI config", append(networkPairs, zap.Error(err))...)
	}

	// Even if we fail to get the subnet, we still attempt to stop and clear the
	// heartbeater and watcher, as they may still be running.
	c.subnetHeartbeatersLock.Lock()
	if hb, exists := c.subnetHeartbeaters[network.Name]; exists {
		hb.stop()
		delete(c.subnetHeartbeaters, network.Name)
	}
	c.subnetHeartbeatersLock.Unlock()

	c.subnetWatchersLock.Lock()
	if watcher, exists := c.subnetWatchers[network.Name]; exists {
		watcher.stop()
		delete(c.subnetWatchers, network.Name)
	}
	c.subnetWatchersLock.Unlock()

	c.logger.Info("successfully deleted network", network.LoggingPairs()...)
}
