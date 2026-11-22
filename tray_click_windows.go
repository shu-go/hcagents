//go:build windows

package main

import (
	"syscall"
	"time"
)

var procGetDoubleClickTime = syscall.NewLazyDLL("user32.dll").NewProc("GetDoubleClickTime")

// doubleClickTime is the user's double-click time (500ms unless changed).
func doubleClickTime() time.Duration {
	ms, _, _ := procGetDoubleClickTime.Call()
	if ms == 0 {
		ms = 500
	}
	return time.Duration(ms) * time.Millisecond
}
