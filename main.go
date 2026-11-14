package main

import (
	"embed"
	"log"
	"runtime"

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
	app := application.New(application.Options{
		Name:        "hcagents",
		Description: "Herdr agents at a glance",
		Icon:        appIcon,
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.github.shu-go.hcagents",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if win != nil {
					win.Show()
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

	toggle := func() {
		if win.IsVisible() {
			win.Hide()
		} else {
			win.Show()
		}
	}

	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		// A template icon: macOS draws its alpha in the menu bar's color.
		tray.SetTemplateIcon(trayIcon(appIcon, black))
	} else {
		tray.SetIcon(trayIcon(appIcon, black))
		tray.SetDarkModeIcon(trayIcon(appIcon, white))
	}
	tray.SetTooltip("hcagents")
	tray.OnClick(toggle)

	onTop := cfg.Window.AlwaysOnTop
	menu := app.NewMenu()
	menu.Add("Show / Hide").OnClick(func(*application.Context) { toggle() })
	menu.AddCheckbox("Always on top", onTop).OnClick(func(ctx *application.Context) {
		onTop = ctx.ClickedMenuItem().Checked()
		win.SetAlwaysOnTop(onTop)
	})
	themeMenu := menu.AddSubmenu("Theme")
	for _, t := range []struct{ label, value string }{
		{"System", config.ThemeSystem},
		{"Light", config.ThemeLight},
		{"Dark", config.ThemeDark},
	} {
		themeMenu.AddRadio(t.label, cfg.Theme == t.value).OnClick(func(*application.Context) {
			svc.SetTheme(t.value)
		})
	}
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
