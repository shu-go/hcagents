//go:build windows

package main

import "syscall"

var procAllowSetForegroundWindow = syscall.NewLazyDLL("user32.dll").NewProc("AllowSetForegroundWindow")

const asfwAny = 0xFFFFFFFF // ASFW_ANY: any process may take the foreground

// allowForeground lets another process take the foreground from us. A second
// hcagents, started by the user, is the foreground process; this lets the
// running one bring its window to the front when the second one hands over.
func allowForeground() {
	procAllowSetForegroundWindow.Call(asfwAny)
}
