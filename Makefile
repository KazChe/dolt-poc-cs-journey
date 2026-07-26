# cs — build helpers. Run `make help` for the list.

# Where `make install` drops the binary. Override with `make install PREFIX=~/.local`.
PREFIX ?= $(shell go env GOPATH)
BINDIR := $(PREFIX)/bin
BIN    := cs

.PHONY: build install test vet clean help

## build: compile the cs binary into ./cs
build:
	go build -o $(BIN) .

## install: build and copy cs onto your PATH ($(BINDIR)/cs)
install:
	go build -o $(BINDIR)/$(BIN) .
	@echo "installed $(BIN) -> $(BINDIR)/$(BIN)"

## test: run the full test suite
test:
	go test ./...

## vet: run go vet across the module
vet:
	go vet ./...

## clean: remove the locally built binary
clean:
	rm -f $(BIN)

## help: list the available targets
help:
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## //'
