package iptables

import (
	"fmt"
	"slices"

	"github.com/coreos/go-iptables/iptables"
	"go.uber.org/zap"

	"github.com/rasorp/smuggle/internal/log"
	"github.com/rasorp/smuggle/internal/types"
)

const (
	// natTableName is the name of the NAT table in iptables.
	natTableName = "nat"

	// smugglePostroutingChainName is the custom chain for smuggle masquerading
	// rules.
	smugglePostroutingChainName = "SMUGGLE-POSTROUTING"

	// smuggleForwardChainName is the custom chain name for Smuggle forwarding
	// rules.
	smuggleForwardChainName = "SMUGGLE-FORWARD"

	// postroutingChainName is the name of the POSTROUTING chain in the NAT
	// table.
	postroutingChainName = "POSTROUTING"

	// forwardChainName is the name of the FORWARD chain in the filter table.
	forwardChainName = "FORWARD"
)

// Manager handles iptables rules for VXLAN networking
type Manager struct {
	ipt    *iptables.IPTables
	logger *zap.Logger
}

// NewManager creates a new iptables manager
func NewManager(logger *zap.Logger) (types.Firewall, error) {
	ipt, err := iptables.New()
	if err != nil {
		return nil, fmt.Errorf("failed to initialize iptables: %w", err)
	}

	return &Manager{
		ipt:    ipt,
		logger: logger.Named(log.ComponentNameIptables),
	}, nil
}

// CreateForwardRules applies forward rules to iptables
func (m *Manager) CreateForwardRules(network *types.Network) error {

	cidr := network.IPv4.Network.String()
	bridgeInterface := network.BridgeInterfaceName()
	networkInterface := network.InterfaceName()

	m.logger.Debug("setting up forward rules",
		zap.String("network_cidr", cidr),
		zap.String("bridge_interface", bridgeInterface),
		zap.String("network_interface", networkInterface),
	)

	// Ensure the custom chain exists
	if err := m.ensureChain("filter", smuggleForwardChainName); err != nil {
		return fmt.Errorf("failed to ensure chain %s: %w", smuggleForwardChainName, err)
	}

	// Apply all rules to the Smuggle forward chain but skip the jump rule as
	// we'll handle it separately.
	for _, rule := range m.forwardRules(cidr, bridgeInterface, networkInterface) {
		if rule.chain == forwardChainName {
			continue
		}
		if err := m.applyRule(rule); err != nil {
			return fmt.Errorf("failed to apply rule: %w", err)
		}
	}

	// Ensure jump rule is FIRST in FORWARD chain and before Docker chains. This
	// is critical because Docker chains don't have a final ACCEPT, so packets
	// that don't match fall through to the DROP policy.
	if err := m.ensureJumpRuleFirst("filter", forwardChainName, smuggleForwardChainName); err != nil {
		return fmt.Errorf("failed to ensure jump rule is first: %w", err)
	}

	m.logger.Info("successfully set up forward rules")
	return nil
}

// DeleteForwardRules removes the forward rules from iptables for the given
// network.
func (m *Manager) DeleteForwardRules(network *types.Network) error {

	networkPairs := network.LoggingPairs()

	m.logger.Debug("deleting forward rules", networkPairs...)

	// Iterate over the rules and delete them. We do not consider errors
	// fatal here, as we want to attempt to delete all rules as possible.
	for _, rule := range m.forwardRules(
		network.IPv4.Network.String(),
		network.BridgeInterfaceName(),
		network.InterfaceName(),
	) {
		if err := m.deleteRule(rule); err != nil {
			m.logger.Error("failed to delete forward rule",
				append(rule.loggingPairs(), zap.Error(err))...,
			)
		}
	}

	m.logger.Info("successfully deleted forward rules", networkPairs...)
	return nil
}

// CreateMasqRules applies masquerading rules to iptables
func (m *Manager) CreateMasqRules(network *types.Network, subnet *types.Subnet) error {

	ipv4Network := network.IPv4.Network
	ipv4Subnet := subnet.IPv4Network

	m.logger.Debug("setting up masquerading rules",
		zap.String("network_cidr", ipv4Network.String()),
		zap.String("subnet_cidr", ipv4Subnet.String()),
	)

	// Ensure the custom chain exists
	if err := m.ensureChain(natTableName, smugglePostroutingChainName); err != nil {
		return fmt.Errorf("failed to ensure chain: %w", err)
	}

	// Iterate over the rules and apply them. Any error is considered fatal as
	// we need these rules to be in place for proper networking.
	for _, rule := range m.masqRules(ipv4Network, ipv4Subnet) {
		if err := m.applyRule(rule); err != nil {
			return fmt.Errorf("failed to apply rule: %w", err)
		}
	}

	m.logger.Info("successfully set up masquerading rules",
		zap.String("network_cidr", ipv4Network.String()),
		zap.String("subnet_cidr", ipv4Subnet.String()),
	)
	return nil
}

