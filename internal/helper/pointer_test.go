package helper

import (
	"testing"

	"github.com/shoenig/test/must"
)

func Test_PointerOf(t *testing.T) {
	tesCases := []struct {
		name     string
		input    any
		expected any
	}{
		{
			name:  "int",
			input: 42,
		},
		{
			name:  "string",
			input: "hello",
		},
		{
			name:  "struct",
			input: struct{ A int }{A: 7},
		},
	}

	for _, tc := range tesCases {
		t.Run(tc.name, func(t *testing.T) {
			actualOutput := PointerOf(tc.input)
			must.Eq(t, tc.input, *actualOutput)
		})
	}
}
