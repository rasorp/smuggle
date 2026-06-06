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

	// Build the set of currently-known network names.
	currentNames := make(map[string]struct{}, len(networks.Networks))
	for _, network := range networks.Networks {
		currentNames[network.Name] = struct{}{}
	}

	if s.knownNetworks == nil {
		// First run: populate the baseline without expiring anything. This
		// prevents a server restart from treating all existing networks as
		// newly deleted.
		s.knownNetworks = currentNames
	} else {
		// Subsequent runs: any name present in knownNetworks but absent from
		// the current set corresponds to a deleted network whose subnets must
		// be expired immediately.
		for name := range s.knownNetworks {
			if _, ok := currentNames[name]; !ok {
				s.expireNetworkSubnets(name)
			}
		}
		s.knownNetworks = currentNames
	}

	for _, network := range networks.Networks {
		s.reapNetworkSubnets(network)
	}
}

// expireNetworkSubnets immediately marks every subnet belonging to the named
// network as expired. It is called when the reaper detects that a network has
// been deleted from the store, so that watching clients receive expiry
// notifications promptly rather than waiting for the natural TTL to elapse.
func (s *Server) expireNetworkSubnets(networkName string) {
	subnets := s.handlers.ListSubnets(networkName)

	s.logger.Info("network deleted, expiring all subnets",
		zap.String("network_name", networkName),
		zap.Int("subnet_count", len(subnets)),
	)

	for _, subnet := range subnets {
		s.handleSubnetExpiration(subnet)
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
