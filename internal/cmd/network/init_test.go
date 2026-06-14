package network

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/shoenig/test/must"
	"github.com/urfave/cli/v3"
)

func TestInitCommand(t *testing.T) {
	testCases := []struct {
		name           string
		args           []string
		setup          func(t *testing.T)
		expectedErr    string
		expectedOutput string
		expectedFile   string
	}{
		{
			name:           "default provider",
			args:           []string{"init"},
			expectedOutput: fmt.Sprintf("successfully wrote file %s\n", networkInitFilename),
			expectedFile:   strings.TrimSpace(vxlanNetworkInitContent),
		},
		{
			name:           "vxlan provider",
			args:           []string{"init", "--provider", "vxlan"},
			expectedOutput: fmt.Sprintf("successfully wrote file %s\n", networkInitFilename),
			expectedFile:   strings.TrimSpace(vxlanNetworkInitContent),
		},
		{
			name:           "wireguard provider",
			args:           []string{"init", "--provider", "wireguard"},
			expectedOutput: fmt.Sprintf("successfully wrote file %s\n", networkInitFilename),
			expectedFile:   strings.TrimSpace(wireguardNetworkInitContent),
		},
		{
			name:        "invalid provider",
			args:        []string{"init", "--provider", "bgp"},
			expectedErr: "invalid provider: \"bgp\". Valid values are \"vxlan\" and \"wireguard\"",
		},
		{
			name: "existing file",
			args: []string{"init"},
			setup: func(t *testing.T) {
				must.NoError(t, os.WriteFile(networkInitFilename, []byte("existing"), 0600))
			},
			expectedErr: networkInitFilename + " already exists",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Chdir(t.TempDir())

			if tc.setup != nil {
				tc.setup(t)
			}

			var buf bytes.Buffer

			cmd := initCommand()
			cmd.Writer = &buf

			err := cmd.Run(context.Background(), tc.args)

			if tc.expectedErr != "" {
				must.Error(t, err)
				must.ErrorContains(t, err, tc.expectedErr)
			} else {
				must.NoError(t, err)
				must.Eq(t, tc.expectedOutput, buf.String())

				content, readErr := os.ReadFile(networkInitFilename)
				must.NoError(t, readErr)
				must.Eq(t, tc.expectedFile, string(content))
			}
		})
	}
}

func Test_initFlags(t *testing.T) {
	flags := initFlags()
	must.Eq(t, 1, len(flags))
	must.Eq(t, "provider", flags[0].Names()[0])
	must.Eq(t, "vxlan", flags[0].(*cli.StringFlag).Value)
}
