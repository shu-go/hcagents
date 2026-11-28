import * as wails from "/wails/runtime.js";

const SERVICE = "main.AgentService";
const call = (method, ...args) => wails.Call.ByName(`${SERVICE}.${method}`, ...args);

const GROUPS = [
  { status: "blocked", label: "Needs you" },
  { status: "done", label: "Done" },
  { status: "working", label: "Working" },
  { status: "idle", label: "Idle" },
];

const SHELLS = new Set(["pwsh", "powershell", "cmd", "bash", "zsh", "fish", "sh", "nu", "shell"]);

// ">_" prompt icon for shells; drawn in the text color.
const SHELL_SVG =
  '<svg viewBox="0 0 16 16" width="100%" height="100%" fill="none" stroke="currentColor" stroke-width="1.6" stroke-linecap="round" stroke-linejoin="round"><path d="M3 4.5 6.5 8 3 11.5"/><path d="M8.5 11.5h4.5"/></svg>';

// pin / unpin icons (drawn in the text color)
const PIN_SVG =
  '<svg viewBox="0 0 16 16" width="100%" height="100%" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"><path d="M6 2.5h4M7 2.5v4L4.5 9.5h7L9 6.5v-4M8 9.5V14"/></svg>';
const PIN_OFF_SVG =
  '<svg viewBox="0 0 16 16" width="100%" height="100%" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round"><path d="M6 2.5h4M7 2.5v4L4.5 9.5h7L9 6.5v-4M8 9.5V14M2.5 2.5l11 11"/></svg>';

const $titlebar = document.querySelector(".titlebar");
const $agents = document.querySelector(".agents");
const $message = document.querySelector(".message");
const $source = document.querySelector(".titlebar-source");
const $userCSS = document.getElementById("user-css");

let last = null;

// ---- collapsed parents: a per-viewer convenience, so localStorage is enough ----
const COLLAPSED_KEY = "hcagents.collapsed";
let collapsed = new Set();
try {
  collapsed = new Set(JSON.parse(localStorage.getItem(COLLAPSED_KEY) || "[]"));
} catch {}
function setCollapsed(terminal, on) {
  if (on) collapsed.add(terminal);
  else collapsed.delete(terminal);
  try {
    localStorage.setItem(COLLAPSED_KEY, JSON.stringify([...collapsed]));
  } catch {}
}

function el(tag, className, text) {
  const e = document.createElement(tag);
  if (className) e.className = className;
  if (text != null) e.textContent = text;
  return e;
}

