package main

// autoShow decides when the window shows or hides itself (auto_hide). It counts
// the agents in the list as the page shows it (the Show filter, pinned agents
// included): it hides when that count drops to 0 and shows when it rises from 0.
// Only on those changes, so a window shown or hidden by hand stays so until the
// next one.
type autoShow struct {
	on   bool
	prev int // the count in the last list; -1 before the first (since switched on)
}

func newAutoShow() *autoShow { return &autoShow{prev: -1} }

// next returns +1 to show the window, -1 to hide it, 0 to leave it. shown is
// the list as the page shows it. A list with an error says nothing about the
// agents and is skipped. Switching on starts over: the first list changes nothing.
func (a *autoShow) next(shown Snapshot, on bool) int {
	if on != a.on {
		a.on, a.prev = on, -1
	}
	if !on || shown.Error != "" {
		return 0
	}
	n := len(shown.Agents)
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
