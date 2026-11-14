package main

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/wailsapp/wails/v3/pkg/application"

	"github.com/shu-go/hcagents/internal/config"
	"github.com/shu-go/hcagents/internal/herdr"
)

// AgentView is one row as the frontend draws it.
type AgentView struct {
	Terminal string `json:"terminal"`
	Status   string `json:"status"` // blocked, done, working, idle; "" for a sub terminal that isn't an agent
	// Group is the section the agent is listed in: the most urgent status of it and its sub-agents.
	Group   string `json:"group,omitempty"`
	App     string `json:"app"`
	Name    string `json:"name"` // the terminal's name (hc run --name), if any
	Space   string `json:"space"`
	Machine string `json:"machine"`
	Tab     string `json:"tab"`
	Title   string `json:"title"`   // the terminal's own title (OSC), as the program set it
	SinceMS int64  `json:"sinceMs"` // when the status started (unix ms), 0 if unknown
	// Detail is what goes after the space name, as Herdr shows it: a named tab,
	// else the agent's title when it tells something, else nothing.
	Detail string `json:"detail"`
	Pinned bool   `json:"pinned"`
	// Children are the terminals this one started with `hc run` (sub-agents).
	Children []AgentView `json:"children"`
}

// Snapshot is what the "agents" event carries.
type Snapshot struct {
	Agents []AgentView `json:"agents"`
	// Source: "app" (herdr-app ls) or "core" (hc ls: Herdr's window isn't running).
	Source string `json:"source"`
	Error  string `json:"error"`
	At     int64  `json:"at"`
}

var statusOrder = map[string]int{"blocked": 0, "done": 1, "working": 2, "idle": 3}

const maxDepth = 8

// AgentService polls Herdr and is called from the frontend.
type AgentService struct {
	cfg    *config.Config
	client *herdr.Client

	mu      sync.Mutex
	last    Snapshot
	meta    map[string]termMeta // terminal -> details from `hc get`
	cssMod  time.Time
	refresh chan struct{}
}

type termMeta struct {
	status  string
	sinceMS int64
	name    string
	title   string
}

func NewAgentService(cfg *config.Config) *AgentService {
	return &AgentService{
		cfg:     cfg,
		client:  &herdr.Client{AppPath: cfg.HerdrApp, CorePath: cfg.Hc},
		meta:    map[string]termMeta{},
		refresh: make(chan struct{}, 1),
	}
}

func (s *AgentService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	go s.loop(ctx)
	return nil
}

func (s *AgentService) loop(ctx context.Context) {
	t := time.NewTicker(s.cfg.Interval.Std())
	defer t.Stop()
	for {
		s.poll(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-s.refresh:
		}
	}
}

func (s *AgentService) poll(ctx context.Context) {
	snap := s.collect(ctx)
	s.mu.Lock()
	s.last = snap
	cssChanged := s.checkCSS()
	s.mu.Unlock()

	app := application.Get()
	app.Event.Emit("agents", snap)
	if cssChanged {
		app.Event.Emit("css", s.UserCSS())
	}
}

