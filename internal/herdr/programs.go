package herdr

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Home is Herdr's app folder (where icons/ and programs.json live):
// override if set, else $HERDR_APP_HOME, else $XDG_CONFIG_HOME/herdr-app, else ~/.config/herdr-app
// (on every OS, as Herdr does).
func Home(override string) string {
	if override != "" {
		return override
	}
	if h := os.Getenv("HERDR_APP_HOME"); h != "" {
		return h
	}
	if x := os.Getenv("XDG_CONFIG_HOME"); x != "" {
		return filepath.Join(x, "herdr-app")
	}
	if u, err := os.UserHomeDir(); err == nil {
		return filepath.Join(u, ".config", "herdr-app")
	}
	return ""
}

// Herdr's own program table is built into herdr-app; this is a copy of its defaults
// (only what picks an icon). The user's <home>/programs.json is merged over it, as Herdr does.
//
//go:embed programs.json
var defaultPrograms []byte

type programFile struct {
	Shells   []string                `json:"shells"`
	Programs map[string]programEntry `json:"programs"`
}

type programEntry struct {
	Icon  *string  `json:"icon"`
	Tint  *bool    `json:"tint"`
	Match []string `json:"match"`
}

// Program is how a terminal shows: Key is the table's key ("claude", "shell",
// "agent" for an unknown agent, "program" for an unknown program).
type Program struct {
	Key  string
	Icon string // relative to Home, e.g. "icons/claude.svg"; "" for none
	Tint bool   // a one-color glyph, drawn in the text color
}

// Programs finds a terminal's Program by the agent's app name or the process name.
type Programs struct {
	home   string
	byKey  map[string]Program
	byName map[string]string // lower-cased key or match name -> key
	shells map[string]bool

	mu    sync.Mutex
	icons map[string]string // key -> data: URL ("" when there is none)
}

// LoadPrograms reads the built-in table and <home>/programs.json over it.
// A broken user file is reported but the built-in table is still used.
func LoadPrograms(home string) (*Programs, error) {
	p := &Programs{home: home, byKey: map[string]Program{}, byName: map[string]string{}, shells: map[string]bool{}, icons: map[string]string{}}
	matches := map[string][]string{}

	var def programFile
	if err := json.Unmarshal(defaultPrograms, &def); err != nil {
		panic("herdr: built-in programs.json: " + err.Error())
	}
	p.merge(def, matches)

	var err error
	if home != "" {
		var b []byte
		b, err = os.ReadFile(filepath.Join(home, "programs.json"))
		if err == nil {
			var user programFile
			if err = json.Unmarshal(b, &user); err == nil {
				p.merge(user, matches)
			}
		} else if errors.Is(err, fs.ErrNotExist) {
			err = nil
		}
	}

	for key := range p.byKey {
		p.byName[strings.ToLower(key)] = key
	}
	for key, names := range matches {
		for _, n := range names {
			n = strings.ToLower(n)
			if _, taken := p.byName[n]; !taken {
				p.byName[n] = key
			}
		}
	}
	return p, err
}

func (p *Programs) merge(f programFile, matches map[string][]string) {
	for _, s := range f.Shells {
		p.shells[strings.ToLower(s)] = true
	}
	for key, e := range f.Programs {
		prog := p.byKey[key]
		prog.Key = key
		if e.Icon != nil {
			prog.Icon = *e.Icon
		}
		if e.Tint != nil {
			prog.Tint = *e.Tint
		}
		p.byKey[key] = prog
		if e.Match != nil {
			matches[key] = e.Match
		}
	}
}

// Lookup finds the Program for an agent's app name or a process name
// ("pwsh", "C:\...\node.exe"). agent: the terminal reports a status.
func (p *Programs) Lookup(name string, agent bool) Program {
	n := strings.ToLower(strings.TrimSuffix(filepath.Base(strings.ReplaceAll(name, `\`, "/")), ".exe"))
	if n != "" && n != "." && n != "/" {
		if key, ok := p.byName[n]; ok {
			return p.byKey[key]
		}
		if p.shells[n] {
			return p.byKey["shell"]
		}
	}
	if agent {
		return p.byKey["agent"]
	}
	return p.byKey["program"]
}

var iconTypes = map[string]string{
	".svg": "image/svg+xml", ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
	".gif": "image/gif", ".webp": "image/webp",
}

// Icon returns a key's icon as a data: URL (read once from Herdr's home), or "" if there is none.
func (p *Programs) Icon(key string) (string, error) {
	prog, ok := p.byKey[key]
	if !ok || prog.Icon == "" || p.home == "" {
		return "", nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if url, ok := p.icons[key]; ok {
		return url, nil
	}
	path := filepath.Join(p.home, filepath.FromSlash(prog.Icon))
	mime, ok := iconTypes[strings.ToLower(filepath.Ext(path))]
	if !ok {
		p.icons[key] = ""
		return "", nil
	}
	if fi, err := os.Stat(path); err == nil && fi.Size() > 1<<20 {
		return "", errors.New(path + ": too large for an icon")
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		p.icons[key] = ""
		return "", nil
	}
	if err != nil {
		return "", err
	}
	p.icons[key] = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(b)
	return p.icons[key], nil
}
