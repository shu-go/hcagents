package main

import (
	"testing"

	"github.com/shu-go/hcagents/internal/config"
)

func agentsIn(groups ...string) Snapshot {
	s := Snapshot{}
	for _, g := range groups {
		s.Agents = append(s.Agents, AgentView{Group: g})
	}
	return s
}

func TestAutoShow(t *testing.T) {
	a := newAutoShow()
	for i, c := range []struct {
		snap Snapshot
		want int
	}{
		{agentsIn(), 0}, // the first list: no change to react to
		{agentsIn("idle", "working"), +1},
		{agentsIn("idle"), 0}, // listed idle agents count: Show decides what is listed
		{Snapshot{Error: "x"}, 0},
		{agentsIn("idle"), 0},
		{agentsIn(), -1}, // the last one went
		{agentsIn(), 0},
	} {
		if got := a.next(c.snap, true); got != c.want {
			t.Errorf("%d: next = %d, want %d", i, got, c.want)
		}
	}
	if b := newAutoShow(); b.next(agentsIn("idle"), true) != 0 {
		t.Error("the first list with agents shouldn't show the window")
	}
}

// With Show ▸ All but idle, the list is what the old "idle" mode counted, except
// that a pinned idle agent stays listed and so keeps the window up.
func TestAutoShowWithShowFilter(t *testing.T) {
	a := newAutoShow()
	next := func(s Snapshot) int { return a.next(shownOf(s, config.ShowActive), true) }
	if got := next(agentsIn("working", "idle")); got != 0 {
		t.Errorf("first: %d", got)
	}
	if got := next(agentsIn("idle", "idle")); got != -1 {
		t.Errorf("all idle: %d, want -1", got)
	}
	if got := next(agentsIn("idle", "blocked")); got != +1 {
		t.Errorf("one needs you: %d, want +1", got)
	}
	pinned := agentsIn("idle")
	pinned.Agents[0].Pinned = true
	if got := next(pinned); got != 0 {
		t.Errorf("a pinned idle agent is still listed: %d, want 0", got)
	}
}

func TestAutoShowOnOff(t *testing.T) {
	a := newAutoShow()
	a.next(agentsIn("working"), true)
	if got := a.next(agentsIn(), false); got != 0 {
		t.Errorf("off: next = %d", got)
	}
	// Switching on starts over: its first list changes nothing.
	if got := a.next(agentsIn(), true); got != 0 {
		t.Errorf("after switching on: next = %d", got)
	}
	if got := a.next(agentsIn("working"), true); got != +1 {
		t.Errorf("on: next = %d, want +1", got)
	}
}
