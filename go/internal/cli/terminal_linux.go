//go:build linux

package cli

import (
	"os"
	"syscall"
	"unsafe"
)

// isTerminal is whether the file is a terminal (its terminal attributes can be read).
func isTerminal(file *os.File) bool {
	var termios syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), syscall.TCGETS, uintptr(unsafe.Pointer(&termios)))
	return errno == 0
}
