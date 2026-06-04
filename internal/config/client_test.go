package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/shoenig/test/must"
	"github.com/urfave/cli/v3"

	"github.com/rasorp/smuggle/internal/helper"
)

func Test_DefaultClientConfig(t *testing.T) {
	defaults := DefaultClientConfig()

	must.NotNil(t, defaults)
	must.Eq(t, "/var/lib/smuggle/client", defaults.DataDir)
	must.False(t, *defaults.DisableIPMasq)
	must.Eq(t, "", defaults.NetworkInterface)
}

func TestClientConfig_Merge(t *testing.T) {
	testCases := []struct {
		name     string
		base     *ClientConfig
		other    *ClientConfig
		expected *ClientConfig
	}{
		{
			name:     "nil base",
			base:     nil,
			other:    &ClientConfig{DataDir: "/custom/dir"},
			expected: &ClientConfig{DataDir: "/custom/dir"},
		},
		{
			name:  "true overrides false",
			base:  &ClientConfig{DataDir: "/base/dir", DisableIPMasq: helper.PointerOf(false)},
			other: &ClientConfig{DataDir: "/other/dir", DisableIPMasq: helper.PointerOf(true)},
			expected: &ClientConfig{
				DataDir:       "/other/dir",
				DisableIPMasq: helper.PointerOf(true),
			},
		},
		{
			// This is the case the original bool type made impossible.
			name:  "false overrides true",
			base:  &ClientConfig{DataDir: "/base/dir", DisableIPMasq: helper.PointerOf(true)},
			other: &ClientConfig{DataDir: "/other/dir", DisableIPMasq: helper.PointerOf(false)},
			expected: &ClientConfig{
				DataDir:       "/other/dir",
				DisableIPMasq: helper.PointerOf(false),
			},
		},
		{
			name:  "nil DisableIPMasq does not override",
			base:  &ClientConfig{DataDir: "/base/dir", DisableIPMasq: helper.PointerOf(true)},
			other: &ClientConfig{DataDir: "/other/dir"},
			expected: &ClientConfig{
				DataDir:       "/other/dir",
				DisableIPMasq: helper.PointerOf(true),
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			must.Eq(t, tc.expected, tc.base.Merge(tc.other))
		})
	}
}

func TestClientConfig_Validate(t *testing.T) {

	if runtime.GOOS != "linux" {
		t.Skip("skipping as client does not run on linux")
	}

	testCases := []struct {
		name          string
		config        *ClientConfig
		expectedError bool
	}{
		{
			name: "valid config",
			config: &ClientConfig{
				DataDir: "/valid/dir",
			},
			expectedError: false,
		},
		{
			name: "empty data dir",
			config: &ClientConfig{
				DataDir: "",
			},
			expectedError: true,
		},
		{
			name: "non-absolute data dir",
			config: &ClientConfig{
				DataDir: "~/my-lovely-horse",
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := tc.config.Validate()
			if tc.expectedError {
				must.Greater(t, 0, len(errs), must.Sprintf("errs: %v", errs))
			} else {
				must.Len(t, 0, errs, must.Sprintf("errs: %v", errs))
			}
		})
	}
}

func Test_ClientConfigCommandFlags(t *testing.T) {
	expectedFlags := []cli.Flag{
		&cli.StringSliceFlag{
			HideDefault: true,
			Name:        serversFlag,
			Usage:       "Smuggle server addresses (host:port) to connect to for store operations",
			Sources:     cli.EnvVars("SMUGGLE_SERVERS"),
		},
		&cli.StringFlag{
			HideDefault: true,
			Name:        dataDirFlag,
			Usage:       "The data directory for the Smuggle client",
			Sources:     cli.EnvVars("SMUGGLE_DATA_DIR"),
		},
		&cli.BoolFlag{
			HideDefault: true,
			Name:        disableIPMasqFlag,
			Usage:       "Disable IP masquerading for client networks",
			Sources:     cli.EnvVars("SMUGGLE_DISABLE_IPMASQ"),
		},
		&cli.StringFlag{
			HideDefault: true,
			Name:        networkInterfaceFlag,
			Usage:       "The network interface to use for client networking",
			Sources:     cli.EnvVars("SMUGGLE_NETWORK_INTERFACE"),
		},
	}
	must.Eq(t, expectedFlags, ClientConfigCommandFlags())
}

