package client

import (
	"time"

	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/types"
)

func (c *Client) startHeartbeaters() {
	for _, subnet := range c.subnets {
		go c.startSubnetHeartbeat(subnet)
	}
}

func (c *Client) startSubnetHeartbeat(cfg *types.Subnet) {
	c.shutdownGroup.Add(1)
	defer c.shutdownGroup.Done()

	// Calculate the heartbeat interval as half of the TTL to ensure we update
	// before expiration. This provides a safety margin.
	heartbeatInterval := types.DefaultSubnetTTL / 3

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	c.logger.Info("starting subnet heartbeat",
		append(cfg.LoggingPairs(), zap.String("interval", heartbeatInterval.String()))...,
	)

	for {
		select {
		case <-ticker.C:
			// Create a copy of the subnet config to update the expiration time
			// without modifying the original reference. Then write this update
			// back to the store.
			cfgCopy := cfg.Copy()
			cfgCopy.Expiration = time.Now().Add(types.DefaultSubnetTTL)

			_, err := c.store.SetSubnet(&types.StoreSetSubnetReq{Subnet: cfg})

			// Adjust the ticker interval based on success or failure. On
			// success, we maintain the regular interval. On failure, we shorten
			// the interval to retry sooner.
			//
			// TODO(jrasell): Consider implementing some form of backoff, so we
			// don't "hammer" the store on persistent failures that may take a
			// while to resolve.
			switch err {
			case nil:
				ticker.Reset(types.DefaultSubnetTTL / 3)

				c.logger.Debug("updated subnet expiration",
					zap.String("network", cfg.NetworkName),
					zap.Time("new_expiration", cfg.Expiration),
				)
			default:
				ticker.Reset(10 * time.Second)

				c.logger.Error("failed to update subnet expiration",
					zap.String("network", cfg.NetworkName),
					zap.Error(err),
				)
			}
		case <-c.shutdownCh:
			c.logger.Info("shutting down subnet heartbeat", zap.String("network", cfg.NetworkName))
			return
		}
	}
}
