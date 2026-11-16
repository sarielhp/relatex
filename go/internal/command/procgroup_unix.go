//go:build !windows

package command

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Group is a started program with every program it starts: on Unix the process group it leads.
type Group struct {
	pid int
}

// StartGroup starts cmd as the leader of a process group of its own, keeping the rest of its
// SysProcAttr (a browser's parent-death signal).
func StartGroup(cmd *exec.Cmd) (*Group, error) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &Group{pid: cmd.Process.Pid}, nil
}

// Kill sends SIGKILL to the whole process group.
func (g *Group) Kill() {
	_ = syscall.Kill(-g.pid, syscall.SIGKILL) // a group already gone has nothing left to kill
}

// Close lets go of the group; on Unix there is nothing to release.
func (g *Group) Close() {}

// DetachedAttributes start a program that outlives relatex's own signals: a process group of its
// own (an editor relatex sync opens).
func DetachedAttributes() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true}
}

// killDescendants kills each descendant of this process by its id (a sandbox's own processes run
// in sessions of their own, out of the group's reach, and outlive it for a moment), and waits up
// to three seconds for them all to be gone. tree is read before the groups die: a process whose
// parent dies passes to another, out of this tree.
func killDescendants(tree map[int]string) {
	for pid := range tree {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); {
		if !anyAlive(tree) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// process is one /proc entry: its parent and its start time (so a reused id is not taken for it).
type process struct {
	parent int
	start  string
}

// descendants are the running processes below root with their start times, read from /proc.
func descendants(root int) map[int]string {
	processes := map[int]process{}
	paths, _ := filepath.Glob("/proc/[0-9]*/stat")
	for _, path := range paths {
		pid, err := strconv.Atoi(filepath.Base(filepath.Dir(path)))
		state, entry, ok := processStat(path)
		if err == nil && ok && state != "Z" {
			processes[pid] = entry
		}
	}
	found := map[int]string{}
	frontier := []int{root}
	for len(frontier) > 0 {
		parent := frontier[0]
		frontier = frontier[1:]
		for pid, entry := range processes {
			if _, seen := found[pid]; !seen && entry.parent == parent {
				found[pid] = entry.start
				frontier = append(frontier, pid)
			}
		}
	}
	return found
}

// processStat is a /proc/PID/stat's state, parent and start time; ok false when it is gone.
func processStat(path string) (state string, entry process, ok bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", process{}, false
	}
	text := string(data)
	fields := strings.Fields(text[strings.LastIndex(text, ")")+1:])
	if len(fields) < 20 {
		return "", process{}, false
	}
	entry.parent, _ = strconv.Atoi(fields[1])
	entry.start = fields[19]
	return fields[0], entry, true
}

// anyAlive is whether one of the processes still runs (a zombie has ended).
func anyAlive(tree map[int]string) bool {
	for pid, start := range tree {
		state, entry, ok := processStat(fmt.Sprintf("/proc/%d/stat", pid))
		if ok && entry.start == start && state != "Z" {
			return true
		}
	}
	return false
}
