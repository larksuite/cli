// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package plugin_e2e exercises the extension/platform plugin contract the way a
// real customer does: it builds a fork of lark-cli with a plugin blank-imported,
// then runs that fork as a subprocess and asserts the real stderr/stdout
// envelopes and exit codes. This is L4 coverage — the in-process unit and
// integration tests (extension/..., cmd/...) assert Go error values in the test
// process and structurally cannot observe envelope serialization, exit codes, or
// the blank-import -> init -> Register -> InstallAll assembly chain.
//
// Mechanism (the "customer build", mirrors xcaddy's build mode):
//  1. `git archive HEAD` a clean tree containing only committed files (so the
//     fork embeds the tracked meta_data stub, reproducing the bare-module state).
//  2. Generate a customer module: go.mod (cli's requires + `replace` to the
//     archived tree) + go.sum copy + main.go (blank-imports the plugin package)
//     + plugin package (its init() calls platform.Register).
//  3. `go build` the fork (offline-capable via the warm module cache), then run
//     it as a subprocess and assert.
package plugin_e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// cleanTree is the git-archived, committed-only source tree of the repo under
// test, shared by every fork build. Populated by TestMain (smoke_test.go) —
// TestMain must live in a _test.go file to be recognized by `go test`, so the
// entry point sits there while the rest of the harness mechanism lives here.
var cleanTree string

// baseDir holds the archive tree plus every generated customer module.
var baseDir string

// repoRoot resolves the lark-cli module root from the test's working directory
// (which `go test` sets to the package dir, tests/plugin_e2e).
func repoRoot() (string, error) {
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// gitArchive extracts HEAD's committed tree into dst by streaming `git archive`
// into `tar -x`. Only tracked files are included — gitignored build artifacts
// (e.g. the fetched meta_data.json) are absent, exactly as a module consumer
// would see them. It wires the two processes with an explicit pipe rather than a
// shell, so dst never reaches a shell command line.
func gitArchive(root, dst string) error {
	archive := exec.Command("git", "archive", "HEAD")
	archive.Dir = root
	extract := exec.Command("tar", "-x", "-C", dst)
	pipe, err := archive.StdoutPipe()
	if err != nil {
		return err
	}
	extract.Stdin = pipe
	// Each process gets its own stderr buffer: os/exec spawns a copy goroutine
	// per command, so a shared strings.Builder would be written concurrently by
	// both (git archive and tar run in parallel) -- a data race, since
	// strings.Builder is not concurrency-safe.
	var archiveErr, extractErr strings.Builder
	archive.Stderr = &archiveErr
	extract.Stderr = &extractErr
	if err := extract.Start(); err != nil {
		return err
	}
	if err := archive.Run(); err != nil {
		_ = extract.Wait()
		return fmt.Errorf("git archive: %w: %s", err, archiveErr.String())
	}
	if err := extract.Wait(); err != nil {
		return fmt.Errorf("tar extract: %w: %s", err, extractErr.String())
	}
	return nil
}

// builtForks caches fork binaries by name so identical forks are built once.
// builtForksMu guards it: no test in this package uses t.Parallel() today, but
// that is an implicit convention a future test could silently break, and an
// unguarded map write would then be a runtime panic. The lock is held across
// the whole build so concurrent callers also dedupe instead of racing to build
// the same fork twice.
var (
	builtForksMu sync.Mutex
	builtForks   = map[string]string{}
)

// buildFork generates a customer module whose plugin package body is pluginSrc,
// builds the fork, and returns the binary path. Forks are cached by name.
func buildFork(t *testing.T, name, pluginSrc string) string {
	t.Helper()
	builtForksMu.Lock()
	defer builtForksMu.Unlock()
	if bin, ok := builtForks[name]; ok {
		return bin
	}
	mod := filepath.Join(baseDir, "fork-"+name)
	if err := os.MkdirAll(filepath.Join(mod, "plugin"), 0o755); err != nil {
		t.Fatalf("mkdir customer module: %v", err)
	}

	// go.mod: reuse cli's require graph, rename the module, replace cli with the
	// local archived tree. This avoids `go mod tidy` (no network at test time).
	rawMod, err := os.ReadFile(filepath.Join(cleanTree, "go.mod"))
	if err != nil {
		t.Fatalf("read archived go.mod: %v", err)
	}
	gomod := strings.Replace(string(rawMod), "module github.com/larksuite/cli", "module larkcustomer", 1)
	gomod += "\nrequire github.com/larksuite/cli v0.0.0\n\nreplace github.com/larksuite/cli => " + cleanTree + "\n"
	writeFile(t, filepath.Join(mod, "go.mod"), gomod)

	// go.sum: transitive dependency hashes are identical to cli's.
	rawSum, err := os.ReadFile(filepath.Join(cleanTree, "go.sum"))
	if err != nil {
		t.Fatalf("read archived go.sum: %v", err)
	}
	writeFile(t, filepath.Join(mod, "go.sum"), string(rawSum))

	writeFile(t, filepath.Join(mod, "main.go"), customerMain)
	writeFile(t, filepath.Join(mod, "plugin", "plugin.go"), pluginSrc)

	bin := filepath.Join(mod, "fork-bin")
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Dir = mod
	// -mod=mod fixes require annotations copied from cli's go.mod; the default
	// GOPROXY resolves any dep missing from the cache (goproxy in CI/dev).
	build.Env = append(os.Environ(), "GOFLAGS=-mod=mod")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fork %q failed: %v\n%s", name, err, out)
	}
	builtForks[name] = bin
	return bin
}

