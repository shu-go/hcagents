package herdr

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"strings"
	"time"
)

// Terminal is the part of the core's terminal record we use
// (events.subscribe, hc.openrpc.json: Terminal).
type Terminal struct {
	Address   string            `json:"address"` // "t7", or "machine/t7"
	Name      *string           `json:"name"`    // hc run --name
	Title     string            `json:"title"`   // as the program set it (OSC)
	Labels    map[string]string `json:"labels"`
	Parent    *string           `json:"parent"` // the terminal that started it with `hc run`
	State     string            `json:"state"`  // live, finished
	Argv      []string          `json:"argv"`
	Status    *TermStatus       `json:"status"`    // nil: not an agent (never reported)
	StatusMS  int64             `json:"status_ms"` // when the status last changed (unix ms)
	CreatedMS int64             `json:"created_ms"`
}

// TermStatus is a program's own status report (OSC 7501).
type TermStatus struct {
	State string `json:"state"` // idle, working, done, blocked, error
	App   string `json:"app"`
}

// StatusState is the status, "" for a terminal that isn't an agent.
func (t Terminal) StatusState() string {
	if t.Status == nil {
		return ""
	}
	return t.Status.State
}

// Program is what runs in it: the agent's own name, else the program (a path for some shells).
func (t Terminal) Program() string {
	if t.Status != nil && t.Status.App != "" {
		return t.Status.App
	}
	if len(t.Argv) > 0 {
		return t.Argv[0]
	}
	return ""
}

// ParentAddr is the parent's address, "" for none.
func (t Terminal) ParentAddr() string {
	if t.Parent == nil {
		return ""
	}
	return *t.Parent
}

// Event is one message of the core's event stream.
type Event struct {
	Seq uint64
	// Type is "snapshot" for the full list (first, and after the stream fell behind),
	// else the event's type: created, removed, status, title, labels, ...
	Type      string
	Terminal  string
	Data      json.RawMessage
	Terminals []Terminal // snapshot only
}

// eventIdle is how long the stream may stay silent: the core sends a heartbeat every 15s.
const eventIdle = 45 * time.Second

// Events runs `hc events --json` and calls fn for each message, in order, until
// ctx ends, the stream ends, or it stays silent too long. It always returns an error.
func (c *Client) Events(ctx context.Context, fn func(Event)) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	cmd := exec.CommandContext(ctx, c.core(), "events", "--json")
	hideWindow(cmd)
	cmd.WaitDelay = waitDelay
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr strings.Builder
	cmd.Stderr = &limitWriter{w: &stderr, n: 4096}
	t := newTree(cmd)
	if err := t.start(); err != nil {
		return err
	}
	defer t.close()

	idle := time.AfterFunc(eventIdle, cancel)
	defer idle.Stop()

	readErr := readEvents(bufio.NewReader(out), func(ev Event) {
		idle.Reset(eventIdle)
		fn(ev)
	}, func() { idle.Reset(eventIdle) })
	cancel()
	waitErr := cmd.Wait()

	msg := strings.TrimSpace(stderr.String())
	switch {
	case readErr != nil && !errors.Is(readErr, io.EOF):
		err = readErr
	case waitErr != nil:
		err = waitErr
	default:
		err = errors.New("stream ended")
	}
	if msg != "" {
		return fmt.Errorf("hc events: %w: %s", err, msg)
	}
	return fmt.Errorf("hc events: %w", err)
}

type wireMsg struct {
	// The first line is the subscribe result itself.
	Seq       *uint64    `json:"seq"`
	Terminals []Terminal `json:"terminals"`
	// Then JSON-RPC notifications.
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Error  *struct {
		Message string `json:"message"`
	} `json:"error"`
}

type wireParams struct {
	Seq       uint64          `json:"seq"`
	Type      string          `json:"type"`
	Terminal  string          `json:"terminal"`
	Data      json.RawMessage `json:"data"`
	Terminals []Terminal      `json:"terminals"`
}

