// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Architecture layering gates for the event tree.
//
// Layout under internal/event:
//
//	kernel   — the root package plus model, catalog, processing, schemas,
//	           application/...: pure event semantics, no I/O, no framework.
//	hosts    — bus, consume: long-running processes that may drive exactly
//	           the adapters pinned in hostAdapterAllowlist.
//	adapters — adapter/...: concrete transports (lark websocket, localbus).
//
// Dependencies must point inward: adapters and hosts may use the kernel, the
// kernel must never reach outward. Once a kernel package touches an adapter,
// a host, the platform SDK, or the network, every consumer of the value
// model silently links transports it never asked for, and the composition
// root loses the ability to swap them. These tests turn that drift into a
// build break.
package event_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	archModulePath          = "github.com/larksuite/cli"
	archAdapterImportPrefix = archModulePath + "/internal/event/adapter"

	// archKernelFileFloor is an idle detector, not a target: the kernel had
	// 19 production files when the floor was set. If the walker, the
	// outwardFacing set, or a path constant rots so the gate scans (almost)
	// nothing, a green run would be meaningless — fail hard instead.
	archKernelFileFloor = 15
)

// outwardFacing lists the internal/event subtrees exempt from kernel purity,
// each with the reason it may face outward. Everything else — including any
// directory added later — is kernel by default and governed by
// TestArchKernelPurity without anyone remembering to opt in.
var outwardFacing = map[string]string{
	"adapter":  "concrete transports (lark websocket, localbus); the outermost ring",
	"bus":      "host process owning the local bus lifecycle",
	"consume":  "host process owning the consumer loop",
	"testutil": "shared test fakes; never linked into production binaries",
}

// archForbiddenKernelImport reports why importPath is banned inside the
// kernel, if it is. TestArchKernelImportDetectorSelfCheck exercises this
// function on synthetic sources so a drifted matcher cannot keep reporting
// green.
func archForbiddenKernelImport(importPath string) (reason string, banned bool) {
	switch {
	case importPath == "github.com/spf13/cobra":
		return "CLI framework; command wiring lives in cmd, not in event semantics", true
	case strings.HasPrefix(importPath, "github.com/larksuite/oapi-sdk-go"):
		return "platform SDK; only adapters may speak the platform wire format", true
	case importPath == "net" || strings.HasPrefix(importPath, "net/"):
		return "networking (any net or net/* package, net/url included); kernel logic must stay I/O-free so any host can embed it", true
	case importPath == archModulePath+"/internal/event/bus",
		importPath == archModulePath+"/internal/event/consume":
		return "host package; the kernel calling its host inverts the dependency direction", true
	case importPath == archAdapterImportPrefix || strings.HasPrefix(importPath, archAdapterImportPrefix+"/"):
		return "concrete adapter; depend on a port and let the composition root inject the implementation", true
	}
	return "", false
}

// archKernelDirs derives the kernel directory set from the tree itself:
// every directory under internal/event (the root included) minus the
// outwardFacing subtrees and testdata. Deriving instead of enumerating means
// a newly added package is governed by default — the gate can only lose
// coverage through an explicit outwardFacing edit, never through forgetting.
func archKernelDirs(t *testing.T) []string {
	t.Helper()
	var dirs []string
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		rel := filepath.ToSlash(path)
		if rel == "." {
			dirs = append(dirs, rel)
			return nil
		}
		if d.Name() == "testdata" {
			return fs.SkipDir
		}
		top, _, _ := strings.Cut(rel, "/")
		if _, outward := outwardFacing[top]; outward {
			return fs.SkipDir
		}
		dirs = append(dirs, rel)
		return nil
	})
	if err != nil {
		t.Fatalf("walk internal/event: %v", err)
	}
	sort.Strings(dirs)
	return dirs
}

// archProductionFilesUnder returns every non-test .go file under root,
// skipping testdata directories.
func archProductionFilesUnder(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(files)
	return files
}

// archImports parses one file with ImportsOnly and returns its import paths.
func archImports(t *testing.T, fset *token.FileSet, file string) []string {
	t.Helper()
	f, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	paths := make([]string, 0, len(f.Imports))
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatalf("unquote import in %s: %v", file, err)
		}
		paths = append(paths, path)
	}
	return paths
}

