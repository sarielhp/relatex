package cli

import (
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/pipeline"
)

// stopState is what relatex synctex removes when a signal stops it: its attempt directory and the
// files it was writing (the SyncTeX file, and with --replace-pdf the PDF).
type stopState struct {
	lock      sync.Mutex
	install   sync.Mutex // held while files are renamed into place: a signal waits for them
	directory string
	partial   []string
	output    string // the conversion's output directory, removed when empty
}

var stopping = &stopState{}

func (s *stopState) attempt(directory string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.directory = directory
}

func (s *stopState) outputDirectory(path string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.output = path
}

// temporary names the files being written (none, or only empty names, once they are in place).
func (s *stopState) temporary(paths ...string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.partial = s.partial[:0]
	for _, path := range paths {
		if path != "" {
			s.partial = append(s.partial, path)
		}
	}
}

// uninterrupted runs body (a check and renames) so that a signal stops the run only after it.
func (s *stopState) uninterrupted(body func() error) error {
	s.install.Lock()
	defer s.install.Unlock()
	return body()
}

// clean kills every program relatex started, then removes the attempt and the partial file.
func (s *stopState) clean() {
	command.KillTree()
	// The stage that ran the killed programs may still write a receipt before it fails.
	time.Sleep(100 * time.Millisecond)
	s.install.Lock()
	s.lock.Lock()
	defer s.lock.Unlock()
	for _, path := range s.partial {
		_ = os.Remove(path)
	}
	if s.directory != "" {
		_ = os.RemoveAll(s.directory)
		_ = os.Remove(pipeline.AttemptLockPath(s.directory))
	}
	if s.output != "" {
		_ = os.Remove(s.output)
	}
}

// stopOnSignal makes SIGTERM, SIGINT and SIGHUP stop the run cleanly: every program it started
// killed with its process group (LuaLaTeX in its sandbox, the bibliography processor), its
// attempt and partial files removed, and the exit status 128 plus the signal's number.
func stopOnSignal() (stop func()) {
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM, syscall.SIGINT, syscall.SIGHUP)
	done := make(chan struct{})
	var signalled atomic.Bool
	go func() {
		select {
		case received := <-signals:
			signalled.Store(true)
			stopping.clean()
			number := 1
			if value, ok := received.(syscall.Signal); ok {
				number = int(value)
			}
			os.Exit(128 + number)
		case <-done:
		}
	}()
	return func() {
		// The run a signal stopped fails as its programs die: the signal's own exit comes instead.
		if signalled.Load() {
			select {}
		}
		signal.Stop(signals)
		close(done)
	}
}
