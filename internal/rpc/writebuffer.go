package rpc

import (
	"context"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/time/rate"

	smugglestore "github.com/rasorp/smuggle/internal/store"
	"github.com/rasorp/smuggle/internal/types"
)

const (
	// defaultMaxRetries is the number of times a failed write is retried
	// before it is treated as a dead-letter item and dropped.
	defaultMaxRetries = 10

	// defaultRetryBaseDelay is the initial backoff delay. Each subsequent
	// attempt doubles the delay up to defaultMaxRetryDelay.
	defaultRetryBaseDelay = 500 * time.Millisecond

	// defaultMaxRetryDelay caps the exponential backoff.
	defaultMaxRetryDelay = 30 * time.Second
)

// writeKey uniquely identifies a subnet by its network and client.
type writeKey struct {
	networkName string
	clientID    string
}

// pendingWrite holds the latest state for a single pending subnet store
// operation.
type pendingWrite struct {
	subnet *types.Subnet

	// deleted indicates whether this item represents a delete operation. When
	// true, subnet is ignored and a delete is issued for the key instead of a
	// set.
	deleted bool

	// retries counts how many consecutive backing-store failures have been
	// observed for this item so far.
	retries int
}

// writeBuffer coalesces SetSubnet and DeleteSubnet calls and forwards them to
// the backing store at a rate-limited pace. Multiple updates to the same subnet
// before a flush collapse into a single write, ensuring the most recent value
// is always what reaches the store.
type writeBuffer struct {
	store   smugglestore.BackingStore
	limiter *rate.Limiter
	logger  *zap.Logger

	// onWriteComplete is called after every successful backing-store write, so
	// that the backing-store index can be propagated to the caller's in-memory
	// cache.
	onWriteComplete func(networkName, clientID string, modifyIndex uint64)

	maxRetries     int
	retryBaseDelay time.Duration
	maxRetryDelay  time.Duration

	mu      sync.Mutex
	pending map[writeKey]*pendingWrite

	// signal is poked (non-blocking) whenever a new item enters pending.
	signal chan struct{}

	stopCh chan struct{}
}

func newWriteBuffer(
	store smugglestore.BackingStore,
	ratePerSec, burst int,
	stopCh chan struct{},
	logger *zap.Logger,
	// onWriteComplete is called after every successful backing-store write with
	// the network name, client ID, and ModifyIndex of the written entry.
	// It must not be nil.
	onWriteComplete func(networkName, clientID string, modifyIndex uint64),
) *writeBuffer {
	wb := &writeBuffer{
		store:           store,
		logger:          logger,
		pending:         make(map[writeKey]*pendingWrite),
		signal:          make(chan struct{}, 1),
		stopCh:          stopCh,
		onWriteComplete: onWriteComplete,
		maxRetries:      defaultMaxRetries,
		retryBaseDelay:  defaultRetryBaseDelay,
		maxRetryDelay:   defaultMaxRetryDelay,
	}

	if ratePerSec > 0 {
		if burst <= 0 {
			burst = ratePerSec
		}
		wb.limiter = rate.NewLimiter(rate.Limit(ratePerSec), burst)
	}

	go wb.run()
	return wb
}

// submitSet enqueues a subnet write, coalescing with any existing pending entry
// for the same key.
func (wb *writeBuffer) submitSet(subnet *types.Subnet) {
	wb.mu.Lock()
	key := writeKey{networkName: subnet.NetworkName, clientID: subnet.ClientID}
	wb.pending[key] = &pendingWrite{subnet: subnet}
	wb.mu.Unlock()
	wb.poke()
}

// submitDelete enqueues a delete operation, superseding any pending set for the
// same key.
func (wb *writeBuffer) submitDelete(networkName, clientID string) {
	wb.mu.Lock()
	key := writeKey{networkName: networkName, clientID: clientID}
	wb.pending[key] = &pendingWrite{deleted: true}
	wb.mu.Unlock()
	wb.poke()
}

// poke signals the run loop that pending work is available without blocking.
func (wb *writeBuffer) poke() {
	select {
	case wb.signal <- struct{}{}:
	default:
	}
}

// run is the background flush goroutine. It drains the pending map one item at
// a time, waiting for a rate-limiter token before each write when a limit is
// configured.
func (wb *writeBuffer) run() {

	// Derive a single context for all rate-limiter waits. The goroutine below
	// cancels it when stopCh fires so a long limiter wait never blocks
	// shutdown.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		select {
		case <-wb.stopCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	for {
		select {
		case <-wb.signal:
		case <-wb.stopCh:
			wb.drain()
			return
		}

		for {
			wb.mu.Lock()
			if len(wb.pending) == 0 {
				wb.mu.Unlock()
				break
			}

			// Take one arbitrary item from the map.
			var key writeKey
			var item *pendingWrite
			for k, v := range wb.pending {
				key = k
				item = v
				break
			}
			delete(wb.pending, key)
			wb.mu.Unlock()

			if wb.limiter != nil {
				if err := wb.limiter.Wait(ctx); err != nil {
					// ctx was cancelled by shutdown. The item was already
					// removed from pending, so re-insert it before draining
					// to ensure it is not lost.
					wb.mu.Lock()
					if _, exists := wb.pending[key]; !exists {
						wb.pending[key] = item
					}
					wb.mu.Unlock()
					wb.drain()
					return
				}
			}

			wb.flush(key, item)
		}
	}
}

