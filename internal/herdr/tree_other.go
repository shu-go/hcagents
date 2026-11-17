//go:build !windows

package herdr

import "os/exec"

// tree is a program. Without shims in between, killing it is enough
// (exec.CommandContext's default cancel).
type tree struct{ cmd *exec.Cmd }

func newTree(cmd *exec.Cmd) *tree { return &tree{cmd: cmd} }

func (t *tree) start() error { return t.cmd.Start() }

func (t *tree) close() {}
