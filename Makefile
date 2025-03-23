# file: /Makefile
# description: Makefile for building and managing O²UL blockchain project
# module: Build
# License: MIT
# Author: Andrew Donelson
# Copyright 2025 Andrew Donelson
# Portions Copyright 2014-2024 The go-ethereum Authors

# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.

.PHONY: o2ul all test lint fmt clean devtools help

GOBIN = ./build/bin
GO ?= latest
GORUN = go run

#? o2ul: Build o2ul.
o2ul:
	$(GORUN) build/ci.go install ./cmd/geth
	@mv $(GOBIN)/geth $(GOBIN)/o2ul
	@echo "Done building."
	@echo "Run \"$(GOBIN)/o2ul\" to launch o2ul."

#? all: Build all packages and executables.
all:
	$(GORUN) build/ci.go install
	@if [ -f $(GOBIN)/geth ]; then mv $(GOBIN)/geth $(GOBIN)/o2ul; fi

#? test: Run the tests.
test: all
	$(GORUN) build/ci.go test

#? lint: Run certain pre-selected linters.
lint: ## Run linters.
	$(GORUN) build/ci.go lint

#? fmt: Ensure consistent code formatting.
fmt:
	gofmt -s -w $(shell find . -name "*.go")

#? clean: Clean go cache, built executables, and the auto generated folder.
clean:
	go clean -cache
	rm -fr build/_workspace/pkg/ $(GOBIN)/*

# The devtools target installs tools required for 'go generate'.
# You need to put $GOBIN (or $GOPATH/bin) in your PATH to use 'go generate'.

#? devtools: Install recommended developer tools.
devtools:
	env GOBIN= go install golang.org/x/tools/cmd/stringer@latest
	env GOBIN= go install github.com/fjl/gencodec@latest
	env GOBIN= go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	env GOBIN= go install ./cmd/abigen
	@type "solc" 2> /dev/null || echo 'Please install solc'
	@type "protoc" 2> /dev/null || echo 'Please install protoc'

#? help: Get more info on make commands.
help: Makefile
	@echo ''
	@echo 'Usage:'
	@echo '  make [target]'
	@echo ''
	@echo 'Targets:'
	@sed -n 's/^#?//p' $< | column -t -s ':' |  sort | sed -e 's/^/ /'