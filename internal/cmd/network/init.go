package network

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"
)

func initCommand() *cli.Command {
	return &cli.Command{
		Name:     "init",
		Category: "network",
		Usage:    "Creates an example network configuration file",
		Action: func(_ context.Context, cmd *cli.Command) error {

			// Check if smuggle-net.json already exists. If it does, return an
			// error rather than blindly overwriting it which might be
			// unexpected behavior.
			_, err := os.Stat(networkInitFilename)
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to check for existing file: %w", err)
			}
			if !os.IsNotExist(err) {
				return fmt.Errorf("%s already exists", networkInitFilename)
			}

			// Write out the example.
			if err := os.WriteFile(
				networkInitFilename,
				[]byte(strings.TrimSpace(networkInitContent)),
				0600,
			); err != nil {
				return fmt.Errorf("failed to write file: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.Writer, "successfully wrote file %s\n", networkInitFilename)
			return nil
		},
	}
}

const (
	// networkInitFilename is the name of the file created by the init command
	// which contains an example network configuration.
	networkInitFilename = "smuggle-net.json"

	// networkInitContent is the content of the example network configuration
	// file created by the init command. This is a basic VXLAN network that can
	// be modified and written to Nomad via:
	// nomad var put smuggle/networks/v1/vxlan data="$(cat smuggle-net.json)".
	networkInitContent = `
{
  "name": "vxlan",
  "ipmasq": true,
  "ipv4": {
    "network": "10.10.0.0/16",
    "size": 24
  },
  "provider": {
    "name": "vxlan",
    "config": {
      "vni": 1,
      "port": 4789
    }
  }
}
`
)