// drain flushes all pending writes without rate limiting. It is called on
// shutdown to ensure in-flight state reaches the backing store before the
// process exits. Rate limiting is intentionally skipped; scheduleRetry checks
// stopCh and will not re-queue items during shutdown.
func (wb *writeBuffer) drain() {
	wb.mu.Lock()
	items := wb.pending
	wb.pending = make(map[writeKey]*pendingWrite)
	wb.mu.Unlock()

	for key, item := range items {
		wb.flush(key, item)
	}
}

// flush writes a single pending item to the backing store by building the
// appropriate store call and passing it to doWrite.
func (wb *writeBuffer) flush(key writeKey, item *pendingWrite) {
	if item.deleted {
		wb.doWrite(key, item, "delete", func() (uint64, error) {
			resp, err := wb.store.DeleteSubnet(&types.StoreDeleteSubnetReq{
				ID:          key.clientID,
				NetworkName: key.networkName,
			})
			if err != nil {
				return 0, err
			}
			return resp.ModifyIndex, nil
		})
	} else {
		wb.doWrite(key, item, "set", func() (uint64, error) {
			resp, err := wb.store.SetSubnet(&types.StoreSetSubnetReq{Subnet: item.subnet})
			if err != nil {
				return 0, err
			}
			return resp.ModifyIndex, nil
		})
	}
}

// doWrite executes fn and calls onWriteComplete on success. On failure it logs
// the error and hands the item to scheduleRetry for exponential-backoff
// re-queuing.
func (wb *writeBuffer) doWrite(key writeKey, item *pendingWrite, op string, fn func() (uint64, error)) {
	modifyIndex, err := fn()
	if err != nil {
		wb.logger.Error("failed to perform backing store write",
			zap.String("operation", op),
			zap.String("network", key.networkName),
			zap.String("client_id", key.clientID),
			zap.Int("attempt", item.retries+1),
			zap.Error(err),
		)
		wb.scheduleRetry(key, item, op)
		return
	}
	wb.onWriteComplete(key.networkName, key.clientID, modifyIndex)
}

// scheduleRetry re-queues a failed write after an exponential backoff delay.
// If the item has already been retried maxRetries times it is treated as a
// dead-letter entry: the failure is logged and the item is discarded.
//
// Before re-inserting, scheduleRetry checks whether a newer write for the same
// key has already arrived in pending. If it has, the retry is superseded and
// silently dropped — the newer value will reach the store on its own flush.
func (wb *writeBuffer) scheduleRetry(key writeKey, item *pendingWrite, op string) {
	item.retries++

	if item.retries > wb.maxRetries {
		wb.logger.Error("backing store write exceeded max retries and has been dropped (dead letter)",
			zap.String("operation", op),
			zap.String("network", key.networkName),
			zap.String("client_id", key.clientID),
			zap.Int("max_retries", wb.maxRetries),
		)
		return
	}

	// Don't schedule retries during shutdown; the item has already been
	// through drain so re-queuing it would have no effect anyway.
	select {
	case <-wb.stopCh:
		wb.logger.Warn("dropping failed write due to shutdown",
			zap.String("operation", op),
			zap.String("network", key.networkName),
			zap.String("client_id", key.clientID),
		)
		return
	default:
	}

	// Exponential backoff: base * 2^(retries-1), capped at maxRetryDelay.
	delay := min(wb.retryBaseDelay*(1<<(item.retries-1)), wb.maxRetryDelay)

	wb.logger.Warn("scheduling retry for failed backing store write",
		zap.String("network", key.networkName),
		zap.String("client_id", key.clientID),
		zap.Int("attempt", item.retries),
		zap.Int("max_retries", wb.maxRetries),
		zap.Duration("backoff", delay),
	)

	go func() {
		t := time.NewTimer(delay)
		defer t.Stop()
		select {
		case <-t.C:
		case <-wb.stopCh:
			return
		}

		wb.mu.Lock()
		// Only re-insert if no newer write has arrived for this key in the
		// meantime. A newer write already carries the latest desired state, so
		// re-inserting the older item would be redundant at best and incorrect
		// at worst (e.g. a delete arriving after a set).
		if _, exists := wb.pending[key]; !exists {
			wb.pending[key] = item
		}
		wb.mu.Unlock()

		wb.poke()
	}()
}
