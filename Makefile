BINARY := keel
MODULE := github.com/getkaze/keel
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)

# Default: build for current platform
.PHONY: build
build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

# Cross-compile for Linux amd64 (EC2 / remote targets)
.PHONY: build-linux
build-linux:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-linux-amd64 .

# Cross-compile for Linux arm64
.PHONY: build-linux-arm64
build-linux-arm64:
	GOOS=linux GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-linux-arm64 .

# Cross-compile for macOS amd64 (Intel Mac)
.PHONY: build-darwin
build-darwin:
	GOOS=darwin GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-darwin-amd64 .

# Cross-compile for macOS arm64 (Apple Silicon)
.PHONY: build-darwin-arm64
build-darwin-arm64:
	GOOS=darwin GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-darwin-arm64 .

# Build for all targets
.PHONY: build-all
build-all: build build-linux build-linux-arm64 build-darwin build-darwin-arm64

.PHONY: clean
clean:
	rm -f bin/$(BINARY) bin/$(BINARY)-linux-* bin/$(BINARY)-darwin-*
