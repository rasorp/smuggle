package client

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/types"
)

// startSubnetUpdateHandler starts one subnet-watcher goroutine per currently
// configured network. It should only be called once during the client start;
// all subsequent network additions are handled by the reconciler.
func (c *Client) startSubnetUpdateHandler() error {
	for _, network := range c.networks {
		wg := c.networkWGFor(network.Name)
		if err := c.startNetworkWatcher(network, wg); err != nil {
			return err
		}
	}
	return nil
}

// startNetworkWatcher opens a watch stream for the given network's subnets and
// launches a goroutine to process the events. The goroutine exits when either
// the per-network stop channel (stored in c.watcherStopChs) or the global
// shutdownCh is closed.
func (c *Client) startNetworkWatcher(network *types.Network, wg *sync.WaitGroup) error {

	c.logger.Info("starting network subnet watcher", network.LoggingPairs()...)

	req := types.StoreWatchSubnetsReq{NetworkName: network.Name}

	resp, err := c.server.WatchSubnets(context.Background(), &req)
	if err != nil {
		return err
	}

	wg.Add(1)
	go c.subnetUpdateHandlerImpl(resp, c.newWatcherStopCh(network.Name), wg)
	return nil
}

func (c *Client) subnetUpdateHandlerImpl(resp *types.StoreWatchSubnetsResp, stopCh <-chan struct{}, wg *sync.WaitGroup) {
	c.shutdownGroup.Add(1)
	defer c.shutdownGroup.Done()
	defer wg.Done()

	for {
		select {
		case err := <-resp.ErrorCh:
			c.logger.Error("error received from subnet watcher", zap.Error(err))
		case set := <-resp.ModifyCh:
			c.handleSubnetSet(set)
		case del := <-resp.DeleteCh:
			c.handleSubnetDelete(del)
		case <-stopCh:
			c.logger.Info("stopping subnet update handler")
			return
		case <-c.shutdownCh:
			c.logger.Info("shutting down subnet update handler")
			return
		}
	}
}

func (c *Client) handleSubnetDelete(subnets []*types.Subnet) {
	for _, subnet := range subnets {

		// We may log more than one message, so capture the pairs here to avoid
		// multiple calls to the function and slice allocations.
		logPairs := subnet.LoggingPairs()

		// If the agent receives a deletion for its own subnet, the cluster
		// state is likely compromised. Local subnets are not tracked via the
		// remote update path so there is nothing to remove.
		if subnet.ClientID == c.getID() {
			c.logger.Warn("received subnet deletion for local client; skipping", logPairs...)
			continue
		}

		c.logger.Debug("deleting remote network subnet", logPairs...)

		_, err := c.networkManager.DeleteRemote(&types.NetworkProviderDeleteRemoteReq{
			Subnet:       subnet,
			LocalSubnets: c.localSubnets(),
		})
		if err != nil {
			c.logger.Error("failed to delete remote network subnet",
				append(logPairs, zap.Error(err))...,
			)
		} else {
			c.logger.Info("successfully deleted remote network subnet", logPairs...)
		}
	}
}

func (c *Client) handleSubnetSet(subnets []*types.Subnet) {
	for _, subnet := range subnets {

		// If the subnet belongs to this host, we do not need to perform the
		// remote set operation. If we did, it would break the local host subnet
		// routing.
		if subnet.ClientID == c.getID() {
			continue
		}

		// We may log more than one message, so capture the pairs here to avoid
		// multiple calls to the function and slice allocations.
		logPairs := subnet.LoggingPairs()

		c.logger.Debug("setting up remote network subnet", logPairs...)

		_, err := c.networkManager.SetRemote(&types.NetworkProviderSetRemoteReq{
			Subnet:       subnet,
			LocalSubnets: c.localSubnets(),
		})
		if err != nil {
			c.logger.Error("failed to set up remote network subnet",
				append(logPairs, zap.Error(err))...,
			)
		} else {
			c.logger.Info("successfully set up remote network subnet", logPairs...)
		}
	}
}
