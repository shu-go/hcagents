package main

import "testing"

func TestAutoShow(t *testing.T) {
	agents := func(n int) Snapshot { return Snapshot{Agents: make([]AgentView, n)} }
	a := newAutoShow()
	for i, c := range []struct {
		snap Snapshot
		want int
	}{
		{agents(0), 0},  // the first list: no change to react to
		{agents(2), +1}, // agents appeared
		{agents(1), 0},
		{Snapshot{Error: "x"}, 0}, // unknown: skipped
		{agents(1), 0},
		{agents(0), -1}, // the last one went
		{agents(0), 0},
	} {
		if got := a.next(c.snap); got != c.want {
			t.Errorf("%d: next = %d, want %d", i, got, c.want)
		}
	}
	if b := newAutoShow(); b.next(agents(3)) != 0 {
		t.Error("the first list with agents shouldn't show the window")
	}
}
