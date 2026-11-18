// Package config loads hcagents.yaml.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

const FileName = "hcagents.yaml"

type Config struct {
	// HerdrApp is the herdr-app executable. Empty: look up in PATH.
	HerdrApp string `yaml:"herdr_app"`
	// Hc is the herdr-core (hc) executable. Empty: look up in PATH.
	Hc string `yaml:"hc"`
	// HerdrHome is Herdr's app folder, for its icons and programs.json.
	// Empty: $HERDR_APP_HOME, else $XDG_CONFIG_HOME/herdr-app, else ~/.config/herdr-app.
	HerdrHome string `yaml:"herdr_home"`
	// StartHerdr starts Herdr's window on startup when it isn't running.
	StartHerdr bool `yaml:"start_herdr"`
	// Interval between reads of Herdr's own list (herdr-app ls) while the window
	// is shown, e.g. "10s". Statuses come from `hc events` as they change.
	Interval Duration `yaml:"interval"`
	// HiddenInterval is the one while the window is hidden.
	HiddenInterval Duration `yaml:"hidden_interval"`
	// CSS is a user stylesheet loaded after the built-in one.
	// A relative path is relative to the config file's folder.
	CSS string `yaml:"css"`
	// Theme is "system" (follow the OS), "light" or "dark". Set from the tray menu too.
	Theme string `yaml:"theme"`
	// Grow is the way the window grows and shrinks as its height follows the list:
	// "up" (the bottom edge stays) or "down" (the top edge stays). Set from the tray menu too.
	Grow string `yaml:"grow"`
	// AutoHide hides the window when the last agent goes, and shows it again
	// when agents appear in the empty list.
	AutoHide bool `yaml:"auto_hide"`

	Window Window `yaml:"window"`

	// Path is the file this config was read from ("" when none was found).
	Path string `yaml:"-"`
}

type Window struct {
	Width       int  `yaml:"width"`
	Height      int  `yaml:"height"`
	AlwaysOnTop bool `yaml:"always_on_top"`
}

func Default() *Config {
	return &Config{
		StartHerdr:     true,
		Interval:       Duration(10 * time.Second),
		HiddenInterval: Duration(30 * time.Second),
		Theme:          ThemeSystem,
		Grow:           GrowUp,
		AutoHide:       true,
		Window: Window{
			Width:       280,
			Height:      360,
			AlwaysOnTop: true,
		},
	}
}

// Candidates returns where the config file is looked for, in order:
// next to the executable, then the user's config folder.
func Candidates() []string {
	var list []string
	if exe, err := os.Executable(); err == nil {
		if p, err := filepath.EvalSymlinks(exe); err == nil {
			exe = p
		}
		list = append(list, filepath.Join(filepath.Dir(exe), FileName))
	}
	if dir, err := os.UserConfigDir(); err == nil {
		list = append(list, filepath.Join(dir, "hcagents", FileName))
	}
	return list
}

// Load reads the first config file found; with none, it returns the defaults.
func Load() (*Config, error) {
	cfg := Default()
	for _, p := range Candidates() {
		b, err := os.ReadFile(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return cfg, err
		}
		if err := yaml.Unmarshal(b, cfg); err != nil {
			return cfg, fmt.Errorf("%s: %w", p, err)
		}
		cfg.Path = p
		break
	}
	if cfg.Interval <= 0 {
		cfg.Interval = Default().Interval
	}
	if cfg.HiddenInterval <= 0 {
		cfg.HiddenInterval = Default().HiddenInterval
	}
	cfg.Theme = NormalizeTheme(cfg.Theme)
	cfg.Grow = NormalizeGrow(cfg.Grow)
	return cfg, nil
}

const (
	GrowUp   = "up"
	GrowDown = "down"
)

// NormalizeGrow returns "down" as it is (any case), else "up".
func NormalizeGrow(s string) string {
	if g := strings.ToLower(strings.TrimSpace(s)); g == GrowDown {
		return g
	}
	return GrowUp
}

const (
	ThemeSystem = "system"
	ThemeLight  = "light"
	ThemeDark   = "dark"
)

// NormalizeTheme returns "light" or "dark" as they are (any case), else "system".
func NormalizeTheme(s string) string {
	switch t := strings.ToLower(strings.TrimSpace(s)); t {
	case ThemeLight, ThemeDark:
		return t
	}
	return ThemeSystem
}

// CSSPath resolves CSS against the config file's folder.
func (c *Config) CSSPath() string {
	if c.CSS == "" || filepath.IsAbs(c.CSS) || c.Path == "" {
		return c.CSS
	}
	return filepath.Join(filepath.Dir(c.Path), c.CSS)
}

// Duration reads "3s", "500ms" and so on.
type Duration time.Duration

func (d *Duration) UnmarshalYAML(n *yaml.Node) error {
	v, err := time.ParseDuration(n.Value)
	if err != nil {
		return fmt.Errorf("interval %q: %w", n.Value, err)
	}
	*d = Duration(v)
	return nil
}

func (d Duration) Std() time.Duration { return time.Duration(d) }
