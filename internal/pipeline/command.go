package pipeline

import (
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
	// maxCommandOutput bounds a command's stdout. Output goes to a
	// temporary file and is streamed to the ingestor, so this protects
	// the disk and the parser, not memory.
	maxCommandOutput = 256 << 20
	// stderrTail is how much of the end of a command's stderr an error
	// quotes.
	stderrTail = 2 << 10
)

// baseEnv are the variables every command inherits from qgate's
// environment; an input's env allowlist adds to them. Their values are
// not secrets and are never redacted.
var baseEnv = [...]string{"PATH", "HOME", "TMPDIR"}

// inputData is what an input's ingestor parses, and what explains a
// parse failure.
type inputData struct {
	io.ReadCloser
	warnings []string
	stderr   string // a command's redacted stderr tail; empty for import mode
}

// openInput returns what an input's ingestor parses: the file at path for
// import mode, or what the command produced for run mode. The caller
// closes it.
func openInput(ctx context.Context, deps InputDeps, in config.Input) (inputData, error) {
	switch in.Mode {
	case config.InputModeImport, "":
		f, err := os.Open(resolve(deps.Dir, in.Path))
		if err != nil {
			return inputData{}, fmt.Errorf("open %s: %w", in.Path, err)
		}
		return inputData{ReadCloser: f}, nil
	case config.InputModeRun:
		return runCommand(ctx, deps, in)
	default:
		return inputData{}, fmt.Errorf("input mode %q is not supported in this version", in.Mode)
	}
}

// commandRun is one execution of a run-mode command and what it left.
type commandRun struct {
	in       config.Input
	stdout   *os.File // rewound; nil once handed to the caller
	stderr   *os.File
	state    *os.ProcessState
	redact   func(string) string
	timeout  time.Duration
	pathPrev time.Time // path's modification time before the run; zero if absent
}

// runCommand executes the input's argv (no shell) in the repo root with a
// minimal environment, in its own process group, under a timeout. The
// whole group is killed on timeout and once the command exits, so no
// child outlives the input. A non-zero exit is not a failure by itself,
// because analyzers commonly exit 1 when they find issues; see judge.
func runCommand(ctx context.Context, deps InputDeps, in config.Input) (inputData, error) {
	if len(in.Command) == 0 {
		return inputData{}, errors.New("run mode needs a command")
	}
	r := &commandRun{in: in, timeout: time.Duration(in.Timeout)}
	if r.timeout <= 0 {
		r.timeout = defaultInputTimeout
	}
	r.redact = redactor(deps.Env, in.Env)
	if in.Path != "" {
		if fi, err := os.Stat(resolve(deps.Dir, in.Path)); err == nil {
			r.pathPrev = fi.ModTime()
		}
	}

	var err error
	if r.stdout, err = os.CreateTemp("", "qgate-stdout-*"); err != nil {
		return inputData{}, fmt.Errorf("capture output of %s: %w", in.Command[0], err)
	}
	if r.stderr, err = os.CreateTemp("", "qgate-stderr-*"); err != nil {
		r.close()
		return inputData{}, fmt.Errorf("capture output of %s: %w", in.Command[0], err)
	}
	defer r.close() // the stdout file survives only if handed to the caller

	if err := r.execute(ctx, deps); err != nil {
		return inputData{}, err
	}
	return r.judge(deps)
}

// execute runs the command to completion, then kills what is left of its
// process group.
func (r *commandRun) execute(ctx context.Context, deps InputDeps) error {
	runCtx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, r.in.Command[0], r.in.Command[1:]...)
	cmd.Dir = deps.Dir // a relative command path resolves against it
	cmd.Env = commandEnv(deps.Env, r.in.Env)
	cmd.Stdout, cmd.Stderr = r.stdout, r.stderr // files: Wait does not wait for children holding them
	ownProcessGroup(cmd)

	err := cmd.Run()
	killProcessGroup(cmd)
	r.state = cmd.ProcessState

	var exitErr *exec.ExitError
	switch {
	case ctx.Err() != nil: // the whole run was cancelled or ran out of time
		return fmt.Errorf("command %s: %w", r.in.Command[0], ctx.Err())
	case errors.Is(runCtx.Err(), context.DeadlineExceeded):
		return fmt.Errorf("command %s timed out after %s", r.in.Command[0], r.timeout)
	case errors.As(err, &exitErr):
		return nil // judged from the exit status and output
	case err != nil:
		return fmt.Errorf("run %s: %w", r.in.Command[0], err)
	}
	return nil
}

