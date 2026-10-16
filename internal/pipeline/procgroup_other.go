//go:build !unix

package pipeline

import "os/exec"

// ownProcessGroup is a no-op where process groups are not available: the
// timeout kills the command itself, not its children.
func ownProcessGroup(*exec.Cmd) {}

// killProcessGroup is a no-op where process groups are not available.
func killProcessGroup(*exec.Cmd) {}
