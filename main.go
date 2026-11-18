package main

import (
	"embed"
	"log"

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
	// Closing the window hides it; quit from the tray.
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		win.Hide()
		e.Cancel()
	})

	ws := trackWindowState(app, win, statePath)
	// The window's height follows the list (the page asks with FitHeight).
	fitter := &windowFitter{win: win, grow: svc.grow}
	svc.onFit = fitter.fit
	svc.visible = win.IsVisible

	toggle := func() {
		if win.IsVisible() {
			win.Hide()
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
	tray.OnClick(toggle)
	auto := newAutoShow()
	svc.onSnapshot = func(snap Snapshot) {
		trayU.update(trayStateOf(snap))
		if !cfg.AutoHide {
			return
		}
		switch auto.next(snap) {
		case +1:
			if !win.IsVisible() {
				win.Show() // a background app can't take the foreground: no focus theft
			}
		case -1:
			if win.IsVisible() {
				win.Hide()
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
