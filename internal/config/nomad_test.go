package config

import (
	"testing"

	"github.com/shoenig/test/must"
	"github.com/urfave/cli/v3"

	"github.com/rasorp/smuggle/internal/helper"
)

func Test_DefaultNomadConfig(t *testing.T) {
	cfg := DefaultNomadConfig()

	must.NotNil(t, cfg)
	must.Eq(t, "http://localhost:4646", cfg.Address)
	must.False(t, *cfg.SkipVerify)
	must.Eq(t, "", cfg.Token)
	must.Eq(t, "", cfg.CACert)
	must.Eq(t, "", cfg.CAPath)
	must.Eq(t, "", cfg.ClientCert)
	must.Eq(t, "", cfg.ClientKey)
	must.Eq(t, "", cfg.TLSServerName)
}

func TestNomadConfig_Merge(t *testing.T) {
	testCases := []struct {
		name     string
		base     *NomadConfig
		other    *NomadConfig
		expected *NomadConfig
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
			other:    DefaultNomadConfig(),
			expected: DefaultNomadConfig(),
		},
		{
			name:     "other nil",
			base:     DefaultNomadConfig(),
			other:    nil,
			expected: DefaultNomadConfig(),
		},
		{
			name: "both set",
			base: &NomadConfig{
				Address:       "http://localhost:4646",
				Token:         "base-token",
				CACert:        "/base/ca.crt",
				CAPath:        "/base/ca",
				ClientCert:    "/base/client.crt",
				ClientKey:     "/base/client.key",
				TLSServerName: "base-server",
				SkipVerify:    helper.PointerOf(false),
			},
			other: &NomadConfig{
				Address:       "https://nomad.example.com:4646",
				Token:         "other-token",
				CACert:        "/other/ca.crt",
				CAPath:        "/other/ca",
				ClientCert:    "/other/client.crt",
				ClientKey:     "/other/client.key",
				TLSServerName: "other-server",
				SkipVerify:    helper.PointerOf(true),
			},
			expected: &NomadConfig{
				Address:       "https://nomad.example.com:4646",
				Token:         "other-token",
				CACert:        "/other/ca.crt",
				CAPath:        "/other/ca",
				ClientCert:    "/other/client.crt",
				ClientKey:     "/other/client.key",
				TLSServerName: "other-server",
				SkipVerify:    helper.PointerOf(true),
			},
		},
		{
			name: "partial override",
			base: &NomadConfig{
				Address:    "http://localhost:4646",
				Token:      "base-token",
				SkipVerify: helper.PointerOf(false),
			},
			other: &NomadConfig{
				Token: "new-token",
			},
			expected: &NomadConfig{
				Address:    "http://localhost:4646",
				Token:      "new-token",
				SkipVerify: helper.PointerOf(false),
			},
		},
		{
			name: "empty string values don't override",
			base: &NomadConfig{
				Address: "http://localhost:4646",
				Token:   "my-token",
			},
			other: &NomadConfig{
				Address: "",
				Token:   "",
			},
			expected: &NomadConfig{
				Address: "http://localhost:4646",
				Token:   "my-token",
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

func TestNomadConfig_Validate(t *testing.T) {
	testCases := []struct {
		name          string
		config        *NomadConfig
		expectedError bool
	}{
		{
			name:          "valid config",
			config:        DefaultNomadConfig(),
			expectedError: false,
		},
		{
			name: "valid config with TLS",
			config: &NomadConfig{
				Address:       "https://nomad.example.com:4646",
				Token:         "my-token",
				CACert:        "/path/to/ca.crt",
				ClientCert:    "/path/to/client.crt",
				ClientKey:     "/path/to/client.key",
				TLSServerName: "nomad.example.com",
				SkipVerify:    helper.PointerOf(false),
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

func Test_NomadConfigCommandFlags(t *testing.T) {
	expectedFlags := []cli.Flag{
		&cli.StringFlag{
			HideDefault: true,
			Name:        nomadAddrFlag,
			Usage:       "The Nomad server address",
			Sources:     cli.EnvVars("NOMAD_ADDR"),
		},
		&cli.StringFlag{
			HideDefault: true,
			Name:        nomadTokenFlag,
			Usage:       "The Nomad ACL token to use for HTTP requests",
			Sources:     cli.EnvVars("NOMAD_TOKEN"),
		},
		&cli.StringFlag{
			HideDefault: true,
			Name:        nomadCACertFlag,
			Usage:       "Path to a PEM encoded CA cert file to use to verify the Nomad server SSL certificate",
			Sources:     cli.EnvVars("NOMAD_CACERT"),
		},
		&cli.StringFlag{
			HideDefault: true,
			Name:        nomadCAPathFlag,
			Usage:       "Path to a directory of PEM encoded CA cert files to verify the Nomad server SSL certificate",
			Sources:     cli.EnvVars("NOMAD_CAPATH"),
		},
		&cli.StringFlag{
			HideDefault: true,
			Name:        nomadClientCertFlag,
			Usage:       "Path to a PEM encoded client certificate for TLS authentication to the Nomad server",
			Sources:     cli.EnvVars("NOMAD_CLIENT_CERT"),
		},
		&cli.StringFlag{
			HideDefault: true,
			Name:        nomadClientKeyFlag,
			Usage:       "Path to an unencrypted PEM encoded private key matching the client certificate",
			Sources:     cli.EnvVars("NOMAD_CLIENT_KEY"),
		},
		&cli.StringFlag{
			HideDefault: true,
			Name:        nomadTLSServerNameFlag,
			Usage:       "The server name to use as the SNI host when connecting via TLS",
			Sources:     cli.EnvVars("NOMAD_TLS_SERVER_NAME"),
		},
		&cli.BoolFlag{
			HideDefault: true,
			Name:        nomadSkipVerifyFlag,
			Usage:       "Do not verify TLS certificate",
			Sources:     cli.EnvVars("NOMAD_SKIP_VERIFY"),
		},
	}
	must.Eq(t, expectedFlags, NomadConfigCommandFlags())
}

func Test_NomadConfigFromCommand(t *testing.T) {
	testCases := []struct {
		name     string
		setFlags func(*cli.Command)
		expected *NomadConfig
	}{
		{
			name:     "no flags",
			setFlags: func(_ *cli.Command) {},
			expected: &NomadConfig{},
		},
		{
			name: "all flags set",
			setFlags: func(cmd *cli.Command) {
				must.NoError(t, cmd.Set(nomadAddrFlag, "https://nomad.example.com:4646"))
				must.NoError(t, cmd.Set(nomadTokenFlag, "my-secret-token"))
				must.NoError(t, cmd.Set(nomadCACertFlag, "/etc/nomad/ca.crt"))
				must.NoError(t, cmd.Set(nomadCAPathFlag, "/etc/nomad/ca"))
				must.NoError(t, cmd.Set(nomadClientCertFlag, "/etc/nomad/client.crt"))
				must.NoError(t, cmd.Set(nomadClientKeyFlag, "/etc/nomad/client.key"))
				must.NoError(t, cmd.Set(nomadTLSServerNameFlag, "nomad.example.com"))
				must.NoError(t, cmd.Set(nomadSkipVerifyFlag, "true"))
			},
			expected: &NomadConfig{
				Address:       "https://nomad.example.com:4646",
				Token:         "my-secret-token",
				CACert:        "/etc/nomad/ca.crt",
				CAPath:        "/etc/nomad/ca",
				ClientCert:    "/etc/nomad/client.crt",
				ClientKey:     "/etc/nomad/client.key",
				TLSServerName: "nomad.example.com",
				SkipVerify:    helper.PointerOf(true),
			},
		},
		{
			name: "partial flags set",
			setFlags: func(cmd *cli.Command) {
				must.NoError(t, cmd.Set(nomadAddrFlag, "http://localhost:4646"))
				must.NoError(t, cmd.Set(nomadTokenFlag, "my-token"))
			},
			expected: &NomadConfig{
				Address: "http://localhost:4646",
				Token:   "my-token",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmdApp := &cli.Command{Flags: NomadConfigCommandFlags()}
			tc.setFlags(cmdApp)
			must.Eq(t, tc.expected, NomadConfigFromCommand(cmdApp))
		})
	}
}