// judge decides from the exit status and what the command wrote whether
// the input has something to parse.
func (r *commandRun) judge(deps InputDeps) (inputData, error) {
	name := r.in.Command[0]
	code := r.state.ExitCode()
	stderr := r.stderrTail()
	if code < 0 {
		return inputData{}, fmt.Errorf("command %s was killed by signal (%s): %s", name, r.state, stderr)
	}
	var warnings []string
	if code != 0 {
		warnings = append(warnings, fmt.Sprintf("command %s exited with status %d", name, code))
	}

	if r.in.Path != "" {
		p := resolve(deps.Dir, r.in.Path)
		fi, err := os.Stat(p)
		if err != nil || !fi.ModTime().After(r.pathPrev) {
			return inputData{}, fmt.Errorf("%s was not written by command %s (exit status %d): %s", r.in.Path, name, code, stderr)
		}
		f, err := os.Open(p)
		if err != nil {
			return inputData{}, fmt.Errorf("open %s: %w", r.in.Path, err)
		}
		return inputData{ReadCloser: f, warnings: warnings, stderr: stderr}, nil
	}

	fi, err := r.stdout.Stat()
	if err != nil {
		return inputData{}, fmt.Errorf("read output of %s: %w", name, err)
	}
	switch {
	case fi.Size() > maxCommandOutput:
		return inputData{}, fmt.Errorf("output of %s exceeds %d MiB", name, maxCommandOutput>>20)
	case fi.Size() == 0 && code != 0:
		return inputData{}, fmt.Errorf("command %s exited with status %d and no output: %s", name, code, stderr)
	}
	if _, err := r.stdout.Seek(0, io.SeekStart); err != nil {
		return inputData{}, fmt.Errorf("read output of %s: %w", name, err)
	}
	out := tempFile{r.stdout}
	r.stdout = nil // ownership passes to the caller
	return inputData{ReadCloser: out, warnings: warnings, stderr: stderr}, nil
}

// stderrTail is the redacted end of the command's stderr, for errors.
func (r *commandRun) stderrTail() string {
	fi, err := r.stderr.Stat()
	if err != nil {
		return "(stderr unreadable)"
	}
	start := max(fi.Size()-stderrTail, 0)
	buf := make([]byte, fi.Size()-start)
	if _, err := r.stderr.ReadAt(buf, start); err != nil && !errors.Is(err, io.EOF) {
		return "(stderr unreadable)"
	}
	s := strings.TrimSpace(r.redact(string(buf)))
	switch {
	case s == "":
		return "(no stderr)"
	case start > 0:
		return "…" + s
	default:
		return s
	}
}

func (r *commandRun) close() {
	for _, f := range []*os.File{r.stdout, r.stderr} {
		if f != nil {
			f.Close()
			os.Remove(f.Name())
		}
	}
}

// tempFile is a temporary file that closing removes.
type tempFile struct{ *os.File }

// Close closes and removes the file.
func (f tempFile) Close() error {
	err := f.File.Close()
	if rmErr := os.Remove(f.Name()); err == nil {
		err = rmErr
	}
	return err
}

// commandEnv is the base variables and the allowlist, taken from qgate's
// own environment when set there, each once.
func commandEnv(env map[string]string, allow []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, k := range append(baseEnv[:], allow...) {
		if v, ok := env[k]; ok && !seen[k] {
			seen[k] = true
			out = append(out, k+"="+v)
		}
	}
	return out
}

// redactor replaces the values of the allowlisted variables a command was
// given with [REDACTED:<NAME>], so a command echoing its credentials never
// puts them into a report. Very short values are left alone: replacing
// them would mangle unrelated text.
func redactor(env map[string]string, allow []string) func(string) string {
	var pairs []string
	for _, k := range allow {
		if v := env[k]; len(v) >= 4 {
			pairs = append(pairs, v, "[REDACTED:"+k+"]")
		}
	}
	if len(pairs) == 0 {
		return func(s string) string { return s }
	}
	return strings.NewReplacer(pairs...).Replace
}

func resolve(dir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(dir, p)
}
