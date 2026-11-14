//go:build !windows

package herdr

import "os/exec"

func hideWindow(cmd *exec.Cmd) {}

func guiLaunch(cmd *exec.Cmd) {}
