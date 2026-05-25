package server

import (
	"time"

	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/types"
)

func (s *Server) startNetworkReaper() {
	s.shutdownGroup.Add(1)
	defer s.shutdownGroup.Done()

	s.logger.Info("starting down network reaper")

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

	// We may log more than one message, so caputre the pairs here to avoid
	// multiple calls to the function and slice allocations.
	logPairs := net.LoggingPairs()

	s.logger.Info("running network subnet reaper", logPairs...)

	req := types.StoreListSubnetsReq{Network: net.Name}

	subnetsResp, err := s.store.ListSubnets(&req)
	if err != nil {
		s.logger.Error("failed to list network subnets for reaping",
			append(logPairs, zap.Error(err))...)
		return
	}

	s.logger.Info("successfully listed network subnets for reaping",
		append(logPairs, zap.Int("subnet_num", len(subnetsResp.Subnets)))...)

	now := time.Now()

	for _, subnet := range subnetsResp.Subnets {

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

	// We may log more than one message, so caputre the pairs here to avoid
	// multiple calls to the function and slice allocations.
	logPairs := subnet.LoggingPairs()

	s.logger.Debug("deleting expired subnet", logPairs...)

	req := types.StoreDeleteSubnetReq{
		ID:          subnet.ClientID,
		NetworkName: subnet.NetworkName,
	}

	// Delete the subnet from the store. If it fails, we can retry on the next
	// run of the reaper. At this point, the subnet is already marked as expired
	// and has been removed from cluster host routing, so this is just cleanup.
	_, err := s.store.DeleteSubnet(&req)
	if err != nil {
		s.logger.Error("failed to delete expired subnet", append(logPairs, zap.Error(err))...)
	} else {
		s.logger.Info("successfully deleted expired subnet", logPairs...)
	}
}

func (s *Server) handleSubnetExpiration(subnet *types.Subnet) {

	// We may log more than one message, so caputre the pairs here to avoid
	// multiple calls to the function and slice allocations.
	logPairs := subnet.LoggingPairs()

	s.logger.Debug("marking subnet as expired", logPairs...)

	subnet.Expired = true

	// Mark the subnet as expired in the store. It would be possible to retry
	// this call until it succeeds, but if the store is having availability
	// issues, we don't want to overload it. The reaper will attempt to mark it
	// again on the next run.
	_, err := s.store.SetSubnet(&types.StoreSetSubnetReq{Subnet: subnet})
	if err != nil {
		s.logger.Error("failed to mark subnet as expired", append(logPairs, zap.Error(err))...)
	} else {
		s.logger.Info("successfully marked subnet as expired", logPairs...)
	}
}
