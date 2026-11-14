//go:build !windows

package herdr

// RaiseWindow does nothing here yet: only Windows is tried so far.
func (c *Client) RaiseWindow() error { return nil }
