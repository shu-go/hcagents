import * as wails from "/wails/runtime.js";

const SERVICE = "main.AgentService";
const call = (method, ...args) => wails.Call.ByName(`${SERVICE}.${method}`, ...args);

const GROUPS = [
  { status: "blocked", label: "Blocked" },
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

function icon(className, app) {
  const isShell = SHELLS.has((app || "").toLowerCase());
  const $icon = el("span", className, isShell ? null : (app || "?").charAt(0).toUpperCase());
  $icon.dataset.app = app || "";
  if (isShell) {
    $icon.classList.add("is-shell");
    $icon.innerHTML = SHELL_SVG;
  }
  return $icon;
}

function showError(e) {
  $message.classList.add("is-error");
  $message.textContent = String(e?.message ?? e);
}

function focusOnClick($row, terminal, canFocus) {
  if (!canFocus) {
    $row.classList.add("is-disabled");
    return;
  }
  $row.addEventListener("click", (ev) => {
    ev.stopPropagation();
    call("Focus", terminal).catch(showError);
  });
}

// renderChildren draws sub-agents (recursively) as a tree.
function renderChildren(children, canFocus) {
  const $list = el("div", "subagents");
  for (const c of children) {
    const status = c.status || "none";
    const $item = el("div", `subagent-item is-${status}`);
    const $row = el("div", `subagent is-${status}`);
    $row.dataset.terminal = c.terminal;
    $row.title = [c.title, c.terminal].filter(Boolean).join("\n");
    $row.append(
      icon("subagent-icon", c.app),
      el("span", "subagent-name", c.name || c.app || c.terminal),
      el("span", "subagent-status"),
    );
    focusOnClick($row, c.terminal, canFocus);
    $item.append($row);
    if (c.children && c.children.length) {
      $item.append(renderChildren(c.children, canFocus));
    }
    $list.append($item);
  }
  return $list;
}

function renderAgent(a, now, canFocus) {
  const $item = el("div", `agent-item is-${a.status}`);
  const $row = el("div", `agent is-${a.status}`);
  $row.dataset.terminal = a.terminal;
  $row.title = [a.title, a.terminal].filter(Boolean).join("\n");

  if (a.pinned) $row.classList.add("is-pinned");
  // detail: a named tab, else the agent's title (as Herdr shows it)
  const place = [a.machine, a.detail].filter(Boolean).map((s) => `· ${s}`).join(" ");
  $row.append(icon("agent-icon", a.app), el("span", "agent-name", a.space), el("span", "agent-place", place));

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
  if (canFocus) {
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
  focusOnClick($row, a.terminal, canFocus);

  $item.append($row);
  if (hasChildren) $item.append(renderChildren(a.children, canFocus));
  return $item;
}

function render(snap) {
  last = snap;
  const now = Date.now();

  $message.classList.toggle("is-error", !!snap.error);
  if (snap.error) {
    $message.textContent = snap.error;
  } else if (snap.source === "core") {
    $message.textContent = "Herdr is not running: click does nothing";
  } else {
    $message.textContent = "";
  }
  $source.textContent = snap.source === "core" ? "core" : "";

  const canFocus = snap.source === "app";
  const agents = snap.agents || [];
  const groups = [];
  const addGroup = (className, label, list) => {
    if (list.length === 0) return;
    const $group = el("section", `group ${className}`);
    const $title = el("div", "group-title");
    $title.append(el("span", "group-name", label), el("span", "group-count", String(list.length)));
    $group.append($title);
    for (const a of list) $group.append(renderAgent(a, now, canFocus));
    groups.push($group);
  };
  // Pinned agents first, in their own group (as Herdr does).
  addGroup("is-pinned", "Pinned", agents.filter((a) => a.pinned));
  for (const g of GROUPS) {
    // group: the most urgent of the agent and its sub-agents (as Herdr sections them)
    addGroup(`is-${g.status}`, g.label, agents.filter((a) => !a.pinned && (a.group || a.status) === g.status));
  }
  $agents.replaceChildren(...groups);
}

function setCSS(css) {
  $userCSS.textContent = css || "";
}

document.querySelector(".refresh-button").addEventListener("click", () => call("Refresh"));
document.querySelector(".hide-button").addEventListener("click", () => wails.Window.Hide());

wails.Events.On("agents", (ev) => render(ev.data));
wails.Events.On("css", (ev) => setCSS(ev.data));

// Elapsed times keep moving between refreshes.
setInterval(() => last && render(last), 30000);

call("UserCSS").then(setCSS);
call("Agents").then((snap) => snap && snap.at && render(snap));
