//go:build windows

package herdr

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// hideWindow keeps a console window from flashing when a GUI app starts a console program.
func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
