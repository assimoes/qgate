//go:build unix

package pipeline

import (
	"os/exec"
	"syscall"
)

// ownProcessGroup starts the command in a new process group, so it and
// every child it starts can be killed together.
func ownProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}

// killProcessGroup kills whatever is left of the command's process group.
// The command itself has exited; this reaps children it left behind.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) // nothing left is not an error
	}
}
