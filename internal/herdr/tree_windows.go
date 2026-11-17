//go:build windows

package herdr

import (
	"os/exec"
	"sync"
	"syscall"
	"unsafe"
)

var (
	kernel32                    = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW        = kernel32.NewProc("CreateJobObjectW")
	procSetInformationJobObject = kernel32.NewProc("SetInformationJobObject")
	procAssignProcessToJob      = kernel32.NewProc("AssignProcessToJobObject")
	procTerminateJobObject      = kernel32.NewProc("TerminateJobObject")
)

const (
	jobObjectExtendedLimitInformation = 9
	jobObjectLimitKillOnJobClose      = 0x2000
	processSetQuota                   = 0x0100
	processTerminate                  = 0x0001
)

type jobBasicLimit struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type jobExtendedLimit struct {
	Basic                 jobBasicLimit
	IoInfo                [6]uint64
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

// tree is a program and whatever it starts. A scoop shim (hc.exe) runs the real
// exe as a child: killing only the shim leaves that child holding our pipes, so
// the program goes in a job object and the whole job is killed.
type tree struct {
	cmd *exec.Cmd
	mu  sync.Mutex
	job syscall.Handle // 0 when no job could be made: kill falls back to the process
}

// newTree makes cmd's context cancel (and timeout) kill the whole tree.
func newTree(cmd *exec.Cmd) *tree {
	t := &tree{cmd: cmd}
	cmd.Cancel = t.kill
	return t
}

// start starts the program in a job that dies with it, and with us (kill-on-close).
// The shim takes a while to start its child, so in practice it joins the job first.
func (t *tree) start() error {
	if err := t.cmd.Start(); err != nil {
		return err
	}
	job := newJob(t.cmd.Process.Pid)
	t.mu.Lock()
	t.job = job
	t.mu.Unlock()
	return nil
}

func newJob(pid int) syscall.Handle {
	h, _, _ := procCreateJobObjectW.Call(0, 0)
	if h == 0 {
		return 0
	}
	job := syscall.Handle(h)
	info := jobExtendedLimit{Basic: jobBasicLimit{LimitFlags: jobObjectLimitKillOnJobClose}}
	r, _, _ := procSetInformationJobObject.Call(uintptr(job), jobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
	if r == 0 {
		syscall.CloseHandle(job)
		return 0
	}
	p, err := syscall.OpenProcess(processSetQuota|processTerminate, false, uint32(pid))
	if err != nil {
		syscall.CloseHandle(job)
		return 0
	}
	defer syscall.CloseHandle(p)
	if r, _, _ := procAssignProcessToJob.Call(uintptr(job), uintptr(p)); r == 0 {
		syscall.CloseHandle(job)
		return 0
	}
	return job
}

// kill ends the program and everything it started.
func (t *tree) kill() error {
	t.mu.Lock()
	if t.job != 0 {
		procTerminateJobObject.Call(uintptr(t.job), 1)
	}
	t.mu.Unlock()
	return t.cmd.Process.Kill()
}

// close releases the job, killing what is left in it. Call after Wait.
func (t *tree) close() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job != 0 {
		syscall.CloseHandle(t.job)
		t.job = 0
	}
}
