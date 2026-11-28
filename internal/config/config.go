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
	// AutoHide hides and shows the window by itself: "empty" (hide when the last
	// agent goes, show when agents appear), "idle" (hide when all agents are idle,
	// show when one isn't) or "off". Set from the tray menu too.
	AutoHide string `yaml:"auto_hide"`
	// Show is which agents the window lists, by section: "all", "active" (all but
	// idle) or "attention" (all but idle and working). Pinned agents are always
	// listed. Set from the tray menu too.
	Show string `yaml:"show"`
	// Opacity in percent: 30 to 100, in steps of 10. The window is opaque while
	// the pointer is over it. Set from the tray menu too.
	Opacity int `yaml:"opacity"`
	// OpacityScope is what Opacity applies to: "background" (the agents stay
	// opaque) or "window" (all of it). Set from the tray menu too.
	OpacityScope string `yaml:"opacity_scope"`
	// StatusMsg shows each agent's own line on what it does now
	// (the status report's msg, e.g. Claude Code's running tool) under its row.
	// Set from the tray menu too.
	StatusMsg bool `yaml:"status_msg"`

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
		AutoHide:       AutoHideEmpty,
		Show:           ShowAll,
		Opacity:        MaxOpacity,
		OpacityScope:   OpacityBackground,
		StatusMsg:      true,
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
	cfg.AutoHide = NormalizeAutoHide(cfg.AutoHide)
	cfg.Show = NormalizeShow(cfg.Show)
	cfg.Opacity = NormalizeOpacity(cfg.Opacity)
	cfg.OpacityScope = NormalizeOpacityScope(cfg.OpacityScope)
	return cfg, nil
}

const (
	OpacityBackground = "background"
	OpacityWindow     = "window"
)

// NormalizeOpacityScope returns "window" as it is (any case), else "background".
func NormalizeOpacityScope(s string) string {
	if v := strings.ToLower(strings.TrimSpace(s)); v == OpacityWindow {
		return v
	}
	return OpacityBackground
}

const (
	ShowAll       = "all"
	ShowActive    = "active"    // all but idle
	ShowAttention = "attention" // all but idle and working: Needs you, Done
)

// NormalizeShow returns "active" or "attention" as they are (any case), else "all".
func NormalizeShow(s string) string {
	switch v := strings.ToLower(strings.TrimSpace(s)); v {
	case ShowActive, ShowAttention:
		return v
	}
	return ShowAll
}

const (
	MinOpacity = 30 // lower and the window is hard to find
	MaxOpacity = 100
)

// NormalizeOpacity rounds a percentage to a step of 10 within MinOpacity..MaxOpacity.
func NormalizeOpacity(pct int) int {
	pct = (pct + 5) / 10 * 10
	return max(MinOpacity, min(MaxOpacity, pct))
}

const (
	AutoHideOff   = "off"
	AutoHideEmpty = "empty"
	AutoHideIdle  = "idle"
)

// NormalizeAutoHide returns "idle" or "off" (also false/no) as they are (any
// case), else "empty" (also true, from when auto_hide was a yes/no setting).
func NormalizeAutoHide(s string) string {
	switch a := strings.ToLower(strings.TrimSpace(s)); a {
	case AutoHideIdle, AutoHideOff:
		return a
	case "false", "no":
		return AutoHideOff
	}
	return AutoHideEmpty
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
