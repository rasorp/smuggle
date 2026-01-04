package agent

import "github.com/urfave/cli/v3"

func Command() *cli.Command {
	return &cli.Command{
		Name:      "agent",
		Usage:     "Run, control, and interrogate Smuggle agents",
		UsageText: "smuggle agent <command> [options] [args]",
		Commands: []*cli.Command{
			runCommand(),
		},
	}
}
