//go:build !linux && !windows

package browser

import "syscall"

// browserProcAttr puts a local Chromium in its own process group (no parent-death signal off Linux).
func browserProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}
