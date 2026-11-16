package cli

import (
	"crypto/rand"
	"fmt"
	"io/fs"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sarielhp/clihelp"

	"github.com/sarielhp/relatex/internal/bundle"
	"github.com/sarielhp/relatex/internal/command"
	"github.com/sarielhp/relatex/internal/exportwriter"
	"github.com/sarielhp/relatex/internal/fspath"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/rec"
)

// exportSpec is ProductCLI#export (the export and reconstruct commands): one format of a bundle
// into a new export directory under --output.
func exportSpec(name string) commandSpec {
	format, description := "html", "Write one format of a saved document bundle into a new directory under --output"
	if name == "reconstruct" {
		format, description = "pdf", "Rebuild the PDF of a saved document bundle into a new directory under --output"
	}
	var request exportRequest
	options := []clihelp.Option{
		clihelp.Required(textFlag("-o, --output <dir>", "The directory the export directory is made in", &request.output)),
		textChoice("--format <format>", "The format to write (default "+format+")", exportwriter.Formats, &request.format),
		textChoice("--allow-fallback <mode>", "image: show a formula that cannot be expressed as a picture instead of failing",
			[]string{"image"}, &request.allowFallback),
		switchFlag("--compile-typst", "Compile the Typst export with typst; the export fails if that fails",
			func() { request.compileTypst = true }),
	}
	request.format = format
	return commandSpec{name: name, description: description,
		usage: Name + " " + name + " DOCUMENT.json[.gz] --output DIR [--format FORMAT]", options: options, args: clihelp.ExactArgs(1),
		run:   func(rest []string) ojson.Object { return export(rest[0], request) },
		human: func(result ojson.Object) string { return exportHuman(request.format, result) }}
}

// exportRequest is what an export command line asked for.
type exportRequest struct {
	format, output, allowFallback string
	compileTypst                  bool
}

// export writes one format of the bundle at path into a new export directory under the output.
func export(path string, request exportRequest) ojson.Object {
	format, output, allowFallback, compileTypst := request.format, request.output, request.allowFallback, request.compileTypst
	payload := readBundle(path)
	bundle.Validate(payload)
	workspace := workspaceRoot(output)
	destination := filepath.Join(workspace, "export-"+time.Now().UTC().Format("20060102-150405")+"-"+randomHex(4))
	attempt := makeTemporary(workspace, ".relatex-export-")
	warn(os.Stderr, "Export attempt: "+attempt)
	root := filepath.Join(attempt, "bundle")
	writer := &exportwriter.Writer{Part: func(key string) any { return payload[key] }, AllowFallback: allowFallback,
		CompileTypst: compileTypst}
	previous := command.Workspace
	command.Workspace = attempt
	result := writer.Write(format, root)
	command.Workspace = previous
	if err := ojson.WriteFile(filepath.Join(root, "manifest.json"), fileHashes(root)); err != nil {
		panic(systemCall(err))
	}
	if _, err := os.Lstat(destination); err == nil {
		rec.Fail("Export output appeared before publication")
	}
	if err := os.Rename(root, destination); err != nil {
		panic(systemCall(err))
	}
	return append(result, ojson.Pair{"directory", destination})
}

// readBundle is DocumentBundle.read with Ruby's failure for a missing file.
func readBundle(path string) rec.Map {
	if _, err := os.Stat(path); err != nil {
		panic(systemFailureAt(err, "rb_file_s_size"))
	}
	return bundle.Read(path)
}

// fileHashes is every file under root (Dir.glob('**/*'): no dot files) and its SHA-256, in
// the glob's order: depth first, each directory's entries sorted.
func fileHashes(root string) ojson.Object {
	paths := []string{}
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if path != root && strings.HasPrefix(entry.Name(), ".") {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info, statErr := os.Stat(path); statErr == nil && info.Mode().IsRegular() {
			paths = append(paths, path)
		}
		return nil
	})
	hashes := ojson.Object{}
	for _, path := range paths {
		hashes = append(hashes, ojson.Pair{fspath.Name(root, path), command.FileDigest(path)})
	}
	return hashes
}

// randomHex is SecureRandom.hex(count).
func randomHex(count int) string {
	bytes := make([]byte, count)
	if _, err := rand.Read(bytes); err != nil {
		rec.Fail("%v", err)
	}
	return fmt.Sprintf("%x", bytes)
}

// makeTemporary is Dir.mktmpdir(prefix, parent): prefix, the date, the process and a random
// base-36 suffix.
func makeTemporary(parent, prefix string) string {
	suffix, err := rand.Int(rand.Reader, big.NewInt(1<<32))
	if err != nil {
		rec.Fail("%v", err)
	}
	path := filepath.Join(parent, fmt.Sprintf("%s%s-%d-%s", prefix, time.Now().Format("20060102"), os.Getpid(), suffix.Text(36)))
	if err := os.Mkdir(path, 0o700); err != nil {
		panic(systemFailureAt(err, "rb_dir_s_mkdir"))
	}
	return path
}

// validateSpec is ProductCLI#validate: the bundle and its cross-layer bindings.
func validateSpec() commandSpec {
	return commandSpec{name: "validate", description: "Check a saved document bundle against its schema and report what it holds",
		usage: Name + " validate DOCUMENT.json[.gz]", args: clihelp.ExactArgs(1), run: validate, human: validateHuman}
}

func validate(rest []string) ojson.Object {
	report := bundle.Validate(readBundle(rest[0]))
	return exportwriter.Ordered(report, "status", "scope", "qualified", "nodes", "images", "fonts")
}
