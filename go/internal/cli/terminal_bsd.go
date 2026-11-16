//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package cli

import (
	"os"

	"golang.org/x/sys/unix"
)

// isTerminal is whether the file is a terminal (its terminal attributes can be read).
func isTerminal(file *os.File) bool {
	_, err := unix.IoctlGetTermios(int(file.Fd()), unix.TIOCGETA)
	return err == nil
}
