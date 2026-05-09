package network

import (
	"testing"

	"github.com/shoenig/test/must"
)

func Test_calcMaxAttempts(t *testing.T) {
	testCases := []struct {
		name      string
		usedCount int
		expected  int
	}{
		{
			name:      "zero used subnets returns floor",
			usedCount: 0,
			expected:  100,
		},
		{
			name:      "sparse cluster returns floor",
			usedCount: 5,
			expected:  100,
		},
		{
			name:      "just below floor multiplier boundary returns floor",
			usedCount: 33,
			expected:  100,
		},
		{
			name:      "just above floor multiplier boundary returns 3x used",
			usedCount: 34,
			expected:  102,
		},
		{
			name:      "midrange returns 3x used",
			usedCount: 200,
			expected:  600,
		},
		{
			name:      "one below ceiling boundary returns 3x used",
			usedCount: 333,
			expected:  999,
		},
		{
			name:      "just above ceiling boundary is capped at 1000",
			usedCount: 334,
			expected:  1000,
		},
		{
			name:      "large used count is capped at 1000",
			usedCount: 1000,
			expected:  1000,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			must.Eq(t, tc.expected, calculateMaxAttempts(tc.usedCount))
		})
	}
}
