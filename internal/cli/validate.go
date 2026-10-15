package cli

import (
	"bytes"
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func (a app) newValidateCommand() *cobra.Command {
	var flags configFlags
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the configuration and print it resolved for a stage (network only to fetch an --org-config URL)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := a.load(cmd, flags)
			if err != nil {
				return err
			}
			var out bytes.Buffer
			source := res.Source
			if res.OrgConfig != "" {
				source += " over org config " + res.OrgConfig
			}
			fmt.Fprintf(&out, "# qgate resolved config for stage %q from %s\n", res.Config.Stage, source)
			enc := yaml.NewEncoder(&out)
			enc.SetIndent(2)
			if err := enc.Encode(res.Document); err != nil {
				return fmt.Errorf("encode resolved config: %w", err)
			}
			if err := enc.Close(); err != nil {
				return fmt.Errorf("encode resolved config: %w", err)
			}
			if len(res.Locked) > 0 {
				fmt.Fprintf(&out, "\n# Locked by the org config %s:\n", res.OrgConfig)
				for _, k := range res.Locked {
					fmt.Fprintf(&out, "#   %s = %s (set by %s)\n", k.Key, k.FormatValue(), k.SetBy)
				}
			}
			_, err = a.inv.Stdout.Write(out.Bytes())
			return err
		},
	}
	flags.register(cmd)
	return cmd
}
