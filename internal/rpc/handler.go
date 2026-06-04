package rpc

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"go.uber.org/zap"

	smugglestore "github.com/rasorp/smuggle/internal/store"
	"github.com/rasorp/smuggle/internal/types"
)

const (
	// defaultWatchJitter is the maximum random delay added before responding
	// to an unblocked Watch call. Spreading responses staggers the data
	// transfers from a fleet of simultaneously-unblocked clients, preventing
	// a thundering herd after a topology change.
	defaultWatchJitter = 20 * time.Second
)

// handlerState holds the shared dependencies for both RPC service handlers.
type handlerState struct {
	backingStore smugglestore.BackingStore
	cache        *subnetCache
	buffer       *writeBuffer
	jitter       time.Duration
	logger       *zap.Logger

	// stopCtx is cancelled when the server is shutting down. It is used as
	// the parent context for Watch calls so that in-flight blockers are
	// interrupted promptly rather than waiting out their full MaxWait timeout.
	stopCtx context.Context
}

// NetworkHandler is registered as the "Network" RPC service.
type NetworkHandler struct{ s *handlerState }

// SubnetHandler is registered as the "Subnet" RPC service.
type SubnetHandler struct{ s *handlerState }

// Handlers groups both RPC service handlers produced by NewHandlers.
type Handlers struct {
	Network *NetworkHandler
	Subnet  *SubnetHandler
}

// HandlerReq holds the dependencies for creating new Handlers.
type HandlerReq struct {
	Store smugglestore.BackingStore

	// WriteRateLimit is the maximum number of writes per second forwarded to
	// the backing store. Zero means unlimited.
	WriteRateLimit int

	// WriteBurst is the token-bucket burst size for the write rate limiter.
	WriteBurst int

	// StopCh is closed when the server is shutting down. It is used by the
	// write buffer to drain and stop its background goroutines cleanly, and
	// to cancel the stop context that interrupts in-flight Watch goroutines.
	StopCh chan struct{}

	Logger *zap.Logger
}

// NewHandlers creates Handlers and populates the cache from the backing store.
// The RPC server must not start accepting connections until this call returns.
func NewHandlers(req *HandlerReq) (*Handlers, error) {
	// Derive a context that is cancelled when stopCh fires. Watch calls use it
	// as their parent so shutdown interrupts blocked goroutines immediately.
	stopCtx, stopCancel := context.WithCancel(context.Background())
	go func() {
		<-req.StopCh
		stopCancel()
	}()

	s := &handlerState{
		backingStore: req.Store,
		cache:        newSubnetCache(),
		jitter:       defaultWatchJitter,
		logger:       req.Logger,
		stopCtx:      stopCtx,
	}

	s.buffer = newWriteBuffer(
		req.Store,
		req.WriteRateLimit,
		req.WriteBurst,
		req.StopCh,
		req.Logger,
		s.cache.syncModifyIndex,
	)

	h := &Handlers{
		Network: &NetworkHandler{s: s},
		Subnet:  &SubnetHandler{s: s},
	}

	if err := h.init(s); err != nil {
		return nil, fmt.Errorf("failed to initialize RPC handler cache: %w", err)
	}

	return h, nil
}

// init seeds the cache from the backing store before the server begins
// accepting requests.
func (h *Handlers) init(s *handlerState) error {
	networksResp, err := s.backingStore.ListNetworks(&types.StoreGetNetworksReq{})
	if err != nil {
		return fmt.Errorf("failed to list networks: %w", err)
	}

	for _, network := range networksResp.Networks {
		subnetsResp, err := s.backingStore.ListSubnets(&types.StoreListSubnetsReq{Network: network.Name})
		if err != nil {
			return fmt.Errorf("failed to list subnets: %w", err)
		}
		for _, subnet := range subnetsResp.Subnets {
			s.cache.load(subnet)
		}
	}

	return nil
}

// List returns all network configurations. Networks are operator-defined
// configuration, not client state, so they are read through to the store
// rather than cached.
func (h *NetworkHandler) List(_ *NetworkListArgs, reply *NetworkListReply) error {
	resp, err := h.s.backingStore.ListNetworks(&types.StoreGetNetworksReq{})
	if err != nil {
		return err
	}
	reply.Networks = resp.Networks
	return nil
}

