package client

import (
	"github.com/urfave/cli/v3"
)

// Command returns the top-level "client" CLI command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "client",
		Usage:     "Run, control, and interrogate a Smuggle client",
		UsageText: "smuggle client <command> [options] [args]",
		Commands:  []*cli.Command{runCommand()},
	}
}
