package network

import (
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"time"

	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/log"
	"github.com/rasorp/smuggle/internal/network/firewall/iptables"
	"github.com/rasorp/smuggle/internal/types"
)

type Manager struct {
	logger      *zap.Logger
	fingerprint *networkFingerprint
	providers   map[string]types.NetworkProvider
	Firewall    types.Firewall
}

func NewManager(logger *zap.Logger, intf string) (*Manager, error) {

	// Network manager is only supported on Linux. It would be possible to
	// constrain this via build tags, but it would require duplicating a lot of
	// the code in this file just to have a stub for non-linux systems that can
	// run the server.
	if runtime.GOOS != "linux" {
		return nil, errors.New("network manager is only supported on Linux")
	}

	f, err := fingerprint(intf)
	if err != nil {
		return nil, err
	}

	firewallManager, err := iptables.NewManager(logger)
	if err != nil {
		return nil, err
	}

	m := Manager{
		logger:      logger.Named(log.ComponentNameNetwork),
		fingerprint: f,
		Firewall:    firewallManager,
	}

	m.setProviderMap()

	return &m, nil
}

func (m *Manager) SetLocal(
	req *types.NetworkProviderSetReq,
) (*types.NetworkProviderSetResp, error) {

	req.HostInteface = m.fingerprint.iface

	provider, ok := m.providers[req.Client.Provider]
	if !ok {
		return nil, fmt.Errorf("unknown network provider %q", req.Client.Provider)
	}

	return provider.SetLocal(req)
}

func (m *Manager) DeleteRemote(
	req *types.NetworkProviderDeleteRemoteReq,
) (*types.NetworkProviderDeleteRemoteResp, error) {

	provider, ok := m.providers[req.Subnet.Provider]
	if !ok {
		return nil, fmt.Errorf("unknown network provider %q", req.Subnet.Provider)
	}

	return provider.DeleteRemote(req)
}

func (m *Manager) SetRemote(
	req *types.NetworkProviderSetRemoteReq,
) (*types.NetworkProviderSetRemoteResp, error) {

	provider, ok := m.providers[req.Subnet.Provider]
	if !ok {
		return nil, fmt.Errorf("unknown network provider %q", req.Subnet.Provider)
	}

	return provider.SetRemote(req)
}

// GenerateIPv4Subnet allocates an available subnet from the configured network
// range. It uses an adaptive strategy that switches between random probing
// which is efficient for sparse networks, and sequential search which is
// efficient for dense networks based on network utilization.
func (m *Manager) GenerateIPv4Subnet(
	id string,
	cfg *types.Network,
	subnets []*types.Subnet,
) (*types.Subnet, error) {

	// Build a set of used subnet IPs for O(1) lookup.
	usedSubnets := make(map[types.IPv4Addr]bool, len(subnets))

	for _, subnet := range subnets {
		if subnet.IPv4Network != nil && subnet.NetworkName == cfg.Name {
			usedSubnets[subnet.IPv4Network.IP] = true
		}
	}

	// Calculate total number of possible subnets in the range and ensure it's
	// valid.
	subnetSize := uint32(1 << (32 - cfg.IPv4.Size))
	totalSubnets := int((uint32(cfg.IPv4.Max)-uint32(cfg.IPv4.Min))/subnetSize) + 1

	if totalSubnets <= 0 {
		return nil, fmt.Errorf("invalid network range: min=%s max=%s size=%d",
			cfg.IPv4.Min, cfg.IPv4.Max, cfg.IPv4.Size)
	}

	// Calculate network utilization.
	utilizationPct := float64(len(usedSubnets)) / float64(totalSubnets)

	// If network is ≥80% utilization, use sequential search otherwise use our
	// random probing strategy.
	if utilizationPct >= 0.8 {
		m.logger.Debug("using sequential search strategy for subnet allocation",
			zap.Float64("utilization", utilizationPct),
			zap.String("network", cfg.Name),
			zap.Int("used", len(usedSubnets)),
			zap.Int("total", totalSubnets))
		return m.findSequentialSubnet(id, cfg, usedSubnets, subnetSize)
	}

	m.logger.Debug("using random probe strategy for subnet allocation",
		zap.Float64("utilization", utilizationPct),
		zap.String("network", cfg.Name),
		zap.Int("used", len(usedSubnets)),
		zap.Int("total", totalSubnets))
	return m.findRandomSubnet(id, cfg, usedSubnets, subnetSize, totalSubnets)
}