// DeleteMasqRules removes masquerading rules from iptables for the given
// network and subnet.
func (m *Manager) DeleteMasqRules(network *types.Network, subnet *types.Subnet) error {

	networkPairs := network.LoggingPairs()

	m.logger.Debug("deleting masquerade rules", networkPairs...)

	// Iterate over the rules and delete them. We do not consider errors fatal
	// here, as we want to attempt to delete all rules as possible.
	for _, rule := range m.masqRules(network.IPv4.Network, subnet.IPv4Network) {
		if err := m.deleteRule(rule); err != nil {
			m.logger.Error("failed to delete masquerade rule",
				append(rule.loggingPairs(), zap.Error(err))...,
			)
		}
	}

	m.logger.Info("successfully deleted masquerade rules", networkPairs...)
	return nil
}

// masqRules generates the iptables rules for masquerading traffic from the
// network subnet to external destinations.
func (m *Manager) masqRules(network *types.IPv4Net, subnet *types.IPv4Net) []rule {
	rules := []rule{
		// Jump from POSTROUTING to our custom chain so we can manage rules
		// independently in our own chain and perform this before other firewall
		// rules.
		{
			id:    "jump-to-smuggle-chain",
			table: natTableName,
			chain: postroutingChainName,
			spec: []string{
				"-m", "comment",
				"--comment", "smuggle masq",
				"-j", smugglePostroutingChainName,
			},
		},
	}

	supportsRandomFully := m.ipt.HasRandomFully()

	networkString := network.String()
	subnetString := subnet.String()

	// NAT traffic from local subnet that's NOT going to the cluster network, so
	// it can reach the internet.
	if supportsRandomFully {
		rules = append(rules, rule{
			id:    "masquerade-to-external-random-fully",
			table: natTableName,
			chain: smugglePostroutingChainName,
			spec: []string{
				"-s", subnetString,
				"!", "-d", networkString,
				"-m", "comment",
				"--comment", "smuggle masq",
				"-j", "MASQUERADE",
				"--random-fully",
			},
		})
	} else {
		rules = append(rules, rule{
			id:    "masquerade-to-external",
			table: natTableName,
			chain: smugglePostroutingChainName,
			spec: []string{
				"-s", subnetString,
				"!", "-d", networkString,
				"-m", "comment",
				"--comment", "smuggle masq",
				"-j", "MASQUERADE",
			},
		})
	}

	return rules
}

// ensureChain ensures an iptables chain exists, creating it if necessary
func (m *Manager) ensureChain(table, chain string) error {
	chains, err := m.ipt.ListChains(table)
	if err != nil {
		return fmt.Errorf("failed to list chains: %w", err)
	}

	// Check if chain already exists
	if slices.Contains(chains, chain) {
		m.logger.Debug("chain already exists, skipping creation",
			zap.String("table", table),
			zap.String("chain", chain),
		)
		return nil
	}

	// Create the chain
	m.logger.Debug("creating iptables chain",
		zap.String("table", table),
		zap.String("chain", chain),
	)

	if err := m.ipt.NewChain(table, chain); err != nil {
		return fmt.Errorf("failed to create chain: %w", err)
	}

	m.logger.Info("successfully created chain",
		zap.String("table", table),
		zap.String("chain", chain),
	)

	return nil
}

// applyRule ensures an iptables rule exists, adding it if necessary
func (m *Manager) applyRule(rule rule) error {

	exists, err := m.ipt.Exists(rule.table, rule.chain, rule.spec...)
	if err != nil {
		return fmt.Errorf("failed to check if rule exists: %w", err)
	}

	// Generate the logging pairs once, that will be used in both branches and
	// potentially multiple times.
	loggingPairs := rule.loggingPairs()

	if !exists {
		m.logger.Debug("applying iptables rule", loggingPairs...)

		if err := m.ipt.Append(rule.table, rule.chain, rule.spec...); err != nil {
			return fmt.Errorf("failed to apply rule: %w", err)
		}

		m.logger.Info("successfully applied iptables rule", loggingPairs...)
	} else {
		m.logger.Debug("iptables rule already exists, skipping apply", loggingPairs...)
	}

	return nil
}

// deleteRule removes an iptables rule if it exists. If it does not exist, it
// is a no-op.
func (m *Manager) deleteRule(rule rule) error {

	exists, err := m.ipt.Exists(rule.table, rule.chain, rule.spec...)
	if err != nil {
		return fmt.Errorf("failed to check if rule exists: %w", err)
	}

	// Generate the logging pairs once, that will be used in both branches and
	// potentially multiple times.
	loggingPairs := rule.loggingPairs()

	if exists {
		m.logger.Debug("deleting iptables rule", loggingPairs...)

		if err := m.ipt.Delete(rule.table, rule.chain, rule.spec...); err != nil {
			return fmt.Errorf("failed to delete rule: %w", err)
		}

		m.logger.Info("successfully deleted iptables rule", loggingPairs...)
	} else {
		m.logger.Debug("iptables rule does not exist, skipping delete", loggingPairs...)
	}

	return nil
}

