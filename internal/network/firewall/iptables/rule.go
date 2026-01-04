package iptables

import "go.uber.org/zap"

// rule represents an iptables rule entry.
type rule struct {
	id    string
	table string
	chain string
	spec  []string
}

// loggingPairs returns zap fields for logging the rule.
func (r rule) loggingPairs() []zap.Field {
	return []zap.Field{
		zap.String("table", r.table),
		zap.String("chain", r.chain),
		zap.Strings("spec", r.spec),
		zap.String("rule_id", r.id),
	}
}
