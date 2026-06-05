# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

BINARY   := lark-cli
MODULE   := github.com/larksuite/cli
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
DATE     := $(shell date +%Y-%m-%d)
LDFLAGS  := -s -w -X $(MODULE)/internal/build.Version=$(VERSION) -X $(MODULE)/internal/build.Date=$(DATE)
PREFIX   ?= /usr/local

.PHONY: all build vet fmt-check test unit-test integration-test examples-build install uninstall clean fetch_meta gitleaks

all: test

fetch_meta:
	python3 scripts/fetch_meta.py

build: fetch_meta
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) .

vet: fetch_meta
	go vet ./...

# fmt-check fails when any file would be reformatted by gofmt. Keep this
# in sync with the fast-gate "Check formatting" step in CI.
fmt-check:
	@unformatted=$$(gofmt -l . | grep -v '^\.claude/' || true); \
	if [ -n "$$unformatted" ]; then \
		echo "Unformatted Go files:"; \
		echo "$$unformatted"; \
		echo "Run 'gofmt -w .' and commit."; \
		exit 1; \
	fi

# ./extension/... keeps the public plugin SDK in the default test matrix.
unit-test: fetch_meta
	go test -race -gcflags="all=-N -l" -count=1 \
		./cmd/... ./internal/... ./shortcuts/... ./extension/...

# examples-build keeps the shipped plugin-SDK examples compilable. If this
# breaks, the plugin author guide's "go build ./..." path is broken.
examples-build:
	go build ./extension/platform/examples/audit-observer
	go build ./extension/platform/examples/readonly-policy

integration-test: build
	go test -v -count=1 ./tests/...

test: vet fmt-check unit-test examples-build integration-test

install: build
	install -d $(PREFIX)/bin
	install -m755 $(BINARY) $(PREFIX)/bin/$(BINARY)
	@echo "OK: $(PREFIX)/bin/$(BINARY) ($(VERSION))"

uninstall:
	rm -f $(PREFIX)/bin/$(BINARY)

clean:
	rm -f $(BINARY)

# Run secret-leak checks locally before pushing.
# Step 1: check-doc-tokens catches realistic-looking example tokens in reference
#         docs and asks you to use _EXAMPLE_TOKEN placeholders instead.
# Step 2: gitleaks scans the full repo for real leaked secrets.
# Install gitleaks: https://github.com/gitleaks/gitleaks#installing
gitleaks:
	@bash scripts/check-doc-tokens.sh
	@command -v gitleaks >/dev/null 2>&1 || { echo "gitleaks not found. Install: brew install gitleaks"; exit 1; }
	gitleaks detect --redact -v --exit-code=2
