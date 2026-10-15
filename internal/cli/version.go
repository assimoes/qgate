package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"qgate/internal/buildinfo"
	"qgate/internal/score"
)

func newVersionCommand(inv Invocation) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version, commit, build date and scoring algorithm version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			bi := buildinfo.Get()
			_, err := fmt.Fprintf(inv.Stdout,
				"version: %s\ncommit: %s\nbuild date: %s\nscoring algorithm: v%d\n",
				bi.Version, bi.Commit, bi.Date, score.AlgorithmVersion)
			return err
		},
	}
}
