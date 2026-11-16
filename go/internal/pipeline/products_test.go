package pipeline

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sarielhp/relatex/internal/engine"
	"github.com/sarielhp/relatex/internal/ojson"
	"github.com/sarielhp/relatex/internal/texsandbox"
)

func stubTexToolchain(t *testing.T, dir string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		enginePath := filepath.Join(dir, "lualatex.bat")
		kpsePath := filepath.Join(dir, "kpsewhich.bat")
		write(t, enginePath, "@echo off\r\necho This is LuaHBTeX, Version 1.21.0\r\n")
		write(t, kpsePath, "@echo off\r\necho kpathsea version 6.4.0\r\n")
		return enginePath
	}
	enginePath := filepath.Join(dir, "lualatex")
	kpsePath := filepath.Join(dir, "kpsewhich")
	write(t, enginePath, "#!/bin/sh\necho 'This is LuaHBTeX, Version 1.21.0'\n")
	write(t, kpsePath, "#!/bin/sh\necho 'kpathsea version 6.4.0'\n")
	if err := os.Chmod(enginePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(kpsePath, 0o755); err != nil {
		t.Fatal(err)
	}
	return enginePath
}

func seedCachedEngine(t *testing.T, opts Options) {
	t.Helper()
	opts.Work = filepath.Join(t.TempDir(), "seed-work")
	seed := newConversion(opts)
	seed.inputStage()
	zip := filepath.Join(seed.archive.Directory, "source.zip")
	doctor := texsandbox.Current(seed.options.Engine)
	config := ojson.Object{
		{"engine", doctor.Report},
		{"jobs", seed.options.Jobs},
		{"identity", engine.SourceIdentity(seed.archive.Manifest)},
	}
	if seed.authorMath {
		config = append(config, ojson.Pair{"author_math", true})
	}
	if seed.options.ProductsOnly {
		config = append(config, ojson.Pair{"external_only", true})
	}
	if date := sourceDate(); len(date) > 0 {
		config = append(config, ojson.Pair{"source_date", date})
	}
	seed.pipeline.stage("engine", ojson.Object{{"zip", zip}}, config, true, func(dir string) any {
		write(t, filepath.Join(dir, "paper.pdf"), "%PDF-1.4\n")
		write(t, filepath.Join(dir, "probe/stub"), "ok")
		write(t, filepath.Join(dir, "engine-products.json"), `{"probe":"probe","pdf":"paper.pdf"}`)
		return ojson.Object{}
	})
}

func TestExecuteProductsSkipsLayoutEvenWhenLayoutOptionIsTrue(t *testing.T) {
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	inputDir := t.TempDir()
	write(t, filepath.Join(inputDir, "paper.tex"), "\\documentclass{article}\n\\begin{document}\nHi\n\\end{document}\n")
	enginePath := stubTexToolchain(t, t.TempDir())
	publish := false

	opts := Defaults()
	opts.Root = root
	opts.Input = inputDir
	opts.Output = filepath.Join(t.TempDir(), "out")
	opts.Work = filepath.Join(t.TempDir(), "work")
	opts.Cache = filepath.Join(t.TempDir(), "cache")
	opts.Engine = enginePath
	opts.ProductsOnly = true
	opts.Layout = true
	opts.Synchronize = false
	opts.Synctex = "none"
	opts.Publish = &publish

	seedCachedEngine(t, opts)
	c := newConversion(opts)
	c.executeProducts()
	if c.pipeline.receipt("layout") != nil {
		t.Fatal("executeProducts ran layoutStage despite ProductsOnly")
	}
	if len(c.degradations) != 0 {
		t.Fatalf("unexpected degradations in products-only run: %v", c.degradations)
	}
	if c.pipeline.receipt("export") == nil {
		t.Fatal("executeProducts did not run productsStage")
	}
}