// TestArchKernelPurity fails when any kernel production file imports the CLI
// framework, the platform SDK, the network, a host, or an adapter. The
// kernel set is derived from the directory tree, with two tripwires against
// the gate itself being hollowed out.
func TestArchKernelPurity(t *testing.T) {
	// Tripwire: this gate proves the kernel/adapter boundary holds. If the
	// adapter tree is gone the boundary does not exist and a green run
	// proves nothing — someone moved the transports without moving the gate.
	if fi, err := os.Stat("adapter"); err != nil || !fi.IsDir() {
		t.Fatalf("internal/event/adapter does not exist (%v) — the boundary this gate guards is gone; relocate the gate together with the adapters", err)
	}

	kernelDirs := archKernelDirs(t)

	// Tripwire: deriving the kernel set means moving a package out of the
	// tree silently shrinks coverage. Pin the named kernel packages so
	// "make the gate green by relocating the offender" fails loudly.
	for _, required := range []string{"application", "catalog", "model", "processing", "schemas"} {
		if !slices.Contains(kernelDirs, required) {
			t.Fatalf("kernel package %q missing from derived gate set %v — if it genuinely moved, update this gate deliberately instead of letting coverage shrink", required, kernelDirs)
		}
	}

	fset := token.NewFileSet()
	parsed := 0
	for _, dir := range kernelDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read %s: %v", dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			file := filepath.Join(dir, name)
			parsed++
			for _, path := range archImports(t, fset, file) {
				if reason, banned := archForbiddenKernelImport(path); banned {
					t.Errorf("%s imports %q: %s", filepath.ToSlash(file), path, reason)
				}
			}
		}
	}
	if parsed < archKernelFileFloor {
		t.Fatalf("parsed only %d kernel production files (floor %d) — the walker or the outwardFacing set is eating the tree; a gate that scans nothing reports green forever", parsed, archKernelFileFloor)
	}
}

// archForbiddenHostImport reports why importPath is banned inside a host
// production file, if it is. Hosts are deliberately held to a looser standard
// than the kernel: they own sockets, so the standard library net packages are
// legitimate there (the local bus is IPC over a socket) and only the platform
// SDK is banned — a host speaking the platform wire format directly would
// bypass the bus and its adapters, which is the seam the architecture exists
// to keep. Matching is segment-boundary safe so a lookalike module name
// cannot trip it.
func archForbiddenHostImport(importPath string) (reason string, banned bool) {
	const sdk = "github.com/larksuite/oapi-sdk-go"
	if importPath == sdk || strings.HasPrefix(importPath, sdk+"/") {
		return "platform SDK; hosts reach the platform only through the bus and its adapters, never by speaking the platform wire format themselves", true
	}
	return "", false
}

// TestArchHostPlatformSDKBan fails when any host production file imports the
// platform SDK. The host set is pinned to the two long-running processes; a
// new host must be added here deliberately.
func TestArchHostPlatformSDKBan(t *testing.T) {
	hosts := []string{"bus", "consume"}

	sawNetImport := false
	for _, host := range hosts {
		if _, ok := outwardFacing[host]; !ok {
			t.Fatalf("host %q is not in the outwardFacing set — the host moved and this gate is scanning a ghost", host)
		}
		files := archProductionFilesUnder(t, host)
		if len(files) == 0 {
			t.Fatalf("no production files found under %s — the walker is idling or the host is gone; fix that before trusting this gate", host)
		}
		fset := token.NewFileSet()
		for _, file := range files {
			for _, path := range archImports(t, fset, file) {
				if path == "net" || strings.HasPrefix(path, "net/") {
					sawNetImport = true
				}
				if reason, banned := archForbiddenHostImport(path); banned {
					t.Errorf("%s imports %q: %s", filepath.ToSlash(file), path, reason)
				}
			}
		}
	}

	// Tripwire: hosts do socket IPC, so the scan of real host files must have
	// seen a net import. Seeing none means the import walk is not reading what
	// the hosts actually import, and a green run would prove nothing. (It also
	// pins the reason net is absent from the ban set: hosts need it.)
	if !sawNetImport {
		t.Fatal("scanned every host production file and found no net import — hosts are socket-IPC processes, so the import scan cannot be trusted; if the hosts genuinely dropped net, update this tripwire deliberately")
	}
}

// TestArchHostImportDetectorSelfCheck runs the host forbidden-import matcher
// on synthetic sources with known outcomes, so a drifted matcher cannot keep
// TestArchHostPlatformSDKBan green on a violating tree.
func TestArchHostImportDetectorSelfCheck(t *testing.T) {
	scan := func(src string) []string {
		t.Helper()
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "synthetic.go", src, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse synthetic source: %v", err)
		}
		var flagged []string
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("unquote synthetic import: %v", err)
			}
			if _, banned := archForbiddenHostImport(path); banned {
				flagged = append(flagged, path)
			}
		}
		sort.Strings(flagged)
		return flagged
	}

	const violating = `package fakehost

import (
	_ "github.com/larksuite/oapi-sdk-go/v3"
	_ "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	_ "net"
)
`
	want := []string{
		"github.com/larksuite/oapi-sdk-go/v3",
		"github.com/larksuite/oapi-sdk-go/v3/service/im/v1",
	}
	if got := scan(violating); !slices.Equal(got, want) {
		t.Fatalf("detector self-check: flagged %v, want exactly %v — the matcher drifted and the host gate cannot be trusted", got, want)
	}

	// net stays legal for hosts (socket IPC), and prefix matching must respect
	// path segment boundaries.
	const clean = `package fakehost

import (
	_ "github.com/larksuite/oapi-sdk-golike"
	_ "net"
	_ "net/http"
)
`
	if got := scan(clean); len(got) != 0 {
		t.Fatalf("detector self-check: clean synthetic source flagged %v — the matcher over-triggers", got)
	}
}

