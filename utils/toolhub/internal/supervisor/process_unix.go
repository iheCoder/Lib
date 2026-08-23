//go:build darwin || linux

package supervisor

import (
	"os/exec"
	"syscall"
)

// configureProcessGroup reproduces terminal foreground-group semantics so one
// interrupt reaches go run, npm, uv, and any children they launch.
func configureProcessGroup(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// interruptProcessGroup is the programmatic equivalent of pressing Ctrl+C.
func interruptProcessGroup(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	return syscall.Kill(-command.Process.Pid, syscall.SIGINT)
}

// killProcessGroup is the bounded fallback for a process group that ignored
// its graceful interrupt.
func killProcessGroup(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	return syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
}
