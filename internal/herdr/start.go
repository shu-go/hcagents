package herdr

import (
	"context"
	"fmt"
	"os/exec"
	"time"
)

// Start starts Herdr's window when it isn't running (when `herdr-app ls` fails).
// It reports whether it started it; a running Herdr is left alone, so its window
// isn't pulled to the front.
func (c *Client) Start(ctx context.Context) (bool, error) {
	if _, err := c.Ls(ctx); err == nil {
		return false, nil
	}
	if err := c.launch(); err != nil {
		return false, err
	}
	return true, nil
}

// WaitReady waits until Herdr's window answers `herdr-app ls`, for up to timeout.
func (c *Client) WaitReady(ctx context.Context, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	t := time.NewTicker(300 * time.Millisecond)
	defer t.Stop()
	for {
		if _, err := c.Ls(ctx); err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("herdr-app didn't come up within %s", timeout)
		case <-t.C:
		}
	}
}

// launch runs `herdr-app` without arguments and doesn't wait for it: it starts
// Herdr, or (when it is running) a second instance hands over to it and exits.
func (c *Client) launch() error {
	allowForeground()
	cmd := exec.Command(c.app())
	guiLaunch(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // reap it
	return nil
}
