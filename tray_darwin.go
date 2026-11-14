//go:build darwin

package main

import "github.com/wailsapp/wails/v3/pkg/application"

// setTrayIcon: not a template icon, since macOS draws those in one color and the
// badge would lose its color (and Wails can't turn a template icon back into a
// plain one). So the glyph follows dark mode here; reapply runs when it changes.
func setTrayIcon(tray *application.SystemTray, ic trayIconSet) {
	if application.Get().Env.IsDarkMode() {
		tray.SetIcon(ic.white)
	} else {
		tray.SetIcon(ic.black)
	}
}
