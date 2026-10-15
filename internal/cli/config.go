package cli

import (
	"errors"

	"github.com/spf13/cobra"

	"qgate/internal/config"
)

const defaultStage = "dev"

// configFlags are the flags every command that resolves config accepts.
type configFlags struct {
	orgConfig  string
	stage      string
	configPath string
	sets       []string
}

func (f *configFlags) register(cmd *cobra.Command) {
	fl := cmd.Flags()
	fl.StringVar(&f.stage, "stage", defaultStage, "stage to run; selects the profile of the same name (env QGATE_STAGE)")
	fl.StringVar(&f.orgConfig, "org-config", "", "org config path or HTTPS URL, layered between builtin defaults and the repo config")
	fl.StringVar(&f.configPath, "config", "", "repo config path (env QGATE_CONFIG; default .qgate.yaml or .qgate.yml in the repo root)")
	fl.StringArrayVar(&f.sets, "set", nil, "override a resolved config value, e.g. --set profile.gate.overall_min=80 (repeatable)")
}

// load resolves the config. Flags beat environment variables, which beat
// defaults. Config problems become usage errors (exit 2).
func (a app) load(cmd *cobra.Command, f configFlags) (*config.Resolved, error) {
	stage := f.stage
	if !cmd.Flags().Changed("stage") {
		stage = envOr(a.inv.Env, "QGATE_STAGE", defaultStage)
	}
	configPath := f.configPath
	if !cmd.Flags().Changed("config") {
		configPath = a.inv.Env["QGATE_CONFIG"]
	}
	res, err := config.Load(cmd.Context(), config.Options{
		Dir:        a.inv.Dir,
		Env:        a.inv.Env,
		OrgConfig:  f.orgConfig,
		HTTP:       a.inv.HTTP,
		ConfigPath: configPath,
		Stage:      stage,
		Sets:       f.sets,
	})
	var cerrs config.Errors
	if errors.As(err, &cerrs) {
		return nil, usageError{err}
	}
	return res, err
}

func envOr(env map[string]string, key, fallback string) string {
	if v := env[key]; v != "" {
		return v
	}
	return fallback
}
