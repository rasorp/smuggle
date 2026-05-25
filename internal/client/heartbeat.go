package client

import (
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/types"
)

// heartbeater is responsible for periodically updating the expiration time
// of a subnet in the store to indicate that the client is still active and
// using that subnet.
type heartbeater struct {
	logger *zap.Logger
	store  types.Store
	subnet *types.Subnet

	// shutdownCh is the client shutdown channel used to indicate that the agent
	// is shutting down and all long-running processes should exit. This is a
	// coarse-grained signal.
	shutdownCh chan struct{}

	// stopCh is used to signal to this specific heartbeater that it should
	// stop. This allows for more fine-grained control, such as when a subnet is
	// removed.
	stopCh chan struct{}

	// stopWGFn is a callback function that should be called when the
	// heartbeater has fully stopped. This allows for proper coordination of
	// shutdown processes across the client.
	stopWGFn func()

	// stopOnce ensures that the stop process is only initiated once, preventing
	// potential issues from multiple stop signals.
	stopOnce sync.Once
}

func (h *heartbeater) start() {

	// Calculate the heartbeat interval as half of the TTL to ensure we update
	// before expiration. This provides a safety margin.
	heartbeatInterval := types.DefaultSubnetTTL / 3

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	h.logger.Info("starting subnet heartbeat",
		append(h.subnet.LoggingPairs(), zap.String("interval", heartbeatInterval.String()))...,
	)

	// This is a small codebase currently and we known this cannot be nil. In
	// the future, if this code is refactored or reused in other contexts, we
	// may want to add some additional safety checks or validation.
	defer h.stopWGFn()

	for {
		select {
		case <-ticker.C:
			// Create a copy of the subnet config to update the expiration time
			// without modifying the original reference. Then write this update
			// back to the store.
			subnetCopy := h.subnet.Copy()
			subnetCopy.Expiration = time.Now().Add(types.DefaultSubnetTTL)

			_, err := h.store.SetSubnet(&types.StoreSetSubnetReq{Subnet: subnetCopy})

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

				h.subnet = subnetCopy

				h.logger.Debug("updated subnet expiration",
					zap.String("network", subnetCopy.NetworkName),
					zap.Time("new_expiration", subnetCopy.Expiration),
				)
			default:
				ticker.Reset(10 * time.Second)

				h.logger.Error("failed to update subnet expiration",
					zap.String("network", subnetCopy.NetworkName),
					zap.Error(err),
				)
			}
		case <-h.shutdownCh:
			h.logger.Info("shutting down subnet heartbeat", zap.String("network", h.subnet.NetworkName))
			return
		case <-h.stopCh:
			h.logger.Info("stopping subnet heartbeat", zap.String("network", h.subnet.NetworkName))
			return
		}
	}
}

func (h *heartbeater) stop() { h.stopOnce.Do(func() { close(h.stopCh) }) }
