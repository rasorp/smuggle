package network

import "github.com/urfave/cli/v3"

func Command() *cli.Command {
	return &cli.Command{
		Name:      "network",
		Usage:     "Initialize and read Smuggle network configurations",
		UsageText: "smuggle network <command> [options] [args]",
		Commands: []*cli.Command{
			initCommand(),
		},
	}
}
