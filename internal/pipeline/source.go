package pipeline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/assimoes/qgate/internal/config"
)

const (
	// defaultInputTimeout bounds a run-mode command whose input sets no
	// timeout.
	defaultInputTimeout = 5 * time.Minute
	// maxCommandOutput bounds what qgate reads from a command's stdout.
	maxCommandOutput = 256 << 20
	// stderrTail is how much of a failed command's stderr an error quotes.
	stderrTail = 2 << 10
	// waitDelay is how long qgate waits for a timed-out command's output
	// pipes to close after killing it, in case a child process holds them.
	waitDelay = 5 * time.Second
)

// baseEnv are the variables every command inherits from qgate's
// environment; an input's env allowlist adds to them.
var baseEnv = [...]string{"PATH", "HOME", "TMPDIR"}

// readSource returns what an input's ingestor parses: the file at path for
// import mode, or the output of the command for run mode.
func readSource(ctx context.Context, deps InputDeps, in config.Input) ([]byte, []string, error) {
	switch in.Mode {
	case "", "import":
		data, err := os.ReadFile(resolve(deps.Dir, in.Path))
		if err != nil {
			return nil, nil, fmt.Errorf("open %s: %w", in.Path, err)
		}
		return data, nil, nil
	case "run":
		return runCommand(ctx, deps, in)
	default:
		return nil, nil, fmt.Errorf("input mode %q is not supported in this version", in.Mode)
	}
}

// runCommand executes the input's argv (no shell) in the repo root with a
// minimal environment and a timeout. A non-zero exit is not a failure by
// itself, because analyzers commonly exit 1 when they find issues; the
// input fails when the command cannot start, times out, or exits non-zero
// without output to parse.
func runCommand(ctx context.Context, deps InputDeps, in config.Input) ([]byte, []string, error) {
	if len(in.Command) == 0 {
		return nil, nil, errors.New("run mode needs a command")
	}
	timeout := time.Duration(in.Timeout)
	if timeout <= 0 {
		timeout = defaultInputTimeout
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	name := in.Command[0]
	if strings.Contains(name, "/") && !filepath.IsAbs(name) {
		name = filepath.Join(deps.Dir, name) // relative to the repo root, not qgate's working directory
	}
	cmd := exec.CommandContext(cctx, name, in.Command[1:]...)
	cmd.Dir = deps.Dir
	cmd.Env = commandEnv(deps.Env, in.Env)
	cmd.WaitDelay = waitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &limitedWriter{w: &stdout, n: maxCommandOutput}
	cmd.Stderr = &limitedWriter{w: &stderr, n: stderrTail * 8}

	err := cmd.Run()
	var exitErr *exec.ExitError
	switch {
	case cctx.Err() == context.DeadlineExceeded:
		return nil, nil, fmt.Errorf("command %s timed out after %s", in.Command[0], timeout)
	case ctx.Err() != nil:
		return nil, nil, ctx.Err()
	case errors.As(err, &exitErr):
		// Judged below, once we know whether there is output.
	case err != nil:
		return nil, nil, fmt.Errorf("run %s: %w", in.Command[0], err)
	}

	var warnings []string
	if code := cmd.ProcessState.ExitCode(); code != 0 {
		if in.Path == "" && stdout.Len() == 0 {
			return nil, nil, fmt.Errorf("command %s exited with status %d and no output: %s",
				in.Command[0], code, tail(stderr.String()))
		}
		warnings = append(warnings, fmt.Sprintf("command %s exited with status %d", in.Command[0], code))
	}
	if in.Path == "" {
		return stdout.Bytes(), warnings, nil
	}
	data, err := os.ReadFile(resolve(deps.Dir, in.Path))
	if err != nil {
		return nil, nil, fmt.Errorf("command %s ran but %s cannot be read: %w", in.Command[0], in.Path, err)
	}
	return data, warnings, nil
}

// commandEnv is the base variables and the allowlist, taken from qgate's
// own environment when set there.
func commandEnv(env map[string]string, allow []string) []string {
	var out []string
	for _, k := range append(baseEnv[:], allow...) {
		if v, ok := env[k]; ok {
			out = append(out, k+"="+v)
		}
	}
	return out
}

func resolve(dir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dir, p)
}

// tail returns the end of a command's stderr for an error message.
func tail(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > stderrTail {
		s = "…" + s[len(s)-stderrTail:]
	}
	if s == "" {
		return "(no stderr)"
	}
	return s
}

// limitedWriter keeps the first n bytes and discards the rest, so a
// runaway command cannot exhaust memory.
type limitedWriter struct {
	w io.Writer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		keep := p[:min(len(p), l.n)]
		if _, err := l.w.Write(keep); err != nil {
			return 0, err
		}
		l.n -= len(keep)
	}
	return len(p), nil
}
