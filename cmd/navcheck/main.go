// navcheck checks which herdr-app command moves the window to an agent's space and tab.
//
//	go run ./cmd/navcheck            # list agents and try every method on each
//	go run ./cmd/navcheck -t t28     # only this terminal
//	go run ./cmd/navcheck -list      # only list agents
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/shu-go/hcagents/internal/herdr"
)

type method struct {
	name string
	run  func(ctx context.Context, c *herdr.Client, a herdr.Agent) error
}

var methods = []method{
	{"agent.focus{terminal}", func(ctx context.Context, c *herdr.Client, a herdr.Agent) error {
		return c.Do(ctx, "agent.focus", map[string]any{"terminal": a.Terminal})
	}},
	{"pane.focus{terminal}", func(ctx context.Context, c *herdr.Client, a herdr.Agent) error {
		return c.Do(ctx, "pane.focus", map[string]any{"terminal": a.Terminal})
	}},
	{"tab.select{id}", func(ctx context.Context, c *herdr.Client, a herdr.Agent) error {
		return c.Do(ctx, "tab.select", map[string]any{"id": a.TabID})
	}},
	{"space.select{id}+tab.select{id}", func(ctx context.Context, c *herdr.Client, a herdr.Agent) error {
		if err := c.Do(ctx, "space.select", map[string]any{"id": a.SpaceID}); err != nil {
			return err
		}
		return c.Do(ctx, "tab.select", map[string]any{"id": a.TabID})
	}},
}

func main() {
	appPath := flag.String("app", "", "path to herdr-app (default: PATH)")
	target := flag.String("t", "", "only this terminal (e.g. t28)")
	listOnly := flag.Bool("list", false, "only list agents")
	flag.Parse()

	ctx := context.Background()
	c := &herdr.Client{AppPath: *appPath}

	snap, err := c.Ls(ctx)
	if err != nil {
		fail(err)
	}
	agents := snap.Agents()
	fmt.Println("agents:")
	for _, a := range agents {
		fmt.Printf("  %-5s %-8s %-12s space=%q machine=%q tab=%q\n", a.Terminal, a.Status, a.App, a.SpaceName, a.Machine, a.TabName)
	}
	if *listOnly {
		return
	}

	orig, err := c.Status(ctx)
	if err != nil {
		fail(err)
	}
	defer restore(ctx, c, orig)

	for _, a := range agents {
		if *target != "" && a.Terminal != *target {
			continue
		}
		away, ok := otherTab(snap, a)
		if !ok {
			fmt.Printf("\n%s: no other tab to move away to, skipped\n", a.Terminal)
			continue
		}
		fmt.Printf("\n%s (%s / %s):\n", a.Terminal, a.SpaceName, a.TabName)
		for _, m := range methods {
			// Move away first, so the method has to switch space and/or tab.
			if err := moveTo(ctx, c, away.spaceID, away.tabID); err != nil {
				fmt.Printf("  %-34s move away failed: %v\n", m.name, err)
				continue
			}
			start := time.Now()
			if err := m.run(ctx, c, a); err != nil {
				fmt.Printf("  %-34s ERROR %v\n", m.name, err)
				continue
			}
			st, ok := waitFor(ctx, c, a, 2*time.Second)
			result := "NG"
			if ok {
				result = "OK"
			}
			fmt.Printf("  %-34s %s (%v) space=%s tab=%s agent=%s\n",
				m.name, result, time.Since(start).Round(time.Millisecond), short(st.Space == a.SpaceID), short(st.Tab == a.TabID), st.Agent)
		}
	}
}

type place struct{ spaceID, tabID string }

// otherTab picks a tab different from the agent's: another space's if there is one.
func otherTab(s *herdr.Snapshot, a herdr.Agent) (place, bool) {
	var sameSpace *place
	for _, sp := range s.Spaces {
		for _, tb := range sp.Tabs {
			if tb.ID == a.TabID {
				continue
			}
			if sp.ID != a.SpaceID {
				return place{sp.ID, tb.ID}, true
			}
			if sameSpace == nil {
				sameSpace = &place{sp.ID, tb.ID}
			}
		}
	}
	if sameSpace != nil {
		return *sameSpace, true
	}
	return place{}, false
}

func moveTo(ctx context.Context, c *herdr.Client, spaceID, tabID string) error {
	if err := c.Do(ctx, "space.select", map[string]any{"id": spaceID}); err != nil {
		return err
	}
	if err := c.Do(ctx, "tab.select", map[string]any{"id": tabID}); err != nil {
		return err
	}
	_, err := waitStatus(ctx, c, 2*time.Second, func(st *herdr.Status) bool {
		return st.Space == spaceID && st.Tab == tabID
	})
	return err
}

func waitFor(ctx context.Context, c *herdr.Client, a herdr.Agent, d time.Duration) (*herdr.Status, bool) {
	st, err := waitStatus(ctx, c, d, func(st *herdr.Status) bool {
		return st.Space == a.SpaceID && st.Tab == a.TabID
	})
	return st, err == nil
}

func waitStatus(ctx context.Context, c *herdr.Client, d time.Duration, ok func(*herdr.Status) bool) (*herdr.Status, error) {
	deadline := time.Now().Add(d)
	var last *herdr.Status
	for {
		st, err := c.Status(ctx)
		if err == nil {
			last = st
			if ok(st) {
				return st, nil
			}
		}
		if time.Now().After(deadline) {
			if last == nil {
				last = &herdr.Status{}
			}
			return last, fmt.Errorf("timeout")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func restore(ctx context.Context, c *herdr.Client, orig *herdr.Status) {
	if err := moveTo(ctx, c, orig.Space, orig.Tab); err != nil {
		fmt.Fprintln(os.Stderr, "restore:", err)
		return
	}
	if orig.FocusedPane != "" {
		_ = c.Do(ctx, "pane.focus", map[string]any{"pane": orig.FocusedPane})
	}
	fmt.Println("\nrestored original space/tab")
}

func short(b bool) string {
	if b {
		return "match"
	}
	return "-"
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