// readEvents parses the stream line by line. beat is called for a heartbeat.
func readEvents(r *bufio.Reader, fn func(Event), beat func()) error {
	var floor uint64 // after a "missed" snapshot, events up to its seq are already in it
	for {
		line, err := r.ReadBytes('\n')
		if len(strings.TrimSpace(string(line))) > 0 {
			var m wireMsg
			if jerr := json.Unmarshal(line, &m); jerr != nil {
				return fmt.Errorf("parse: %w", jerr)
			}
			switch {
			case m.Error != nil:
				return errors.New(m.Error.Message)
			case m.Method == "" && m.Seq != nil:
				floor = *m.Seq
				fn(Event{Seq: *m.Seq, Type: "snapshot", Terminals: m.Terminals})
			case m.Method == "missed":
				var p wireParams
				if jerr := json.Unmarshal(m.Params, &p); jerr != nil {
					return fmt.Errorf("parse missed: %w", jerr)
				}
				floor = p.Seq
				fn(Event{Seq: p.Seq, Type: "snapshot", Terminals: p.Terminals})
			case m.Method == "event":
				var p wireParams
				if jerr := json.Unmarshal(m.Params, &p); jerr != nil {
					return fmt.Errorf("parse event: %w", jerr)
				}
				if p.Seq > floor {
					fn(Event{Seq: p.Seq, Type: p.Type, Terminal: p.Terminal, Data: p.Data})
				}
			case m.Method == "heartbeat":
				beat()
			}
		}
		if err != nil {
			return err
		}
	}
}

// Terminals is the core's live terminals by address, kept up to date with Apply.
type Terminals map[string]*Terminal

// Apply updates ts with one message of the stream (now: unix ms). changed: something
// shown may have changed; moved: terminals came, went or were relabeled (Herdr
// keeps a terminal's space and tab in its labels), so Herdr's own list may differ too.
func (ts Terminals) Apply(ev Event, now int64) (changed, moved bool, err error) {
	switch ev.Type {
	case "snapshot":
		for k := range ts {
			delete(ts, k)
		}
		for i := range ev.Terminals {
			t := ev.Terminals[i]
			if t.State == "" || t.State == "live" {
				ts[t.Address] = &t
			}
		}
		return true, true, nil
	case "created", "restarted":
		var t Terminal
		if err := json.Unmarshal(ev.Data, &t); err != nil {
			return false, false, fmt.Errorf("%s: %w", ev.Type, err)
		}
		if t.Address == "" {
			t.Address = ev.Terminal
		}
		ts[t.Address] = &t
		return true, true, nil
	case "removed", "finished":
		_, ok := ts[ev.Terminal]
		delete(ts, ev.Terminal)
		return ok, ok, nil
	case "status":
		var d struct {
			Status *TermStatus `json:"status"`
		}
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			return false, false, fmt.Errorf("status: %w", err)
		}
		t, ok := ts[ev.Terminal]
		if !ok {
			return false, false, nil
		}
		old, oldApp := t.StatusState(), t.Program()
		t.Status = d.Status
		if t.StatusState() != old {
			t.StatusMS = now // the event doesn't carry status_ms
		}
		// A new message alone isn't shown.
		return t.StatusState() != old || t.Program() != oldApp, false, nil
	case "title":
		var d struct {
			Title string `json:"title"`
		}
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			return false, false, fmt.Errorf("title: %w", err)
		}
		t, ok := ts[ev.Terminal]
		if !ok || t.Title == d.Title {
			return false, false, nil
		}
		t.Title = d.Title
		return true, false, nil
	case "labels":
		var d struct {
			Labels map[string]string `json:"labels"`
			Record *Terminal         `json:"record"` // when it newly matches the filter
		}
		if err := json.Unmarshal(ev.Data, &d); err != nil {
			return false, false, fmt.Errorf("labels: %w", err)
		}
		if d.Record != nil {
			ts[ev.Terminal] = d.Record
			return true, true, nil
		}
		t, ok := ts[ev.Terminal]
		if !ok {
			return false, false, nil
		}
		t.Labels = d.Labels
		return true, true, nil
	}
	// turn, size, foreground, keep, lost, doc: nothing we show.
	return false, false, nil
}

// Sorted returns copies of the terminals, oldest first (the order hc lists them).
func (ts Terminals) Sorted() []Terminal {
	list := make([]Terminal, 0, len(ts))
	for _, t := range ts {
		list = append(list, *t)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].CreatedMS != list[j].CreatedMS {
			return list[i].CreatedMS < list[j].CreatedMS
		}
		return list[i].Address < list[j].Address
	})
	return list
}

// limitWriter keeps the first n bytes and drops the rest.
type limitWriter struct {
	w io.Writer
	n int
}

func (l *limitWriter) Write(p []byte) (int, error) {
	if l.n > 0 {
		k := min(len(p), l.n)
		l.w.Write(p[:k])
		l.n -= k
	}
	return len(p), nil
}
