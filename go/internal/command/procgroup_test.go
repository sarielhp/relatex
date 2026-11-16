package command

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// helperRole makes the test binary a helper program: "parent" starts a "child" and waits for it,
// "child" appends a byte to the file named by RELATEX_HELPER_BEAT every 20 ms (its heartbeat),
// "echo-args" prints each argument on a line and exits 0.
const helperRole = "RELATEX_HELPER_ROLE"

func TestMain(m *testing.M) {
	switch os.Getenv(helperRole) {
	case "parent":
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), helperRole+"=child")
		if child.Run() != nil {
			os.Exit(3)
		}
		os.Exit(0)
	case "child":
		beat()
	case "echo-args":
		fmt.Println(strings.Join(os.Args[1:], "\n"))
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func beat() {
	file, err := os.OpenFile(os.Getenv("RELATEX_HELPER_BEAT"), os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		os.Exit(4)
	}
	for {
		if _, err := file.Write([]byte{'.'}); err != nil {
			os.Exit(5)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func size(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// stopped waits for the heartbeat file to stop growing: false when it still grows after 5 s.
func stopped(path string) bool {
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		before := size(path)
		time.Sleep(300 * time.Millisecond)
		if size(path) == before {
			return true
		}
	}
	return false
}

// A timeout kills the program Run started together with the program it started in turn: a
// process group on Unix, a job object on Windows.
func TestRunTimeoutKillsTheWholeGroup(t *testing.T) {
	directory := t.TempDir()
	heartbeat := filepath.Join(directory, "beat")
	environment := map[string]string{helperRole: "parent", "RELATEX_HELPER_BEAT": heartbeat}
	result := Run([]string{os.Args[0]}, directory, filepath.Join(directory, "log"), 2*time.Second, environment)
	if result["timeout"] != true {
		t.Fatalf("result %v, want a timeout", result)
	}
	if size(heartbeat) == 0 {
		t.Fatal("the child never ran")
	}
	if !stopped(heartbeat) {
		t.Fatal("the child of the timed-out program survives")
	}
}

// A program Run started ends with its exit status, and nothing of its group is held after it.
func TestRunReportsTheExitStatus(t *testing.T) {
	directory := t.TempDir()
	environment := map[string]string{helperRole: "parent", "RELATEX_HELPER_BEAT": filepath.Join(directory, "no", "such", "beat")}
	result := Run([]string{os.Args[0]}, directory, filepath.Join(directory, "log"), time.Minute, environment)
	if result["timeout"] != false || result["exit"] != int64(3) || result["signal"] != nil {
		t.Fatalf("result %v, want exit 3", result)
	}
	if len(Running()) != 0 {
		t.Fatal("a finished program is still held")
	}
}
