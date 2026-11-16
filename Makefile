# Build relatex from this repository: needs Go 1.26 and nothing else.
.PHONY: all stage build test install clean
all: build

# Regenerates go/internal/resources/data (committed in release commits, so this is a no-op there) from tex/, schemas/, ...
stage:
	go run -C go ./cmd/stageresources

build: stage
	go build -C go -trimpath -o ../build/relatex ./cmd/relatex
	go build -C go -trimpath -o ../build/try_arxiv ./cmd/try_arxiv

test: build
	go test -C go ./...

install: build
	install -d $(HOME)/bin
	install build/relatex $(HOME)/bin/relatex
	ln -sf relatex $(HOME)/bin/rlx
	install build/try_arxiv $(HOME)/bin/try_arxiv

clean:
	rm -rf build
