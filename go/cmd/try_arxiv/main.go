// Command try_arxiv downloads arXiv papers' LaTeX sources and runs relatex on each, printing one
// line per paper:
//
//	try_arxiv [--id ARXIVID]... [--random N] [--dest DIR] [--relatex PATH] [--delay SECONDS]
//
// It is polite to arXiv: one request at a time, at least three seconds apart, with a descriptive
// User-Agent. --random picks from arXiv's listing of recent cs.* papers (one extra request).
package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/relatex/internal/tryarxiv"
)

type idList []string

func (l *idList) String() string     { return strings.Join(*l, ",") }
func (l *idList) Set(v string) error { *l = append(*l, v); return nil }

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "try_arxiv:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	var ids idList
	flags := flag.NewFlagSet("try_arxiv", flag.ContinueOnError)
	flags.Var(&ids, "id", "arXiv identifier to try (repeatable), e.g. 2401.00001")
	random := flags.Int("random", 0, "also try `N` random recent cs.* papers")
	dest := flags.String("dest", "arxiv-papers", "directory the papers are unpacked into (one subdirectory each)")
	relatex := flags.String("relatex", "", "the relatex executable (default: beside try_arxiv, else on PATH)")
	delay := flags.Int("delay", 3, "seconds between requests to arXiv (at least 3)")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	program, err := findRelatex(*relatex)
	if err != nil {
		return err
	}
	client := tryarxiv.NewClient(time.Duration(*delay) * time.Second)
	options := tryarxiv.Options{IDs: ids, Random: *random, Category: "cs", Dest: *dest, Relatex: program}
	pick := func(all []string, n int) []string {
		return tryarxiv.Pick(all, n, rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())))
	}
	return tryarxiv.Run(context.Background(), client, options, os.Stdout, pick)
}

func findRelatex(explicit string) (string, error) {
	if explicit != "" {
		return filepath.Abs(explicit)
	}
	if self, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(self), "relatex")
		if _, err := os.Stat(sibling); err == nil {
			return sibling, nil
		}
	}
	found, err := exec.LookPath("relatex")
	if err != nil {
		return "", fmt.Errorf("relatex not found beside try_arxiv or on PATH (build it with make build, or pass --relatex)")
	}
	return found, nil
}
