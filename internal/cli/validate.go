package cli

import (
	"bytes"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func (a app) newValidateCommand() *cobra.Command {
	var flags configFlags
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the configuration and print it resolved for a stage (no network)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := a.load(cmd, flags)
			if err != nil {
				return err
			}
			source := "builtin defaults only"
			if res.RepoConfig != "" {
				source = res.RepoConfig
				if rel, err := filepath.Rel(a.inv.Dir, source); err == nil && filepath.IsLocal(rel) {
					source = rel
				}
			}
			var out bytes.Buffer
			fmt.Fprintf(&out, "# qgate resolved config for stage %q from %s\n", res.Config.Stage, source)
			enc := yaml.NewEncoder(&out)
			enc.SetIndent(2)
			if err := enc.Encode(res.Document); err != nil {
				return fmt.Errorf("encode resolved config: %w", err)
			}
			if err := enc.Close(); err != nil {
				return fmt.Errorf("encode resolved config: %w", err)
			}
			_, err = a.inv.Stdout.Write(out.Bytes())
			return err
		},
	}
	flags.register(cmd)
	return cmd
}
