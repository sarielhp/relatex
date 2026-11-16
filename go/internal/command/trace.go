package command

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"runtime/pprof"
	"strings"
	"sync"
	"time"
)

var (
	traceStart = time.Now()
	traceOn    = os.Getenv("RELATEX_TRACE") != ""
	traceLock  sync.Mutex
	// memoryDirectory is $RELATEX_MEMPROFILE: where Trace records memory at every label.
	memoryDirectory = os.Getenv("RELATEX_MEMPROFILE")
	memoryCount     int
)

var unsafeLabel = regexp.MustCompile(`[^A-Za-z0-9]+`)

// Trace writes "+seconds label" to stderr when RELATEX_TRACE is set: where a run spends its time.
// When RELATEX_MEMPROFILE=DIR is set it also appends the process's peak and live memory to
// DIR/memory.log and writes a heap profile DIR/NNN-label.pprof at the label.
func Trace(label string) {
	if memoryDirectory != "" {
		traceMemory(label)
	}
	if !traceOn {
		return
	}
	traceLock.Lock()
	defer traceLock.Unlock()
	fmt.Fprintf(os.Stderr, "+%.1fs %s\n", time.Since(traceStart).Seconds(), label)
}

// traceMemory records one memory sample: VmHWM (peak resident set), VmRSS and the Go heap.
func traceMemory(label string) {
	traceLock.Lock()
	defer traceLock.Unlock()
	if err := os.MkdirAll(memoryDirectory, 0o755); err != nil {
		return
	}
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	memoryCount++
	line := fmt.Sprintf("%3d +%.1fs hwm=%dMB rss=%dMB heap=%dMB sys=%dMB %s\n", memoryCount,
		time.Since(traceStart).Seconds(), statusMB("VmHWM"), statusMB("VmRSS"), stats.HeapAlloc>>20, stats.Sys>>20, label)
	if log, err := os.OpenFile(filepath.Join(memoryDirectory, "memory.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
		log.WriteString(line)
		log.Close()
	}
	name := fmt.Sprintf("%03d-%s.pprof", memoryCount, strings.Trim(unsafeLabel.ReplaceAllString(label, "-"), "-"))
	if len(name) > 100 {
		name = name[:100] + ".pprof"
	}
	if file, err := os.Create(filepath.Join(memoryDirectory, name)); err == nil {
		runtime.GC()
		pprof.Lookup("heap").WriteTo(file, 0)
		file.Close()
	}
}

// statusMB is a /proc/self/status field in megabytes (0 where there is no /proc).
func statusMB(field string) int {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0
	}
	for _, line := range strings.Split(string(data), "\n") {
		if rest, ok := strings.CutPrefix(line, field+":"); ok {
			var kb int
			fmt.Sscanf(strings.TrimSpace(rest), "%d", &kb)
			return kb >> 10
		}
	}
	return 0
}
