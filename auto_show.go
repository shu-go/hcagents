package main

import "github.com/shu-go/hcagents/internal/config"

// autoShow decides when the window shows or hides itself (auto_hide). It counts
// the agents that matter for the mode: all of them ("empty") or those not idle
// ("idle"). It hides when that count drops to 0 and shows when it rises from 0.
// Only on those changes, so a window shown or hidden by hand stays so until the next one.
type autoShow struct {
	mode string
	prev int // the count in the last list; -1 before the first (of this mode)
}

func newAutoShow() *autoShow { return &autoShow{prev: -1} }

// next returns +1 to show the window, -1 to hide it, 0 to leave it.
// A list with an error says nothing about the agents and is skipped.
// A new mode starts over: its first list changes nothing.
func (a *autoShow) next(snap Snapshot, mode string) int {
	if mode != a.mode {
		a.mode, a.prev = mode, -1
	}
	if mode == config.AutoHideOff || snap.Error != "" {
		return 0
	}
	n := 0
	for _, ag := range snap.Agents {
		// Group: the agent with its sub-agents, so a parent waiting on a working child counts.
		if mode != config.AutoHideIdle || ag.Group != "idle" {
			n++
		}
	}
	prev := a.prev
	a.prev = n
	switch {
	case prev == 0 && n > 0:
		return +1
	case prev > 0 && n == 0:
		return -1
	}
	return 0
}
