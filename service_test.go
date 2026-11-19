package main

import "testing"

func TestFirstAgent(t *testing.T) {
	for _, c := range []struct {
		name   string
		agents []AgentView
		want   string
	}{
		{"none", nil, ""},
		{"top of the sorted list", []AgentView{{Terminal: "a", Group: "blocked"}, {Terminal: "b", Group: "idle"}}, "a"},
		{"pinned first", []AgentView{{Terminal: "a", Group: "blocked"}, {Terminal: "b", Group: "idle", Pinned: true}}, "b"},
		{"unlisted section skipped", []AgentView{{Terminal: "a", Group: "error"}, {Terminal: "b", Group: "idle"}}, "b"},
		{"status without group", []AgentView{{Terminal: "a", Status: "working"}}, "a"},
	} {
		if got := firstAgent(Snapshot{Agents: c.agents}); got != c.want {
			t.Errorf("%s: firstAgent = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestLiveStatus(t *testing.T) {
	for _, c := range []struct{ app, core, want string }{
		{"working", "done", "done"}, // the core is newer than the last herdr-app ls
		{"idle", "working", "working"},
		{"idle", "done", "idle"},        // Herdr saw it
		{"blocked", "error", "blocked"}, // Herdr's own mapping
		{"idle", "", "idle"},            // no core record
	} {
		if got := liveStatus(c.app, c.core); got != c.want {
			t.Errorf("liveStatus(%q, %q) = %q, want %q", c.app, c.core, got, c.want)
		}
	}
}
