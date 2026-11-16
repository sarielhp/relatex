//go:build !windows

package command

import (
	"errors"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestKillRunningStopsEveryProgramGroup(t *testing.T) {
	defer func() { stopping = false }()
	directory := t.TempDir()
	done := make(chan int64, 1)
	go func() {
		// The shell starts a child of its own: the group's kill must reach it too.
		stage := Run([]string{"/bin/sh", "-c", "sleep 60 & wait"}, directory, filepath.Join(directory, "log"), time.Minute, nil)
		number, _ := stage["signal"].(int64)
		done <- number
	}()
	deadline := time.Now().Add(5 * time.Second)
	for len(Running()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	groups := Running()
	if len(groups) != 1 {
		t.Fatalf("running groups %v", groups)
	}
	if KillRunning() != 1 {
		t.Fatal("one group to kill")
	}
	select {
	case number := <-done:
		if number != int64(syscall.SIGKILL) {
			t.Fatalf("the program ended by signal %d", number)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the killed program did not end")
	}
	// The shell's child, killed with it, is gone once its new parent has reaped it.
	err := syscall.Kill(-groups[0], 0)
	for deadline := time.Now().Add(5 * time.Second); err == nil && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
		err = syscall.Kill(-groups[0], 0)
	}
	if !errors.Is(err, syscall.ESRCH) {
		t.Fatalf("a process of the killed group survives: %v", err)
	}
	if len(Running()) != 0 {
		t.Fatal("a finished program is no longer held")
	}
}
