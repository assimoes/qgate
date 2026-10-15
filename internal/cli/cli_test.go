package cli_test

import (
	"strings"
	"testing"

	"qgate/internal/clitest"
)

func TestVersionPrintsBuildAndScoringInfo(t *testing.T) {
	t.Parallel()
	res := clitest.Run(t, clitest.Options{}, "version")

	if res.Code != 0 {
		t.Fatalf("exit code = %d, want 0; stderr:\n%s", res.Code, res.Stderr)
	}
	for _, want := range []string{"version:", "commit:", "build date:", "scoring algorithm: v1"} {
		if !strings.Contains(res.Stdout, want) {
			t.Errorf("stdout missing %q:\n%s", want, res.Stdout)
		}
	}
}

func TestUsageErrorsExitTwo(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []string
	}{
		{"unknown command", []string{"frobnicate"}},
		{"unknown flag on root", []string{"--nope"}},
		{"unknown flag on run", []string{"run", "--nope"}},
		{"unexpected argument", []string{"version", "extra"}},
		{"no command", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			res := clitest.Run(t, clitest.Options{}, tt.args...)
			if res.Code != 2 {
				t.Errorf("exit code = %d, want 2; stderr:\n%s", res.Code, res.Stderr)
			}
			if res.Stderr == "" {
				t.Error("stderr is empty, want an explanation")
			}
		})
	}
}
