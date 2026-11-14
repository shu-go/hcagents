package main

import (
	"bytes"
	"image/png"
	"testing"
)

func TestTrayStateOf(t *testing.T) {
	tests := []struct {
		name  string
		snap  Snapshot
		badge string
		tip   string
	}{
		{"none", Snapshot{Source: "app"}, "", "hcagents: no agents"},
		{"idle only", Snapshot{Source: "app", Agents: []AgentView{{Status: "idle", Group: "idle"}}}, "", "hcagents: 1 idle"},
		{
			"most urgent wins",
			Snapshot{Source: "app", Agents: []AgentView{
				{Status: "working", Group: "working"},
				{Status: "idle", Group: "blocked"}, // a sub-agent needs you
				{Status: "done", Group: "done"},
				{Status: "working", Group: "working"},
			}},
			"blocked", "hcagents: 1 needs you, 1 done, 2 working",
		},
		{"working", Snapshot{Source: "app", Agents: []AgentView{{Status: "working"}}}, "working", "hcagents: 1 working"},
		{"core", Snapshot{Source: "core", Agents: []AgentView{{Status: "done", Group: "done"}}}, "done", "hcagents: 1 done (Herdr is not running)"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			st := trayStateOf(tt.snap)
			if st.badge != tt.badge || st.tooltip != tt.tip {
				t.Errorf("got (%q, %q), want (%q, %q)", st.badge, st.tooltip, tt.badge, tt.tip)
			}
		})
	}
}

func TestWithBadge(t *testing.T) {
	plain := trayIcon(appIcon, black)
	out := withBadge(plain, badgeColors["blocked"])
	img, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	b := img.Bounds()
	size := min(b.Dx(), b.Dy())
	// The badge's center is the badge color.
	c := int(float64(size) * 0.22)
	r, g, bl, a := img.At(b.Max.X-c-1, b.Max.Y-c-1).RGBA()
	if a>>8 != 255 || r>>8 != 0xe5 || g>>8 != 0x48 || bl>>8 != 0x4d {
		t.Errorf("badge center = %x %x %x %x", r>>8, g>>8, bl>>8, a>>8)
	}
	// The top left corner is untouched.
	src, _ := png.Decode(bytes.NewReader(plain))
	if img.At(b.Min.X+2, b.Min.Y+2) != src.At(b.Min.X+2, b.Min.Y+2) {
		t.Error("withBadge changed the top left")
	}
}
