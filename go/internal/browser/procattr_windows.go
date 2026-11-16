//go:build windows

package browser

import "syscall"

// browserProcAttr starts a local browser with no extra attributes: command.StartGroup puts it in a
// job object that dies with relatex (the Windows form of Linux's parent-death signal).
func browserProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{}
}
