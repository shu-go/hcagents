package config

import (
	"testing"

	"go.yaml.in/yaml/v3"
)

func TestNormalizeOpacity(t *testing.T) {
	for in, want := range map[int]int{
		100: 100, 70: 70, 74: 70, 75: 80, 30: 30, 20: 20, 4: 0, 0: 0, -5: 0, -20: 0, 150: 100,
	} {
		if got := NormalizeOpacity(in); got != want {
			t.Errorf("NormalizeOpacity(%d) = %d, want %d", in, got, want)
		}
	}
}

func TestAutoHideYAML(t *testing.T) {
	for in, want := range map[string]bool{
		"true": true, "false": false, "yes": true, "no": false, "On": true, "OFF": false,
		// the older modes
		"empty": true, "idle": true, "off": false,
		`""`: true, "x": true,
	} {
		var c struct {
			A AutoHide `yaml:"auto_hide"`
		}
		c.A = true
		if err := yaml.Unmarshal([]byte("auto_hide: "+in), &c); err != nil {
			t.Fatalf("%s: %v", in, err)
		}
		if bool(c.A) != want {
			t.Errorf("auto_hide: %s = %v, want %v", in, c.A, want)
		}
	}
}

func TestNormalizeOpacityScope(t *testing.T) {
	for in, want := range map[string]string{
		"": "background", "background": "background", "Window": "window", " window ": "window", "x": "background",
	} {
		if got := NormalizeOpacityScope(in); got != want {
			t.Errorf("NormalizeOpacityScope(%q) = %q, want %q", in, got, want)
		}
	}
}