// forwardRules generates iptables rules for forwarding traffic that allows
// traffic to be forwarded to and from the network range.
func (m *Manager) forwardRules(networkCIDR, bridgeInterface, networkInterface string) []rule {
	return []rule{
		// Jump to custom chain to manage forward rules independently. This
		// ensures Smuggle rules are evaluated before other node firewall rules.
		{
			id:    "jump-to-forward-chain",
			table: "filter",
			chain: forwardChainName,
			spec: []string{
				"-m", "comment",
				"--comment", "smuggle forward",
				"-j", smuggleForwardChainName,
			},
		},
		// Allow established and related connections for return traffic.
		{
			id:    "accept-established-related",
			table: "filter",
			chain: smuggleForwardChainName,
			spec: []string{
				"-m", "conntrack",
				"--ctstate", "RELATED,ESTABLISHED",
				"-m", "comment",
				"--comment", "smuggle forward established",
				"-j", "ACCEPT",
			},
		},
		// Allow forwarding packets from the bridge to external destinations
		// (internet), but NOT to other cluster networks.
		{
			id:    "accept-forward-from-bridge-to-external",
			table: "filter",
			chain: smuggleForwardChainName,
			spec: []string{
				"-i", bridgeInterface,
				"-s", networkCIDR,
				"!", "-d", networkCIDR,
				"-m", "comment",
				"--comment", "smuggle forward to external",
				"-j", "ACCEPT",
			},
		},
		// Allow forwarding packets to the bridge from external sources. This
		// primarily handles return traffic that doesn't match ESTABLISHED,
		// RELATED.
		{
			id:    "accept-forward-to-bridge-from-external",
			table: "filter",
			chain: smuggleForwardChainName,
			spec: []string{
				"-o", bridgeInterface,
				"-d", networkCIDR,
				"!", "-s", networkCIDR,
				"-m", "comment",
				"--comment", "smuggle forward from external",
				"-j", "ACCEPT",
			},
		},
		// Allow forwarding within the same network on the same node (bridge to
		// bridge).
		{
			id:    "accept-forward-within-network-local",
			table: "filter",
			chain: smuggleForwardChainName,
			spec: []string{
				"-i", bridgeInterface,
				"-o", bridgeInterface,
				"-s", networkCIDR,
				"-d", networkCIDR,
				"-m", "comment",
				"--comment", "smuggle forward intra-network local",
				"-j", "ACCEPT",
			},
		},
		// Allow forwarding from bridge to Smuggle (containers to remote nodes).
		{
			id:    "accept-forward-bridge-to-vxlan",
			table: "filter",
			chain: smuggleForwardChainName,
			spec: []string{
				"-i", bridgeInterface,
				"-o", networkInterface,
				"-s", networkCIDR,
				"-d", networkCIDR,
				"-m", "comment",
				"--comment", "smuggle forward bridge to vxlan",
				"-j", "ACCEPT",
			},
		},
		// Allow forwarding from Smuggle to bridge (remote nodes to containers).
		{
			id:    "accept-forward-smuggle-to-bridge",
			table: "filter",
			chain: smuggleForwardChainName,
			spec: []string{
				"-i", networkInterface,
				"-o", bridgeInterface,
				"-s", networkCIDR,
				"-d", networkCIDR,
				"-m", "comment",
				"--comment", "smuggle forward vxlan to bridge",
				"-j", "ACCEPT",
			},
		},
	}
}

// ensureJumpRuleFirst ensures a jump rule exists and is at position 1.
// This is necessary to ensure Smuggle rules run before Docker's chains.
func (m *Manager) ensureJumpRuleFirst(table, chain, targetChain string) error {
	ruleSpec := []string{"-m", "comment", "--comment", "smuggle forward", "-j", targetChain}

	// Check if rule exists
	exists, err := m.ipt.Exists(table, chain, ruleSpec...)
	if err != nil {
		return fmt.Errorf("failed to check if jump rule exists: %w", err)
	}

	if exists {
		// Rule exists but might not be first. Delete and re-insert.
		if err := m.ipt.Delete(table, chain, ruleSpec...); err != nil {
			m.logger.Warn("failed to delete existing jump rule, will try to insert anyway",
				zap.Error(err))
		}
	}

	// Insert at position 1 (first rule, before Docker chains)
	if err := m.ipt.Insert(table, chain, 1, ruleSpec...); err != nil {
		return fmt.Errorf("failed to insert jump rule at position 1: %w", err)
	}

	m.logger.Info("ensured jump rule is first in chain",
		zap.String("table", table),
		zap.String("chain", chain),
		zap.String("target", targetChain),
	)

	return nil
}

