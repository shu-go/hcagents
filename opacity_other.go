//go:build !windows && !((darwin || linux) && cgo)

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// setWindowOpacity: no native window to reach here.
func setWindowOpacity(*application.WebviewWindow, int) {}
