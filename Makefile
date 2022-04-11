#!make

# Usage:
# make help

## VARIABLES
BINARY_NAME = kube-role-gen
RELEASE_VERSION ?= v0.0.1
# Colour Outputs
GREEN := \033[0;32m
CLEAR := \033[00m

## Targets
build: clean
	@echo "$(GREEN)INFO: Building $(BINARY_NAME) $(CLEAR)"
	go get -v
	go mod tidy
	GOARCH=amd64 GOOS=darwin  go build -o $(BINARY_NAME)-darwin  main.go
	GOARCH=amd64 GOOS=linux   go build -o $(BINARY_NAME)-linux   main.go
	GOARCH=amd64 GOOS=windows go build -o $(BINARY_NAME)-windows main.go

clean:
	@echo "$(GREEN)CLEAN: Removing all binaries and temp files $(CLEAR)"
	go clean
	rm -f $(BINARY_NAME)-darwin $(BINARY_NAME)-linux $(BINARY_NAME)-windows
	rm -f role_base.yaml role_merged.yaml

run: clean build
	@echo "$(GREEN)RUN: Check Help $(CLEAR)"
	./${BINARY_NAME}-darwin -h

build_and_run: build run

release: run
	@echo "$(GREEN)RELEASE: Creating a GitHub release $(CLEAR)"
	gh release create $(RELEASE_VERSION) --generate-notes || true
	gh release upload v0.0.1 $(BINARY_NAME)-darwin $(BINARY_NAME)-linux $(BINARY_NAME)-windows --clobber
	$(MAKE) clean

release_undo:
	@echo "$(GREEN)UNDO RELEASE: Deleting a GitHub release and the corresponding tag $(CLEAR)"
	gh release delete $(RELEASE_VERSION) --yes || true
	git push --delete origin $(RELEASE_VERSION) || true
	git tag --delete $(RELEASE_VERSION) || true

help:
	@echo "$(GREEN)HELP: make <command> $(CLEAR)"
	@echo "  build           Build packages for different architectures"
	@echo "  run             Run a command to make sure package is working fine"
	@echo "  build_and_run   Build and Run"
	@echo "  clean           Remove the built packages (if any)"
	@echo "  release         Create a GitHub release"
	@echo "  release_undo    Delete a GitHub release"
