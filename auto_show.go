package main

// autoShow decides when the window shows or hides itself (auto_hide): it shows
// when agents appear in an empty list and hides when the last one goes. Only on
// those changes, so a window shown or hidden by hand stays so until the next one.
type autoShow struct {
	prev int // agents in the last list; -1 before the first
}

func newAutoShow() *autoShow { return &autoShow{prev: -1} }

// next returns +1 to show the window, -1 to hide it, 0 to leave it.
// A list with an error says nothing about the agents and is skipped.
func (a *autoShow) next(snap Snapshot) int {
	if snap.Error != "" {
		return 0
	}
	n := len(snap.Agents)
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