// findRandomSubnet attempts to find an available subnet by random probing. This
// is efficient when the network is sparsely allocated and provides better
// distribution across the address space compared to sequential allocation.
func (m *Manager) findRandomSubnet(
	id string,
	cfg *types.Network,
	usedSubnets map[types.IPv4Addr]bool,
	subnetSize uint32,
	totalSubnets int,
) (*types.Subnet, error) {

	// Calculate number of probing attempts based on utilization. Try up to 3
	// times the number of used subnets with the bounds of 100 and 1000.
	maxAttempts := max(len(usedSubnets)*3, 100)
	maxAttempts = max(maxAttempts, 1000)

	for attempt := 0; attempt < maxAttempts; attempt++ {

		randomIndex := randInt(0, totalSubnets)
		candidateIP := cfg.IPv4.Min + types.IPv4Addr(randomIndex)*types.IPv4Addr(subnetSize)

		if candidateIP > cfg.IPv4.Max {
			continue
		}

		// Check if this subnet is available and return it if so.
		if !usedSubnets[candidateIP] {
			return m.createSubnet(id, cfg, candidateIP), nil
		}
	}

	// If we reach this point, our random probing attempts were exhausted
	// without finding an available subnet. Attempt to use the sequential search
	// as a fallback.
	m.logger.Debug("random probing exhausted, falling back to sequential search",
		zap.String("network", cfg.Name),
		zap.Int("attempts", maxAttempts))

	return m.findSequentialSubnet(id, cfg, usedSubnets, subnetSize)
}

// findSequentialSubnet searches linearly for the first available subnet.
func (m *Manager) findSequentialSubnet(
	id string,
	cfg *types.Network,
	usedSubnets map[types.IPv4Addr]bool,
	subnetSize uint32,
) (*types.Subnet, error) {

	for candidateIP := cfg.IPv4.Min; candidateIP <= cfg.IPv4.Max; candidateIP += types.IPv4Addr(subnetSize) {
		if !usedSubnets[candidateIP] {
			return m.createSubnet(id, cfg, candidateIP), nil
		}

		// Prevent overflow when approaching the end of the address space.
		if candidateIP > cfg.IPv4.Max-types.IPv4Addr(subnetSize) {
			break
		}
	}

	// If we reached this point, the network is completely full and we have no
	// available subnets to allocate.
	return nil, fmt.Errorf("network %s is full", cfg.Name)
}

// createSubnet constructs a new Subnet object with the given IP address and
// populates it with client and network metadata.
func (m *Manager) createSubnet(id string, cfg *types.Network, ip types.IPv4Addr) *types.Subnet {
	return &types.Subnet{
		ClientID:    id,
		NetworkName: cfg.Name,
		Provider:    cfg.Provider.Name,
		Config:      cfg.Provider.Config,
		HostIPv4:    &m.fingerprint.ipv4Addr,
		Expiration:  time.Now().Add(types.DefaultSubnetTTL),
		MTU:         m.fingerprint.iface.MTU - 50,
		IPv4Network: &types.IPv4Net{
			IP:   ip,
			Size: cfg.IPv4.Size,
		},
	}
}

// rnd is a package-level random number generator. It is initialized once with
// a seed based on the current time and is used to generate random integers for
// subnet allocation.
var rnd *rand.Rand

func init() { rnd = rand.New(rand.NewSource(time.Now().UnixNano())) }

func randInt(lo, hi int) int { return lo + int(rnd.Int31n(int32(hi-lo))) }
