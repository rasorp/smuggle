package server

import (
	"time"

	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/types"
)

func (s *Server) startNetworkReaper() {
	s.shutdownGroup.Add(1)
	defer s.shutdownGroup.Done()

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
		s.runSubnetReap(network)
	}
}

func (s *Server) runSubnetReap(net *types.Network) {
	s.logger.Info("running subnet reaper", zap.String("network", net.Name))

	req := types.StoreListSubnetsReq{Network: net.Name}

	subnetsResp, err := s.store.ListSubnets(&req)
	if err != nil {
		s.logger.Error("failed to list subnets for reaping",
			zap.String("network", net.Name),
			zap.Error(err),
		)
		return
	}

	s.logger.Info("successfully listed subnets for reaping",
		zap.String("network", net.Name),
		zap.Int("num", len(subnetsResp.Subnets)),
	)

	now := time.Now()

	for _, subnet := range subnetsResp.Subnets {

		//
		if subnet.Expired && subnet.Expiration.Add(s.cfg.Reaper.Threshold).Before(now) {
			s.handleSubnetExpired(subnet)
			continue
		}

		if subnet.Expiration.Before(now) {
			s.handleSubnetExpiration(subnet)
			continue
		}
	}
}

func (s *Server) handleSubnetExpired(subnet *types.Subnet) {

	req := types.StoreDeleteSubnetReq{
		ID:          subnet.ClientID,
		NetworkName: subnet.NetworkName,
	}

	// Delete the subnet from the store. If it fails, we can retry on the next
	// run of the reaper. At this point, the subnet is already marked as expired
	// and has been removed from cluster host routing, so this is just cleanup.
	_, err := s.store.DeleteSubnet(&req)
	if err != nil {
		s.logger.Error("failed to delete expired subnet",
			append(subnet.LoggingPairs(), zap.Error(err))...,
		)
	} else {
		s.logger.Info("successfully deleted expired subnet", subnet.LoggingPairs()...)
	}
}

func (s *Server) handleSubnetExpiration(subnet *types.Subnet) {

	subnet.Expired = true

	// Mark the subnet as expired in the store. It would be possible to retry
	// this call until it succeeds, but if the store is having availability
	// issues, we don't want to overload it. The reaper will attempt to mark it
	// again on the next run.
	_, err := s.store.SetSubnet(&types.StoreSetSubnetReq{Subnet: subnet})
	if err != nil {
		s.logger.Error("failed to mark subnet as expired",
			append(subnet.LoggingPairs(), zap.Error(err))...,
		)
	} else {
		s.logger.Info("successfully marked subnet as expired", subnet.LoggingPairs()...)
	}
}
