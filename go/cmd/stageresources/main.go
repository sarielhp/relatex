// Command stageresources copies the resources relatex embeds into go/internal/resources/data
// (see package resourcestager). Run it before building cmd/relatex:
//
//	go run -C go ./cmd/stageresources [REPOSITORY_ROOT]
//
// Without an argument it searches upward from the working directory for the repository root
// (the directory with VERSION and go/go.mod).
package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sarielhp/relatex/internal/resourcestager"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "stageresources:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) > 1 {
		return errors.New("usage: stageresources [REPOSITORY_ROOT]")
	}
	root, err := repositoryRoot(arguments)
	if err != nil {
		return err
	}
	count, err := resourcestager.Stage(root)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "stageresources: %d files\n", count)
	return nil
}

func repositoryRoot(arguments []string) (string, error) {
	if len(arguments) == 1 {
		return filepath.Abs(arguments[0])
	}
	directory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if isRoot(directory) {
			return directory, nil
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			return "", errors.New("no repository root (VERSION and go/go.mod) above the working directory")
		}
		directory = parent
	}
}

func isRoot(directory string) bool {
	for _, name := range []string{"VERSION", filepath.Join("go", "go.mod")} {
		if _, err := os.Stat(filepath.Join(directory, name)); err != nil {
			return false
		}
	}
	return true
}
