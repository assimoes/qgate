package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"qgate/internal/buildinfo"
	"qgate/internal/score"
)

func (a app) newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit, build date and scoring algorithm version",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			bi := buildinfo.Get()
			_, err := fmt.Fprintf(a.inv.Stdout,
				"version: %s\ncommit: %s\nbuild date: %s\nscoring algorithm: v%d\n",
				bi.Version, bi.Commit, bi.Date, score.AlgorithmVersion)
			return err
		},
	}
}
