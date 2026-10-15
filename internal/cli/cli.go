// Package cli is qgate's command-line layer and composition root. It is the
// only place that turns errors into process exit codes.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"qgate/internal/ingest"
	"qgate/internal/ingest/sarif"
)

// Invocation is everything a qgate process receives from its environment.
// Tests build one directly to run the CLI in-process.
type Invocation struct {
	Args   []string
	Env    map[string]string
	Dir    string // working directory; relative paths resolve against it
	Stdout io.Writer
	Stderr io.Writer
	// HTTP makes qgate's outbound HTTP requests; nil uses a default client.
	HTTP *http.Client
}

// defaultHTTPTimeout bounds each outbound request when the invocation did
// not supply its own client.
const defaultHTTPTimeout = 30 * time.Second

// app carries what every command needs.
type app struct {
	inv       Invocation
	logger    *slog.Logger
	ingestors *ingest.Registry
}

// builtinIngestors are the analyzer report formats this build reads.
func builtinIngestors() *ingest.Registry {
	return ingest.NewRegistry(sarif.New())
}

// Execute runs qgate with inv and returns the process exit code.
func Execute(ctx context.Context, inv Invocation) int {
	if inv.HTTP == nil {
		inv.HTTP = &http.Client{Timeout: defaultHTTPTimeout}
	}
	a := app{inv: inv, logger: slog.New(slog.NewTextHandler(inv.Stderr, nil)), ingestors: builtinIngestors()}
	root := a.newRootCommand()
	root.SetArgs(inv.Args)
	root.SetOut(inv.Stdout)
	root.SetErr(inv.Stderr)

	err := classify(root.ExecuteContext(ctx))
	if err != nil {
		// The final error is for a human reading the CI log, so it is
		// printed plainly rather than as a structured log record.
		fmt.Fprintf(inv.Stderr, "qgate: %v\n", err)
	}
	return exitCode(err)
}

func (a app) newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "qgate",
		Short:         "AI-assisted quality gate for CI/CD pipelines",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		// Without RunE, cobra would print help and exit 0 for a bare `qgate`.
		RunE: func(*cobra.Command, []string) error {
			return usageErrorf("missing command; see `qgate --help`")
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.SetHelpCommand(newHelpCommand(root))
	root.AddCommand(
		a.newVersionCommand(),
		a.newRunCommand(),
		a.newValidateCommand(),
	)
	wrapRunE(root)
	return root
}

// commandError marks an error returned from inside a running command, as
// opposed to cobra rejecting the command line before anything ran.
type commandError struct{ err error }

func (e commandError) Error() string { return e.err.Error() }
func (e commandError) Unwrap() error { return e.err }

// wrapRunE wraps every command's RunE so its errors are marked as
// commandErrors.
func wrapRunE(cmd *cobra.Command) {
	if run := cmd.RunE; run != nil {
		cmd.RunE = func(c *cobra.Command, args []string) error {
			if err := run(c, args); err != nil {
				return commandError{err}
			}
			return nil
		}
	}
	for _, sub := range cmd.Commands() {
		wrapRunE(sub)
	}
}

// classify turns cobra's own rejections (unknown command, flag or argument)
// into usage errors and leaves errors from running commands as they are.
func classify(err error) error {
	var ce commandError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &ce):
		return ce.err
	default:
		return usageError{err}
	}
}

// newHelpCommand replaces cobra's help command, which exits 0 for unknown
// topics, with one that treats them as usage errors.
func newHelpCommand(root *cobra.Command) *cobra.Command {
	return &cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		RunE: func(_ *cobra.Command, args []string) error {
			target, rest, err := root.Find(args)
			if err != nil || len(rest) > 0 {
				return usageErrorf("unknown help topic %q", args)
			}
			return target.Help()
		},
	}
}
