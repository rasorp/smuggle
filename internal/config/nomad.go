package config

import (
	"github.com/hashicorp/nomad/api"
	"github.com/urfave/cli/v3"

	"github.com/rasorp/smuggle/internal/helper"
)

const (
	nomadAddrFlag          = "nomad-addr"
	nomadTokenFlag         = "nomad-token"
	nomadCACertFlag        = "nomad-ca-cert"
	nomadCAPathFlag        = "nomad-ca-path"
	nomadClientCertFlag    = "nomad-client-cert"
	nomadClientKeyFlag     = "nomad-client-key"
	nomadTLSServerNameFlag = "nomad-tls-server-name"
	nomadSkipVerifyFlag    = "nomad-skip-verify"
)

type NomadConfig struct {
	Address       string `hcl:"address" json:"address"`
	Token         string `hcl:"token" json:"token"`
	CACert        string `hcl:"ca_cert" json:"ca_cert"`
	CAPath        string `hcl:"ca_path" json:"ca_path"`
	ClientCert    string `hcl:"client_cert" json:"client_cert"`
	ClientKey     string `hcl:"client_key" json:"client_key"`
	TLSServerName string `hcl:"tls_server_name" json:"tls_server_name"`
	SkipVerify    *bool  `hcl:"skip_verify" json:"skip_verify"`
}

func DefaultNomadConfig() *NomadConfig {
	return &NomadConfig{
		Address:    "http://localhost:4646",
		SkipVerify: helper.PointerOf(false),
	}
}

func (n *NomadConfig) Merge(other *NomadConfig) *NomadConfig {
	if n == nil {
		return other
	}
	if other == nil {
		return n
	}

	result := *n

	if other.Address != "" {
		result.Address = other.Address
	}
	if other.Token != "" {
		result.Token = other.Token
	}
	if other.CACert != "" {
		result.CACert = other.CACert
	}
	if other.CAPath != "" {
		result.CAPath = other.CAPath
	}
	if other.ClientCert != "" {
		result.ClientCert = other.ClientCert
	}
	if other.ClientKey != "" {
		result.ClientKey = other.ClientKey
	}
	if other.TLSServerName != "" {
		result.TLSServerName = other.TLSServerName
	}
	if other.SkipVerify != nil {
		result.SkipVerify = other.SkipVerify
	}

	return &result
}

func (n *NomadConfig) Validate() []error {
	var errs []error
	return errs
}

func NomadConfigCommandFlags() []cli.Flag {
	return []cli.Flag{
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
}

func NomadConfigFromCommand(cmd *cli.Command) *NomadConfig {
	return &NomadConfig{
		Address:       cmd.String(nomadAddrFlag),
		Token:         cmd.String(nomadTokenFlag),
		CACert:        cmd.String(nomadCACertFlag),
		CAPath:        cmd.String(nomadCAPathFlag),
		ClientCert:    cmd.String(nomadClientCertFlag),
		ClientKey:     cmd.String(nomadClientKeyFlag),
		TLSServerName: cmd.String(nomadTLSServerNameFlag),
		SkipVerify: func() *bool {
			if cmd.IsSet(nomadSkipVerifyFlag) {
				val := cmd.Bool(nomadSkipVerifyFlag)
				return &val
			}
			return nil
		}(),
	}
}

func NomadClient(cfg *NomadConfig) (*api.Client, error) {

	nomadConfig := api.DefaultConfig()
	if cfg != nil {
		if cfg.Address != "" {
			nomadConfig.Address = cfg.Address
		}
		if cfg.Token != "" {
			nomadConfig.SecretID = cfg.Token
		}
		if cfg.CACert != "" {
			nomadConfig.TLSConfig.CACert = cfg.CACert
		}
		if cfg.CAPath != "" {
			nomadConfig.TLSConfig.CAPath = cfg.CAPath
		}
		if cfg.ClientCert != "" {
			nomadConfig.TLSConfig.ClientCert = cfg.ClientCert
		}
		if cfg.ClientKey != "" {
			nomadConfig.TLSConfig.ClientKey = cfg.ClientKey
		}
		if cfg.TLSServerName != "" {
			nomadConfig.TLSConfig.TLSServerName = cfg.TLSServerName
		}
		if cfg.SkipVerify != nil {
			nomadConfig.TLSConfig.Insecure = *cfg.SkipVerify
		}
	}

	return api.NewClient(nomadConfig)
}
