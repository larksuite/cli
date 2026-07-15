# Copyright (c) 2026 Lark Technologies Pte. Ltd.
# SPDX-License-Identifier: MIT

BINARY   := lark-cli
MODULE   := github.com/larksuite/cli
VERSION  := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
DATE     := $(shell date +%Y-%m-%d)
NODE     ?= node
QUALITY_GATE_CHANGED_FROM ?= $(shell bash scripts/resolve-changed-from.sh)
QUALITY_GATE_CHANGED_FROM_RESOLVED = $(if $(strip $(QUALITY_GATE_CHANGED_FROM)),$(QUALITY_GATE_CHANGED_FROM),$(shell bash scripts/resolve-changed-from.sh))
QUALITY_GATE_DIR ?= .tmp/quality-gate
QUALITY_GATE_MANIFEST_OUT ?= $(QUALITY_GATE_DIR)/command-manifest.json
QUALITY_GATE_COMMAND_INDEX_OUT ?= $(QUALITY_GATE_DIR)/command-index.json
QUALITY_GATE_FACTS_OUT ?= $(QUALITY_GATE_DIR)/facts.json
PUBLIC_CONTENT_METADATA ?= $(QUALITY_GATE_DIR)/public-content-metadata.json
LDFLAGS  := -s -w -X $(MODULE)/internal/build.Version=$(VERSION) -X $(MODULE)/internal/build.Date=$(DATE)
PREFIX   ?= /usr/local

# The repository's Go 1.23 CI toolchain does not support -race on riscv64.
# Prefer GOARCH passed to make (for example, `make GOARCH=riscv64 unit-test`)
# over `go env GOARCH`, because command-line make variables are not visible to
# $(shell ...).
TEST_GOARCH := $(or $(GOARCH),$(shell go env GOARCH))
RACE_FLAG := $(if $(filter riscv64,$(TEST_GOARCH)),,-race)

.PHONY: all build vet fmt-check script-test test unit-test integration-test examples-build quality-gate install uninstall clean fetch_meta gitleaks sidecar-test

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

script-test:
	bash scripts/resolve-changed-from.test.sh
	bash scripts/ci-workflow.test.sh
	bash scripts/semantic-review-workflow.test.sh
	$(NODE) --test scripts/e2e_domains.test.js scripts/fetch_e2e_tat.test.js scripts/semantic-review-verify-artifact.test.js scripts/pr-quality-summary.test.js scripts/semantic-review-publish.test.js scripts/ci-quality-summary-publish.test.js

# ./extension/... keeps the public plugin SDK in the default test matrix.
unit-test: fetch_meta
	go test $(RACE_FLAG) -gcflags="all=-N -l" -count=1 \
		./cmd/... ./internal/... ./shortcuts/... ./extension/...

# examples-build keeps the shipped plugin-SDK examples compilable. If this
# breaks, the plugin author guide's "go build ./..." path is broken.
examples-build:
	go build ./extension/platform/examples/audit-observer
	go build ./extension/platform/examples/readonly-policy

# ./tests/... includes tests/plugin_e2e, which builds ~20 customer-fork
# binaries (~1 min warm; a cold module cache also downloads via GOPROXY).
# Deliberate: local `make test` exercises the L4 plugin contract by default.
integration-test: build
	go test -v -count=1 ./tests/...

test: vet fmt-check script-test unit-test examples-build integration-test

quality-gate: build
	mkdir -p $(QUALITY_GATE_DIR) $(dir $(QUALITY_GATE_FACTS_OUT)) $(dir $(PUBLIC_CONTENT_METADATA))
	test -f $(PUBLIC_CONTENT_METADATA) || printf '{}\n' > $(PUBLIC_CONTENT_METADATA)
	LARKSUITE_CLI_REMOTE_META=off \
	LARKSUITE_CLI_NO_UPDATE_NOTIFIER=1 \
	LARKSUITE_CLI_NO_SKILLS_NOTIFIER=1 \
	go run ./internal/qualitygate/cmd/manifest-export \
		--manifest-out $(QUALITY_GATE_MANIFEST_OUT) \
		--command-index-out $(QUALITY_GATE_COMMAND_INDEX_OUT)
	LARKSUITE_CLI_APP_ID=dry-run \
	LARKSUITE_CLI_APP_SECRET=dry-run \
	LARKSUITE_CLI_BRAND=feishu \
	LARKSUITE_CLI_CONFIG_DIR=$${TMPDIR:-/tmp}/quality-gate-cli-config \
	LARKSUITE_CLI_REMOTE_META=off \
	LARKSUITE_CLI_NO_UPDATE_NOTIFIER=1 \
	LARKSUITE_CLI_NO_SKILLS_NOTIFIER=1 \
	go run ./internal/qualitygate/cmd/quality-gate check \
		--repo . \
		--cli-bin ./$(BINARY) \
		--changed-from $(QUALITY_GATE_CHANGED_FROM_RESOLVED) \
		--manifest $(QUALITY_GATE_MANIFEST_OUT) \
		--command-index $(QUALITY_GATE_COMMAND_INDEX_OUT) \
		--public-content-metadata $(PUBLIC_CONTENT_METADATA) \
		--facts-out $(QUALITY_GATE_FACTS_OUT)

install: build
	install -d $(PREFIX)/bin
	install -m755 $(BINARY) $(PREFIX)/bin/$(BINARY)
	@echo "OK: $(PREFIX)/bin/$(BINARY) ($(VERSION))"

uninstall:
	rm -f $(PREFIX)/bin/$(BINARY)

clean:
	rm -f $(BINARY)

# sidecar-test compiles and runs the authsidecar* build-tagged code that the
# default CI matrix never sees (they carry //go:build tags).
sidecar-test:
	go build -tags authsidecar -o /dev/null .
	go test $(RACE_FLAG) -count=1 -tags authsidecar ./extension/credential/sidecar/ ./extension/transport/sidecar/ ./internal/cmdutil/
	go test $(RACE_FLAG) -count=1 -tags authsidecar_demo ./sidecar/server-demo/
	go test $(RACE_FLAG) -count=1 -tags authsidecar ./tests/sidecar_e2e/

# Run secret-leak checks locally before pushing.
# Step 1: check-doc-tokens catches realistic-looking example tokens in reference
#         docs and asks you to use _EXAMPLE_TOKEN placeholders instead.
# Step 2: gitleaks scans the full repo for real leaked secrets.
# Install gitleaks: https://github.com/gitleaks/gitleaks#installing
gitleaks:
	@bash scripts/check-doc-tokens.sh
	@command -v gitleaks >/dev/null 2>&1 || { echo "gitleaks not found. Install: brew install gitleaks"; exit 1; }
	gitleaks detect --redact -v --exit-code=2