func Test_ClientConfigFromComand(t *testing.T) {
	testCases := []struct {
		name     string
		setFlags func(*cli.Command)
		expected *ClientConfig
	}{
		{
			name:     "no flags",
			setFlags: func(_ *cli.Command) {},
			expected: &ClientConfig{
				Servers: &ServersConfig{},
			},
		},
		{
			name: "all flags",
			setFlags: func(cmd *cli.Command) {
				must.NoError(t, cmd.Set(dataDirFlag, "/custom/dir"))
				must.NoError(t, cmd.Set(disableIPMasqFlag, "true"))
				must.NoError(t, cmd.Set(networkInterfaceFlag, "eth0"))
			},
			expected: &ClientConfig{
				DataDir:          "/custom/dir",
				DisableIPMasq:    helper.PointerOf(true),
				NetworkInterface: "eth0",
				Servers:          &ServersConfig{},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmdApp := &cli.Command{Flags: ClientConfigCommandFlags()}
			tc.setFlags(cmdApp)
			must.Eq(t, tc.expected, ClientConfigFromCommand(cmdApp))
		})
	}
}

func Test_DefaultClientAgentConfig(t *testing.T) {
	cfg := DefaultClientAgentConfig()

	must.NotNil(t, cfg)
	must.Eq(t, DefaultClientConfig(), cfg.Client)
	must.Eq(t, DefaultHTTPConfig(), cfg.HTTP)
	must.Eq(t, DefaultLogConfig(), cfg.Log)
}

func TestClientAgentConfig_Merge(t *testing.T) {
	testCases := []struct {
		name     string
		base     *ClientAgentConfig
		other    *ClientAgentConfig
		expected *ClientAgentConfig
	}{
		{
			name:     "both nil",
			base:     nil,
			other:    nil,
			expected: nil,
		},
		{
			name:     "base nil",
			base:     nil,
			other:    DefaultClientAgentConfig(),
			expected: DefaultClientAgentConfig(),
		},
		{
			name:     "other nil",
			base:     DefaultClientAgentConfig(),
			other:    nil,
			expected: DefaultClientAgentConfig(),
		},
		{
			name: "other overrides base",
			base: &ClientAgentConfig{
				Client: &ClientConfig{DataDir: "/var/lib/smuggle/client"},
				HTTP:   &HTTPConfig{Port: 9090},
				Log:    &LogConfig{Level: "info"},
			},
			other: &ClientAgentConfig{
				Client: &ClientConfig{DataDir: "/data/smuggle", NetworkInterface: "eth0"},
				HTTP:   &HTTPConfig{Port: 8080},
				Log:    &LogConfig{Level: "debug"},
			},
			expected: &ClientAgentConfig{
				Client: &ClientConfig{DataDir: "/data/smuggle", NetworkInterface: "eth0"},
				HTTP:   &HTTPConfig{Port: 8080},
				Log:    &LogConfig{Level: "debug"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := tc.base.Merge(tc.other)
			must.Eq(t, tc.expected, result)
		})
	}
}

func TestClientAgentConfig_Validate(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("skipping as client does not run on linux")
	}

	testCases := []struct {
		name          string
		config        *ClientAgentConfig
		expectedError bool
	}{
		{
			name:          "valid default config",
			config:        DefaultClientAgentConfig(),
			expectedError: false,
		},
		{
			name: "invalid client config",
			config: &ClientAgentConfig{
				Client: &ClientConfig{DataDir: "relative/path"},
				HTTP:   DefaultHTTPConfig(),
				Log:    DefaultLogConfig(),
			},
			expectedError: true,
		},
		{
			name: "invalid http config",
			config: &ClientAgentConfig{
				Client: DefaultClientConfig(),
				HTTP: &HTTPConfig{
					Enabled:        helper.PointerOf(false),
					DebugEnabled:   helper.PointerOf(true),
					Address:        "localhost",
					AccessLogLevel: "debug",
					Port:           9090,
				},
				Log: DefaultLogConfig(),
			},
			expectedError: true,
		},
		{
			name: "invalid log config",
			config: &ClientAgentConfig{
				Client: DefaultClientConfig(),
				HTTP:   DefaultHTTPConfig(),
				Log:    &LogConfig{Level: "invalid-level"},
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			errs := tc.config.Validate()
			if tc.expectedError {
				must.Greater(t, 0, len(errs), must.Sprintf("errs: %v", errs))
			} else {
				must.Len(t, 0, errs, must.Sprintf("errs: %v", errs))
			}
		})
	}
}

func Test_ClientAgentConfigCommandFlags(t *testing.T) {
	expected := []cli.Flag{
		&cli.StringSliceFlag{
			Name:  "config",
			Usage: "The path to a configuration file to load",
		},
	}
	expected = append(expected, ClientConfigCommandFlags()...)
	expected = append(expected, HTTPConfigCommandFlags()...)
	expected = append(expected, LogConfigCommandFlags()...)

	must.Eq(t, expected, ClientAgentConfigCommandFlags())
}

