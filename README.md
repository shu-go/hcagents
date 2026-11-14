# hcagents

[日本語](README_ja.md)

A small always-on-top window that shows your [Herdr](https://herdr.dev) agents,
the way Herdr's own agents sidebar does: for keeping an eye on them while you
work in other apps.

- Lists every agent Herdr knows, grouped as **Pinned / Blocked / Done / Working / Idle**,
  in the same order as Herdr (most urgent first, then the most recent change)
- Shows each agent's space, machine (for remote ones) and the same detail Herdr
  shows: a named tab, or the agent's title
- Sub-agents as a tree under their parent (collapsible)
- Click a row to switch Herdr to that agent's space and tab (a sub-agent opens
  as a peek). On Windows, Herdr's window is also brought to the front
- Pin / unpin agents (shared with Herdr)
- Herdr's own icons for agents and programs
- Frameless, always on top, lives in the system tray; remembers its position and size
- The look is plain HTML + CSS with readable class names: restyle it with your own stylesheet

Built with Go and [Wails v3](https://v3alpha.wails.io).

## Requirements

- Herdr: `herdr-app` and `hc` (herdr-core), on `PATH` or set in the config
- Windows 10/11 (WebView2), macOS 11+, or Linux (GTK 3, WebKitGTK 4.1)

Developed and used on Windows. The macOS and Linux builds are written but not tested yet.

## Build

Needs Go 1.27+, [Task](https://taskfile.dev) and the `wails3` CLI
(`go install github.com/wailsapp/wails/v3/cmd/wails3@v3.1.0`; only for the icon
and Windows resource files).

```sh
task build   # bin/hcagents.exe, bin/hcagents.app or bin/hcagents
task run     # build and start
```

| Task | What it does |
|---|---|
| `task build` | Build for this OS (no cross builds: macOS and Linux need cgo) |
| `task run` | Build and start |
| `task check` | gofmt and go vet |
| `task test` | go test |
| `task icons` | Make `build/windows/icon.ico` and `build/darwin/icons.icns` from `build/appicon.png` |
| `task navcheck` | Check which herdr-app command switches to each agent's space/tab |
| `task clean` | Remove `bin/` |

Without Task, `go build -ldflags "-H windowsgui" .` (Windows) or `go build .`
also works; the Windows exe then has no icon of its own.

## Usage

Start it and it sits in the corner of your screen and in the system tray.

- **Click a row**: go to that agent in Herdr
- **⌄** (on rows with sub-agents): collapse or expand them
- **Pin** (on hover, in place of the elapsed time): pin or unpin
- **Title bar**: drag to move; ⟳ refreshes now; × hides the window
- **Tray icon**: click to show / hide; the menu has Show / Hide, Always on top, Refresh and Quit

When Herdr's window isn't running, hcagents still lists the agents from herdr-core
(the title bar shows "core", with a note above the list), but rows can't be clicked: there is no window to switch.

## Configuration

Optional. Copy [hcagents.example.yaml](hcagents.example.yaml) to `hcagents.yaml`,
either next to the executable or in your config folder:

| OS | Path |
|---|---|
| Windows | `%AppData%\hcagents\hcagents.yaml` |
| macOS | `~/Library/Application Support/hcagents/hcagents.yaml` |
| Linux | `~/.config/hcagents/hcagents.yaml` |

The first one found is used.

```yaml
herdr_app: ""     # herdr-app executable; empty: from PATH
hc: ""            # hc executable; empty: from PATH
herdr_home: ""    # Herdr's app folder (icons); empty: $HERDR_APP_HOME, else ~/.config/herdr-app
interval: 3s      # how often to refresh
css: user.css     # your stylesheet, relative to this file; reloaded when it changes
window:
  width: 280      # size on first start
  height: 360
  always_on_top: true
```

On macOS, apps started from Finder don't get your shell's `PATH` or `HERDR_APP_HOME`,
so set `herdr_app`, `hc` and `herdr_home` there.

The window's position and size are saved in `state.yaml`, next to `hcagents.yaml`
(or in the config folder above). Delete it to reset.

## Styling

Your stylesheet (`css:` in the config) is loaded after the built-in
[assets/style.css](assets/style.css), so it can override anything. The comment
at the top of that file describes the structure; in short:

```
.agents
  .group.is-pinned | .is-blocked | .is-done | .is-working | .is-idle
    .group-title (.group-name .group-count)
    .agent-item.is-<status> [.is-collapsed]
      .agent.is-<status> [.is-disabled] [.is-pinned]
        .agent-icon .agent-name .agent-place .agent-toggle .agent-pin .agent-elapsed .agent-status
      .subagents
        .subagent-item.is-<status>
          .subagent (.subagent-icon .subagent-name .subagent-status)
          .subagents   (nested)
```

Most colors and sizes are CSS variables, for example:

```css
:root {
  --color-bg: #ffffff;
  --color-fg: #222222;
  --color-working: #b8860b;
  --font-size: 12px;
  --row-height: 26px;
}

/* hide the elapsed time */
.agent-elapsed { display: none; }
```

## Icons

Agent and program icons are Herdr's own, read at run time from Herdr's app
folder (`icons/`); hcagents doesn't ship them. Which icon a name gets comes from
a copy of Herdr's built-in program table, with your `programs.json` in Herdr's
folder merged over it, as Herdr does. If Herdr's folder isn't found, rows show a
letter instead.

## Notes

- **Done** is short-lived: Herdr turns it into idle once you look at the agent in Herdr.
- Sub-agent details come from the local herdr-core, so remote machines' sub-agents
  show with less detail.
- Bringing Herdr's window to the front after a click is Windows only for now.

## License

BSD 2-Clause. See [LICENSE](LICENSE).
