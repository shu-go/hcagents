// Package herdr wraps the herdr-app and hc command line tools.
package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Client runs herdr-app / hc. Empty paths mean "look up in PATH".
type Client struct {
	AppPath  string // herdr-app
	CorePath string // hc
	Timeout  time.Duration
}

func (c *Client) app() string {
	if c.AppPath != "" {
		return c.AppPath
	}
	return "herdr-app"
}

func (c *Client) core() string {
	if c.CorePath != "" {
		return c.CorePath
	}
	return "hc"
}

// waitDelay is how long Wait waits for the pipes after the program ended (or was
// killed): a child that inherited them can't hold the caller forever.
const waitDelay = 2 * time.Second

func (c *Client) run(ctx context.Context, name string, args ...string) ([]byte, error) {
	timeout := c.Timeout
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	hideWindow(cmd)
	cmd.WaitDelay = waitDelay
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	t := newTree(cmd) // a timeout kills a shim's child too
	err := t.start()
	if err == nil {
		err = cmd.Wait()
	}
	t.close()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return nil, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, msg)
	}
	return stdout.Bytes(), nil
}

// Snapshot is the output of `herdr-app ls`.
type Snapshot struct {
	Spaces []Space `json:"spaces"`
}

type Space struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Folder  string  `json:"folder"`
	Machine *string `json:"machine"`
	Active  bool    `json:"active"`
	Tabs    []Tab   `json:"tabs"`
}

type Tab struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Active    bool          `json:"active"`
	Settled   bool          `json:"settled"`
	Terminals []TabTerminal `json:"terminals"`
}

type TabTerminal struct {
	Terminal  string   `json:"terminal"`
	Pane      string   `json:"pane"`
	Status    *string  `json:"status"`
	App       *string  `json:"app"`
	Name      *string  `json:"name"`
	SubAgents []string `json:"sub_agents"` // addresses
}

// Status is the output of `herdr-app status` (only the fields we use).
type Status struct {
	Space       string `json:"space"`
	Tab         string `json:"tab"`
	FocusedPane string `json:"focused_pane"`
	Layout      string `json:"layout"`
	Agent       string `json:"agent"`
}

// Agent is one agent terminal, flattened with its space and tab.
type Agent struct {
	Terminal  string
	Pane      string
	Status    string // working, blocked, done, idle
	App       string
	Name      string
	SpaceID   string
	SpaceName string
	Machine   string // "" for this computer
	TabID     string
	TabName   string
	SubAgents []string
}

func (c *Client) Ls(ctx context.Context) (*Snapshot, error) {
	out, err := c.run(ctx, c.app(), "ls")
	if err != nil {
		return nil, err
	}
	var s Snapshot
	if err := json.Unmarshal(out, &s); err != nil {
		return nil, fmt.Errorf("herdr-app ls: %w", err)
	}
	return &s, nil
}

func (c *Client) Status(ctx context.Context) (*Status, error) {
	out, err := c.run(ctx, c.app(), "status")
	if err != nil {
		return nil, err
	}
	var s Status
	if err := json.Unmarshal(out, &s); err != nil {
		return nil, fmt.Errorf("herdr-app status: %w", err)
	}
	return &s, nil
}

// Do runs an app command: herdr-app do <command> <json args>.
func (c *Client) Do(ctx context.Context, command string, args map[string]any) error {
	a := []string{"do", command}
	if len(args) > 0 {
		b, err := json.Marshal(args)
		if err != nil {
			return err
		}
		a = append(a, string(b))
	}
	_, err := c.run(ctx, c.app(), a...)
	return err
}

// Agents returns the terminals that report an agent status; other terminals are skipped.
func (s *Snapshot) Agents() []Agent {
	var list []Agent
	for _, sp := range s.Spaces {
		machine := ""
		if sp.Machine != nil {
			machine = *sp.Machine
		}
		for _, tb := range sp.Tabs {
			for _, t := range tb.Terminals {
				if t.Status == nil || *t.Status == "" {
					continue
				}
				a := Agent{
					Terminal:  t.Terminal,
					Pane:      t.Pane,
					Status:    *t.Status,
					SpaceID:   sp.ID,
					SpaceName: sp.Name,
					Machine:   machine,
					TabID:     tb.ID,
					TabName:   tb.Name,
					SubAgents: t.SubAgents,
				}
				if t.App != nil {
					a.App = *t.App
				}
				if t.Name != nil {
					a.Name = *t.Name
				}
				list = append(list, a)
			}
		}
	}
	return list
}