function elapsed(sinceMs, now) {
  if (!sinceMs) return "";
  const m = Math.floor((now - sinceMs) / 60000);
  if (m < 1) return "<1m";
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h}h`;
  return `${Math.floor(h / 24)}d`;
}

// ---- icons: Herdr's own (from its home folder), fetched once per key ----
const iconURLs = new Map(); // key -> data: URL, "" when Herdr has none
const iconWaits = new Map(); // key -> Promise<string>
function iconURL(key) {
  if (!iconWaits.has(key)) {
    iconWaits.set(
      key,
      call("Icon", key)
        .catch(() => "")
        .then((url) => {
          iconURLs.set(key, url || "");
          return url || "";
        }),
    );
  }
  return iconWaits.get(key);
}

// setIconImage puts Herdr's picture in $icon: a one-color glyph (tint) is drawn in
// the text color through a mask, anything else as an image.
function setIconImage($icon, url, tint) {
  if (!url) return;
  let $pic;
  if (tint) {
    $pic = el("span", "icon-glyph");
    $pic.style.maskImage = $pic.style.webkitMaskImage = `url("${url}")`;
  } else {
    $pic = el("img", "icon-image");
    $pic.src = url;
    $pic.alt = "";
  }
  $icon.replaceChildren($pic);
  $icon.classList.add("has-image", tint ? "is-tint" : "is-picture");
}

// icon draws a row's icon: Herdr's picture when there is one, else a letter
// (or ">_" for a shell) until it loads or when Herdr's icons aren't found.
function icon(className, item) {
  const app = item.app || "";
  const isShell = item.icon === "shell" || SHELLS.has(app.toLowerCase());
  const $icon = el("span", className, isShell ? null : (app || "?").charAt(0).toUpperCase());
  $icon.dataset.app = app;
  if (item.icon) $icon.dataset.icon = item.icon;
  if (isShell) {
    $icon.classList.add("is-shell");
    $icon.innerHTML = SHELL_SVG;
  }
  if (item.icon) {
    if (iconURLs.has(item.icon)) setIconImage($icon, iconURLs.get(item.icon), item.tint);
    else iconURL(item.icon).then((url) => setIconImage($icon, url, item.tint));
  }
  return $icon;
}

function showError(e) {
  $message.classList.add("is-error");
  $message.textContent = String(e?.message ?? e);
  fitWindow();
}

// starting: the terminal clicked while Herdr wasn't running, until Herdr is up
// and shows it (Focus starts Herdr first, which takes a few seconds).
let starting = null;

function focusOnClick($row, terminal) {
  if (starting === terminal) $row.classList.add("is-starting");
  $row.addEventListener("click", (ev) => {
    ev.stopPropagation();
    const wasCore = last?.source === "core";
    if (wasCore) {
      starting = terminal;
      render(last);
    }
    const done = () => {
      if (wasCore && starting === terminal) {
        starting = null;
        render(last);
      }
    };
    call("Focus", terminal).then(done, (e) => {
      done();
      showError(e); // after render, so the message stays
    });
  });
}

// msgLine is an agent's own line on what it does now (status_msg:), under its row.
function msgLine(className, msg) {
  const $msg = el("div", className, msg);
  $msg.title = msg;
  return $msg;
}

// renderChildren draws sub-agents (recursively) as a tree.
function renderChildren(children) {
  const $list = el("div", "subagents");
  for (const c of children) {
    const status = c.status || "none";
    const $item = el("div", `subagent-item is-${status}`);
    const $row = el("div", `subagent is-${status}`);
    $row.dataset.terminal = c.terminal;
    $row.title = [c.title, c.terminal].filter(Boolean).join("\n");
    $row.append(
      icon("subagent-icon", c),
      el("span", "subagent-name", c.name || c.app || c.terminal),
      el("span", "subagent-status"),
    );
    focusOnClick($row, c.terminal);
    $item.append($row);
    if (c.msg) $item.append(msgLine("subagent-msg", c.msg));
    if (c.children && c.children.length) {
      $item.append(renderChildren(c.children));
    }
    $list.append($item);
  }
  return $list;
}

// canPin: pinning goes through Herdr's window, so only while it runs.
function renderAgent(a, now, canPin) {
  const $item = el("div", `agent-item is-${a.status}`);
  const $row = el("div", `agent is-${a.status}`);
  $row.dataset.terminal = a.terminal;
  $row.title = [a.title, a.terminal].filter(Boolean).join("\n");

  if (a.pinned) $row.classList.add("is-pinned");
  // detail: a named tab, else the agent's title (as Herdr shows it). No "·"
  // separators: the row's gap and the muted color set it apart from the name.
  const place = [a.machine, a.detail].filter(Boolean).join(" ");
  $row.append(icon("agent-icon", a), el("span", "agent-name", a.space), el("span", "agent-place", place));

  const hasChildren = a.children && a.children.length > 0;
  if (hasChildren) {
    const isCollapsed = collapsed.has(a.terminal);
    $item.classList.toggle("is-collapsed", isCollapsed);
    const $toggle = el("button", "agent-toggle", "⌄");
    $toggle.title = isCollapsed ? "Show sub-agents" : "Hide sub-agents";
    $toggle.addEventListener("click", (ev) => {
      ev.stopPropagation();
      setCollapsed(a.terminal, !collapsed.has(a.terminal));
      render(last);
    });
    $row.append($toggle);
  }
  // Shown on hover in place of the elapsed time (CSS), like Herdr's pin button.
  if (canPin) {
    const $pin = el("button", "agent-pin");
    $pin.innerHTML = a.pinned ? PIN_OFF_SVG : PIN_SVG;
    $pin.title = a.pinned ? "Unpin" : "Pin to the top";
    $pin.addEventListener("click", (ev) => {
      ev.stopPropagation();
      call("Pin", a.terminal, !a.pinned).catch(showError);
    });
    $row.append($pin);
  }
  $row.append(el("span", "agent-elapsed", elapsed(a.sinceMs, now)), el("span", "agent-status"));
  focusOnClick($row, a.terminal);

  $item.append($row);
  if (a.msg) $item.append(msgLine("agent-msg", a.msg));
  if (hasChildren) $item.append(renderChildren(a.children));
  return $item;
}

function render(snap) {
  last = snap;
  const now = Date.now();

  $message.classList.toggle("is-error", !!snap.error);
  if (snap.error) {
    $message.textContent = snap.error;
  } else if (starting) {
    $message.textContent = "Starting Herdr…";
  } else if (snap.source === "core") {
    $message.textContent = "Herdr is not running: click an agent to start it";
  } else {
    $message.textContent = "";
  }
  $source.textContent = snap.source === "core" ? "core" : "";

  const canPin = snap.source === "app";
  const agents = snap.agents || [];
  const groups = [];
  const addGroup = (className, label, list) => {
    if (list.length === 0) return;
    const $group = el("section", `group ${className}`);
    const $title = el("div", "group-title");
    $title.append(el("span", "group-name", label), el("span", "group-count", String(list.length)));
    $group.append($title);
    for (const a of list) $group.append(renderAgent(a, now, canPin));
    groups.push($group);
  };
  // Pinned agents first, in their own group (as Herdr does).
  addGroup("is-pinned", "Pinned", agents.filter((a) => a.pinned));
  for (const g of GROUPS) {
    // group: the most urgent of the agent and its sub-agents (as Herdr sections them)
    addGroup(`is-${g.status}`, g.label, agents.filter((a) => !a.pinned && (a.group || a.status) === g.status));
  }
  $agents.replaceChildren(...groups);
  fitWindow();
}

// ---- window height: as tall as the content (the app grows it up or down) ----
let lastFit = "";
function fitWindow() {
  // scrollHeight: the list's full height, even while it scrolls.
  const content = Math.ceil($titlebar.offsetHeight + $message.offsetHeight + $agents.scrollHeight);
  const inner = window.innerHeight;
  const key = `${content}/${inner}`;
  if (key === lastFit) return; // nothing changed since the last ask
  lastFit = key;
  call("FitHeight", content, inner).catch(() => {});
}

function setCSS(css) {
  $userCSS.textContent = css || "";
  fitWindow();
}

// ---- theme: <html data-theme> is "light" or "dark" ----
// The app resolves the tray menu's System / Light / Dark (System: the OS's
// setting, read natively: the webview's prefers-color-scheme doesn't follow the
// OS) and sends the result. The last one is kept in localStorage, so the next
// start begins in it (see index.html).
const THEME_KEY = "hcagents.theme";
function setTheme(t) {
  if (t !== "light" && t !== "dark") return;
  document.documentElement.dataset.theme = t;
  try {
    localStorage.setItem(THEME_KEY, t);
  } catch {}
}

// ---- opacity: the window is transparent; the page paints it (style.css) ----
// <html data-opacity="background" | "window"> with --opacity (0 to 0.9), only
// while the level is below 100%; none at 100%. Kept in localStorage like the
// theme, for the first paint (index.html).
const OPACITY_KEY = "hcagents.opacity";
function setOpacity(o) {
  // Not `|| 100`: 0% is a level too.
  const n = Number(o?.level);
  const level = Number.isFinite(n) ? Math.min(100, Math.max(0, n)) : 100;
  const scope = o?.scope === "window" ? "window" : "background";
  const root = document.documentElement;
  if (level < 100) {
    root.dataset.opacity = scope;
    root.style.setProperty("--opacity", String(level / 100));
  } else {
    delete root.dataset.opacity;
    root.style.removeProperty("--opacity");
  }
  try {
    localStorage.setItem(OPACITY_KEY, JSON.stringify({ level, scope }));
  } catch {}
}

// <html data-hover> while the pointer is over the window: opaque meanwhile. Not
// :hover, which stays on when the window hides under the pointer.
function setHover(on) {
  if (on) document.documentElement.dataset.hover = "";
  else delete document.documentElement.dataset.hover;
}
document.documentElement.addEventListener("pointerenter", () => setHover(true));
document.documentElement.addEventListener("pointermove", () => setHover(true));
document.documentElement.addEventListener("pointerleave", () => setHover(false));

document.querySelector(".refresh-button").addEventListener("click", () => call("Refresh"));
document.querySelector(".hide-button").addEventListener("click", () => {
  setHover(false);
  wails.Window.Hide();
});

wails.Events.On("agents", (ev) => render(ev.data));
wails.Events.On("css", (ev) => setCSS(ev.data));
wails.Events.On("theme", (ev) => setTheme(ev.data));
wails.Events.On("opacity", (ev) => setOpacity(ev.data));
wails.Events.On("hidden", () => setHover(false)); // the app hid the window

// Elapsed times keep moving between refreshes.
setInterval(() => last && render(last), 30000);

// A web font or the user's CSS may change the rows' height after a render.
document.fonts?.ready.then(fitWindow);

call("UserCSS").then(setCSS);
call("Theme").then(setTheme);
call("Opacity").then(setOpacity);
call("Agents").then((snap) => snap && snap.at && render(snap));
