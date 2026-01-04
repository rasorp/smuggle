package config

import (
	"os"
	"slices"
	"testing"
	"time"

	"github.com/shoenig/test/must"
	"github.com/urfave/cli/v3"

	"github.com/rasorp/smuggle/internal/helper"
)

func Test_DefaultAgentConfig(t *testing.T) {
	cfg := DefaultAgentConfig()

	must.Eq(t, DefaultClientConfig(), cfg.Client)
	must.Eq(t, DefaultHTTPConfig(), cfg.HTTP)
	must.Eq(t, DefaultLogConfig(), cfg.Log)
	must.Eq(t, DefaultNomadConfig(), cfg.Nomad)
	must.Eq(t, DefaultServerConfig(), cfg.Server)
	must.Eq(t, DefaultStoreConfig(), cfg.Store)
}

func TestAgentConfig_Merge(t *testing.T) {
	testCases := []struct {
		name     string
		base     *AgentConfig
		other    *AgentConfig
		expected *AgentConfig
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
			other:    DefaultAgentConfig(),
			expected: DefaultAgentConfig(),
		},
		{
			name:     "other nil",
			base:     DefaultAgentConfig(),
			other:    nil,
			expected: DefaultAgentConfig(),
		},
		{
			name: "merge client config",
			base: &AgentConfig{
				Client: &ClientConfig{
					Enabled: helper.PointerOf(true),
					DataDir: "/base/dir",
				},
				HTTP:   DefaultHTTPConfig(),
				Log:    DefaultLogConfig(),
				Nomad:  DefaultNomadConfig(),
				Server: DefaultServerConfig(),
				Store:  DefaultStoreConfig(),
			},
			other: &AgentConfig{
				Client: &ClientConfig{
					DataDir: "/other/dir",
				},
			},
			expected: &AgentConfig{
				Client: &ClientConfig{
					Enabled: helper.PointerOf(true),
					DataDir: "/other/dir",
				},
				HTTP:   DefaultHTTPConfig(),
				Log:    DefaultLogConfig(),
				Nomad:  DefaultNomadConfig(),
				Server: DefaultServerConfig(),
				Store:  DefaultStoreConfig(),
			},
		},
		{
			name: "merge all configs",
			base: DefaultAgentConfig(),
			other: &AgentConfig{
				Client: &ClientConfig{
					Enabled: helper.PointerOf(true),
					DataDir: "/custom/dir",
				},
				HTTP: &HTTPConfig{
					Port: 8080,
				},
				Log: &LogConfig{
					Level: "debug",
				},
				Nomad: &NomadConfig{
					Token: "my-token",
				},
				Server: &ServerConfig{
					Enabled: helper.PointerOf(true),
				},
				Store: &StoreConfig{
					Backend: "nvar",
				},
			},
			expected: &AgentConfig{
				Client: &ClientConfig{
					Enabled:       helper.PointerOf(true),
					DataDir:       "/custom/dir",
					DisableIPMasq: false,
				},
				HTTP: &HTTPConfig{
					Enabled:        helper.PointerOf(true),
					DebugEnabled:   helper.PointerOf(false),
					Address:        "localhost",
					AccessLogLevel: "debug",
					Port:           8080,
				},
				Log: &LogConfig{
					Level:            "debug",
					JSON:             helper.PointerOf(false),
					IncludeLine:      helper.PointerOf(false),
					EnableStacktrace: helper.PointerOf(false),
				},
				Nomad: &NomadConfig{
					Address:    "http://localhost:4646",
					Token:      "my-token",
					SkipVerify: helper.PointerOf(false),
				},
				Server: DefaultServerConfig().Merge(&ServerConfig{
					Enabled: helper.PointerOf(true),
				}),
				Store: &StoreConfig{
					Backend: "nvar",
					NVar:    &StoreNVarConfig{Path: "smuggle/"},
				},
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

func TestAgentConfig_Validate(t *testing.T) {
	testCases := []struct {
		name          string
		config        *AgentConfig
		expectedError bool
	}{
		{
			name: "valid server config",
			config: &AgentConfig{
				Client: DefaultClientConfig(),
				HTTP:   DefaultHTTPConfig(),
				Log:    DefaultLogConfig(),
				Nomad:  DefaultNomadConfig(),
				Server: &ServerConfig{
					Enabled: helper.PointerOf(true),
				},
				Store: DefaultStoreConfig(),
			},
			expectedError: false,
		},
		{
			name: "both client and server enabled",
			config: &AgentConfig{
				Client: &ClientConfig{
					Enabled: helper.PointerOf(true),
					DataDir: "/var/lib/smuggle/client",
				},
				HTTP:  DefaultHTTPConfig(),
				Log:   DefaultLogConfig(),
				Nomad: DefaultNomadConfig(),
				Server: &ServerConfig{
					Enabled: helper.PointerOf(true),
				},
				Store: DefaultStoreConfig(),
			},
			expectedError: true,
		},
		{
			name: "neither client nor server enabled",
			config: &AgentConfig{
				Client: DefaultClientConfig(),
				HTTP:   DefaultHTTPConfig(),
				Log:    DefaultLogConfig(),
				Nomad:  DefaultNomadConfig(),
				Server: DefaultServerConfig(),
				Store:  DefaultStoreConfig(),
			},
			expectedError: true,
		},
		{
			name: "invalid log level",
			config: &AgentConfig{
				Client: &ClientConfig{
					Enabled: helper.PointerOf(true),
					DataDir: "/var/lib/smuggle/client",
				},
				HTTP: DefaultHTTPConfig(),
				Log: &LogConfig{
					Level: "invalid",
				},
				Nomad:  DefaultNomadConfig(),
				Server: DefaultServerConfig(),
				Store:  DefaultStoreConfig(),
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

func Test_AgentConfigCommandFlags(t *testing.T) {
	flags := AgentConfigCommandFlags()

	must.NotNil(t, flags)

	actual := slices.ContainsFunc(
		flags,
		func(f cli.Flag) bool {
			return f.Names()[0] == "config"
		},
	)
	must.True(t, actual)
}

func Test_AgentConfigFromCommand(t *testing.T) {
	testCases := []struct {
		name     string
		setFlags func(*cli.Command)
		expected *AgentConfig
	}{
		{
			name:     "no flags",
			setFlags: func(_ *cli.Command) {},
			expected: &AgentConfig{
				Client: &ClientConfig{},
				HTTP:   &HTTPConfig{},
				Log:    &LogConfig{},
				Nomad:  &NomadConfig{},
				Server: &ServerConfig{
					Reaper: &ReaperConfig{},
				},
				Store: &StoreConfig{
					NVar: &StoreNVarConfig{},
				},
			},
		},
		{
			name: "all flags",
			setFlags: func(cmd *cli.Command) {
				must.NoError(t, cmd.Set(clientEnabledFlag, "true"))
				must.NoError(t, cmd.Set(clientDataDirFlag, "/opt/smuggle/subnet"))
				must.NoError(t, cmd.Set(clientDisableIPMasqFlag, "true"))
				must.NoError(t, cmd.Set(clientNetworkInterfaceFlag, "eth0"))

				must.NoError(t, cmd.Set(httpEnabledFlag, "true"))
				must.NoError(t, cmd.Set(httpAddressFlag, "192.168.130.191"))
				must.NoError(t, cmd.Set(httpPortFlag, "1234"))
				must.NoError(t, cmd.Set(httpAccessLogLevelFlag, "info"))
				must.NoError(t, cmd.Set(httpEnableDebugFlag, "true"))

				must.NoError(t, cmd.Set(logLevalFlag, "warn"))
				must.NoError(t, cmd.Set(logJSONFlag, "true"))
				must.NoError(t, cmd.Set(logIncludeLineFlag, "true"))
				must.NoError(t, cmd.Set(logEnableStacktraceFlag, "true"))

				must.NoError(t, cmd.Set(nomadAddrFlag, "https://nomad.example.com:4646"))
				must.NoError(t, cmd.Set(nomadTokenFlag, "test-token-123"))
				must.NoError(t, cmd.Set(nomadCACertFlag, "/etc/nomad/ca.pem"))
				must.NoError(t, cmd.Set(nomadCAPathFlag, "/etc/nomad/ca-dir"))
				must.NoError(t, cmd.Set(nomadClientCertFlag, "/etc/nomad/client.pem"))
				must.NoError(t, cmd.Set(nomadClientKeyFlag, "/etc/nomad/client-key.pem"))
				must.NoError(t, cmd.Set(nomadTLSServerNameFlag, "server.nomad.example.com"))
				must.NoError(t, cmd.Set(nomadSkipVerifyFlag, "true"))

				must.NoError(t, cmd.Set(serverEnabledFlag, "true"))
				must.NoError(t, cmd.Set(serverReaperIntervalFlag, "10m"))
				must.NoError(t, cmd.Set(serverReaperThresholdFlag, "15m"))

				must.NoError(t, cmd.Set(storeBackendFlag, "nvar"))
				must.NoError(t, cmd.Set(storeNVarPathFlag, "custom/path/"))
			},
			expected: &AgentConfig{
				Client: &ClientConfig{
					Enabled:          helper.PointerOf(true),
					DataDir:          "/opt/smuggle/subnet",
					DisableIPMasq:    true,
					NetworkInterface: "eth0",
				},
				HTTP: &HTTPConfig{
					Enabled:        helper.PointerOf(true),
					Address:        "192.168.130.191",
					Port:           1234,
					AccessLogLevel: "info",
					DebugEnabled:   helper.PointerOf(true),
				},
				Log: &LogConfig{
					Level:            "warn",
					JSON:             helper.PointerOf(true),
					IncludeLine:      helper.PointerOf(true),
					EnableStacktrace: helper.PointerOf(true),
				},
				Nomad: &NomadConfig{
					Address:       "https://nomad.example.com:4646",
					Token:         "test-token-123",
					CACert:        "/etc/nomad/ca.pem",
					CAPath:        "/etc/nomad/ca-dir",
					ClientCert:    "/etc/nomad/client.pem",
					ClientKey:     "/etc/nomad/client-key.pem",
					TLSServerName: "server.nomad.example.com",
					SkipVerify:    helper.PointerOf(true),
				},
				Server: &ServerConfig{
					Enabled: helper.PointerOf(true),
					Reaper: &ReaperConfig{
						Interval:  10 * time.Minute,
						Threshold: 15 * time.Minute,
					},
				},
				Store: &StoreConfig{
					Backend: "nvar",
					NVar: &StoreNVarConfig{
						Path: "custom/path/",
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmdApp := &cli.Command{Flags: AgentConfigCommandFlags()}
			tc.setFlags(cmdApp)
			must.Eq(t, tc.expected, AgentConfigFromCommand(cmdApp))
		})
	}
}

func Test_ParseAgentConfgigFile(t *testing.T) {
	testCases := []struct {
		name        string
		fileContent string
		fileExt     string
		expected    *AgentConfig
		expectError bool
	}{
		{
			name: "single HCL parameter",
			fileContent: `
server {
  enabled = true
}`,
			fileExt: "hcl",
			expected: &AgentConfig{
				Server: &ServerConfig{
					Enabled: helper.PointerOf(true),
				},
			},
			expectError: false,
		},
		{
			name: "single JSON parameter",
			fileContent: `
{
  "server": {
	"enabled": true
  }
}`,
			fileExt: "json",
			expected: &AgentConfig{
				Server: &ServerConfig{
					Enabled: helper.PointerOf(true),
				},
			},
			expectError: false,
		},
		{
			name: "full HCL config",
			fileContent: `
client {
  enabled            = true
  data_dir           = "/var/lib/smuggle/client"
  disable_ipmasq     = false
  network_interface  = "eth0"
}

http {
  enabled           = true
  debug_enabled     = false
  address           = "0.0.0.0"
  access_log_level  = "info"
  port              = 9090
}

log {
  level             = "debug"
  json              = true
  include_line      = false
  enable_stacktrace = false
}

nomad {
  address         = "https://nomad.example.com:4646"
  token           = "secret-token-123"
  ca_cert         = "/etc/nomad/ca.pem"
  ca_path         = "/etc/nomad/ca-dir"
  client_cert     = "/etc/nomad/client.pem"
  client_key      = "/etc/nomad/client-key.pem"
  tls_server_name = "server.nomad.example.com"
  skip_verify     = true
}

server {
  enabled = false
  reaper {
    interval  = "5m"
    threshold = "10m"
  }
}

store {
  backend = "nvar"
  nvar {
    path = "smuggle/production/"
  }
}
`,
			fileExt: "hcl",
			expected: &AgentConfig{
				Client: &ClientConfig{
					Enabled:          helper.PointerOf(true),
					DataDir:          "/var/lib/smuggle/client",
					DisableIPMasq:    false,
					NetworkInterface: "eth0",
				},
				HTTP: &HTTPConfig{
					Enabled:        helper.PointerOf(true),
					DebugEnabled:   helper.PointerOf(false),
					Address:        "0.0.0.0",
					AccessLogLevel: "info",
					Port:           9090,
				},
				Log: &LogConfig{
					Level:            "debug",
					JSON:             helper.PointerOf(true),
					IncludeLine:      helper.PointerOf(false),
					EnableStacktrace: helper.PointerOf(false),
				},
				Nomad: &NomadConfig{
					Address:       "https://nomad.example.com:4646",
					Token:         "secret-token-123",
					CACert:        "/etc/nomad/ca.pem",
					CAPath:        "/etc/nomad/ca-dir",
					ClientCert:    "/etc/nomad/client.pem",
					ClientKey:     "/etc/nomad/client-key.pem",
					TLSServerName: "server.nomad.example.com",
					SkipVerify:    helper.PointerOf(true),
				},
				Server: &ServerConfig{
					Enabled: helper.PointerOf(false),
					Reaper: &ReaperConfig{
						IntervalHCL:  "5m",
						Interval:     5 * time.Minute,
						ThresholdHCL: "10m",
						Threshold:    10 * time.Minute,
					},
				},
				Store: &StoreConfig{
					Backend: "nvar",
					NVar: &StoreNVarConfig{
						Path: "smuggle/production/",
					},
				},
			},
			expectError: false,
		},
		{
			name: "full JSON config",
			fileContent: `
			{
  "client": {
    "enabled": true,
    "data_dir": "/var/lib/smuggle/client",
    "disable_ipmasq": false,
    "network_interface": "eth0"
  },
  "http": {
    "enabled": true,
    "debug_enabled": false,
    "address": "0.0.0.0",
    "access_log_level": "info",
    "port": 9090
  },
  "log": {
    "level": "debug",
    "json": true,
    "include_line": false,
    "enable_stacktrace": false
  },
  "nomad": {
    "address": "https://nomad.example.com:4646",
    "token": "secret-token-123",
    "ca_cert": "/etc/nomad/ca.pem",
    "ca_path": "/etc/nomad/ca-dir",
    "client_cert": "/etc/nomad/client.pem",
    "client_key": "/etc/nomad/client-key.pem",
    "tls_server_name": "server.nomad.example.com",
    "skip_verify": true
  },
  "server": {
    "enabled": false,
    "reaper": {
      "interval": "5m",
      "threshold": "10m"
    }
  },
  "store": {
    "backend": "nvar",
    "nvar": {
      "path": "smuggle/production/"
    }
  }
}
`,
			fileExt: "json",
			expected: &AgentConfig{
				Client: &ClientConfig{
					Enabled:          helper.PointerOf(true),
					DataDir:          "/var/lib/smuggle/client",
					DisableIPMasq:    false,
					NetworkInterface: "eth0",
				},
				HTTP: &HTTPConfig{
					Enabled:        helper.PointerOf(true),
					DebugEnabled:   helper.PointerOf(false),
					Address:        "0.0.0.0",
					AccessLogLevel: "info",
					Port:           9090,
				},
				Log: &LogConfig{
					Level:            "debug",
					JSON:             helper.PointerOf(true),
					IncludeLine:      helper.PointerOf(false),
					EnableStacktrace: helper.PointerOf(false),
				},
				Nomad: &NomadConfig{
					Address:       "https://nomad.example.com:4646",
					Token:         "secret-token-123",
					CACert:        "/etc/nomad/ca.pem",
					CAPath:        "/etc/nomad/ca-dir",
					ClientCert:    "/etc/nomad/client.pem",
					ClientKey:     "/etc/nomad/client-key.pem",
					TLSServerName: "server.nomad.example.com",
					SkipVerify:    helper.PointerOf(true),
				},
				Server: &ServerConfig{
					Enabled: helper.PointerOf(false),
					Reaper: &ReaperConfig{
						IntervalHCL:  "5m",
						Interval:     5 * time.Minute,
						ThresholdHCL: "10m",
						Threshold:    10 * time.Minute,
					},
				},
				Store: &StoreConfig{
					Backend: "nvar",
					NVar: &StoreNVarConfig{
						Path: "smuggle/production/",
					},
				},
			},
			expectError: false,
		},
		{
			name: "invalid HCL config",
			fileContent: `
client {
  disable_ipmasq = "not-a-bool"
}`,
			fileExt:     "hcl",
			expected:    nil,
			expectError: true,
		},
		{
			name: "invalid JSON config",
			fileContent: `
{
  "client": {
	"disable_ipmasq": "not-a-bool"
  }
}`,
			fileExt:     "json",
			expected:    nil,
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {

			tmpFile, err := os.CreateTemp(t.TempDir(), "config-*."+tc.fileExt)
			must.NoError(t, err)
			t.Cleanup(func() { _ = os.Remove(tmpFile.Name()) })

			_, err = tmpFile.WriteString(tc.fileContent)
			must.NoError(t, err)
			must.NoError(t, tmpFile.Close())

			result, err := parseAgentConfgigFile(tmpFile.Name())

			if tc.expectError {
				must.Error(t, err)
			} else {
				must.NoError(t, err)
				must.Eq(t, tc.expected, result)
			}
		})
	}
}
