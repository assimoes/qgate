// Package gitctx resolves what a run reviews: the base and head commits and
// the lines the change added or modified (design §7). It shells out to git.
package gitctx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/sourcegraph/go-diff/diff"
)

// Errors that mean qgate was invoked somewhere it cannot work, as opposed to
// git failing unexpectedly.
var (
	ErrNotARepository = errors.New("not inside a git repository")
	ErrNoBase         = errors.New("cannot determine the base commit: pass --base, or fetch origin so its default branch is known")
)

// FileStatus is how a file changed between base and head.
type FileStatus string

// File statuses.
const (
	StatusAdded    FileStatus = "added"
	StatusModified FileStatus = "modified"
	StatusRenamed  FileStatus = "renamed"
	StatusDeleted  FileStatus = "deleted"
)

// LineRange is an inclusive range of 1-based lines on the head side.
type LineRange struct {
	Start, End int
}

// ChangedFile is one file the change touched.
type ChangedFile struct {
	Path        string // head-side path, repo-relative with forward slashes
	OldPath     string // base-side path when renamed or deleted
	Status      FileStatus
	Binary      bool
	AddedRanges []LineRange // added or modified lines; none for binary or deleted files
}

// Context is what a run reviews.
type Context struct {
	RepoRoot string
	BaseSHA  string
	HeadSHA  string
	Files    []ChangedFile // sorted by path
	// Workspaces are absolute directories analyzers may have prefixed
	// paths with: the repo root as git and as the caller spell it (they
	// differ through symlinks) and the CI workspace.
	Workspaces []string

	git   git
	index map[string]int
}

// Options select the commits and files to review.
type Options struct {
	Dir     string            // any directory inside the repository
	Base    string            // commit-ish; empty to use the merge base with origin's default branch
	Head    string            // commit-ish; empty for HEAD
	Env     map[string]string // environment git runs with
	Include []string          // project.include globs
	Exclude []string          // project.exclude globs
}

