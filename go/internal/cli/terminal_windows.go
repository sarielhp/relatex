//go:build windows

package cli

import (
	"os"

	"golang.org/x/sys/windows"
)

// isTerminal is whether the file is a console that shows colours: its console mode can be read,
// and it accepts escape sequences (ENABLE_VIRTUAL_TERMINAL_PROCESSING, which this turns on).
func isTerminal(file *os.File) bool {
	handle := windows.Handle(file.Fd())
	var mode uint32
	if windows.GetConsoleMode(handle, &mode) != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
