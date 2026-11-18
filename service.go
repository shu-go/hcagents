package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/shu-go/hcagents/internal/config"
	"github.com/shu-go/hcagents/internal/herdr"
)

// AgentView is one row as the frontend draws it.
type AgentView struct {
	Terminal string `json:"terminal"`
	Status   string `json:"status"` // blocked, done, working, idle; "" for a sub terminal that isn't an agent
	// Group is the section the agent is listed in: the most urgent status of it and its sub-agents.
	Group string `json:"group,omitempty"`
	App   string `json:"app"`
	// Icon is the key in Herdr's program table ("claude", "shell", "agent", ...);
	// the frontend gets the picture with Icon(key). Tint: a one-color glyph.
	Icon    string `json:"icon"`
	Tint    bool   `json:"tint"`
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
	// Source: "app" (herdr-app ls) or "core" (hc events only: Herdr's window isn't running).
	Source string `json:"source"`
	Error  string `json:"error"`
	At     int64  `json:"at"`
}

var statusOrder = map[string]int{"blocked": 0, "done": 1, "working": 2, "idle": 3}

const maxDepth = 8

// AgentService polls Herdr and is called from the frontend.
type AgentService struct {
	cfg      *config.Config
	client   *herdr.Client
	programs *herdr.Programs

	mu      sync.Mutex
	last    Snapshot
	cssMod  time.Time
	refresh chan struct{} // poll now, herdr-app ls included
	changed chan struct{} // the core's terminals changed

	// The core's live terminals, kept by watch from `hc events`.
	coreMu sync.Mutex
	coreOK bool // the stream is up (its snapshot arrived)
	core   herdr.Terminals
	needLs bool // terminals came or went: ask herdr-app ls again

	// Used by the loop goroutine only.
	app    *herdr.Snapshot // the last herdr-app ls, nil when Herdr's window isn't running
	appErr error
	since  map[string]since // terminal -> when its status started

	startMu sync.Mutex // one Herdr start (and wait) at a time

	// onSnapshot, when set, gets each new snapshot (for the tray). Unexported:
	// Wails binds only exported methods and fields stay out of the bindings.
	onSnapshot func(Snapshot)
	// onFit, when set, resizes the window to the page's content (FitHeight).
	onFit func(content, inner int)
	// visible, when set, tells whether the window is shown (for the refresh interval).
	visible func() bool
	// onOpacity and onHover, when set, change the window's opacity (opacityCtl).
	onOpacity func(pct int)
	onHover   func(over bool)
}

type since struct {
	key string // changeKey of the status
	ms  int64
}

func NewAgentService(cfg *config.Config) *AgentService {
	programs, err := herdr.LoadPrograms(herdr.Home(cfg.HerdrHome))
	if err != nil {
		log.Printf("herdr programs.json: %v", err)
	}
	return &AgentService{
		cfg:      cfg,
		client:   &herdr.Client{AppPath: cfg.HerdrApp, CorePath: cfg.Hc},
		programs: programs,
		refresh:  make(chan struct{}, 1),
		changed:  make(chan struct{}, 1),
		core:     herdr.Terminals{},
		since:    map[string]since{},
	}
}

func (s *AgentService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	if s.cfg.StartHerdr {
		go s.startHerdr(ctx) // the list shows the core's agents meanwhile
	}
	// The OS switched between light and dark: matters for the "system" setting.
	application.Get().Event.OnApplicationEvent(events.Common.ThemeChanged, func(*application.ApplicationEvent) {
		s.emitTheme()
	})
	go s.watch(ctx)
	go s.loop(ctx)
	return nil
}

// herdrStartTimeout is how long to wait for a Herdr we started to answer.
const herdrStartTimeout = 15 * time.Second

// startHerdr starts Herdr's window if it isn't running yet, and waits until it answers.
func (s *AgentService) startHerdr(ctx context.Context) {
	if err := s.ensureHerdr(ctx); err != nil {
		log.Println(err)
	}
}

// ensureHerdr starts Herdr's window when it isn't running and waits until it
// answers. One at a time: a click while Herdr is starting waits for that start.
func (s *AgentService) ensureHerdr(ctx context.Context) error {
	s.startMu.Lock()
	defer s.startMu.Unlock()
	started, err := s.client.Start(ctx)
	if err != nil {
		return fmt.Errorf("start herdr-app: %w", err)
	}
	if !started {
		return nil
	}
	log.Println("started herdr-app")
	if err := s.client.WaitReady(ctx, herdrStartTimeout); err != nil {
		return err
	}
	s.Refresh()
	return nil
}

