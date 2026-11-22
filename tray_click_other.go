//go:build !windows

package main

import "time"

// doubleClickTime is 0: Wails reports tray double clicks on Windows only, so a
// click needn't wait for one.
func doubleClickTime() time.Duration { return 0 }
