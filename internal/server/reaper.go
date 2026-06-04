package server

import (
	"time"

	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/types"
)

func (s *Server) startNetworkReaper() {
	s.shutdownGroup.Add(1)
	defer s.shutdownGroup.Done()

	s.logger.Info("starting network reaper")

	// Perform an initial run of the reaper on startup, so we don't have to wait
	// for the first interval to elapse.
	s.networkReaper()

	ticker := time.NewTicker(s.cfg.Reaper.Interval)
	defer ticker.Stop()

	// Run the reaper at the configured interval until shutdown is signaled.
	// Errors are logged within the reaper run and are not terminal to the
	// server process. This means transient errors will be retried on the next
	// interval.
	for {
		select {
		case <-s.shutdownCh:
			s.logger.Info("shutting down network reaper")
			return
		case <-ticker.C:
			s.networkReaper()
			ticker.Reset(s.cfg.Reaper.Interval)
		}
	}
}

func (s *Server) networkReaper() {

	networks, err := s.store.ListNetworks(&types.StoreGetNetworksReq{})
	if err != nil {
		s.logger.Error("failed to list networks", zap.Error(err))
		return
	}

	for _, network := range networks.Networks {
		s.reapNetworkSubnets(network)
	}
}

func (s *Server) reapNetworkSubnets(net *types.Network) {

	// We may log more than one message, so capture the pairs here to avoid
	// multiple calls to the function and slice allocations.
	logPairs := net.LoggingPairs()

	s.logger.Info("running network subnet reaper", logPairs...)

	subnetsResp := s.handlers.ListSubnets(net.Name)

	s.logger.Info("successfully listed network subnets for reaping",
		append(logPairs, zap.Int("subnet_num", len(subnetsResp)))...)

	now := time.Now()

	for _, subnet := range subnetsResp {

		// If the subnet is already marked as expired, check if it has been
		// expired for long enough to be deleted.
		if subnet.Expired {
			if subnet.Expiration.Add(s.cfg.Reaper.Threshold).Before(now) {
				s.handleSubnetExpired(subnet)
			}
			continue
		}

		// If the subnet is not marked as expired, but its expiration time has
		// passed, mark it as expired.
		if subnet.Expiration.Before(now) {
			s.handleSubnetExpiration(subnet)
			continue
		}
	}
}

func (s *Server) handleSubnetExpired(subnet *types.Subnet) {

	// We may log more than one message, so capture the pairs here to avoid
	// multiple calls to the function and slice allocations.
	logPairs := subnet.LoggingPairs()

	s.logger.Debug("deleting expired subnet", logPairs...)

	s.handlers.DeleteSubnet(subnet.NetworkName, subnet.ClientID)
	s.logger.Info("enqueued expired subnet for deletion", logPairs...)
}

func (s *Server) handleSubnetExpiration(subnet *types.Subnet) {

	// We may log more than one message, so capture the pairs here to avoid
	// multiple calls to the function and slice allocations.
	logPairs := subnet.LoggingPairs()

	s.logger.Debug("marking subnet as expired", logPairs...)

	subnet.Expired = true

	// Update the cache immediately so watching clients see the expiration on
	// the next poll. The backing store write is handled asynchronously by the
	// write buffer.
	s.handlers.SetSubnet(subnet)
	s.logger.Info("enqueued subnet expiration", logPairs...)
}
