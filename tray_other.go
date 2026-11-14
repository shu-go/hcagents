//go:build !darwin

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// setTrayIcon: Windows and Linux pick the black or white glyph for the taskbar themselves.
func setTrayIcon(tray *application.SystemTray, ic trayIconSet) {
	tray.SetIcon(ic.black)
	tray.SetDarkModeIcon(ic.white)
}
