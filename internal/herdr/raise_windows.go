//go:build windows

package herdr

import (
	"os/exec"
	"syscall"
)

var procAllowSetForegroundWindow = syscall.NewLazyDLL("user32.dll").NewProc("AllowSetForegroundWindow")

const asfwAny = 0xFFFFFFFF // ASFW_ANY: any process may take the foreground

// RaiseWindow brings Herdr's window to the front by running `herdr-app` again:
// a second instance hands over to the running one and exits (if Herdr isn't
// running, this starts it).
//
// Windows only lets the foreground process give the foreground away, so call this
// right after a click in our own window, while we still are the foreground process.
func (c *Client) RaiseWindow() error {
	procAllowSetForegroundWindow.Call(asfwAny)

	cmd := exec.Command(c.app())
	// No console for the shim, but no SW_HIDE either: a fresh Herdr would start hidden.
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // reap it
	return nil
}
