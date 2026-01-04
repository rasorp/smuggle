package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/urfave/cli/v3"

	"github.com/rasorp/smuggle/internal/cmd/agent"
	"github.com/rasorp/smuggle/internal/cmd/network"
	clihelp "github.com/rasorp/smuggle/internal/helper/cli"
	"github.com/rasorp/smuggle/internal/version"
)

func main() {

	cli.VersionPrinter = func(cmd *cli.Command) {
		_, _ = fmt.Fprint(cmd.Writer, clihelp.FormatKV([]string{
			"Version|" + cmd.Version,
			"Build Time|" + version.BuildTime,
			"Build Commit|" + version.BuildCommit,
		}))
		_, _ = fmt.Fprint(cmd.Writer, "\n")
	}

	cliApp := cli.Command{
		Commands: []*cli.Command{
			agent.Command(),
			network.Command(),
		},
		Name:  "smuggle",
		Usage: "Layer 3 network fabric for IBM HashiCorp Nomad",
		Description: strings.TrimSpace(
			`
Smuggle is a lightweight layer 3 overlay network fabric for IBM HashiCorp Nomad.
It currently supports VXLAN overlays.

While other container networking solutions exist, most are focused on Kubernetes
and are either incompatible with Nomad or require additional services to be run
which are not typically part of a Nomad deployment. Smuggle is designed
specifically for Nomad and aims to be simple to deploy and operate without
additional required dependencies.`),
		Version:         version.Get(),
		HideHelpCommand: true,
	}

	if err := cliApp.Run(context.Background(), os.Args); err != nil {
		_, _ = fmt.Fprint(os.Stderr, err.Error()+"\n")
		os.Exit(1)
	}
}
