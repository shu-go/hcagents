//go:build linux && cgo && gtk3

package main

/*
#cgo pkg-config: gtk+-3.0
#include <gtk/gtk.h>

static void hcSetWindowOpacity(void *w, double a) {
	gtk_widget_set_opacity(GTK_WIDGET(w), a);
}
*/
import "C"

import "github.com/wailsapp/wails/v3/pkg/application"

// setWindowOpacity sets the GtkWindow's opacity (GTK 3 build, -tags gtk3).
func setWindowOpacity(win *application.WebviewWindow, pct int) {
	if w := win.NativeWindow(); w != nil {
		C.hcSetWindowOpacity(w, C.double(float64(pct)/100))
	}
}
