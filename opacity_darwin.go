//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static void hcSetWindowAlpha(void *w, double a) {
	[(NSWindow *)w setAlphaValue:a];
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

// setWindowOpacity sets the NSWindow's alpha. Call on the main thread.
func setWindowOpacity(win *application.WebviewWindow, pct int) {
	if w := win.NativeWindow(); w != nil {
		C.hcSetWindowAlpha(w, C.double(float64(pct)/100))
	}
}
