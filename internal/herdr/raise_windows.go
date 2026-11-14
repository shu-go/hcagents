//go:build windows

package herdr

import "syscall"

var procAllowSetForegroundWindow = syscall.NewLazyDLL("user32.dll").NewProc("AllowSetForegroundWindow")

const asfwAny = 0xFFFFFFFF // ASFW_ANY: any process may take the foreground

// allowForeground lets the herdr-app we start take the foreground.
// Windows only lets the foreground process give the foreground away, so this
// works right after a click in our own window, while we still are the foreground process.
func allowForeground() {
	procAllowSetForegroundWindow.Call(asfwAny)
}

// RaiseWindow brings Herdr's window to the front by running `herdr-app` again:
// a second instance hands over to the running one and exits (if Herdr isn't
// running, this starts it). Call it right after a click (see allowForeground).
func (c *Client) RaiseWindow() error {
	return c.launch()
}
