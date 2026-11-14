package main

import (
	"fmt"
	"image/color"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Tray badge colors, by group. Fixed (not the page's theme): they sit on the
// taskbar / menu bar, which follows the OS.
var badgeColors = map[string]color.NRGBA{
	"blocked": {R: 0xe5, G: 0x48, B: 0x4d, A: 255}, // Needs you: red
	"done":    {R: 0x3d, G: 0x8b, B: 0xfd, A: 255}, // blue
	"working": {R: 0xf5, G: 0xb8, B: 0x00, A: 255}, // yellow
}

// badgeOrder: the most urgent first. Idle gets no badge.
var badgeOrder = []string{"blocked", "done", "working"}

// tooltipLabels names the counts in the tooltip, in this order.
var tooltipLabels = []struct{ group, label string }{
	{"blocked", "needs you"},
	{"done", "done"},
	{"working", "working"},
	{"idle", "idle"},
}

// trayState is what the tray shows for a snapshot.
type trayState struct {
	badge   string // "blocked", "done", "working" or "" (none)
	tooltip string
}

// trayStateOf counts the agents by the group they are listed in (the most urgent
// status of an agent and its sub-agents, as the list's sections).
func trayStateOf(snap Snapshot) trayState {
	counts := map[string]int{}
	for _, a := range snap.Agents {
		g := a.Group
		if g == "" {
			g = a.Status
		}
		counts[g]++
	}
	var st trayState
	for _, g := range badgeOrder {
		if counts[g] > 0 {
			st.badge = g
			break
		}
	}
	var parts []string
	for _, l := range tooltipLabels {
		if n := counts[l.group]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, l.label))
		}
	}
	st.tooltip = "hcagents"
	if len(parts) == 0 {
		st.tooltip += ": no agents"
	} else {
		st.tooltip += ": " + strings.Join(parts, ", ")
	}
	if snap.Source == "core" {
		st.tooltip += " (Herdr is not running)"
	}
	return st
}

// trayIconSet is one tray picture in both glyph colors.
type trayIconSet struct {
	black []byte // for light taskbars
	white []byte // for dark taskbars
}

// trayUpdater keeps the tray's icon and tooltip in step with the agents.
type trayUpdater struct {
	tray  *application.SystemTray
	icons map[string]trayIconSet // badge ("" for none) -> icons

	mu   sync.Mutex
	last trayState
	set  bool
}

func newTrayUpdater(tray *application.SystemTray, src []byte) *trayUpdater {
	plain := trayIconSet{black: trayIcon(src, black), white: trayIcon(src, white)}
	icons := map[string]trayIconSet{"": plain}
	for g, c := range badgeColors {
		icons[g] = trayIconSet{black: withBadge(plain.black, c), white: withBadge(plain.white, c)}
	}
	return &trayUpdater{tray: tray, icons: icons}
}

// update shows st; nothing happens when it is what the tray already shows.
func (u *trayUpdater) update(st trayState) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.set && st == u.last {
		return
	}
	if !u.set || st.badge != u.last.badge {
		setTrayIcon(u.tray, u.icons[st.badge])
	}
	if !u.set || st.tooltip != u.last.tooltip {
		u.tray.SetTooltip(st.tooltip)
	}
	u.last, u.set = st, true
}

// reapply draws the current icon again (the app started, or the OS switched light / dark).
func (u *trayUpdater) reapply() {
	u.mu.Lock()
	defer u.mu.Unlock()
	setTrayIcon(u.tray, u.icons[u.last.badge]) // u.last.badge is "" before the first update
}
