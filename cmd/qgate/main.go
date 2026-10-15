// Command qgate is the quality gate CLI. All behavior lives in internal/cli;
// main only adapts the process environment to an Invocation.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"qgate/internal/cli"
)

func main() {
	os.Exit(run())
}

func run() int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "qgate: working directory: %v\n", err)
		return cli.ExitFatalRuntime
	}
	return cli.Execute(ctx, cli.Invocation{
		Args:   os.Args[1:],
		Env:    environ(),
		Dir:    dir,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	})
}

func environ() map[string]string {
	env := make(map[string]string)
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			env[k] = v
		}
	}
	return env
}