// List returns all subnets for a network from the in-memory cache.
func (h *SubnetHandler) List(args *SubnetListArgs, reply *SubnetListReply) error {
	_, subnets := h.s.cache.list(args.NetworkName)
	reply.Subnets = subnets
	return nil
}

// Get returns a single subnet from the in-memory cache, or a nil Subnet when
// not found.
func (h *SubnetHandler) Get(args *SubnetGetArgs, reply *SubnetGetReply) error {
	reply.Subnet = h.s.cache.get(args.NetworkName, args.ID)
	return nil
}

// Set updates the cache immediately so subsequent reads reflect the change,
// then enqueues a rate-limited write to the backing store. Watchers are not
// notified until the write is confirmed.
func (h *SubnetHandler) Set(args *SubnetSetArgs, reply *SubnetSetReply) error {
	if args.Subnet == nil {
		return errors.New("subnet must not be nil")
	}
	h.s.cache.set(args.Subnet)
	h.s.buffer.submitSet(args.Subnet)
	return nil
}

// Delete removes the subnet from the cache immediately and enqueues a
// rate-limited delete to the backing store. Watchers are not notified until
// the delete is confirmed.
func (h *SubnetHandler) Delete(args *SubnetDeleteArgs, _ *SubnetDeleteReply) error {
	h.s.cache.delete(args.NetworkName, args.ID)
	h.s.buffer.submitDelete(args.NetworkName, args.ID)
	return nil
}

// Watch is a blocking query. It holds the request until the cache index
// advances past WaitIndex or MaxWait elapses, then adds a random jitter delay
// (up to MaxJitter) before responding. The jitter staggers when a fleet of
// simultaneously-waiting clients receive their response after a topology
// change, preventing a thundering herd of simultaneous data transfers.
func (h *SubnetHandler) Watch(args *SubnetWatchArgs, reply *SubnetWatchReply) error {

	// Cap MaxWait to the server default regardless of what the client sends.
	// An uncapped value would allow a single caller to hold a server goroutine
	// open indefinitely.
	maxWait := args.MaxWait
	if maxWait <= 0 || maxWait > defaultWatchMaxWait {
		maxWait = defaultWatchMaxWait
	}

	ctx, cancel := context.WithTimeout(h.s.stopCtx, maxWait)
	defer cancel()

	err := h.s.cache.waitForChange(ctx, args.WaitIndex)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	// Only jitter when there was an actual change (not a timeout): we want to
	// spread clients that all unblock simultaneously, but not delay timeouts
	// that are already staggered by MaxWait.
	if h.s.jitter > 0 && err == nil {
		t := time.NewTimer(time.Duration(rand.Int63n(int64(h.s.jitter))))
		defer t.Stop()
		select {
		case <-t.C:
		case <-h.s.stopCtx.Done():
		}
	}

	idx, subnets := h.s.cache.list(args.NetworkName)
	reply.Index = idx
	reply.Subnets = subnets
	return nil
}

// ListSubnets returns all subnets for a network from the in-memory cache.
// This is the read path for server-side consumers such as the reaper.
func (h *Handlers) ListSubnets(networkName string) []*types.Subnet {
	_, subnets := h.Subnet.s.cache.list(networkName)
	return subnets
}

// SetSubnet updates the cache immediately and enqueues a rate-limited write to
// the backing store. Server-side consumers such as the reaper must use this
// method rather than writing to the backing store directly so that the cache
// stays consistent. Watchers are notified once the write is confirmed.
func (h *Handlers) SetSubnet(subnet *types.Subnet) {
	h.Subnet.s.cache.set(subnet)
	h.Subnet.s.buffer.submitSet(subnet)
}

// DeleteSubnet removes the subnet from the cache immediately and enqueues a
// rate-limited delete to the backing store. Server-side consumers such as the
// reaper must use this method rather than deleting from the backing store
// directly so that the cache stays consistent. Watchers are notified once the
// delete is confirmed.
func (h *Handlers) DeleteSubnet(networkName, clientID string) {
	h.Subnet.s.cache.delete(networkName, clientID)
	h.Subnet.s.buffer.submitDelete(networkName, clientID)
}
