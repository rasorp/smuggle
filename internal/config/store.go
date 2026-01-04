package config

import (
	"errors"
	"fmt"

	"github.com/urfave/cli/v3"
)

const (
	storeBackendFlag  = "store-backend"
	storeNVarPathFlag = "store-nvar-path"
)

type StoreConfig struct {
	Backend string           `hcl:"backend" json:"backend"`
	NVar    *StoreNVarConfig `hcl:"nvar,block" json:"nvar"`
}

type StoreNVarConfig struct {
	Path string `hcl:"path" json:"path"`
}

func DefaultStoreConfig() *StoreConfig {
	return &StoreConfig{
		Backend: "nvar",
		NVar: &StoreNVarConfig{
			Path: "smuggle/",
		},
	}
}

func (s *StoreConfig) Merge(other *StoreConfig) *StoreConfig {
	if s == nil {
		return other
	}
	if other == nil {
		return s
	}

	result := *s

	if other.Backend != "" {
		result.Backend = other.Backend
	}
	if other.NVar != nil {
		if result.NVar == nil {
			result.NVar = &StoreNVarConfig{}
		}
		if other.NVar.Path != "" {
			result.NVar.Path = other.NVar.Path
		}
	}

	return &result
}

func (s *StoreConfig) Validate() []error {
	var errs []error

	switch s.Backend {
	case "nvar":
		if s.NVar == nil {
			errs = append(errs, errors.New("nvar backend set without nvar configuration"))
		} else if s.NVar.Path == "" {
			errs = append(errs, errors.New("nvar backend requires path to be set"))
		}
	default:
		errs = append(errs, fmt.Errorf("unsupported backend: %q", s.Backend))
	}

	return errs
}

func StoreConfigCommandFlags() []cli.Flag {
	return []cli.Flag{
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
}

func StoreConfigFromCommand(cmd *cli.Command) *StoreConfig {
	return &StoreConfig{
		Backend: cmd.String(storeBackendFlag),
		NVar: &StoreNVarConfig{
			Path: cmd.String(storeNVarPathFlag),
		},
	}
}
