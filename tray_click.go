package main

import (
	"sync"
	"time"
)

// clickGate tells a tray click from a double click. Windows sends click,
// double click, click: a click waits out the double-click time (delay) before
// it acts, so a double click cancels it and the window doesn't flash; the click
// after the double click is dropped. With delay 0 (no double clicks reported:
// macOS, Linux) a click acts at once.
type clickGate struct {
	delay  time.Duration
	single func() // a click
	double func() // a double click

	mu        sync.Mutex
	pending   *time.Timer
	dropUntil time.Time
}

// dropAfterDouble is how long after a double click its trailing click may come.
const dropAfterDouble = time.Second

func (g *clickGate) click() {
	g.mu.Lock()
	defer g.mu.Unlock()
	if time.Now().Before(g.dropUntil) {
		g.dropUntil = time.Time{}
		return
	}
	if g.delay <= 0 {
		g.single()
		return
	}
	if g.pending != nil {
		g.pending.Stop()
	}
	g.pending = time.AfterFunc(g.delay, func() {
		g.mu.Lock()
		g.pending = nil
		g.mu.Unlock()
		g.single()
	})
}

func (g *clickGate) doubleClick() {
	g.mu.Lock()
	if g.pending != nil {
		g.pending.Stop()
		g.pending = nil
	}
	g.dropUntil = time.Now().Add(dropAfterDouble)
	g.mu.Unlock()
	g.double()
}
