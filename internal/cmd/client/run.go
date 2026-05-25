package client

import (
	"context"
	"os"

	"github.com/urfave/cli/v3"

	"github.com/rasorp/smuggle/internal/agent"
	"github.com/rasorp/smuggle/internal/config"
)

func runCommand() *cli.Command {
	return &cli.Command{
		Name:     "run",
		Category: "client",
		Usage:    "Run a Smuggle client",
		Flags:    config.ClientAgentConfigCommandFlags(),
		Action: func(_ context.Context, cmd *cli.Command) error {

			// Start with the default configuration as our base.
			defaultCfg := config.DefaultClientAgentConfig()

			// Load configuration from file(s) and merge them with the default
			// config.
			fileCfg, err := config.ClientAgentConfigFromFiles(cmd)
			if err != nil {
				return err
			}
			defaultCfg = defaultCfg.Merge(fileCfg)

			// Merge in any configuration provided via command line flags which
			// will override any previous configuration settings.
			defaultCfg = defaultCfg.Merge(config.ClientAgentConfigFromCommand(cmd))

			if errs := defaultCfg.Validate(); len(errs) > 0 {
				_, _ = cmd.ErrWriter.Write([]byte("Configuration Validation Errors:\n"))
				for _, err := range errs {
					_, _ = cmd.ErrWriter.Write([]byte("\t- " + err.Error() + "\n"))
				}
				os.Exit(1)
			}

			clienAgent, err := agent.NewClientAgent(defaultCfg)
			if err != nil {
				return err
			}

			if err := clienAgent.Start(); err != nil {
				return err
			}
			clienAgent.WaitForSignal()
			return nil
		},
	}
}
