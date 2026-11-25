package main

import (
	"reflect"
	"testing"

	"github.com/shu-go/hcagents/internal/config"
)

func TestShownOf(t *testing.T) {
	snap := Snapshot{Source: "app", Agents: []AgentView{
		{Terminal: "b", Group: "blocked"},
		{Terminal: "d", Group: "done"},
		{Terminal: "w", Group: "working"},
		{Terminal: "i", Group: "idle"},
		{Terminal: "pi", Group: "idle", Pinned: true},
		{Terminal: "s", Status: "idle"}, // no Group: its own status
		{Terminal: "c", Group: "working", Status: "idle", Children: []AgentView{{Terminal: "c1", Status: "working"}}},
	}}
	terminals := func(s Snapshot) []string {
		var list []string
		for _, a := range s.Agents {
			list = append(list, a.Terminal)
		}
		return list
	}
	for mode, want := range map[string][]string{
		config.ShowAll:       {"b", "d", "w", "i", "pi", "s", "c"},
		config.ShowActive:    {"b", "d", "w", "pi", "c"},
		config.ShowAttention: {"b", "d", "pi"},
	} {
		got := shownOf(snap, mode)
		if !reflect.DeepEqual(terminals(got), want) {
			t.Errorf("%s: %v, want %v", mode, terminals(got), want)
		}
		if got.Source != "app" {
			t.Errorf("%s: Source = %q", mode, got.Source)
		}
	}
	if len(snap.Agents) != 7 {
		t.Error("shownOf changed its input")
	}
}

func TestNormalizeShow(t *testing.T) {
	for in, want := range map[string]string{
		"": "all", "all": "all", "Active": "active", " attention ": "attention", "x": "all",
	} {
		if got := config.NormalizeShow(in); got != want {
			t.Errorf("NormalizeShow(%q) = %q, want %q", in, got, want)
		}
	}
}