// CreateIsolation creates REJECT rules to prevent cross-network communication.
// For each pair of networks, it creates rules that reject traffic from one
// network's interfaces to another network's interfaces. This uses the +
// wildcard to match all interfaces belonging to a network (both bridge and
// VXLAN).
func (m *Manager) CreateIsolation(networks []*types.Network) error {

	// There is no need to apply isolation rules if there are less than 2
	// networks.
	if len(networks) < 2 {
		m.logger.Debug("no isolation rules needed")
		return nil
	}

	// Ensure the custom chain exists
	if err := m.ensureChain("filter", smuggleForwardChainName); err != nil {
		return fmt.Errorf("failed to ensure chain: %w", err)
	}

	// Track rules we need to ensure exist
	var isolationRules []rule

	// For each network, create REJECT rules to all other networks
	for _, sourceNetwork := range networks {

		for _, destNetwork := range networks {
			// Skip if same network
			if sourceNetwork.Name == destNetwork.Name {
				continue
			}

			// Create REJECT rule for this network pair. The + wildcard matches
			// both bridge and VXLAN interfaces.
			isolationRules = append(
				isolationRules,
				m.isolationRule(sourceNetwork.Name+"+", destNetwork.Name+"+"),
			)
		}
	}

	m.logger.Debug("creating isolation rules")

	for _, rule := range isolationRules {
		if err := m.ensureIsolationRule(rule); err != nil {
			return fmt.Errorf("failed to create isolation rule: %w", err)
		}
	}

	m.logger.Info("successfully created isolation rules")
	return nil
}

// ensureIsolationRule ensures an isolation REJECT rule exists in the chain.
// Unlike applyRule which appends, this inserts the rule at a specific position
// to ensure isolation rules run before ACCEPT rules.
func (m *Manager) ensureIsolationRule(rule rule) error {
	exists, err := m.ipt.Exists(rule.table, rule.chain, rule.spec...)
	if err != nil {
		return fmt.Errorf("failed to check if rule exists: %w", err)
	}

	if !exists {
		m.logger.Debug("creating isolation rule", rule.loggingPairs()...)

		// Insert at position 2 which is immediately after ESTABLISHED,RELATED
		// which is at position 1. This ensures isolation rules run before any
		// ACCEPT rules.
		if err := m.ipt.Insert(rule.table, rule.chain, 2, rule.spec...); err != nil {
			return fmt.Errorf("failed to create isolation rule: %w", err)
		}
	}

	return nil
}

// DeleteIsolation removes the isolation rules for the deleted networks, while
// keeping the rules for the existing networks intact.
func (m *Manager) DeleteIsolation(exist, deleted []*types.Network) error {

	if len(deleted) == 0 {
		m.logger.Debug("no networks to delete isolation rules for")
		return nil
	}

	m.logger.Debug("deleting network isolation rules")

	// Build a combined list of all networks (existing + deleted) to check against
	allNetworks := append([]*types.Network{}, exist...)
	allNetworks = append(allNetworks, deleted...)

	// For each deleted network, remove all isolation rules where it appears
	// as either source or destination
	for _, deletedNetwork := range deleted {
		deletedPrefix := deletedNetwork.Name + "+"

		for _, otherNetwork := range allNetworks {

			// Skip if same network.
			if deletedNetwork.Name == otherNetwork.Name {
				continue
			}

			otherPrefix := otherNetwork.Name + "+"

			// Delete rule where deleted network is the SOURCE (deleted -> other)
			if err := m.deleteRule(m.isolationRule(deletedPrefix, otherPrefix)); err != nil {
				return fmt.Errorf("failed to delete isolation rule: %w", err)
			}

			// Delete rule where deleted network is the DESTINATION (other -> deleted)
			if err := m.deleteRule(m.isolationRule(otherPrefix, deletedPrefix)); err != nil {
				return fmt.Errorf("failed to delete isolation rule: %w", err)
			}
		}
	}

	m.logger.Info("successfully deleted network isolation rules")
	return nil
}

// isolationRule generates the isolation REJECT rule for the given source and
// destination interface prefixes.
func (m *Manager) isolationRule(src, dst string) rule {
	return rule{
		id:    fmt.Sprintf("reject-%s-to-%s", src, dst),
		table: "filter",
		chain: smuggleForwardChainName,
		spec: []string{
			"-i", src,
			"-o", dst,
			"-m", "comment",
			"--comment", fmt.Sprintf("smuggle isolate %s from %s", src, dst),
			"-j", "REJECT",
			"--reject-with", "icmp-net-prohibited",
		},
	}
}