// watch keeps s.core up to date from the core's event stream (`hc events`), and
// starts the stream again when it ends.
func (s *AgentService) watch(ctx context.Context) {
	backoff := time.Second
	for {
		began := time.Now()
		err := s.client.Events(ctx, s.onEvent)
		s.coreMu.Lock()
		wasOK := s.coreOK
		s.coreOK = false
		clear(s.core)
		s.coreMu.Unlock()
		if wasOK {
			s.coreChanged()
		}
		if ctx.Err() != nil {
			return
		}
		log.Println(err)
		if time.Since(began) > time.Minute {
			backoff = time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, 30*time.Second)
	}
}

func (s *AgentService) onEvent(ev herdr.Event) {
	s.coreMu.Lock()
	changed, moved, err := s.core.Apply(ev, time.Now().UnixMilli())
	if ev.Type == "snapshot" {
		s.coreOK = true
	}
	// A status change: Herdr's own view (done turns idle once seen) is read again too.
	if moved || (changed && ev.Type == "status") {
		s.needLs = true
	}
	s.coreMu.Unlock()
	if err != nil {
		log.Println("hc events:", err)
	}
	if changed {
		s.coreChanged()
	}
}

// coreChanged asks the loop to rebuild the list.
func (s *AgentService) coreChanged() {
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

// settle is how long a burst of core events (titles change often) is gathered into one update.
const settle = 300 * time.Millisecond

// loop builds and emits the list: at once when the core reports a change, and
// every interval with herdr-app ls too (Herdr's window, spaces and tabs).
func (s *AgentService) loop(ctx context.Context) {
	t := time.NewTimer(s.interval()) // Reset below drops a stale tick (Go 1.23+ timers)
	defer t.Stop()
	// Give the core's stream a moment for its snapshot, so the first list is whole
	// (and doesn't flash an error while Herdr's window is closed).
	select {
	case <-ctx.Done():
		return
	case <-s.changed:
	case <-time.After(2 * time.Second):
	}
	full := true
	for {
		s.poll(ctx, full)
		if full {
			t.Reset(s.interval())
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			full = true
		case <-s.refresh:
			full = true
		case <-s.changed:
			select {
			case <-ctx.Done():
				return
			case <-time.After(settle):
			}
			s.coreMu.Lock()
			full = s.needLs
			s.coreMu.Unlock()
		}
	}
}

// interval is the time until the next herdr-app ls: longer while the window is
// hidden. The core's changes come as they happen either way (the tray badge follows).
func (s *AgentService) interval() time.Duration {
	if s.visible != nil && !s.visible() {
		return s.cfg.HiddenInterval.Std()
	}
	return s.cfg.Interval.Std()
}

// poll builds the list and emits it. full: ask herdr-app ls again first; such a
// poll is always emitted (the page redraws the elapsed times), a core-only one
// only when the list changed.
func (s *AgentService) poll(ctx context.Context, full bool) {
	if full {
		s.coreMu.Lock()
		s.needLs = false
		s.coreMu.Unlock()
		s.app, s.appErr = s.client.Ls(ctx)
	}
	snap := s.collect()
	s.mu.Lock()
	same := !full && sameSnapshot(s.last, snap)
	s.last = snap
	cssChanged := s.checkCSS()
	s.mu.Unlock()

	app := application.Get()
	if !same {
		if s.onSnapshot != nil {
			s.onSnapshot(snap)
		}
		app.Event.Emit("agents", snap)
	}
	if cssChanged {
		app.Event.Emit("css", s.UserCSS())
	}
}

// sameSnapshot: a and b draw the same (At aside).
func sameSnapshot(a, b Snapshot) bool {
	a.At, b.At = 0, 0
	return reflect.DeepEqual(a, b)
}

func (s *AgentService) collect() Snapshot {
	snap := Snapshot{At: time.Now().UnixMilli()}

	// The core's terminals give sub-agents' parents and statuses, titles, names and pins.
	s.coreMu.Lock()
	coreOK := s.coreOK
	list := s.core.Sorted()
	s.coreMu.Unlock()
	core := map[string]herdr.Terminal{}
	children := map[string][]string{} // parent -> children, oldest first
	for _, t := range list {
		core[t.Address] = t
		if p := t.ParentAddr(); p != "" {
			children[p] = append(children[p], t.Address)
		}
	}

	if s.app != nil && s.appErr == nil {
		snap.Source = "app"
		for _, a := range s.app.Agents() {
			// Herdr's sub_agents also covers other machines' children.
			for _, c := range a.SubAgents {
				if !contains(children[a.Terminal], c) {
					children[a.Terminal] = append(children[a.Terminal], c)
				}
			}
			snap.Agents = append(snap.Agents, AgentView{
				Terminal: a.Terminal, Status: liveStatus(a.Status, core[a.Terminal].StatusState()), App: a.App,
				Space: a.SpaceName, Machine: a.Machine, Tab: a.TabName,
			})
		}
	} else if coreOK {
		// Herdr's window isn't running: show what the core knows, without ids.
		snap.Source = "core"
		for _, t := range list {
			if t.StatusState() == "" {
				continue
			}
			space, tab, _ := strings.Cut(t.Labels["where"], " / ")
			snap.Agents = append(snap.Agents, AgentView{
				Terminal: t.Address, Status: t.StatusState(), App: t.Program(), Space: space, Tab: tab,
			})
		}
	} else {
		if s.appErr != nil {
			snap.Error = s.appErr.Error()
		} else {
			snap.Error = "no answer from Herdr yet"
		}
		return snap
	}

	// Pinned agents (terminal.pin): the label is on the terminal, so every Herdr client agrees.
	for i := range snap.Agents {
		snap.Agents[i].Pinned = core[snap.Agents[i].Terminal].Labels[pinnedLabel] == "1"
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
	s.fillMeta(snap.Agents, core)
	setDetails(snap.Agents)
	s.setIcons(snap.Agents, true)

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

// setIcons picks each row's icon as Herdr does: by the agent's app name, else the
// program it runs. top: the top-level rows, which are agents even without an app name.
func (s *AgentService) setIcons(list []AgentView, top bool) {
	for i := range list {
		p := s.programs.Lookup(list[i].App, top || list[i].Status != "")
		list[i].Icon, list[i].Tint = p.Key, p.Tint
		s.setIcons(list[i].Children, false)
	}
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

// liveStatus is a top-level agent's status: the core's, which arrives as it changes,
// over Herdr's from the last herdr-app ls. But Herdr's idle for the core's done is
// Herdr's own news (seen), and words Herdr maps itself (error) stay Herdr's.
func liveStatus(app, core string) string {
	if _, known := statusOrder[core]; !known {
		return app
	}
	if core == "done" && app == "idle" {
		return app
	}
	return core
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

func buildChildren(parent string, children map[string][]string, core map[string]herdr.Terminal, seen map[string]bool, depth int) []AgentView {
	if depth >= maxDepth || seen[parent] {
		return nil
	}
	seen[parent] = true
	var list []AgentView
	for _, addr := range children[parent] {
		v := AgentView{Terminal: addr}
		if t, ok := core[addr]; ok {
			v.Status = subStatus(t.StatusState())
			v.App = programName(t.Program())
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

// fillMeta adds the core's details: name, title, and when the status started.
// The start is kept across done -> idle (see changeKey), so it is tracked here.
func (s *AgentService) fillMeta(agents []AgentView, core map[string]herdr.Terminal) {
	now := time.Now().UnixMilli()
	seen := map[string]bool{}
	var walk func(list []AgentView)
	walk = func(list []AgentView) {
		for i := range list {
			a := &list[i]
			seen[a.Terminal] = true
			t, got := core[a.Terminal]
			m, ok := s.since[a.Terminal]
			if !ok || m.key != changeKey(a.Status) {
				m = since{key: changeKey(a.Status), ms: now}
				if got && t.StatusMS > 0 {
					m.ms = t.StatusMS
				}
				s.since[a.Terminal] = m
			}
			if a.Status != "" {
				a.SinceMS = m.ms
			}
			if got {
				if a.Name == "" && t.Name != nil {
					a.Name = *t.Name
				}
				a.Title = t.Title
			}
			walk(a.Children)
		}
	}
	walk(agents)
	for k := range s.since {
		if !seen[k] {
			delete(s.since, k)
		}
	}
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
// Then (Windows only, for now) it brings Herdr's window to the front.
// When Herdr's window isn't running, it starts it first.
func (s *AgentService) Focus(terminal string) error {
	ctx := context.Background()
	focus := func() error {
		return s.client.Do(ctx, "agent.focus", map[string]any{"terminal": terminal})
	}
	if err := focus(); err == nil {
		if err := s.client.RaiseWindow(); err != nil {
			log.Println("raise Herdr:", err)
		}
		return nil
	}
	// Herdr's window may not be running: start it (it comes up in front), then try again.
	// The core's terminal addresses are the ones herdr-app uses, so they work as they are.
	if err := s.ensureHerdr(ctx); err != nil {
		return err
	}
	return focus()
}

// Icon returns the picture for an AgentView's Icon key as a data: URL, taken from
// Herdr's home (icons/), or "" when Herdr has none there.
func (s *AgentService) Icon(key string) (string, error) {
	return s.programs.Icon(key)
}

// Refresh polls now.
func (s *AgentService) Refresh() {
	select {
	case s.refresh <- struct{}{}:
	default:
	}
}

// Theme returns the colors to draw in: "light" or "dark". For the "system"
// setting it is the OS's, read here rather than through the page's
// prefers-color-scheme, which the webview doesn't keep in step with the OS.
func (s *AgentService) Theme() string {
	s.mu.Lock()
	t := s.cfg.Theme
	s.mu.Unlock()
	if t != config.ThemeSystem {
		return t
	}
	if application.Get().Env.IsDarkMode() {
		return config.ThemeDark
	}
	return config.ThemeLight
}

// emitTheme tells the page the colors to draw in (the "theme" event).
func (s *AgentService) emitTheme() {
	application.Get().Event.Emit("theme", s.Theme())
}

// SetTheme switches the theme setting ("system", "light" or "dark") now and saves
// it in the config file (which is created next to the executable if there is none yet).
func (s *AgentService) SetTheme(theme string) {
	theme = config.NormalizeTheme(theme)
	s.mu.Lock()
	s.cfg.Theme = theme
	err := s.cfg.SetValue("theme", theme)
	s.mu.Unlock()
	if err != nil {
		log.Println("save theme:", err)
	}
	s.emitTheme()
}

// FitHeight makes the window as tall as the page's content: content is the height
// the page needs, inner its current viewport height (both in CSS px).
func (s *AgentService) FitHeight(content, inner int) {
	if s.onFit != nil {
		s.onFit(content, inner)
	}
}

// grow is the grow setting ("up" or "down").
func (s *AgentService) grow() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.Grow
}

// setGrow switches the way the window grows ("up" or "down") and saves it in the config file.
func (s *AgentService) setGrow(grow string) {
	grow = config.NormalizeGrow(grow)
	s.mu.Lock()
	s.cfg.Grow = grow
	err := s.cfg.SetValue("grow", grow)
	s.mu.Unlock()
	if err != nil {
		log.Println("save grow:", err)
	}
}

// autoHide is the auto_hide setting ("empty", "idle" or "off").
func (s *AgentService) autoHide() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cfg.AutoHide
}

// setAutoHide switches when the window hides and shows itself, and saves it in the config file.
func (s *AgentService) setAutoHide(mode string) {
	mode = config.NormalizeAutoHide(mode)
	s.mu.Lock()
	s.cfg.AutoHide = mode
	err := s.cfg.SetValue("auto_hide", mode)
	s.mu.Unlock()
	if err != nil {
		log.Println("save auto_hide:", err)
	}
}

// setOpacity switches the window's opacity (percent) and saves it in the config file.
func (s *AgentService) setOpacity(pct int) {
	pct = config.NormalizeOpacity(pct)
	s.mu.Lock()
	s.cfg.Opacity = pct
	err := s.cfg.SetValue("opacity", strconv.Itoa(pct))
	s.mu.Unlock()
	if err != nil {
		log.Println("save opacity:", err)
	}
	if s.onOpacity != nil {
		s.onOpacity(pct)
	}
}

// Hover tells whether the pointer is over the page: the window is opaque meanwhile.
func (s *AgentService) Hover(over bool) {
	if s.onHover != nil {
		s.onHover(over)
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
