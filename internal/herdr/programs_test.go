package herdr

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLookup(t *testing.T) {
	p, err := LoadPrograms("")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name  string
		agent bool
		key   string
		tint  bool
	}{
		{"claude-code", true, "claude", false}, // an agent's app name, via match
		{"copilot", true, "copilot", true},
		{"", true, "agent", true},                                        // an agent that didn't say its name
		{"", false, "program", true},                                     // nothing known
		{`C:\Program Files\PowerShell\7\pwsh.exe`, false, "shell", true}, // a shell, by process path
		{"Rscript", false, "R", true},                                    // keys and matches ignore case
		{"something-else", false, "program", true},
	} {
		got := p.Lookup(c.name, c.agent)
		if got.Key != c.key || got.Tint != c.tint {
			t.Errorf("Lookup(%q, %v) = %s tint=%v, want %s tint=%v", c.name, c.agent, got.Key, got.Tint, c.key, c.tint)
		}
	}
}

func TestUserProgramsAndIcon(t *testing.T) {
	home := t.TempDir()
	write := func(rel, s string) {
		path := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("programs.json", `{"programs": {
		"claude-alt": {"icon": "icons/alt.png", "match": ["my-claude"]},
		"codex": {"tint": true}
	}}`)
	write("icons/alt.png", "png")
	write("icons/claude.svg", "<svg/>")

	p, err := LoadPrograms(home)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.Lookup("my-claude", true); got.Key != "claude-alt" || got.Icon != "icons/alt.png" {
		t.Errorf("user entry: got %+v", got)
	}
	if got := p.Lookup("codex", true); !got.Tint || got.Icon != "icons/codex.svg" {
		t.Errorf("user override should keep the built-in icon: got %+v", got)
	}

	url, err := p.Icon("claude")
	if err != nil || !strings.HasPrefix(url, "data:image/svg+xml;base64,") {
		t.Errorf("Icon(claude) = %q, %v", url, err)
	}
	if url, _ := p.Icon("claude-alt"); !strings.HasPrefix(url, "data:image/png;base64,") {
		t.Errorf("Icon(claude-alt) = %q", url)
	}
	if url, err := p.Icon("codex"); url != "" || err != nil {
		t.Errorf("a missing file should give no icon: %q, %v", url, err)
	}
}