func Test_ClientAgentConfigFromCommand(t *testing.T) {
	testCases := []struct {
		name     string
		setFlags func(*cli.Command)
		expected *ClientAgentConfig
	}{
		{
			name:     "no flags",
			setFlags: func(_ *cli.Command) {},
			expected: &ClientAgentConfig{
				Client: &ClientConfig{Servers: &ServersConfig{}},
				HTTP:   &HTTPConfig{},
				Log:    &LogConfig{},
			},
		},
		{
			name: "one flag per sub-config",
			setFlags: func(cmd *cli.Command) {
				must.NoError(t, cmd.Set(dataDirFlag, "/data/smuggle"))
				must.NoError(t, cmd.Set(httpPortFlag, "8080"))
				must.NoError(t, cmd.Set(logLevalFlag, "debug"))
			},
			expected: &ClientAgentConfig{
				Client: &ClientConfig{DataDir: "/data/smuggle", Servers: &ServersConfig{}},
				HTTP:   &HTTPConfig{Port: 8080},
				Log:    &LogConfig{Level: "debug"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmdApp := &cli.Command{Flags: ClientAgentConfigCommandFlags()}
			tc.setFlags(cmdApp)
			must.Eq(t, tc.expected, ClientAgentConfigFromCommand(cmdApp))
		})
	}
}

func Test_ClientAgentConfigFromFiles(t *testing.T) {
	validHCL := `
client {
  data_dir          = "/var/lib/smuggle/client"
  network_interface = "eth0"
}
log {
  level = "debug"
}
`

	validJSON := `{
  "log": { "level": "warn" }
}`

	secondHCL := `
log {
  level = "warn"
}
`

	malformedHCL := `this is not { valid hcl <<<`

	testCases := []struct {
		name          string
		setFlags      func(*cli.Command, string)
		expected      *ClientAgentConfig
		expectedError bool
	}{
		{
			name:     "no config files",
			setFlags: func(_ *cli.Command, _ string) {},
			expected: nil,
		},
		{
			name: "valid hcl file",
			setFlags: func(cmd *cli.Command, dir string) {
				f := filepath.Join(dir, "config.hcl")
				must.NoError(t, os.WriteFile(f, []byte(validHCL), 0600))
				must.NoError(t, cmd.Set("config", f))
			},
			expected: &ClientAgentConfig{
				Client: &ClientConfig{
					DataDir:          "/var/lib/smuggle/client",
					NetworkInterface: "eth0",
				},
				Log: &LogConfig{Level: "debug"},
			},
		},
		{
			name: "valid json file",
			setFlags: func(cmd *cli.Command, dir string) {
				f := filepath.Join(dir, "config.json")
				must.NoError(t, os.WriteFile(f, []byte(validJSON), 0600))
				must.NoError(t, cmd.Set("config", f))
			},
			expected: &ClientAgentConfig{
				Log: &LogConfig{Level: "warn"},
			},
		},
		{
			name: "multiple files merged",
			setFlags: func(cmd *cli.Command, dir string) {
				f1 := filepath.Join(dir, "config1.hcl")
				must.NoError(t, os.WriteFile(f1, []byte(validHCL), 0600))
				must.NoError(t, cmd.Set("config", f1))

				f2 := filepath.Join(dir, "config2.hcl")
				must.NoError(t, os.WriteFile(f2, []byte(secondHCL), 0600))
				must.NoError(t, cmd.Set("config", f2))
			},
			expected: &ClientAgentConfig{
				Client: &ClientConfig{
					DataDir:          "/var/lib/smuggle/client",
					NetworkInterface: "eth0",
				},
				Log: &LogConfig{Level: "warn"},
			},
		},
		{
			name: "non-existent file",
			setFlags: func(cmd *cli.Command, dir string) {
				must.NoError(t, cmd.Set("config", filepath.Join(dir, "does-not-exist.hcl")))
			},
			expectedError: true,
		},
		{
			name: "unsupported file extension",
			setFlags: func(cmd *cli.Command, dir string) {
				f := filepath.Join(dir, "config.toml")
				must.NoError(t, os.WriteFile(f, []byte{}, 0600))
				must.NoError(t, cmd.Set("config", f))
			},
			expectedError: true,
		},
		{
			name: "malformed hcl file",
			setFlags: func(cmd *cli.Command, dir string) {
				f := filepath.Join(dir, "config.hcl")
				must.NoError(t, os.WriteFile(f, []byte(malformedHCL), 0600))
				must.NoError(t, cmd.Set("config", f))
			},
			expectedError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmdApp := &cli.Command{Flags: ClientAgentConfigCommandFlags()}
			tc.setFlags(cmdApp, t.TempDir())

			result, err := ClientAgentConfigFromFiles(cmdApp)
			if tc.expectedError {
				must.Error(t, err)
			} else {
				must.NoError(t, err)
				must.Eq(t, tc.expected, result)
			}
		})
	}
}