// Resolve builds the review context.
func Resolve(ctx context.Context, opts Options) (*Context, error) {
	g := git{dir: opts.Dir, env: gitEnv(opts.Env)}
	root, err := g.run(ctx, "rev-parse", "--show-toplevel")
	var ge *gitError
	switch {
	case errors.As(err, &ge) && strings.Contains(ge.stderr, "not a git repository"):
		return nil, ErrNotARepository
	case err != nil:
		return nil, fmt.Errorf("find repository root: %w", err)
	}
	g.dir = root

	head := opts.Head
	if head == "" {
		head = "HEAD"
	}
	headSHA, err := g.commit(ctx, head)
	if err != nil {
		return nil, fmt.Errorf("resolve head %q: %w", head, err)
	}

	baseSHA := ""
	if opts.Base != "" {
		if baseSHA, err = g.commit(ctx, opts.Base); err != nil {
			return nil, fmt.Errorf("resolve base %q: %w", opts.Base, err)
		}
	} else {
		def, err := g.run(ctx, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
		if err != nil || def == "" {
			return nil, ErrNoBase
		}
		if baseSHA, err = g.run(ctx, "merge-base", headSHA, def); err != nil {
			return nil, fmt.Errorf("merge base of %s and %s: %w", head, def, err)
		}
	}

	out, err := g.output(ctx, "-c", "core.quotePath=off", "diff", "--no-color", "--no-ext-diff",
		"--unified=0", "--find-renames", baseSHA+"..."+headSHA)
	if err != nil {
		return nil, fmt.Errorf("diff %s...%s: %w", baseSHA, headSHA, err)
	}
	files, err := parseDiff(out)
	if err != nil {
		return nil, err
	}

	c := &Context{RepoRoot: root, BaseSHA: baseSHA, HeadSHA: headSHA, git: g, index: map[string]int{},
		Workspaces: workspaces(root, opts.Dir, opts.Env)}
	for _, f := range files {
		if Selected(f.Path, opts.Include, opts.Exclude) {
			c.index[f.Path] = len(c.Files)
			c.Files = append(c.Files, f)
		}
	}
	return c, nil
}

// File returns the changed file at a head-side path.
func (c *Context) File(path string) (ChangedFile, bool) {
	i, ok := c.index[path]
	if !ok {
		return ChangedFile{}, false
	}
	return c.Files[i], true
}

// FileLines returns a file's lines at the head commit, or nil when the file
// does not exist there. Any other git failure is an error.
func (c *Context) FileLines(ctx context.Context, path string) ([]string, error) {
	listed, err := c.git.run(ctx, "ls-tree", "--name-only", c.HeadSHA, "--", path)
	if err != nil {
		return nil, fmt.Errorf("look up %s at head: %w", path, err)
	}
	if listed != path {
		return nil, nil
	}
	out, err := c.git.output(ctx, "show", c.HeadSHA+":"+path)
	if err != nil {
		return nil, fmt.Errorf("read %s at head: %w", path, err)
	}
	return strings.Split(strings.TrimSuffix(string(out), "\n"), "\n"), nil
}

// workspaces lists the absolute prefixes analyzers may put on paths.
func workspaces(root, dir string, env map[string]string) []string {
	ws := []string{root}
	add := func(p string) {
		if p != "" && !slices.Contains(ws, p) {
			ws = append(ws, p)
		}
	}
	if abs, err := filepath.Abs(dir); err == nil {
		add(abs)
		if real, err := filepath.EvalSymlinks(abs); err == nil {
			add(real)
		}
	}
	for _, k := range []string{"GITHUB_WORKSPACE", "CI_PROJECT_DIR"} {
		add(env[k])
	}
	return ws
}

// Selected reports whether path passes the project's include and exclude
// globs. Invalid globs were rejected when the config was loaded.
func Selected(path string, include, exclude []string) bool {
	if len(include) > 0 && !matchAny(include, path) {
		return false
	}
	return !matchAny(exclude, path)
}

func matchAny(globs []string, path string) bool {
	for _, g := range globs {
		if ok, _ := doublestar.Match(g, path); ok {
			return true
		}
	}
	return false
}

// parseDiff turns a --unified=0 diff into changed files. With no context
// lines, every hunk's new side is exactly the added or modified lines.
func parseDiff(out []byte) ([]ChangedFile, error) {
	fds, err := diff.ParseMultiFileDiff(out)
	if err != nil {
		return nil, fmt.Errorf("parse diff: %w", err)
	}
	var files []ChangedFile
	for _, fd := range fds {
		f := ChangedFile{Status: StatusModified}
		oldPath := strings.TrimPrefix(fd.OrigName, "a/")
		newPath := strings.TrimPrefix(fd.NewName, "b/")
		for _, ext := range fd.Extended {
			switch {
			case strings.HasPrefix(ext, "new file mode"):
				f.Status = StatusAdded
			case strings.HasPrefix(ext, "deleted file mode"):
				f.Status = StatusDeleted
			case strings.HasPrefix(ext, "rename from"):
				f.Status = StatusRenamed
			case strings.HasPrefix(ext, "Binary files"), ext == "GIT binary patch":
				f.Binary = true
			}
		}
		switch f.Status {
		case StatusDeleted:
			f.Path, f.OldPath = oldPath, oldPath
		case StatusRenamed:
			f.Path, f.OldPath = newPath, oldPath
		default:
			f.Path = newPath
		}
		if f.Status != StatusDeleted && !f.Binary {
			for _, h := range fd.Hunks {
				if h.NewLines > 0 {
					start := int(h.NewStartLine)
					f.AddedRanges = append(f.AddedRanges, LineRange{Start: start, End: start + int(h.NewLines) - 1})
				}
			}
		}
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// git runs git commands in one repository.
type git struct {
	dir string
	env []string
}

// gitError is git exiting unsuccessfully, with what it printed.
type gitError struct {
	command string
	err     error
	stderr  string
}

func (e *gitError) Error() string {
	if e.stderr != "" {
		return fmt.Sprintf("git %s: %v: %s", e.command, e.err, e.stderr)
	}
	return fmt.Sprintf("git %s: %v", e.command, e.err)
}

func (e *gitError) Unwrap() error { return e.err }

func (g git) output(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = g.dir
	cmd.Env = g.env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, fmt.Errorf("git %s: %w", args[0], ctxErr)
		}
		return nil, &gitError{command: args[0], err: err, stderr: strings.TrimSpace(stderr.String())}
	}
	return out, nil
}

func (g git) run(ctx context.Context, args ...string) (string, error) {
	out, err := g.output(ctx, args...)
	return strings.TrimSpace(string(out)), err
}

func (g git) commit(ctx context.Context, ref string) (string, error) {
	return g.run(ctx, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
}

// gitEnv is the environment git runs with: the caller's HOME and PATH (so
// the user's git identity and helpers still work) and settings that keep
// git non-interactive and its output stable.
func gitEnv(env map[string]string) []string {
	out := []string{"GIT_TERMINAL_PROMPT=0", "LC_ALL=C", "GIT_PAGER=cat"}
	for _, k := range []string{"HOME", "PATH", "GIT_CONFIG_NOSYSTEM", "GIT_CONFIG_GLOBAL"} {
		if v, ok := env[k]; ok {
			out = append(out, k+"="+v)
		}
	}
	return out
}
