package rpc

import (
	"context"
	"sync"

	"github.com/rasorp/smuggle/internal/types"
)

// subnetCacheKey is a composite key used to index subnets in the cache.
type subnetCacheKey struct {
	networkName string
	clientID    string
}

// subnetCache is a thread-safe in-memory snapshot of subnet state. It tracks a
// monotonically increasing index that advances only when a write is confirmed
// by the backing store, and broadcasts to all blocked callers via a
// replace-on-close notify channel, enabling efficient blocking.
type subnetCache struct {

	// mu is used to protect all fields.
	mu sync.RWMutex

	index uint64
	data  map[subnetCacheKey]*types.Subnet

	// notifyCh is closed when the index is advanced, then replaced.
	notifyCh chan struct{}
}

func newSubnetCache() *subnetCache {
	return &subnetCache{
		data:     make(map[subnetCacheKey]*types.Subnet),
		notifyCh: make(chan struct{}),
	}
}

// load stores a copy of subnet and advances the index to the subnet's
// ModifyIndex if it is greater than the current value. It is intended for use
// during initialization; there are no callers of waitForChange at this point
// so any channel close is a no-op. A copy is stored for consistency with set
// and to ensure the cache owns its entries independently of the backing store.
func (c *subnetCache) load(subnet *types.Subnet) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[subnetCacheKey{subnet.NetworkName, subnet.ClientID}] = subnet.Copy()
	c.advanceIndexLocked(subnet.ModifyIndex)
}

// syncModifyIndex updates the stored subnet's ModifyIndex and advances the
// cache index to modifyIndex if it is greater than the current value, notifying
// any waiters. It is called by the write buffer after a successful
// backing-store write to align the cache index with the durable backend-store
// index.
func (c *subnetCache) syncModifyIndex(networkName, clientID string, modifyIndex uint64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := subnetCacheKey{networkName, clientID}
	if s, ok := c.data[key]; ok {
		s.ModifyIndex = modifyIndex
	}
	c.advanceIndexLocked(modifyIndex)
}

// set stores a copy of subnet in the cache. The index is not advanced and
// waiters are not notified; that happens via syncModifyIndex once the write
// has been confirmed by the backing store. A copy is stored so the cache
// owns its entries and callers cannot mutate them through a retained pointer.
func (c *subnetCache) set(subnet *types.Subnet) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data[subnetCacheKey{subnet.NetworkName, subnet.ClientID}] = subnet.Copy()
}

// delete removes a subnet from the cache. The index is not advanced and
// waiters are not notified; that happens via syncModifyIndex once the delete
// has been confirmed by the backing store.
func (c *subnetCache) delete(networkName, clientID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.data, subnetCacheKey{networkName, clientID})
}

// get returns a copy of the subnet for the given network and client, or nil
// if not found. A copy is returned so callers cannot mutate cached state.
func (c *subnetCache) get(networkName, clientID string) *types.Subnet {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data[subnetCacheKey{networkName, clientID}].Copy()
}

// list returns copies of all subnets for a network and the current cache
// index. Copies are returned so callers cannot mutate cached state.
func (c *subnetCache) list(networkName string) (uint64, []*types.Subnet) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var subnets []*types.Subnet
	for k, s := range c.data {
		if k.networkName == networkName {
			subnets = append(subnets, s.Copy())
		}
	}
	return c.index, subnets
}

// waitForChange blocks until the cache index exceeds lastIndex or ctx is
// cancelled.
func (c *subnetCache) waitForChange(ctx context.Context, lastIndex uint64) error {
	for {
		c.mu.RLock()
		idx := c.index
		ch := c.notifyCh
		c.mu.RUnlock()

		if idx > lastIndex {
			return nil
		}

		select {
		case <-ch:
			// index may have changed; loop to re-check
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// advanceIndexLocked sets the index to v and broadcasts to all waiters if v
// exceeds the current value. Must be called with mu held.
func (c *subnetCache) advanceIndexLocked(v uint64) {
	if v > c.index {
		c.index = v
		old := c.notifyCh
		c.notifyCh = make(chan struct{})
		close(old)
	}
}
