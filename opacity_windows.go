//go:build windows

package main

import (
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"
)

var (
	user32                         = syscall.NewLazyDLL("user32.dll")
	procGetWindowLongPtrW          = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW          = user32.NewProc("SetWindowLongPtrW")
	procSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
)

const (
	wsExLayered = 0x00080000
	lwaAlpha    = 0x2
)

var gwlExStyle int32 = -20 // a variable: a negative constant can't convert to uintptr

// setWindowOpacity makes the window a layered one (the first time it isn't opaque)
// and sets its alpha. Wails uses the same for its transparent frameless windows.
func setWindowOpacity(win *application.WebviewWindow, pct int) {
	hwnd := uintptr(win.NativeWindow())
	if hwnd == 0 {
		return
	}
	ex, _, _ := procGetWindowLongPtrW.Call(hwnd, uintptr(gwlExStyle))
	if ex&wsExLayered == 0 {
		if pct >= 100 {
			return // nothing to do: leave the window as Wails made it
		}
		procSetWindowLongPtrW.Call(hwnd, uintptr(gwlExStyle), ex|wsExLayered)
	}
	procSetLayeredWindowAttributes.Call(hwnd, 0, uintptr(pct*255/100), lwaAlpha)
}
