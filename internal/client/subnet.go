package client

import (
	"context"
	"sync"

	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/network"
	"github.com/rasorp/smuggle/internal/types"
)

type subnetWatcher struct {
	cID            string
	logger         *zap.Logger
	store          types.Store
	network        *types.Network
	networkManager *network.Manager

	// localSubnet is the local subnet allocated for this network on the
	// current host. It is used to populate LocalSubnets in SetRemote and
	// DeleteRemote calls for policy-based routing.
	localSubnet *types.Subnet

	// shutdownCh is the client shutdown channel used to indicate that the agent
	// is shutting down and all long-running processes should exit. This is a
	// coarse-grained signal.
	shutdownCh chan struct{}

	// stopCh is used to signal to this specific subnet watcher that it should
	// stop. This allows for more fine-grained control, such as when a subnet is
	// removed.
	stopCh chan struct{}

	// stopWGFn is a callback function that should be called when the
	// subnet watcher has fully stopped. This allows for proper coordination of
	// shutdown processes across the client.
	stopWGFn func()

	// stopOnce ensures that the stop process is only initiated once, preventing
	// potential issues from multiple stop signals.
	stopOnce sync.Once
}

func (s *subnetWatcher) start() {

	s.logger.Debug("starting remote subnet watcher", zap.String("network_name", s.network.Name))

	req := &types.StoreWatchSubnetsReq{
		NetworkName: s.network.Name,
	}

	// The watch subnets store call currently will only ever return a nil error,
	// so we can ignore it here. In the future, if the nvar store implementation
	// changes or a new store is added, we may need to handle errors here.
	resp, _ := s.store.WatchSubnets(context.Background(), req)
	go s.runHandler(resp)
}

func (s *subnetWatcher) runHandler(req *types.StoreWatchSubnetsResp) {

	// This is a small codebase currently and we known this cannot be nil. In
	// the future, if this code is refactored or reused in other contexts, we
	// may want to add some additional safety checks or validation.
	defer s.stopWGFn()

	for {
		select {
		case err := <-req.ErrorCh:
			s.logger.Error("error received from subnet watcher", zap.Error(err))
		case set := <-req.ModifyCh:
			s.handleSubnetSet(set)
		case del := <-req.DeleteCh:
			s.handleSubnetDelete(del)
		case <-s.shutdownCh:
			s.logger.Info("shutting down subnet update handler")
			return
		case <-s.stopCh:
			s.logger.Info("stopping subnet update handler")
			return
		}
	}
}

func (s *subnetWatcher) handleSubnetDelete(subnets []*types.Subnet) {
	for _, subnet := range subnets {

		// If the agent has got an update about itself being expired, the
		// cluster stability is likely compromised. As the addition is not
		// handled here, we simply skip the deletion attempt as it won't because
		// we don't add local subnets this way.
		if subnet.ClientID == s.cID {
			s.logger.Warn("received subnet deletion for local client; skipping",
				subnet.LoggingPairs()...,
			)
			continue
		}

		s.logger.Debug("deleting remote subnet networking", subnet.LoggingPairs()...)

		var localSubnets []*types.Subnet
		if s.localSubnet != nil {
			localSubnets = []*types.Subnet{s.localSubnet}
		}

		_, err := s.networkManager.DeleteRemote(&types.NetworkProviderDeleteRemoteReq{
			Subnet:       subnet,
			LocalSubnets: localSubnets,
		})
		if err != nil {
			s.logger.Error("failed to delete remote subnet networking",
				append(subnet.LoggingPairs(), zap.Error(err))...,
			)
		} else {
			s.logger.Info("successfully deleted remote subnet networking", subnet.LoggingPairs()...)
		}
	}
}

func (s *subnetWatcher) handleSubnetSet(subnets []*types.Subnet) {
	for _, subnet := range subnets {

		// If the subnet belongs to this host, we do not need to perform the
		// remote set operation. If we did, it would break the local host subnet
		// routing.
		if subnet.ClientID == s.cID {
			continue
		}

		s.logger.Debug("setting up remote subnet networking", subnet.LoggingPairs()...)

		var localSubnets []*types.Subnet
		if s.localSubnet != nil {
			localSubnets = []*types.Subnet{s.localSubnet}
		}

		_, err := s.networkManager.SetRemote(&types.NetworkProviderSetRemoteReq{
			Subnet:       subnet,
			LocalSubnets: localSubnets,
		})
		if err != nil {
			s.logger.Error("failed to set up remote subnet networking",
				append(subnet.LoggingPairs(), zap.Error(err))...,
			)
		} else {
			s.logger.Info("successfully set up remote subnet networking", subnet.LoggingPairs()...)
		}
	}
}

func (s *subnetWatcher) stop() { s.stopOnce.Do(func() { close(s.stopCh) }) }
