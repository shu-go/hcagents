package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// newFileHeader starts a config file that hcagents creates itself.
const newFileHeader = "# hcagents settings. See hcagents.example.yaml for all of them.\n"

// SetValue writes `key: value` at the top level of the config file. Only that
// line changes (its comment, the other lines and their comments stay as they
// are); a missing key is added at the end. value is written as a plain scalar.
//
// With no config file yet, it creates one next to the executable (or, when that
// folder can't be written, in the user's config folder) and uses it from then on.
func (c *Config) SetValue(key, value string) error {
	src := []byte(newFileHeader)
	if c.Path != "" {
		b, err := os.ReadFile(c.Path)
		if err != nil {
			return err
		}
		src = b
	}
	out := setTopLevel(src, key, value)

	// The result must still read, and read back the value.
	var check map[string]any
	if err := yaml.Unmarshal(out, &check); err != nil {
		return fmt.Errorf("%s: %w", c.Path, err)
	}
	if got := fmt.Sprint(check[key]); got != value {
		return fmt.Errorf("%s: couldn't set %s (reads back %q)", c.Path, key, got)
	}

	if c.Path != "" {
		return writeFile(c.Path, out)
	}
	var errs []error
	for _, p := range newFileCandidates() {
		if err := writeFile(p, out); err != nil {
			errs = append(errs, err)
			continue
		}
		c.Path = p
		return nil
	}
	if len(errs) == 0 {
		return errors.New("no place for the config file")
	}
	return errors.Join(errs...)
}

// setTopLevel replaces the value on the top-level `key:` line, or appends the line.
func setTopLevel(src []byte, key, value string) []byte {
	// key: value  # comment      (value plain, "double" or 'single' quoted; CRLF kept)
	re := regexp.MustCompile(`(?m)^(` + regexp.QuoteMeta(key) + `:)[ \t]*(?:"[^"\r\n]*"|'[^'\r\n]*'|[^#\r\n]*?)([ \t]+#[^\r\n]*)?([ \t]*\r?)$`)
	if m := re.FindSubmatchIndex(src); m != nil {
		var b bytes.Buffer
		b.Write(src[:m[0]])
		b.Write(src[m[2]:m[3]]) // "key:"
		b.WriteByte(' ')
		b.WriteString(value)
		if m[4] >= 0 {
			b.Write(src[m[4]:m[5]]) // the comment
		}
		b.Write(src[m[6]:m[7]]) // "\r" of a CRLF line
		b.Write(src[m[1]:])
		return b.Bytes()
	}

	nl := "\n"
	if bytes.Contains(src, []byte("\r\n")) {
		nl = "\r\n"
	}
	out := append([]byte{}, src...)
	if len(out) > 0 && out[len(out)-1] != '\n' {
		out = append(out, nl...)
	}
	return append(out, key+": "+value+nl...)
}

// newFileCandidates is where a new config file goes: next to the executable, else
// the user's config folder. Not inside a macOS .app bundle: that would break its signature.
func newFileCandidates() []string {
	var list []string
	for _, p := range Candidates() {
		if strings.Contains(filepath.ToSlash(p), ".app/Contents/") {
			continue
		}
		list = append(list, p)
	}
	return list
}

// writeFile writes the file atomically (a temporary file, then a rename).
func writeFile(path string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
