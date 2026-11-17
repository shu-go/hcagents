package main

import "testing"

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
