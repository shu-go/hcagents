package main

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// opacityCtl keeps the whole window at the opacity setting, and fully opaque
// while the pointer is over it. Wails has no opacity API, so setWindowOpacity
// (opacity_*.go) goes to the OS window.
type opacityCtl struct {
	win *application.WebviewWindow

	mu    sync.Mutex
	level int // percent
	hover bool
}

// set changes the opacity setting (percent) and applies it.
func (o *opacityCtl) set(level int) {
	o.mu.Lock()
	o.level = level
	o.mu.Unlock()
	o.apply()
}

// setHover tells whether the pointer is over the window.
func (o *opacityCtl) setHover(over bool) {
	o.mu.Lock()
	changed := o.hover != over
	o.hover = over
	o.mu.Unlock()
	if changed {
		o.apply()
	}
}

// apply sets the window's opacity now. Also needed after each SetBounds:
// on Windows, Wails resets a layered window's alpha to opaque there.
func (o *opacityCtl) apply() {
	o.mu.Lock()
	pct := o.level
	if o.hover {
		pct = 100
	}
	o.mu.Unlock()
	// On the UI thread; async, since apply may be called from it (tray clicks).
	application.InvokeAsync(func() { setWindowOpacity(o.win, pct) })
}