// hostAdapterAllowlist pins, per host, the exact adapter import paths its
// production code uses today.
//
// This is a ceiling, not a design: deleting an entry is always welcome;
// adding one means a host wired in another adapter, and the question to ask
// is whether that capability should instead be a port owned by the host and
// injected by the cmd/event composition root.
var hostAdapterAllowlist = map[string][]string{
	"bus": {
		archAdapterImportPrefix + "/localbus/busdiscover",
		archAdapterImportPrefix + "/localbus/protocol",
		archAdapterImportPrefix + "/localbus/transport",
	},
	"consume": {
		archAdapterImportPrefix + "/localbus/protocol",
		archAdapterImportPrefix + "/localbus/transport",
	},
}

// TestArchHostAdapterAllowlist checks the host→adapter edges in both
// directions: an adapter import outside the allowlist is a new coupling that
// skipped review, and an allowlist entry no adapter import matches is a
// ceiling wider than reality — it would let a removed dependency come back
// without anyone looking.
func TestArchHostAdapterAllowlist(t *testing.T) {
	hosts := make([]string, 0, len(hostAdapterAllowlist))
	for host := range hostAdapterAllowlist {
		hosts = append(hosts, host)
	}
	sort.Strings(hosts)

	for _, host := range hosts {
		// An allowlist key that is not an outward-facing package is dead
		// text: the kernel purity gate already bans those dirs from touching
		// adapters, so the entry would silently grant nothing to no one.
		if _, ok := outwardFacing[host]; !ok {
			t.Errorf("hostAdapterAllowlist key %q is not in the outwardFacing set — the entry is dead text; either the host moved or the entry must go", host)
			continue
		}

		files := archProductionFilesUnder(t, host)
		if len(files) == 0 {
			t.Errorf("no production files found under %s — the walker is idling or the host is gone; fix that before trusting this gate", host)
			continue
		}

		allowed := make(map[string]bool, len(hostAdapterAllowlist[host]))
		for _, path := range hostAdapterAllowlist[host] {
			allowed[path] = true
		}
		used := make(map[string]bool)

		fset := token.NewFileSet()
		for _, file := range files {
			for _, path := range archImports(t, fset, file) {
				if path != archAdapterImportPrefix && !strings.HasPrefix(path, archAdapterImportPrefix+"/") {
					continue
				}
				if allowed[path] {
					used[path] = true
					continue
				}
				msg := "declare the needed capability as a port owned by this package and let the cmd/event composition root inject the implementation"
				if strings.HasPrefix(path, archAdapterImportPrefix+"/lark") {
					msg += " — a host wired straight to the platform adapter bypasses the bus, which is the reason the bus exists"
				}
				t.Errorf("%s imports %q outside hostAdapterAllowlist[%q]: %s", filepath.ToSlash(file), path, host, msg)
			}
		}

		for _, path := range hostAdapterAllowlist[host] {
			if !used[path] {
				t.Errorf("stale hostAdapterAllowlist entry %q for host %q: no production file imports it — delete the entry so a re-introduction has to pass review", path, host)
			}
		}
	}
}

// TestArchKernelImportDetectorSelfCheck runs the kernel forbidden-import
// matcher on synthetic sources with known outcomes. If a path constant
// drifts from the real package layout, TestArchKernelPurity would keep
// reporting green on a violating tree; this test makes that failure mode a
// build break of its own.
func TestArchKernelImportDetectorSelfCheck(t *testing.T) {
	scan := func(src string) []string {
		t.Helper()
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "synthetic.go", src, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse synthetic source: %v", err)
		}
		var flagged []string
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("unquote synthetic import: %v", err)
			}
			if _, banned := archForbiddenKernelImport(path); banned {
				flagged = append(flagged, path)
			}
		}
		sort.Strings(flagged)
		return flagged
	}

	const violating = `package fakekernel

import (
	_ "context"
	_ "github.com/larksuite/cli/internal/event/adapter/localbus/protocol"
	_ "github.com/larksuite/cli/internal/event/bus"
	_ "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
	_ "github.com/spf13/cobra"
	_ "net/http"
)
`
	want := []string{
		"github.com/larksuite/cli/internal/event/adapter/localbus/protocol",
		"github.com/larksuite/cli/internal/event/bus",
		"github.com/larksuite/oapi-sdk-go/v3/service/im/v1",
		"github.com/spf13/cobra",
		"net/http",
	}
	if got := scan(violating); !slices.Equal(got, want) {
		t.Fatalf("detector self-check: flagged %v, want exactly %v — the matcher drifted from the package layout and the purity gate cannot be trusted", got, want)
	}

	const clean = `package fakekernel

import (
	_ "context"
	_ "encoding/json"
	_ "github.com/larksuite/cli/internal/event/adapterlike"
	_ "github.com/larksuite/cli/internal/event/model"
	_ "network"
)
`
	if got := scan(clean); len(got) != 0 {
		t.Fatalf("detector self-check: clean synthetic source flagged %v — the matcher over-triggers (prefix matching must respect path segment boundaries)", got)
	}
}
