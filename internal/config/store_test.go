package config

import (
	"testing"

	"github.com/shoenig/test/must"
	"github.com/urfave/cli/v3"
)

func Test_DefaultStoreConfig(t *testing.T) {
	cfg := DefaultStoreConfig()

	must.Eq(t, "nvar", cfg.Backend)
	must.NotNil(t, cfg.NVar)
	must.Eq(t, "smuggle/", cfg.NVar.Path)
}

func TestStoreConfig_Merge(t *testing.T) {
	testCases := []struct {
		name     string
		base     *StoreConfig
		other    *StoreConfig
		expected *StoreConfig
	}{
		{
			name:     "both nil",
			base:     nil,
			other:    nil,
			expected: nil,
		},
		{
			name: "base nil",
			base: nil,
			other: &StoreConfig{
				Backend: "nvar",
				NVar: &StoreNVarConfig{
					Path: "my-path",
				},
			},
			expected: &StoreConfig{
				Backend: "nvar",
				NVar: &StoreNVarConfig{
					Path: "my-path",
				},
			},
		},
		{
			name: "other nil",
			base: &StoreConfig{
				Backend: "nvar",
				NVar: &StoreNVarConfig{
					Path: "my-path",
				},
			},
			other: nil,
			expected: &StoreConfig{
				Backend: "nvar",
				NVar: &StoreNVarConfig{
					Path: "my-path",
				},
			},
		},
		{
			name: "full",
			base: &StoreConfig{
				Backend: "nvar",
				NVar: &StoreNVarConfig{
					Path: "my-path",
				},
			},
			other: &StoreConfig{
				Backend: "",
				NVar: &StoreNVarConfig{
					Path: "my-new-path",
				},
			},
			expected: &StoreConfig{
				Backend: "nvar",
				NVar: &StoreNVarConfig{
					Path: "my-new-path",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			must.Eq(t, tc.expected, tc.base.Merge(tc.other))
		})
	}
}

func TestStoreConfig_Validate(t *testing.T) {
	testCases := []struct {
		name          string
		config        *StoreConfig
		expectedError bool
	}{
		{
			name: "valid nvar config",
			config: &StoreConfig{
				Backend: "nvar",
				NVar: &StoreNVarConfig{
					Path: "smuggle-platform/",
				},
			},
			expectedError: false,
		},
		{
			name: "empty nvar path",
			config: &StoreConfig{
				Backend: "nvar",
				NVar: &StoreNVarConfig{
					Path: "",
				},
			},
			expectedError: true,
		},
		{
			name: "unsupported backend",
			config: &StoreConfig{
				Backend: "etcd",
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

func Test_StoreConfigCommandFlags(t *testing.T) {
	expectedFlags := []cli.Flag{
		&cli.StringFlag{
			HideDefault: true,
			Name:        storeBackendFlag,
			Usage:       "The backend to use for storing network configuration",
			Sources:     cli.EnvVars("SMUGGLE_STORE_BACKEND"),
		},
		&cli.StringFlag{
			HideDefault: true,
			Name:        storeNVarPathFlag,
			Usage:       "The path prefix to use when storing network configuration in Nomad variables",
			Sources:     cli.EnvVars("SMUGGLE_STORE_NVAR_PATH"),
		},
	}
	must.Eq(t, expectedFlags, StoreConfigCommandFlags())
}

func Test_StoreConfigFromCommand(t *testing.T) {
	testCases := []struct {
		name     string
		setFlags func(*cli.Command)
		expected *StoreConfig
	}{
		{
			name:     "no flags",
			setFlags: func(_ *cli.Command) {},
			expected: &StoreConfig{
				Backend: "",
				NVar:    &StoreNVarConfig{},
			},
		},
		{
			name: "all flags",
			setFlags: func(cmd *cli.Command) {
				must.NoError(t, cmd.Set(storeBackendFlag, "nvar"))
				must.NoError(t, cmd.Set(storeNVarPathFlag, "my-path"))
			},
			expected: &StoreConfig{
				Backend: "nvar",
				NVar: &StoreNVarConfig{
					Path: "my-path",
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			cmdApp := &cli.Command{Flags: StoreConfigCommandFlags()}
			tc.setFlags(cmdApp)
			must.Eq(t, tc.expected, StoreConfigFromCommand(cmdApp))
		})
	}
}
