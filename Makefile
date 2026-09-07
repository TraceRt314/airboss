VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
PREFIX  ?= $(HOME)/.local

build:
	go build -ldflags "-X main.version=$(VERSION)" -o torre-tui .

install: build
	./install.sh

test: build
	go vet ./...
	bash -n scripts/*
	./torre-tui -version

screenshot: build
	python3 docs/screenshot.py docs/screenshot.svg

.PHONY: build install test screenshot
