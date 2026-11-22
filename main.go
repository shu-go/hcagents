package main

import (
	"embed"
	"log"
	"strconv"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/shu-go/hcagents/internal/config"
)

//go:embed assets
var assets embed.FS

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Println("config:", err) // keep going with the defaults
	}

	svc := NewAgentService(cfg)

	// One hcagents at a time: a second start shows the running one's window and exits.
	// Set before application.New, which is where a second instance hands over and
	// exits: it lets the running one take the foreground (Windows).
	allowForeground()
	var win *application.WebviewWindow
	// show shows the window and refreshes at once: while hidden, the list was
	// refreshed less often (hidden_interval).
	show := func() {
		win.Show()
		svc.Refresh()
	}
	app := application.New(application.Options{
		Name:        "hcagents",
		Description: "Herdr agents at a glance",
		Icon:        appIcon,
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.github.shu-go.hcagents",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if win != nil {
					show()
					win.Focus()
				}
			},
		},
		Services: []application.Service{application.NewService(svc)},
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(assets),
		},
		Mac: application.MacOptions{
			ActivationPolicy: application.ActivationPolicyAccessory, // tray app: no Dock icon
		},
	})

	winOpts := application.WebviewWindowOptions{
		Name:        "main",
		Title:       "hcagents",
		Width:       cfg.Window.Width,
		Height:      cfg.Window.Height,
		Frameless:   true,
		AlwaysOnTop: cfg.Window.AlwaysOnTop,
		URL:         "/",
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: true,
		},
	}
	statePath := cfg.StatePath()
	applySavedBounds(statePath, &winOpts) // where it was last time
	win = app.Window.NewWithOptions(winOpts)

	// The window's opacity (opacity:), full while the pointer is over it.
	opacity := &opacityCtl{win: win, level: cfg.Opacity}
	svc.onOpacity = opacity.set
	svc.onHover = opacity.setHover
	win.OnWindowEvent(events.Common.WindowRuntimeReady, func(*application.WindowEvent) { opacity.apply() })
	// hide hides the window. The pointer may leave it without the page noticing.
	hide := func() {
		win.Hide()
		opacity.setHover(false)
	}

	// Closing the window hides it; quit from the tray.
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		hide()
		e.Cancel()
	})

	ws := trackWindowState(app, win, statePath)
	// The window's height follows the list (the page asks with FitHeight).
	fitter := &windowFitter{win: win, grow: svc.grow, resized: opacity.apply}
	svc.onFit = fitter.fit
	svc.visible = win.IsVisible

	toggle := func() {
		if win.IsVisible() {
			hide()
		} else {
			show()
		}
	}

	tray := app.SystemTray.New()
	// The icon gets a badge for the most urgent group (Needs you / Done / Working),
	// the tooltip the counts; both follow each refresh.
	trayU := newTrayUpdater(tray, appIcon)
	setTrayIcon(tray, trayU.icons[""])
	tray.SetTooltip("hcagents")
	// A double click (Windows) focuses the top agent, like a click on its row.
	// Windows sends click, double click, click: the double click undoes the first
	// click's toggle and drops the second, so the window stays as it was.
	var dropClickUntil time.Time
	tray.OnClick(func() {
		if time.Now().Before(dropClickUntil) {
			dropClickUntil = time.Time{}
			return
		}
		toggle()
	})
	tray.OnDoubleClick(func() {
		toggle()
		dropClickUntil = time.Now().Add(time.Second)
		allowForeground() // now, while the click lets us: Herdr comes to the front
		go svc.focusFirst()
	})
	auto := newAutoShow()
	svc.onSnapshot = func(snap Snapshot) {
		trayU.update(trayStateOf(snap))
		switch auto.next(snap, svc.autoHide()) {
		case +1:
			if !win.IsVisible() {
				win.Show() // a background app can't take the foreground: no focus theft
			}
		case -1:
			if win.IsVisible() {
				hide()
			}
		}
	}
	// macOS draws the glyph in black or white by dark mode (see tray_darwin.go).
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) { trayU.reapply() })
	app.Event.OnApplicationEvent(events.Common.ThemeChanged, func(*application.ApplicationEvent) { trayU.reapply() })

	onTop := cfg.Window.AlwaysOnTop
	menu := app.NewMenu()
	menu.Add("Show / Hide").OnClick(func(*application.Context) { toggle() })
	menu.AddCheckbox("Always on top", onTop).OnClick(func(ctx *application.Context) {
		onTop = ctx.ClickedMenuItem().Checked()
		win.SetAlwaysOnTop(onTop)
	})
	addRadios(menu.AddSubmenu("Theme"), cfg.Theme, svc.SetTheme, []radioChoice{
		{"System", config.ThemeSystem},
		{"Light", config.ThemeLight},
		{"Dark", config.ThemeDark},
	})
	addRadios(menu.AddSubmenu("Grow"), cfg.Grow, svc.setGrow, []radioChoice{
		{"Upward", config.GrowUp},
		{"Downward", config.GrowDown},
	})
	addRadios(menu.AddSubmenu("Auto hide"), cfg.AutoHide, svc.setAutoHide, []radioChoice{
		{"When no agents", config.AutoHideEmpty},
		{"When all idle", config.AutoHideIdle},
		{"Off", config.AutoHideOff},
	})
	var opacities []radioChoice
	for pct := config.MaxOpacity; pct >= config.MinOpacity; pct -= 10 {
		opacities = append(opacities, radioChoice{strconv.Itoa(pct) + "%", strconv.Itoa(pct)})
	}
	addRadios(menu.AddSubmenu("Opacity"), strconv.Itoa(cfg.Opacity), func(v string) {
		pct, _ := strconv.Atoi(v)
		svc.setOpacity(pct)
	}, opacities)
	menu.AddCheckbox("Status messages", cfg.StatusMsg).OnClick(func(ctx *application.Context) {
		svc.setStatusMsg(ctx.ClickedMenuItem().Checked())
	})
	menu.Add("Refresh").OnClick(func(*application.Context) { svc.Refresh() })
	menu.AddSeparator()
	menu.Add("Quit").OnClick(func(*application.Context) {
		ws.flush()
		app.Quit()
	})
	tray.SetMenu(menu)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

type radioChoice struct{ label, value string }

// addRadios adds a radio group to m; picking one calls set with its value.
// The other items are unchecked here: Wails groups radios only in Menu.Update,
// which a tray menu set before app.Run never gets (Windows). Their stale checked
// flag then made a click on them a no-op (it returns early for a checked radio).
func addRadios(m *application.Menu, current string, set func(string), choices []radioChoice) {
	items := make([]*application.MenuItem, len(choices))
	for i, c := range choices {
		items[i] = m.AddRadio(c.label, current == c.value)
	}
	for i, c := range choices {
		items[i].OnClick(func(*application.Context) {
			for j, it := range items {
				it.SetChecked(j == i)
			}
			set(c.value)
		})
	}
}
