package clitest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// Repo is a throwaway git repository built commit by commit, for tests that
// need real history (diffs, renames, merge bases).
type Repo struct {
	t   *testing.T
	Dir string
}

// NewRepo creates an empty repository on branch main in a temp directory.
// Git runs isolated from the user's and system's git config.
func NewRepo(t *testing.T) *Repo {
	t.Helper()
	r := &Repo{t: t, Dir: t.TempDir()}
	r.Git("init", "-q", "-b", "main")
	r.Git("config", "user.name", "qgate test")
	r.Git("config", "user.email", "test@qgate.invalid")
	r.Git("config", "commit.gpgsign", "false")
	return r
}

// Git runs a git command in the repository and returns its trimmed stdout.
func (r *Repo) Git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	cmd.Env = append(os.Environ(),
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_CONFIG_GLOBAL="+os.DevNull,
		"GIT_AUTHOR_DATE=2026-01-01T00:00:00Z",
		"GIT_COMMITTER_DATE=2026-01-01T00:00:00Z",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// Write creates or replaces a file, creating parent directories.
func (r *Repo) Write(path, content string) {
	r.t.Helper()
	full := filepath.Join(r.Dir, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// Remove deletes a tracked file.
func (r *Repo) Remove(path string) {
	r.t.Helper()
	r.Git("rm", "-q", path)
}

// Rename moves a tracked file.
func (r *Repo) Rename(from, to string) {
	r.t.Helper()
	if err := os.MkdirAll(filepath.Dir(filepath.Join(r.Dir, to)), 0o755); err != nil {
		r.t.Fatal(err)
	}
	r.Git("mv", from, to)
}

// Commit stages everything and commits, returning the new commit's SHA.
func (r *Repo) Commit(msg string) string {
	r.t.Helper()
	r.Git("add", "-A")
	r.Git("commit", "-q", "-m", msg)
	return r.Git("rev-parse", "HEAD")
}

// SetOrigin points origin/main (and origin's HEAD) at sha without any
// network, so qgate's merge-base fallback has a default branch to use.
func (r *Repo) SetOrigin(sha string) {
	r.t.Helper()
	r.Git("update-ref", "refs/remotes/origin/main", sha)
	r.Git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
}

// Lines builds file content from numbered lines "line 1".."line n".
func Lines(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		b.WriteString("line " + strconv.Itoa(i) + "\n")
	}
	return b.String()
}