func (s *AgentService) collect(ctx context.Context) Snapshot {
	snap := Snapshot{At: time.Now().UnixMilli()}

	// The core's list gives sub-agents' parents and statuses. It only knows this machine.
	core := map[string]herdr.CoreListTerminal{}
	children := map[string][]string{} // parent -> children, in hc's order
	cl, coreErr := s.client.CoreLs(ctx)
	if coreErr == nil {
		for _, t := range cl.Terminals {
			core[t.Address] = t
			if t.Parent != "" {
				children[t.Parent] = append(children[t.Parent], t.Address)
			}
		}
	}

	// Pinned agents (terminal.pin): the label is on the terminal, so every Herdr client agrees.
	pinned := map[string]bool{}
	if pl, err := s.client.CoreLs(ctx, pinnedLabel+"=1"); err == nil {
		for _, t := range pl.Terminals {
			pinned[t.Address] = true
		}
	}

	if ls, err := s.client.Ls(ctx); err == nil {
		snap.Source = "app"
		for _, a := range ls.Agents() {
			// Herdr's sub_agents also covers other machines' children.
			for _, c := range a.SubAgents {
				if !contains(children[a.Terminal], c) {
					children[a.Terminal] = append(children[a.Terminal], c)
				}
			}
			snap.Agents = append(snap.Agents, AgentView{
				Terminal: a.Terminal, Status: a.Status, App: a.App,
				Space: a.SpaceName, Machine: a.Machine, Tab: a.TabName,
			})
		}
	} else if coreErr == nil {
		// Herdr's window isn't running: show what the core knows, without ids.
		snap.Source = "core"
		for _, t := range cl.Terminals {
			if t.Status == "" {
				continue
			}
			space, tab, _ := strings.Cut(t.Where, " / ")
			snap.Agents = append(snap.Agents, AgentView{
				Terminal: t.Address, Status: t.Status, App: t.Agent, Space: space, Tab: tab,
			})
		}
	} else {
		snap.Error = err.Error()
		return snap
	}

	for i := range snap.Agents {
		snap.Agents[i].Pinned = pinned[snap.Agents[i].Terminal]
	}

	// A sub-agent given its own pane (terminal.keep) is also a tab's terminal.
	// Like Herdr, show it only under its parent while the parent is alive, unless it's pinned.
	alive := map[string]bool{}
	for addr := range core {
		alive[addr] = true
	}
	for _, a := range snap.Agents {
		alive[a.Terminal] = true
	}
	isChild := map[string]bool{}
	for parent, list := range children {
		if !alive[parent] {
			continue
		}
		for _, c := range list {
			isChild[c] = true
		}
	}
	top := snap.Agents[:0]
	for _, a := range snap.Agents {
		if a.Pinned || !isChild[a.Terminal] {
			top = append(top, a)
		}
	}
	snap.Agents = top

	for i := range snap.Agents {
		snap.Agents[i].Children = buildChildren(snap.Agents[i].Terminal, children, core, map[string]bool{}, 0)
	}
	s.fillMeta(ctx, snap.Agents)
	setDetails(snap.Agents)

	// Like Herdr's agents list (priority sort): an agent's section is the most urgent
	// of it and its sub-agents; within a section, the most recent change comes first.
	for i := range snap.Agents {
		snap.Agents[i].Group = urgency(&snap.Agents[i])
	}
	sort.SliceStable(snap.Agents, func(i, j int) bool {
		a, b := &snap.Agents[i], &snap.Agents[j]
		if ra, rb := rank(a.Group), rank(b.Group); ra != rb {
			return ra < rb
		}
		return a.SinceMS > b.SinceMS
	})
	return snap
}

// urgency is the most urgent status of a and its sub-agents (blocked > done > working > idle).
func urgency(a *AgentView) string {
	best := a.Status
	var walk func(list []AgentView)
	walk = func(list []AgentView) {
		for i := range list {
			if rank(list[i].Status) < rank(best) {
				best = list[i].Status
			}
			walk(list[i].Children)
		}
	}
	walk(a.Children)
	return best
}

// subStatus is how Herdr shows a sub-agent: never done (it reports to its parent,
// and the parent's done is the news); an error counts as blocked.
func subStatus(s string) string {
	switch s {
	case "done":
		return "idle"
	case "error":
		return "blocked"
	}
	return s
}

// changeKey is the status as far as "when did it change" goes: done and idle are one,
// since looking at a finished agent turns done into idle without the agent doing anything.
func changeKey(s string) string {
	if s == "done" {
		return "idle"
	}
	return s
}

func buildChildren(parent string, children map[string][]string, core map[string]herdr.CoreListTerminal, seen map[string]bool, depth int) []AgentView {
	if depth >= maxDepth || seen[parent] {
		return nil
	}
	seen[parent] = true
	var list []AgentView
	for _, addr := range children[parent] {
		v := AgentView{Terminal: addr}
		if t, ok := core[addr]; ok {
			v.Status = subStatus(t.Status)
			v.App = programName(t.Agent)
			v.Name = t.Name
		}
		v.Children = buildChildren(addr, children, core, seen, depth+1)
		list = append(list, v)
	}
	return list
}

