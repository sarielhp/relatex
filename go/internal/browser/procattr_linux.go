package browser

import "syscall"

// browserProcAttr puts a local Chromium in its own process group and has the kernel send it SIGTERM
// when relatex dies, however it dies: a killed conversion (a timeout, a stopped run) left its browser
// running, adopted by the session manager, for days (15 leaked browsers, 124 processes, 2026-10-29).
func browserProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGTERM}
}
