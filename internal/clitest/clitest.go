// Package clitest is the Seam A test harness: it invokes the qgate CLI
// in-process and captures everything a CI job would observe.
package clitest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"qgate/internal/cli"
)

// Result is what one CLI invocation produced.
type Result struct {
	Code   int
	Stdout string
	Stderr string
	// Report is the raw JSON report at cli.DefaultReportPath, or nil if none was written.
	Report []byte
}

// Options customize an invocation. The zero value runs in a fresh temp
// directory with an empty environment.
type Options struct {
	Dir string
	Env map[string]string
}

// Run invokes the CLI with args and returns what it produced.
func Run(t *testing.T, opts Options, args ...string) Result {
	t.Helper()
	dir := opts.Dir
	if dir == "" {
		dir = t.TempDir()
	}
	var stdout, stderr bytes.Buffer
	code := cli.Execute(context.Background(), cli.Invocation{
		Args:   args,
		Env:    opts.Env,
		Dir:    dir,
		Stdout: &stdout,
		Stderr: &stderr,
	})
	res := Result{Code: code, Stdout: stdout.String(), Stderr: stderr.String()}
	data, err := os.ReadFile(filepath.Join(dir, cli.DefaultReportPath))
	switch {
	case err == nil:
		res.Report = data
	case !errors.Is(err, fs.ErrNotExist):
		t.Fatalf("read report: %v", err)
	}
	return res
}

// DecodeReport unmarshals the report into a generic map for assertions.
func (r Result) DecodeReport(t *testing.T) map[string]any {
	t.Helper()
	if r.Report == nil {
		t.Fatalf("no report was written; stderr:\n%s", r.Stderr)
	}
	var m map[string]any
	if err := json.Unmarshal(r.Report, &m); err != nil {
		t.Fatalf("report is not valid JSON: %v", err)
	}
	return m
}