// programName turns "C:\...\pwsh.exe" into "pwsh".
func programName(p string) string {
	p = filepath.Base(strings.ReplaceAll(p, `\`, "/"))
	return strings.TrimSuffix(p, ".exe")
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func rank(status string) int {
	if r, ok := statusOrder[status]; ok {
		return r
	}
	return len(statusOrder)
}

// fillMeta adds details from `hc get`. Top-level agents are asked every time
// (their title changes as they work, and it is shown); sub-agents only when they
// appear or their status changes.
func (s *AgentService) fillMeta(ctx context.Context, agents []AgentView) {
	fresh := s.getAll(ctx, agents)

	seen := map[string]bool{}
	var walk func(list []AgentView)
	walk = func(list []AgentView) {
		for i := range list {
			a := &list[i]
			seen[a.Terminal] = true
			m, ok := s.meta[a.Terminal]
			t, got := fresh[a.Terminal]
			if !ok || m.status != changeKey(a.Status) {
				m = termMeta{status: changeKey(a.Status), sinceMS: time.Now().UnixMilli()}
				if !got {
					t, _ = s.client.Get(ctx, a.Terminal)
				}
				if t != nil && t.StatusMS > 0 {
					m.sinceMS = t.StatusMS
				}
			}
			if t != nil {
				m.title = t.Title
				if t.Name != nil {
					m.name = *t.Name
				}
			}
			s.meta[a.Terminal] = m
			if a.Status != "" {
				a.SinceMS = m.sinceMS
			}
			if a.Name == "" {
				a.Name = m.name
			}
			a.Title = m.title
			walk(a.Children)
		}
	}
	walk(agents)
	for k := range s.meta {
		if !seen[k] {
			delete(s.meta, k)
		}
	}
}

// getAll runs `hc get` for the top-level agents, a few at a time.
func (s *AgentService) getAll(ctx context.Context, agents []AgentView) map[string]*herdr.CoreTerminal {
	var (
		mu  sync.Mutex
		wg  sync.WaitGroup
		out = map[string]*herdr.CoreTerminal{}
		sem = make(chan struct{}, 4)
	)
	for _, a := range agents {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			if t, err := s.client.Get(ctx, addr); err == nil {
				mu.Lock()
				out[addr] = t
				mu.Unlock()
			}
		}(a.Terminal)
	}
	wg.Wait()
	return out
}

const pinnedLabel = "herdr.pinned"

// setDetails picks each row's detail the way Herdr's agents list does
// (mods/agents/main.js: title() and the "space · detail" rules).
func setDetails(agents []AgentView) {
	perTab := map[string]int{}
	for _, a := range agents {
		perTab[a.Space+"\x00"+a.Tab]++
	}
	label := make([]string, len(agents))
	for i, a := range agents {
		// herdr-app ls doesn't say whether a tab was named; an unnamed tab is called by its number.
		named := !isDigits(a.Tab) && perTab[a.Space+"\x00"+a.Tab] == 1
		label[i] = a.Tab
		if (!named || sameProgram(a.Tab, a.App)) && tellingTitle(a.Title, a.App) {
			label[i] = cleanTitle(a.Title)
		}
		if !isDigits(label[i]) {
			agents[i].Detail = label[i]
		}
	}
	// A bare tab number says nothing, unless two rows of a space would otherwise read the same.
	bare := map[string]int{}
	for _, a := range agents {
		if a.Detail == "" {
			bare[a.Space]++
		}
	}
	for i := range agents {
		if agents[i].Detail == "" && bare[agents[i].Space] > 1 {
			agents[i].Detail = label[i]
		}
	}
}

// cleanTitle drops a leading status glyph ("✳ ", "⠋ ") and spaces.
func cleanTitle(s string) string {
	return strings.TrimSpace(strings.TrimLeftFunc(s, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}))
}

// tellingTitle: a title that says more than the program's name, and isn't a path or user@host.
func tellingTitle(title, app string) bool {
	live := cleanTitle(title)
	if live == "" || strings.Contains(live, "@") || sameProgram(live, app) {
		return false
	}
	t := strings.TrimLeftFunc(title, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '~' && r != '/'
	})
	if strings.HasPrefix(t, "~") || strings.HasPrefix(t, "/") {
		return false
	}
	if len(t) >= 3 && unicode.IsLetter(rune(t[0])) && t[1] == ':' && t[2] == '\\' {
		return false
	}
	return true
}

// sameProgram compares a text with a program key loosely: "Claude Code" is "claude-code".
func sameProgram(text, app string) bool {
	norm := func(s string) string {
		var b strings.Builder
		for _, r := range strings.ToLower(s) {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	return app != "" && norm(text) == norm(app)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// --- called from the frontend ---

// Pin pins an agent to the top of the list (in Herdr too), or unpins it.
func (s *AgentService) Pin(terminal string, pinned bool) error {
	err := s.client.Do(context.Background(), "terminal.pin", map[string]any{"terminal": terminal, "pinned": pinned})
	s.Refresh()
	return err
}

// Agents returns the latest snapshot (for the first draw).
func (s *AgentService) Agents() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.last
}

// Focus shows the agent in Herdr: its space and tab, or a peek for a sub-agent.
func (s *AgentService) Focus(terminal string) error {
	return s.client.Do(context.Background(), "agent.focus", map[string]any{"terminal": terminal})
}

// Refresh polls now.
func (s *AgentService) Refresh() {
	select {
	case s.refresh <- struct{}{}:
	default:
	}
}

// UserCSS returns the user's stylesheet ("" if none).
func (s *AgentService) UserCSS() string {
	p := s.cfg.CSSPath()
	if p == "" {
		return ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return ""
	}
	return string(b)
}

// checkCSS reports whether the user's stylesheet changed since the last check. Call with mu held.
func (s *AgentService) checkCSS() bool {
	p := s.cfg.CSSPath()
	if p == "" {
		return false
	}
	fi, err := os.Stat(p)
	if err != nil {
		return false
	}
	if fi.ModTime().Equal(s.cssMod) {
		return false
	}
	first := s.cssMod.IsZero()
	s.cssMod = fi.ModTime()
	return !first
}
