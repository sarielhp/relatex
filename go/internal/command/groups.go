package command

import (
	"os"
	"sync"
)

// Every program Run starts leads a group of its own (StartGroup: a process group on Unix, a job
// object on Windows), so a timeout kills it with all it started. A signal sent to relatex's own
// group does not reach those groups: running reaches them, so a command stopped by SIGTERM or
// SIGINT (decision 101) kills them before it exits.
var (
	runningLock sync.Mutex
	running     = map[int]*Group{}
	stopping    bool
)

// hold records a started program's group until release; a program started while relatex is
// stopping is killed at once.
func hold(pid int, group *Group) (release func()) {
	runningLock.Lock()
	defer runningLock.Unlock()
	if stopping {
		group.Kill()
	}
	running[pid] = group
	return func() {
		runningLock.Lock()
		defer runningLock.Unlock()
		delete(running, pid)
	}
}

// KillRunning kills the group of every program Run is waiting for, and every one it starts from
// now on: relatex is stopping. It returns how many groups it killed.
func KillRunning() int {
	runningLock.Lock()
	defer runningLock.Unlock()
	stopping = true
	for _, group := range running {
		group.Kill()
	}
	return len(running)
}

// Running is the ids of the programs Run is waiting for (on Unix each leads its process group).
func Running() []int {
	runningLock.Lock()
	defer runningLock.Unlock()
	groups := make([]int, 0, len(running))
	for pid := range running {
		groups = append(groups, pid)
	}
	return groups
}

// KillTree kills every program relatex started, at any depth: the groups Run holds, then (on
// Unix) each descendant of this process by its id, waiting up to three seconds for them all to be
// gone.
func KillTree() {
	// Read first: a process whose parent dies passes to another, out of this tree.
	tree := descendants(os.Getpid())
	KillRunning()
	killDescendants(tree)
}
