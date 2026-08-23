//go:build windows

package supervisor

import (
	"os"
	"os/exec"
)

// Windows is not the primary platform, but a direct-process fallback keeps the
// package buildable without pretending Unix process groups exist there.
func configureProcessGroup(command *exec.Cmd) {}

// interruptProcessGroup requests the nearest Windows equivalent of Ctrl+C.
func interruptProcessGroup(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	return command.Process.Signal(os.Interrupt)
}

// killProcessGroup force-stops the direct child on the fallback platform.
func killProcessGroup(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	return command.Process.Kill()
}
