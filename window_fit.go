package main

import (
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/shu-go/hcagents/internal/config"
)

// minFitHeight keeps at least the title bar on screen, whatever the page says.
const minFitHeight = 40

// windowFitter makes the window's height follow the list: the page tells how tall
// its content is, and the window grows or shrinks up or down to fit it.
type windowFitter struct {
	win  *application.WebviewWindow
	grow func() string // config.GrowUp or config.GrowDown

	mu sync.Mutex // one resize at a time (bound calls come on their own goroutines)
}

// fit resizes the window so the page's content (CSS px) fits. inner is the page's
// current viewport height: the difference from the window's height is the frame
// (none when frameless, but this way it doesn't matter).
func (f *windowFitter) fit(content, inner int) {
	if content <= 0 || inner <= 0 {
		return
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.win.IsMinimised() || f.win.IsMaximised() || f.win.IsFullscreen() {
		return
	}
	b := f.win.Bounds()
	if b.Height <= 0 {
		return
	}
	want := b.Height + content - inner
	area := application.Rect{Y: b.Y - 1<<20, Height: 2 << 20} // screen unknown: no limit
	if s, err := f.win.GetScreen(); err == nil && s != nil {
		area = s.WorkArea
	}
	nb := fitBounds(b, area, want, f.grow())
	if nb != b {
		f.win.SetBounds(nb)
	}
}

// fitBounds is b with its height set to want, keeping the bottom edge (grow "up")
// or the top edge ("down") where it is. It doesn't grow past the screen's work
// area on the growing side; then the list scrolls.
func fitBounds(b, area application.Rect, want int, grow string) application.Rect {
	var room int
	if grow == config.GrowDown {
		room = area.Y + area.Height - b.Y
	} else {
		room = b.Y + b.Height - area.Y
	}
	if want > room {
		want = room
	}
	if want < minFitHeight {
		want = minFitHeight
	}
	nb := b
	nb.Height = want
	if grow != config.GrowDown {
		nb.Y = b.Y + b.Height - want
	}
	return nb
}
