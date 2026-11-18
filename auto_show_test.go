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

func TestAutoShowEmpty(t *testing.T) {
	a := newAutoShow()
	for i, c := range []struct {
		snap Snapshot
		want int
	}{
		{agentsIn(), 0}, // the first list: no change to react to
		{agentsIn("idle", "working"), +1},
		{agentsIn("idle"), 0}, // idle agents still count
		{Snapshot{Error: "x"}, 0},
		{agentsIn("idle"), 0},
		{agentsIn(), -1}, // the last one went
		{agentsIn(), 0},
	} {
		if got := a.next(c.snap, config.AutoHideEmpty); got != c.want {
			t.Errorf("%d: next = %d, want %d", i, got, c.want)
		}
	}
	if b := newAutoShow(); b.next(agentsIn("idle"), config.AutoHideEmpty) != 0 {
		t.Error("the first list with agents shouldn't show the window")
	}
}

func TestAutoShowIdle(t *testing.T) {
	a := newAutoShow()
	for i, c := range []struct {
		snap Snapshot
		want int
	}{
		{agentsIn("working", "idle"), 0},
		{agentsIn("done", "idle"), 0}, // done isn't idle yet
		{agentsIn("idle", "idle"), -1},
		{agentsIn("idle"), 0},
		{agentsIn("idle", "blocked"), +1},
		{agentsIn(), -1}, // no agents: none busy either
		{agentsIn("idle"), 0},
	} {
		if got := a.next(c.snap, config.AutoHideIdle); got != c.want {
			t.Errorf("%d: next = %d, want %d", i, got, c.want)
		}
	}
}

func TestAutoShowModes(t *testing.T) {
	a := newAutoShow()
	a.next(agentsIn("working"), config.AutoHideEmpty)
	if got := a.next(agentsIn(), config.AutoHideOff); got != 0 {
		t.Errorf("off: next = %d", got)
	}
	// A switch starts over: the first list of the new mode changes nothing.
	if got := a.next(agentsIn("idle"), config.AutoHideIdle); got != 0 {
		t.Errorf("after a switch: next = %d", got)
	}
	if got := a.next(agentsIn("working"), config.AutoHideIdle); got != +1 {
		t.Errorf("idle: next = %d, want +1", got)
	}
}

func TestNormalizeAutoHide(t *testing.T) {
	for in, want := range map[string]string{
		"": "empty", "true": "empty", "empty": "empty", "Idle": "idle",
		"off": "off", "false": "off", "x": "empty",
	} {
		if got := config.NormalizeAutoHide(in); got != want {
			t.Errorf("NormalizeAutoHide(%q) = %q, want %q", in, got, want)
		}
	}
}
