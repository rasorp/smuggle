package main

import (
	"errors"
	"fmt"
	"net"
	"sync"
)

// PeerEntry holds the state for a single registered customer peer.
type PeerEntry struct {
	PublicKey string
	TunnelIP  net.IP
}

// PeerRegistry is an in-memory store of customer WireGuard peers backed by a
// pre-computed pool of allocatable tunnel IPs. It is safe for concurrent use.
type PeerRegistry struct {
	mu        sync.RWMutex
	peers     map[string]*PeerEntry // keyed by base64 public key string
	pool      []net.IP              // ordered list of allocatable IPs
	allocated map[string]bool       // tunnel IP string -> in use
}

// NewPeerRegistry builds a PeerRegistry whose IP pool covers every host
// address in tunnelSubnet except the first (reserved for the gateway itself)
// and the broadcast address.
func NewPeerRegistry(tunnelSubnet *net.IPNet) *PeerRegistry {
	var pool []net.IP

	// Start from the first host address and skip it — that belongs to the
	// gateway interface. Customer IPs begin at the second host address.
	ip := make(net.IP, 4)
	copy(ip, tunnelSubnet.IP.To4())
	incrementIP(ip) // network addr -> gateway IP
	incrementIP(ip) // gateway IP  -> first customer IP

	for tunnelSubnet.Contains(ip) {
		if isBroadcast(ip, tunnelSubnet) {
			break
		}
		pool = append(pool, cloneIP(ip))
		incrementIP(ip)
	}

	return &PeerRegistry{
		peers:     make(map[string]*PeerEntry),
		pool:      pool,
		allocated: make(map[string]bool),
	}
}

// Allocate reserves the next free tunnel IP for publicKey and stores the
// resulting PeerEntry. It returns the allocated IP or an error if the pool is
// exhausted or the key is already registered.
func (r *PeerRegistry) Allocate(publicKey string) (net.IP, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.peers[publicKey]; exists {
		return nil, fmt.Errorf("peer %q is already registered", publicKey)
	}

	for _, ip := range r.pool {
		if !r.allocated[ip.String()] {
			r.allocated[ip.String()] = true
			r.peers[publicKey] = &PeerEntry{
				PublicKey: publicKey,
				TunnelIP:  cloneIP(ip),
			}
			return cloneIP(ip), nil
		}
	}

	return nil, errors.New("tunnel IP pool exhausted")
}

// Release removes publicKey from the registry and marks its tunnel IP as free.
// It returns the released IP so the caller can clean up the corresponding
// WireGuard peer and iptables rule.
func (r *PeerRegistry) Release(publicKey string) (net.IP, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.peers[publicKey]
	if !ok {
		return nil, fmt.Errorf("peer %q not found", publicKey)
	}

	ip := cloneIP(entry.TunnelIP)
	delete(r.allocated, ip.String())
	delete(r.peers, publicKey)

	return ip, nil
}

// Get returns the PeerEntry for publicKey. The bool is false if not found.
// The returned entry must not be mutated by the caller.
func (r *PeerRegistry) Get(publicKey string) (*PeerEntry, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entry, ok := r.peers[publicKey]
	return entry, ok
}

// All returns a snapshot of all current peer entries.
func (r *PeerRegistry) All() []*PeerEntry {
	r.mu.RLock()
	defer r.mu.RUnlock()
	entries := make([]*PeerEntry, 0, len(r.peers))
	for _, e := range r.peers {
		entries = append(entries, e)
	}
	return entries
}

// ── IP helpers ────────────────────────────────────────────────────────────────

func cloneIP(ip net.IP) net.IP {
	clone := make(net.IP, len(ip))
	copy(clone, ip)
	return clone
}

// incrementIP increments ip in-place by 1, carrying over as needed.
func incrementIP(ip net.IP) {
	for i := len(ip) - 1; i >= 0; i-- {
		ip[i]++
		if ip[i] != 0 {
			break
		}
	}
}

// isBroadcast reports whether ip is the broadcast (all-ones host) address of
// subnet. Both ip and subnet.IP must be 4-byte IPv4 slices.
func isBroadcast(ip net.IP, subnet *net.IPNet) bool {
	ip4 := ip.To4()
	base := subnet.IP.To4()
	if ip4 == nil || base == nil {
		return false
	}
	for i := 0; i < 4; i++ {
		if ip4[i] != base[i]|^subnet.Mask[i] {
			return false
		}
	}
	return true
}
