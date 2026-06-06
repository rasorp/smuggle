package client

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/types"
)

// startHeartbeaters starts a heartbeat goroutine for each subnet that is
// currently configured.
func (c *Client) startHeartbeaters() {
	for subnet := range c.subnets.Items() {
		wg := c.networkWGFor(subnet.NetworkName)
		wg.Add(1)
		stopCh := c.newSubnetStopCh(subnet.NetworkName)
		go c.startSubnetHeartbeat(subnet, stopCh, wg)
	}
}

// startSubnetHeartbeat runs the heartbeat loop for a single subnet. It
// refreshes the subnet's expiration at a fraction of the TTL to ensure it has
// a safety margin before expiration.
func (c *Client) startSubnetHeartbeat(subnet *types.Subnet, stopCh <-chan struct{}, wg *sync.WaitGroup) {
	c.shutdownGroup.Add(1)
	defer c.shutdownGroup.Done()
	defer wg.Done()

	// Calculate the heartbeat interval as a third of the TTL to ensure we
	// refresh well before expiration, providing a safety margin against
	// transient failures.
	heartbeatInterval := types.DefaultSubnetTTL / 3

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	// We may log more than one message, so capture the pairs here to avoid
	// multiple calls to the function and slice allocations. All fields are
	// static.
	logPairs := subnet.LoggingPairs()

	c.logger.Info("starting subnet heartbeat",
		append(logPairs, zap.String("interval", heartbeatInterval.String()))...,
	)

	for {
		select {
		case <-ticker.C:
			// Create a copy of the subnet config to update the expiration time
			// without modifying the original reference. Then write this update
			// back to the server.
			subnetCopy := subnet.Copy()
			subnetCopy.Expiration = time.Now().Add(types.DefaultSubnetTTL)

			c.logger.Debug("updating subnet expiration",
				append(logPairs, zap.Time("expiration", subnetCopy.Expiration))...,
			)

			_, err := c.server.SetSubnet(&types.StoreSetSubnetReq{Subnet: subnetCopy})

			// Adjust the ticker interval based on success or failure. On
			// success, we maintain the regular interval. On failure, we shorten
			// the interval to retry sooner.
			//
			// TODO(jrasell): Consider implementing some form of backoff, so we
			// don't "hammer" the server on persistent failures that may take a
			// while to resolve.
			switch err {
			case nil:
				ticker.Reset(types.DefaultSubnetTTL / 3)

				c.logger.Info("successfully updated subnet expiration",
					append(logPairs, zap.Time("expiration", subnetCopy.Expiration))...,
				)
			default:
				ticker.Reset(10 * time.Second)

				c.logger.Error("failed to update subnet expiration",
					append(logPairs, zap.Error(err))...,
				)
			}

		case <-stopCh:
			c.logger.Info("stopping subnet heartbeat", logPairs...)
			return

		case <-c.shutdownCh:
			c.logger.Info("shutting down subnet heartbeat", logPairs...)
			return
		}
	}
}
