//go:build windows

package cli

import "golang.org/x/sys/windows"

// lowerPriority runs this process in the idle priority class, Windows' nice 19 (decision 129): the
// class covers every thread, and programs started later inherit it.
func lowerPriority() {
	// A failure leaves the normal priority: the build still runs, only less politely.
	_ = windows.SetPriorityClass(windows.CurrentProcess(), windows.IDLE_PRIORITY_CLASS)
}
