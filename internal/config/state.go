package config

import (
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

const StateFileName = "state.yaml"

// State is what the app remembers by itself (written by the app, not the user).
type State struct {
	Window *WindowBounds `yaml:"window,omitempty"`
}

// WindowBounds is in device-independent pixels; X/Y are absolute (multi-monitor).
type WindowBounds struct {
	X      int `yaml:"x"`
	Y      int `yaml:"y"`
	Width  int `yaml:"width"`
	Height int `yaml:"height"`
}

// StatePath is next to the config file in use, else in the user's config folder.
func (c *Config) StatePath() string {
	if c.Path != "" {
		return filepath.Join(filepath.Dir(c.Path), StateFileName)
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "hcagents", StateFileName)
	}
	return ""
}

// LoadState returns an empty State when the file is missing or broken.
func LoadState(path string) *State {
	s := &State{}
	if path == "" {
		return s
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return s
	}
	if err := yaml.Unmarshal(b, s); err != nil {
		return &State{}
	}
	return s
}

// SaveState writes the file atomically.
func SaveState(path string, s *State) error {
	if path == "" {
		return nil
	}
	b, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append([]byte("# Written by hcagents. Edit hcagents.yaml instead.\n"), b...), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
