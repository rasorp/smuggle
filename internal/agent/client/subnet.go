package client

import (
	"context"

	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/types"
)

func (c *Client) startSubnetUpdateHandler() error {

	for _, network := range c.networks {
		c.logger.Debug("starting subnet watcher for network", zap.String("network_name", network.Name))

		req := &types.StoreWatchSubnetsReq{
			Context:     context.Background(),
			NetworkName: network.Name,
		}

		resp, err := c.store.WatchSubnets(req)
		if err != nil {
			return err
		}

		go c.subnetUpdateHandlerImpl(resp)
	}
	return nil
}

func (c *Client) subnetUpdateHandlerImpl(req *types.StoreWatchSubnetsResp) {
	c.shutdownGroup.Add(1)
	defer c.shutdownGroup.Done()

	for {
		select {
		case err := <-req.ErrorCh:
			c.logger.Error("error received from subnet watcher", zap.Error(err))
		case set := <-req.ModifyCh:
			c.handleSubnetSet(set)
		case del := <-req.DeleteCh:
			c.handleSubnetDelete(del)
		case <-c.shutdownCh:
			c.logger.Info("shutting down subnet update handler")
			return
		}
	}
}

func (c *Client) handleSubnetDelete(subnets []*types.Subnet) {
	for _, subnet := range subnets {

		// If the agent has got an update about itself being expired, the
		// cluster stability is likely compromised. As the addition is not
		// hanled here, we simply skip the deletion attempt as it won't because
		// we don't add local subnets this way.
		if subnet.ClientID == c.getID() {
			c.logger.Warn("received subnet deletion for local client; skipping",
				subnet.LoggingPairs()...,
			)
			continue
		}

		c.logger.Debug("deleting remote subnet networking", subnet.LoggingPairs()...)

		_, err := c.networkManager.DeleteRemote(&types.NetworkProviderDeleteRemoteReq{Subnet: subnet})
		if err != nil {
			c.logger.Error("failed to delete remote subnet networking",
				append(subnet.LoggingPairs(), zap.Error(err))...,
			)
		} else {
			c.logger.Info("successfully deleted remote subnet networking", subnet.LoggingPairs()...)
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

		c.logger.Debug("setting up remote subnet networking", subnet.LoggingPairs()...)

		_, err := c.networkManager.SetRemote(&types.NetworkProviderSetRemoteReq{Subnet: subnet})
		if err != nil {
			c.logger.Error("failed to set up remote subnet networking",
				append(subnet.LoggingPairs(), zap.Error(err))...,
			)
		} else {
			c.logger.Info("successfully set up remote subnet networking", subnet.LoggingPairs()...)
		}
	}
}
