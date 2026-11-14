package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetTopLevel(t *testing.T) {
	tests := []struct {
		name, src, want string
	}{
		{"replace", "a: 1\ntheme: system\nb: 2\n", "a: 1\ntheme: dark\nb: 2\n"},
		{"keep comment", "theme: system  # the look\n", "theme: dark  # the look\n"},
		{"quoted", "theme: \"light\"\n", "theme: dark\n"},
		{"empty value", "theme:\nb: 2\n", "theme: dark\nb: 2\n"},
		{"crlf", "a: 1\r\ntheme: light\r\nb: 2\r\n", "a: 1\r\ntheme: dark\r\nb: 2\r\n"},
		{"append", "# hi\na: 1\n", "# hi\na: 1\ntheme: dark\n"},
		{"append, no final newline", "a: 1", "a: 1\ntheme: dark\n"},
		{"append crlf", "a: 1\r\n", "a: 1\r\ntheme: dark\r\n"},
		{"commented out stays", "# theme: light\n", "# theme: light\ntheme: dark\n"},
		{"nested stays", "window:\n  theme: x\n", "window:\n  theme: x\ntheme: dark\n"},
		{"other key with the same start", "themes: x\n", "themes: x\ntheme: dark\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(setTopLevel([]byte(tt.src), "theme", "dark")); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSetValueExistingFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), FileName)
	src := "# settings\ninterval: 3s # how often\ntheme: system\n"
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &Config{Path: p}
	if err := c.SetValue("theme", "light"); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if want := "# settings\ninterval: 3s # how often\ntheme: light\n"; string(b) != want {
		t.Errorf("got %q, want %q", b, want)
	}
}

func TestSetValueBrokenFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), FileName)
	src := "interval: [3s\n"
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &Config{Path: p}
	if err := c.SetValue("theme", "light"); err == nil {
		t.Fatal("want an error for a broken file")
	}
	if b, _ := os.ReadFile(p); string(b) != src {
		t.Errorf("a broken file was changed: %q", b)
	}
}

func TestNormalizeTheme(t *testing.T) {
	for in, want := range map[string]string{
		"": "system", "system": "system", "Light": "light", " dark ": "dark", "blue": "system",
	} {
		if got := NormalizeTheme(in); got != want {
			t.Errorf("NormalizeTheme(%q) = %q, want %q", in, got, want)
		}
	}
}
