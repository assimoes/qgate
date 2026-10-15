// Package cli is qgate's command-line layer and composition root. It is the
// only place that turns errors into process exit codes.
package cli

import (
	"context"
	"io"
	"log/slog"

	"github.com/spf13/cobra"
)

// Invocation is everything a qgate process receives from its environment.
// Tests build one directly to run the CLI in-process.
type Invocation struct {
	Args   []string
	Env    map[string]string
	Dir    string // working directory; relative paths resolve against it
	Stdout io.Writer
	Stderr io.Writer
}

// Execute runs qgate with inv and returns the process exit code.
func Execute(ctx context.Context, inv Invocation) int {
	logger := slog.New(slog.NewTextHandler(inv.Stderr, nil))
	root := newRootCommand(inv, logger)
	root.SetArgs(inv.Args)
	root.SetOut(inv.Stdout)
	root.SetErr(inv.Stderr)

	// Cobra rejects unknown commands, flags and arguments before any
	// command starts; every such error is a usage error.
	started := false
	root.PersistentPreRun = func(*cobra.Command, []string) { started = true }

	err := root.ExecuteContext(ctx)
	if err != nil && !started {
		err = usageError{err}
	}
	if err != nil {
		logger.Error(err.Error())
	}
	return exitCode(err)
}

func newRootCommand(inv Invocation, logger *slog.Logger) *cobra.Command {
	root := &cobra.Command{
		Use:           "qgate",
		Short:         "AI-assisted quality gate for CI/CD pipelines",
		SilenceUsage:  true,
		SilenceErrors: true,
		// Cobra treats an unknown subcommand of a command without Run as a
		// help request; returning an error makes it a usage error instead.
		RunE: func(cmd *cobra.Command, args []string) error {
			return usageErrorf("missing command; see `qgate --help`")
		},
		Args: cobra.NoArgs,
	}
	root.AddCommand(
		newVersionCommand(inv),
		newRunCommand(inv, logger),
	)
	return root
}
