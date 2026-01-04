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

// New creates a new iptables manager
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

// masqRules generates the iptables rules for masquerading traffic from the
// network subnet to external destinations.
func (i *Manager) masqRules(network *types.IPv4Net, subnet *types.IPv4Net) []rule {
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

	supportsRandomFully := i.ipt.HasRandomFully()

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

// SetupMasqRules applies masquerading rules to iptables
func (i *Manager) SetupMasqRules(network *types.Network, subnet *types.Subnet) error {

	ipv4Network := network.IPv4.Network
	ipv4Subnet := subnet.IPv4Network

	i.logger.Debug("setting up masquerading rules",
		zap.String("network_cidr", ipv4Network.String()),
		zap.String("subnet_cidr", ipv4Subnet.String()),
	)

	// Ensure the custom chain exists
	if err := i.ensureChain(natTableName, smugglePostroutingChainName); err != nil {
		return fmt.Errorf("failed to ensure chain: %w", err)
	}

	// Iterate over the rules and apply them. Any error is considered fatal as
	// we need these rules to be in place for proper networking.
	for _, rule := range i.masqRules(ipv4Network, ipv4Subnet) {
		if err := i.applyRule(rule); err != nil {
			return fmt.Errorf("failed to apply rule: %w", err)
		}
	}

	i.logger.Info("successfully set up masquerading rules",
		zap.String("network_cidr", ipv4Network.String()),
		zap.String("subnet_cidr", ipv4Subnet.String()),
	)
	return nil
}

// ensureChain ensures an iptables chain exists, creating it if necessary
func (i *Manager) ensureChain(table, chain string) error {
	chains, err := i.ipt.ListChains(table)
	if err != nil {
		return fmt.Errorf("failed to list chains: %w", err)
	}

	// Check if chain already exists
	if slices.Contains(chains, chain) {
		i.logger.Debug("chain already exists, skipping creation",
			zap.String("table", table),
			zap.String("chain", chain),
		)
		return nil
	}

	// Create the chain
	i.logger.Debug("creating iptables chain",
		zap.String("table", table),
		zap.String("chain", chain),
	)

	if err := i.ipt.NewChain(table, chain); err != nil {
		return fmt.Errorf("failed to create chain: %w", err)
	}

	i.logger.Info("successfully created chain",
		zap.String("table", table),
		zap.String("chain", chain),
	)

	return nil
}

// applyRule ensures an iptables rule exists, adding it if necessary
func (i *Manager) applyRule(rule rule) error {
	exists, err := i.ipt.Exists(rule.table, rule.chain, rule.spec...)
	if err != nil {
		return fmt.Errorf("failed to check if rule exists: %w", err)
	}

	// Generate the logging pairs once, that will be used in both branches and
	// potentially multiple times.
	loggingPairs := rule.loggingPairs()

	if !exists {
		i.logger.Debug("applying iptables rule", loggingPairs...)

		if err := i.ipt.Append(rule.table, rule.chain, rule.spec...); err != nil {
			return fmt.Errorf("failed to apply rule: %w", err)
		}

		i.logger.Info("successfully applied iptables rule", loggingPairs...)
	} else {
		i.logger.Debug("iptables rule already exists, skipping apply", loggingPairs...)
	}

	return nil
}

// forwardRules generates iptables rules for forwarding traffic that allows
// traffic to be forwarded to and from the network range.
func (i *Manager) forwardRules(networkCIDR, bridgeInterface, networkInterface string) []rule {
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

// SetupForwardRules applies forward rules to iptables
func (i *Manager) SetupForwardRules(network *types.Network) error {

	cidr := network.IPv4.Network.String()
	bridgeInterface := network.BridgeInterfaceName()
	networkInterface := network.InterfaceName()

	i.logger.Debug("setting up forward rules",
		zap.String("network_cidr", cidr),
		zap.String("bridge_interface", bridgeInterface),
		zap.String("network_interface", networkInterface),
	)

	// Ensure the custom chain exists
	if err := i.ensureChain("filter", smuggleForwardChainName); err != nil {
		return fmt.Errorf("failed to ensure chain %s: %w", smuggleForwardChainName, err)
	}

	// Apply all rules to the Smuggle forward chain but Skip the jump rule as
	// we'll handle it separately.
	for _, rule := range i.forwardRules(cidr, bridgeInterface, networkInterface) {
		if rule.chain == forwardChainName {
			continue
		}
		if err := i.applyRule(rule); err != nil {
			return fmt.Errorf("failed to apply rule: %w", err)
		}
	}

	// Ensure jump rule is FIRST in FORWARD chain and before Docker chains. This
	// is critical because Docker chains don't have a final ACCEPT, so packets
	// that don't match fall through to the DROP policy.
	if err := i.ensureJumpRuleFirst("filter", forwardChainName, smuggleForwardChainName); err != nil {
		return fmt.Errorf("failed to ensure jump rule is first: %w", err)
	}

	i.logger.Info("successfully set up forward rules")
	return nil
}

// ensureJumpRuleFirst ensures a jump rule exists and is at position 1
// This is necessary to ensure Smuggle rules run before Docker's chains
func (i *Manager) ensureJumpRuleFirst(table, chain, targetChain string) error {
	ruleSpec := []string{"-m", "comment", "--comment", "smuggle forward", "-j", targetChain}

	// Check if rule exists
	exists, err := i.ipt.Exists(table, chain, ruleSpec...)
	if err != nil {
		return fmt.Errorf("failed to check if jump rule exists: %w", err)
	}

	if exists {
		// Rule exists but might not be first. Delete and re-insert.
		if err := i.ipt.Delete(table, chain, ruleSpec...); err != nil {
			i.logger.Warn("failed to delete existing jump rule, will try to insert anyway",
				zap.Error(err))
		}
	}

	// Insert at position 1 (first rule, before Docker chains)
	if err := i.ipt.Insert(table, chain, 1, ruleSpec...); err != nil {
		return fmt.Errorf("failed to insert jump rule at position 1: %w", err)
	}

	i.logger.Info("ensured jump rule is first in chain",
		zap.String("table", table),
		zap.String("chain", chain),
		zap.String("target", targetChain),
	)

	return nil
}

// EnsureIsolation creates REJECT rules to prevent cross-network communication.
// For each pair of networks, it creates rules that reject traffic from one
// network's interfaces to another network's interfaces. This uses the +
// wildcard to match all interfaces belonging to a network (both bridge and
// VXLAN).
func (i *Manager) EnsureIsolation(networks []*types.Network) error {

	// There is no need to apply isolation rules if there are less than 2
	// networks.
	if len(networks) < 2 {
		i.logger.Debug("no isolation rules needed", zap.Int("network_count", len(networks)))
		return nil
	}

	i.logger.Info("ensuring network isolation",
		zap.Int("network_count", len(networks)))

	// Ensure the custom chain exists
	if err := i.ensureChain("filter", smuggleForwardChainName); err != nil {
		return fmt.Errorf("failed to ensure chain: %w", err)
	}

	// Track rules we need to ensure exist
	var isolationRules []rule

	// For each network, create REJECT rules to all other networks
	for _, sourceNetwork := range networks {
		sourcePrefix := sourceNetwork.Name + "+"

		for _, destNetwork := range networks {
			// Skip if same network
			if sourceNetwork.Name == destNetwork.Name {
				continue
			}

			destPrefix := destNetwork.Name + "+"

			// Create REJECT rule for this network pair. The + wildcard matches
			// both bridge and VXLAN interfaces.
			isolationRules = append(isolationRules, rule{
				id:    fmt.Sprintf("reject-%s-to-%s", sourceNetwork.Name, destNetwork.Name),
				table: "filter",
				chain: smuggleForwardChainName,
				spec: []string{
					"-i", sourcePrefix,
					"-o", destPrefix,
					"-m", "comment",
					"--comment", fmt.Sprintf("smuggle isolate %s from %s", sourceNetwork.Name, destNetwork.Name),
					"-j", "REJECT",
					"--reject-with", "icmp-net-prohibited",
				},
			})
		}
	}

	i.logger.Debug("applying isolation rules",
		zap.Int("rule_count", len(isolationRules)))

	// Apply all isolation rules
	// These need to be inserted near the beginning of the chain, right after
	// ESTABLISHED,RELATED but before any ACCEPT rules
	for _, rule := range isolationRules {
		if err := i.ensureIsolationRule(rule); err != nil {
			return fmt.Errorf("failed to apply isolation rule: %w", err)
		}
	}

	i.logger.Info("successfully ensured network isolation",
		zap.Int("network_count", len(networks)),
		zap.Int("rule_count", len(isolationRules)))

	return nil
}

// ensureIsolationRule ensures an isolation REJECT rule exists in the chain.
// Unlike applyRule which appends, this inserts the rule at a specific position
// to ensure isolation rules run before ACCEPT rules.
func (i *Manager) ensureIsolationRule(rule rule) error {
	exists, err := i.ipt.Exists(rule.table, rule.chain, rule.spec...)
	if err != nil {
		return fmt.Errorf("failed to check if rule exists: %w", err)
	}

	loggingPairs := rule.loggingPairs()

	if !exists {
		i.logger.Debug("inserting isolation rule", loggingPairs...)

		// Insert at position 2 (right after ESTABLISHED,RELATED which is at position 1)
		// This ensures isolation rules run before any ACCEPT rules
		if err := i.ipt.Insert(rule.table, rule.chain, 2, rule.spec...); err != nil {
			return fmt.Errorf("failed to insert isolation rule: %w", err)
		}

		i.logger.Info("successfully inserted isolation rule", loggingPairs...)
	} else {
		i.logger.Debug("isolation rule already exists, skipping", loggingPairs...)
	}

	return nil
}
