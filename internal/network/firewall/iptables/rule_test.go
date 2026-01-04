package iptables

import (
	"testing"

	"github.com/shoenig/test/must"
	"go.uber.org/zap"
)

func Test_rule_loggingPairs(t *testing.T) {

	testRule := rule{
		id:    "test-rule-1",
		table: "filter",
		chain: "INPUT",
		spec:  []string{"-p", "tcp", "--dport", "80", "-j", "ACCEPT"},
	}

	expectedPairs := []zap.Field{
		zap.String("rule_id", "test-rule-1"),
		zap.String("table", "filter"),
		zap.String("chain", "INPUT"),
		zap.Strings("spec", []string{"-p", "tcp", "--dport", "80", "-j", "ACCEPT"}),
	}

	must.SliceContainsAll(t, expectedPairs, testRule.loggingPairs())
}
