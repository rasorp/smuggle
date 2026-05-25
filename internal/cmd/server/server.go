package server

import (
	"github.com/urfave/cli/v3"
)

// Command returns the top-level "server" CLI command.
func Command() *cli.Command {
	return &cli.Command{
		Name:      "server",
		Usage:     "Run, control, and interrogate a Smuggle server",
		UsageText: "smuggle server <command> [options] [args]",
		Commands:  []*cli.Command{runCommand()},
	}
}
