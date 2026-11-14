package main

import (
	"log"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/shu-go/hcagents/internal/config"
)

// windowState restores the window's position and size and saves them when they change.
type windowState struct {
	app  *application.App
	win  *application.WebviewWindow
	path string

	mu    sync.Mutex
	timer *time.Timer
	ready bool // don't save the moves made while restoring
}

// applySavedBounds puts the saved position and size into the window options
// (setting them after creation is undone by Wails' initial centering).
func applySavedBounds(path string, opts *application.WebviewWindowOptions) {
	b := config.LoadState(path).Window
	if b == nil || b.Width <= 0 || b.Height <= 0 {
		return
	}
	opts.InitialPosition = application.WindowXY
	opts.X, opts.Y = b.X, b.Y
	opts.Width, opts.Height = b.Width, b.Height
}

// trackWindowState saves the window's bounds when they change. Once the window is up,
// it is centered if its saved spot is no longer on any screen (a monitor was removed).
func trackWindowState(app *application.App, win *application.WebviewWindow, path string) *windowState {
	ws := &windowState{app: app, win: win, path: path}
	win.OnWindowEvent(events.Common.WindowDidMove, func(*application.WindowEvent) { ws.changed() })
	win.OnWindowEvent(events.Common.WindowDidResize, func(*application.WindowEvent) { ws.changed() })
	var once sync.Once
	win.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) {
		once.Do(func() {
			r := win.Bounds()
			if !ws.onScreen(&config.WindowBounds{X: r.X, Y: r.Y, Width: r.Width, Height: r.Height}) {
				win.Center()
			}
			ws.mu.Lock()
			ws.ready = true
			ws.mu.Unlock()
		})
	})
	return ws
}

// onScreen: at least the top-left part of the title bar is on some screen,
// so the window can be dragged back (a monitor may have been removed).
func (ws *windowState) onScreen(b *config.WindowBounds) bool {
	const grab = 40
	for _, s := range ws.app.Screen.GetAll() {
		a := s.WorkArea
		if b.X+grab > a.X && b.X < a.X+a.Width-grab &&
			b.Y >= a.Y && b.Y < a.Y+a.Height-grab {
			return true
		}
	}
	return false
}

// changed saves a moment after the last move or resize.
func (ws *windowState) changed() {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if !ws.ready {
		return
	}
	if ws.timer != nil {
		ws.timer.Stop()
	}
	ws.timer = time.AfterFunc(500*time.Millisecond, ws.save)
}

func (ws *windowState) save() {
	if ws.win.IsMinimised() || ws.win.IsMaximised() {
		return
	}
	r := ws.win.Bounds()
	if r.Width <= 0 || r.Height <= 0 {
		return
	}
	st := config.LoadState(ws.path)
	st.Window = &config.WindowBounds{X: r.X, Y: r.Y, Width: r.Width, Height: r.Height}
	if err := config.SaveState(ws.path, st); err != nil {
		log.Println("save window state:", err)
	}
}

// flush saves now (on quit).
func (ws *windowState) flush() {
	ws.mu.Lock()
	pending := ws.timer != nil && ws.timer.Stop()
	ws.mu.Unlock()
	if pending {
		ws.save()
	}
}
