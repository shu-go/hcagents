package herdr

import (
	"context"
	"os/exec"
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

// launch runs `herdr-app` without arguments and doesn't wait for it.
func (c *Client) launch() error {
	cmd := exec.Command(c.app())
	guiLaunch(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() // reap it
	return nil
}
