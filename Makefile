# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

BINARY   := lark-cli
MODULE   := github.com/larksuite/cli
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
DATE     := $(shell date +%Y-%m-%d)
LDFLAGS  := -s -w -X $(MODULE)/internal/build.Version=$(VERSION) -X $(MODULE)/internal/build.Date=$(DATE)
PREFIX   ?= /usr/local

.PHONY: all build vet test unit-test integration-test install uninstall clean fetch_meta gitleaks

all: test

fetch_meta:
	python3 scripts/fetch_meta.py

build: fetch_meta
	go build -trimpath -ldflags "$(LDFLAGS)" -o $(BINARY) .

vet: fetch_meta
	go vet ./...

unit-test: fetch_meta
	go test -race -gcflags="all=-N -l" -count=1 ./cmd/... ./internal/... ./shortcuts/...

integration-test: build
	go test -v -count=1 ./tests/...

test: vet unit-test integration-test

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