const customerMain = `// Code generated by plugin_e2e; DO NOT EDIT.
package main

import (
	"os"

	"github.com/larksuite/cli/cmd"
	_ "larkcustomer/plugin" // blank import triggers plugin init() -> platform.Register
)

func main() { os.Exit(cmd.Execute()) }
`

// result is a subprocess run outcome.
type result struct {
	stdout string
	stderr string
	exit   int
}

// run executes the fork binary with args in an isolated, offline environment and
// captures stdout/stderr/exit. Each call gets a fresh empty
// LARKSUITE_CLI_CONFIG_DIR and LARKSUITE_CLI_REMOTE_META=off, so the fork never
// inherits the host's ~/.lark-cli cache or makes a startup metadata fetch to the
// open platform. That reproduces the bare-module customer state (no embedded
// metadata, cold cache) deterministically on any machine, including CI: without
// it, whether a command's assertion is reached depends on whether a live network
// fetch happened to succeed. Tests that need runtime metadata seed it explicitly
// via runWithSeededCatalog.
func run(t *testing.T, bin string, args ...string) result {
	t.Helper()
	return runWithEnv(t, bin, isolatedEnv(t), args...)
}

// baseEnv is the host environment with every LARKSUITE_CLI_* variable removed.
// Appending overrides to a raw os.Environ() only isolates the variables we
// explicitly set — a developer machine exporting, say, LARKSUITE_CLI_AUTH_PROXY
// or LARKSUITE_CLI_BRAND would leak them into the fork (the transport
// interceptor and credential providers read them via os.Getenv directly),
// breaking the "deterministic on any machine" guarantee. Stripping the whole
// namespace first makes the fork's CLI-facing environment exactly the
// variables the harness sets, everywhere.
func baseEnv() []string {
	env := os.Environ()
	kept := env[:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, "LARKSUITE_CLI_") {
			kept = append(kept, kv)
		}
	}
	return kept
}

// isolatedEnv is the bare-module, offline environment shared by run() and (as a
// base) by runWithSeededCatalog.
func isolatedEnv(t *testing.T) []string {
	t.Helper()
	return append(baseEnv(),
		"LARKSUITE_CLI_NO_UPDATE_NOTIFIER=1",
		"LARKSUITE_CLI_NO_SKILLS_NOTIFIER=1",
		"LARKSUITE_CLI_CONFIG_DIR="+t.TempDir(),
		"LARKSUITE_CLI_REMOTE_META=off",
	)
}

// runWithEnv runs bin as a subprocess with the given full environment, capturing
// stdout/stderr/exit.
func runWithEnv(t *testing.T, bin string, env []string, args ...string) result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c := exec.CommandContext(ctx, bin, args...)
	c.Env = env
	var stdout, stderr strings.Builder
	c.Stdout = &stdout
	c.Stderr = &stderr
	err := c.Run()
	// A fork that hangs is killed by the context and surfaces as a generic
	// exit=-1 ExitError; name the timeout explicitly so the failure reads as
	// "hung" rather than "crashed".
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("run %v: timed out after 60s; stdout=%s stderr=%s", args, stdout.String(), stderr.String())
	}
	exit := 0
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			exit = ee.ExitCode()
		} else {
			t.Fatalf("run %v: %v", args, err)
		}
	}
	return result{stdout: stdout.String(), stderr: stderr.String(), exit: exit}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
