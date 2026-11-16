//go:build !windows

package cli

import (
	"os"
	"strconv"
	"syscall"
)

// lowerPriority runs this process at nice 19, as a background build should: Linux keeps the
// priority per thread, so every thread is lowered, and the threads and programs started later
// inherit it.
func lowerPriority() {
	for range 2 {
		tasks, err := os.ReadDir("/proc/self/task")
		if err != nil {
			_ = syscall.Setpriority(syscall.PRIO_PROCESS, 0, 19)
			return
		}
		for _, task := range tasks {
			if id, convErr := strconv.Atoi(task.Name()); convErr == nil {
				_ = syscall.Setpriority(syscall.PRIO_PROCESS, id, 19)
			}
		}
	}
}
