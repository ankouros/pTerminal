SHELL := /usr/bin/env bash
BIN := bin/pterminal
RELEASE_ROOT := release

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || date +%Y%m%d)
GOOS ?= linux
GOARCH ?= $(shell go env GOARCH)
RELEASE_DIR := $(RELEASE_ROOT)/pterminal-$(VERSION)-$(GOOS)-$(GOARCH)

LDFLAGS := -s -w

.PHONY: build run clean assets fmt vet release portable

build:
	go build -o $(BIN) ./cmd/pterminal

run: build
	./$(BIN)

assets:
	bash scripts/fetch_assets.sh

fmt:
	gofmt -w .

vet:
	go vet ./...

clean:
	rm -rf bin $(RELEASE_ROOT)

release:
	@test -f internal/ui/assets/vendor/xterm.js || (echo "Missing xterm assets. Run: make assets" && exit 1)
	@mkdir -p "$(RELEASE_DIR)"
	@echo "Building release: $(RELEASE_DIR)"
	CGO_ENABLED=1 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -trimpath -ldflags "$(LDFLAGS)" -o "$(RELEASE_DIR)/pterminal" ./cmd/pterminal
	@command -v strip >/dev/null 2>&1 && strip "$(RELEASE_DIR)/pterminal" || true
	@cp -f packaging/pterminal.desktop "$(RELEASE_DIR)/" || true
	@cp -f packaging/pterminal.svg "$(RELEASE_DIR)/" || true
	@cp -f packaging/release_README.md "$(RELEASE_DIR)/README.md" || true
	@cp -f scripts/check_deps.sh "$(RELEASE_DIR)/" || true
	@cp -f scripts/run_release.sh "$(RELEASE_DIR)/" || true
	@chmod +x "$(RELEASE_DIR)/check_deps.sh" "$(RELEASE_DIR)/run_release.sh" 2>/dev/null || true
	@tar -C "$(RELEASE_ROOT)" -czf "$(RELEASE_DIR).tar.gz" "$(notdir $(RELEASE_DIR))"
	@echo "Release created:"
	@echo "  $(RELEASE_DIR)/pterminal"
	@echo "  $(RELEASE_DIR).tar.gz"

portable:
	@test -f internal/ui/assets/vendor/xterm.js || (echo "Missing xterm assets. Run: make assets" && exit 1)
	@mkdir -p "$(RELEASE_DIR)"
	@echo "Building base release: $(RELEASE_DIR)"
	CGO_ENABLED=1 GOOS=$(GOOS) GOARCH=$(GOARCH) \
		go build -trimpath -ldflags "$(LDFLAGS)" -o "$(RELEASE_DIR)/pterminal" ./cmd/pterminal
	@command -v strip >/dev/null 2>&1 && strip "$(RELEASE_DIR)/pterminal" || true
	@echo "Building portable folder (best-effort shared libs next to executable)..."
	@bash scripts/build_portable_bundle.sh "$(RELEASE_DIR)/pterminal" "$(RELEASE_DIR)/portable"
	@tar -C "$(RELEASE_DIR)" -czf "$(RELEASE_DIR)-portable.tar.gz" portable
	@echo "Portable release created:"
	@echo "  $(RELEASE_DIR)/portable/run_portable.sh"
	@echo "  $(RELEASE_DIR)-portable.tar.gz"
