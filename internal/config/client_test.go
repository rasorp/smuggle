package config

import (
	"runtime"
	"testing"

	"github.com/shoenig/test/must"
	"github.com/urfave/cli/v3"

	"github.com/rasorp/smuggle/internal/helper"
)

func Test_DefaultClientConfig(t *testing.T) {
	defaults := DefaultClientConfig()

	must.NotNil(t, defaults)
	must.False(t, *defaults.Enabled)
	must.Eq(t, "/var/lib/smuggle/client", defaults.DataDir)
	must.False(t, defaults.DisableIPMasq)
	must.Eq(t, "", defaults.NetworkInterface)
}

func TestClientConfig_IsEnabled(t *testing.T) {
	testCases := []struct {
		name     string
		config   *ClientConfig
		expected bool
	}{
		{
			name:     "nil config",
			config:   nil,
			expected: false,
		},
		{
			name:     "enabled nil",
			config:   &ClientConfig{Enabled: nil},
			expected: false,
		},
		{
			name:     "enabled false",
			config:   &ClientConfig{Enabled: helper.PointerOf(false)},
			expected: false,
		},
		{
			name:     "enabled true",
			config:   &ClientConfig{Enabled: helper.PointerOf(true)},
			expected: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			must.Eq(t, tc.expected, tc.config.IsEnabled())
		})
	}
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
			name:  "override fields",
			base:  &ClientConfig{DataDir: "/base/dir", DisableIPMasq: false},
			other: &ClientConfig{DataDir: "/other/dir", DisableIPMasq: true},
			expected: &ClientConfig{
				DataDir:       "/other/dir",
				DisableIPMasq: true,
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
				Enabled: helper.PointerOf(true),
				DataDir: "/valid/dir",
			},
			expectedError: false,
		},
		{
			name: "empty data dir",
			config: &ClientConfig{
				Enabled: helper.PointerOf(true),
				DataDir: "",
			},
			expectedError: true,
		},
		{
			name: "non-absolute data dir",
			config: &ClientConfig{
				Enabled: helper.PointerOf(true),
				DataDir: "~/my-lovely-horse",
			},
			expectedError: true,
		},
		{
			name: "client disabled",
			config: &ClientConfig{
				Enabled: helper.PointerOf(false),
				DataDir: "~/my-lovely-horse",
			},
			expectedError: false,
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
		&cli.BoolFlag{
			HideDefault: true,
			Name:        clientEnabledFlag,
			Usage:       "Enable or disable the Smuggle client functionality",
			Sources:     cli.EnvVars("SMUGGLE_CLIENT_ENABLED"),
		},
		&cli.StringFlag{
			HideDefault: true,
			Name:        clientDataDirFlag,
			Usage:       "The data directory for the Smuggle client",
			Sources:     cli.EnvVars("SMUGGLE_CLIENT_DATA_DIR"),
		},
		&cli.BoolFlag{
			HideDefault: true,
			Name:        clientDisableIPMasqFlag,
			Usage:       "Disable IP masquerading for client networks",
			Sources:     cli.EnvVars("SMUGGLE_CLIENT_DISABLE_IPMASQ"),
		},
		&cli.StringFlag{
			HideDefault: true,
			Name:        clientNetworkInterfaceFlag,
			Usage:       "The network interface to use for client networking",
			Sources:     cli.EnvVars("SMUGGLE_CLIENT_NETWORK_INTERFACE"),
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
			expected: &ClientConfig{},
		},
		{
			name: "all flags",
			setFlags: func(cmd *cli.Command) {
				must.NoError(t, cmd.Set(clientEnabledFlag, "true"))
				must.NoError(t, cmd.Set(clientDataDirFlag, "/custom/dir"))
				must.NoError(t, cmd.Set(clientDisableIPMasqFlag, "true"))
				must.NoError(t, cmd.Set(clientNetworkInterfaceFlag, "eth0"))
			},
			expected: &ClientConfig{
				Enabled:          helper.PointerOf(true),
				DataDir:          "/custom/dir",
				DisableIPMasq:    true,
				NetworkInterface: "eth0",
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
