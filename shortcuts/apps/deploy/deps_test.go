// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/fileio"
)

// permissiveFIO delegates to os without the cwd sandbox check so the scanner
// can be driven with absolute t.TempDir paths. Production goes through the
// cwd-bounded LocalFileIO, which these tests deliberately do not exercise.
type permissiveFIO struct{}

func (permissiveFIO) Open(name string) (fileio.File, error)     { return os.Open(name) }
func (permissiveFIO) Stat(name string) (fileio.FileInfo, error) { return os.Stat(name) }
func (permissiveFIO) ResolvePath(p string) (string, error)      { return p, nil }
func (permissiveFIO) Save(string, fileio.SaveOptions, io.Reader) (fileio.SaveResult, error) {
	panic("Save not used in deploy unit tests")
}

// collectRels runs CollectFile on root/entry and returns the published paths.
func collectRels(t *testing.T, root, entry string) ([]string, []string) {
	t.Helper()
	cands, _, skipped, err := CollectFile(permissiveFIO{}, filepath.Join(root, entry))
	if err != nil {
		t.Fatalf("CollectFile: %v", err)
	}
	if len(cands) == 0 || cands[0].RelPath != entry {
		t.Fatalf("entry must be the first candidate, got %+v", cands)
	}
	rels := make([]string, 0, len(cands))
	for _, c := range cands {
		rels = append(rels, c.RelPath)
	}
	sort.Strings(rels)
	notes := make([]string, 0, len(skipped))
	for _, sk := range skipped {
		notes = append(notes, sk.String())
	}
	return rels, notes
}

func TestCollectFileFollowsDependencyClosure(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "page.html"), `
<html><head>
  <link rel="stylesheet" href="style.css">
  <link rel="icon" href="./assets/favicon.png">
  <style>body { background: url("assets/bg.jpg"); }</style>
</head><body>
  <img src="assets/logo.png" srcset="assets/logo.png 1x, assets/logo@2x.png 2x">
  <div style="background-image:url(assets/inline.png)"></div>
  <script src="app.js"></script>
  <script src="lib/boot.js"></script>
</body></html>`)
	mustWrite(t, filepath.Join(root, "style.css"), `@import "theme.css";
.a { background: url('assets/bg.jpg'); }`)
	mustWrite(t, filepath.Join(root, "theme.css"), `.b{}`)
	mustWrite(t, filepath.Join(root, "app.js"), `document.title = "ok";`)
	mustWrite(t, filepath.Join(root, "lib", "boot.js"), `window.booted = true;`)
	for _, a := range []string{"favicon.png", "bg.jpg", "logo.png", "logo@2x.png", "inline.png"} {
		mustWrite(t, filepath.Join(root, "assets", a), "img")
	}
	// Not referenced by anything: --file-path publishes the closure, not the
	// directory, so this must stay out.
	mustWrite(t, filepath.Join(root, "unrelated.html"), "<html></html>")

	rels, skipped := collectRels(t, root, "page.html")
	want := []string{
		"app.js", "assets/bg.jpg", "assets/favicon.png", "assets/inline.png",
		"assets/logo.png", "assets/logo@2x.png", "lib/boot.js",
		"page.html", "style.css", "theme.css",
	}
	if strings.Join(rels, ",") != strings.Join(want, ",") {
		t.Fatalf("closure mismatch:\n got %v\nwant %v", rels, want)
	}
	if len(skipped) != 0 {
		t.Fatalf("expected no skips, got %v", skipped)
	}
}

// ESM module specifiers are followed when a payload does use them, but no other
// test depends on module scripts: a bare HTML page normally loads plain
// scripts, and the closure must be provable without ESM semantics.
func TestCollectFileFollowsESMSpecifiers(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "page.html"), `<script type="module" src="app.js"></script>`)
	mustWrite(t, filepath.Join(root, "app.js"), `import {x} from "./lib/util.js";
import "./side.js";
const p = import('./lazy.js');
import react from "react";`)
	mustWrite(t, filepath.Join(root, "lib", "util.js"), `export const x = 1;`)
	mustWrite(t, filepath.Join(root, "side.js"), ``)
	mustWrite(t, filepath.Join(root, "lazy.js"), ``)

	rels, skipped := collectRels(t, root, "page.html")
	want := []string{"app.js", "lazy.js", "lib/util.js", "page.html", "side.js"}
	if strings.Join(rels, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", rels, want)
	}
	// "react" is a package name, not a file in the payload: following it would
	// be wrong rather than merely useless, so it must not even be reported.
	if len(skipped) != 0 {
		t.Fatalf("a bare specifier must not be reported as a skip: %v", skipped)
	}
}

func TestCollectFileIgnoresExternalAndInertReferences(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "page.html"), `
<link href="https://cdn.example.com/a.css">
<link href="//cdn.example.com/b.css">
<script src="http://cdn.example.com/c.js"></script>
<img src="data:image/png;base64,AAAA">
<a href="other.html">nav</a>
<a href="#section">anchor</a>
<img src="#">
<!-- <link href="ghost.css"> -->
<form action="submit.php"></form>`)
	mustWrite(t, filepath.Join(root, "other.html"), "<html></html>")
	mustWrite(t, filepath.Join(root, "ghost.css"), ".x{}")
	mustWrite(t, filepath.Join(root, "submit.php"), "<?php")

	rels, skipped := collectRels(t, root, "page.html")
	if strings.Join(rels, ",") != "page.html" {
		t.Fatalf("only the entry should be published, got %v", rels)
	}
	if len(skipped) != 0 {
		t.Fatalf("external references must not be reported as skips, got %v", skipped)
	}
}

func TestCollectFileStripsQueryAndFragment(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "page.html"), `
<link href="style.css?v=2">
<img src="assets/icon%20one.png#frag">
<script src="app.js#v=1"></script>`)
	mustWrite(t, filepath.Join(root, "style.css"), ".a{}")
	mustWrite(t, filepath.Join(root, "app.js"), "")
	mustWrite(t, filepath.Join(root, "assets", "icon one.png"), "img")

	rels, skipped := collectRels(t, root, "page.html")
	want := []string{"app.js", "assets/icon one.png", "page.html", "style.css"}
	if strings.Join(rels, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", rels, want)
	}
	if len(skipped) != 0 {
		t.Fatalf("unexpected skips: %v", skipped)
	}
}

func TestCollectFileReportsMissingDependency(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "page.html"), `<link href="style.css"><img src="gone.png">`)
	mustWrite(t, filepath.Join(root, "style.css"), ".a{}")

	rels, skipped := collectRels(t, root, "page.html")
	if strings.Join(rels, ",") != "page.html,style.css" {
		t.Fatalf("a missing dependency must not drop the rest: %v", rels)
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0], "gone.png") ||
		!strings.Contains(skipped[0], "does not exist") ||
		!strings.Contains(skipped[0], "page.html") {
		t.Fatalf("skip note should name the file, the referrer and the reason: %v", skipped)
	}
}

func TestCollectFileRejectsDependencyAboveEntryDirectory(t *testing.T) {
	root := t.TempDir()
	site := filepath.Join(root, "site")
	mustWrite(t, filepath.Join(site, "page.html"), `<link href="../shared/theme.css">`)
	mustWrite(t, filepath.Join(root, "shared", "theme.css"), ".a{}")

	rels, skipped := collectRels(t, site, "page.html")
	if strings.Join(rels, ",") != "page.html" {
		t.Fatalf("a dependency above the entry directory must not be published: %v", rels)
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0], "../shared/theme.css") ||
		!strings.Contains(skipped[0], "outside the entry file's directory") {
		t.Fatalf("expected one out-of-root note, got %v", skipped)
	}
}

// A root-absolute reference is read as site-root-absolute, which for a
// single-file publish means the entry's own directory.
func TestCollectFileResolvesRootAbsoluteAgainstEntryDirectory(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "page.html"), `<link href="/style.css">`)
	mustWrite(t, filepath.Join(root, "style.css"), ".a{}")

	rels, skipped := collectRels(t, root, "page.html")
	if strings.Join(rels, ",") != "page.html,style.css" {
		t.Fatalf("got %v", rels)
	}
	if len(skipped) != 0 {
		t.Fatalf("unexpected skips: %v", skipped)
	}
}

// The scan must not become a way to reach content the flag validation would
// have rejected: a symlink pointing out of the payload root is dropped even
// though it stats as a regular file.
func TestCollectFileRejectsSymlinkEscapingRoot(t *testing.T) {
	root := t.TempDir()
	site := filepath.Join(root, "site")
	mustWrite(t, filepath.Join(site, "page.html"), `<link href="secret.css">`)
	mustWrite(t, filepath.Join(root, "outside.css"), ".a{}")
	if err := os.Symlink(filepath.Join(root, "outside.css"), filepath.Join(site, "secret.css")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	rels, skipped := collectRels(t, site, "page.html")
	if strings.Join(rels, ",") != "page.html" {
		t.Fatalf("symlinked escape must not be published: %v", rels)
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0], "outside the entry file's directory") {
		t.Fatalf("expected an out-of-root note, got %v", skipped)
	}
}

func TestCollectFileTerminatesOnReferenceCycle(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "page.html"), `<iframe src="b.html"></iframe>`)
	mustWrite(t, filepath.Join(root, "b.html"), `<iframe src="page.html"></iframe><link href="c.css">`)
	mustWrite(t, filepath.Join(root, "c.css"), `@import "c.css";`)

	rels, _ := collectRels(t, root, "page.html")
	if strings.Join(rels, ",") != "b.html,c.css,page.html" {
		t.Fatalf("got %v", rels)
	}
}

func TestCollectFileStopsAtFileLimit(t *testing.T) {
	root := t.TempDir()
	var refs strings.Builder
	for i := 0; i < maxDepFiles+10; i++ {
		name := filepath.Join(root, "a", strings.Repeat("x", 1)+string(rune('a'+i%26))+"-"+itoa(i)+".css")
		mustWrite(t, name, ".a{}")
		refs.WriteString(`<link href="a/` + filepath.Base(name) + `">`)
	}
	mustWrite(t, filepath.Join(root, "page.html"), refs.String())

	cands, _, skipped, err := CollectFile(permissiveFIO{}, filepath.Join(root, "page.html"))
	if err != nil {
		t.Fatalf("CollectFile: %v", err)
	}
	if len(cands) != maxDepFiles {
		t.Fatalf("expected the closure capped at %d, got %d", maxDepFiles, len(cands))
	}
	// One line, not one per dropped reference: a page that blows the cap blows
	// it by dozens, and repeating the same sentence buries everything else.
	if len(skipped) != 1 || !strings.Contains(skipped[0].Why, "200-file limit") ||
		skipped[0].Kind != SkipCapped {
		t.Fatalf("the cap must be reported exactly once, got %v", skipped)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b []byte
	for i > 0 {
		b = append([]byte{byte('0' + i%10)}, b...)
		i /= 10
	}
	return string(b)
}

func TestResolveRefClassification(t *testing.T) {
	cases := []struct {
		from, ref string
		want      string
		status    refStatus
	}{
		{"index.html", "style.css", "style.css", refOK},
		{"a/b.html", "../c.css", "c.css", refOK},
		{"a/b.html", "c.css", "a/c.css", refOK},
		{"index.html", "/deep/x.css", "deep/x.css", refOK},
		{"a/b.html", "../../escape.css", "", refOutside},
		{"index.html", "https://x/y.css", "", refIgnore},
		{"index.html", "//x/y.css", "", refIgnore},
		{"index.html", "data:text/css,a", "", refIgnore},
		{"index.html", "mailto:a@b.c", "", refIgnore},
		{"index.html", "#top", "", refIgnore},
		{"index.html", "assets/", "", refIgnore},
		{"index.html", "  ", "", refIgnore},
	}
	for _, c := range cases {
		got, status := resolveRef(c.from, c.ref)
		if got != c.want || status != c.status {
			t.Errorf("resolveRef(%q, %q) = (%q, %d), want (%q, %d)",
				c.from, c.ref, got, status, c.want, c.status)
		}
	}
}

// A dry-run that returns a green light and a real publish that fails is worse
// than either alone: the caller previews, sees success, and only finds out when
// the app is already being created. Validate must reject the collision, which
// means both input forms must run the manifest build.
func TestCollectFileRecordsWhoPulledEachDependencyIn(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "report.html"), `<iframe src="index.html"></iframe>`)
	mustWrite(t, filepath.Join(root, "index.html"), `<html></html>`)

	cands, _, _, err := CollectFile(permissiveFIO{}, filepath.Join(root, "report.html"))
	if err != nil {
		t.Fatalf("CollectFile: %v", err)
	}
	if len(cands) != 2 || cands[0].Via != "" || cands[1].Via != "report.html" {
		t.Fatalf("Via must name the referrer, got %+v", cands)
	}
	_, _, err = BuildManifest(cands, "report.html")
	if err == nil || !strings.Contains(err.Error(), "report.html references it") {
		t.Fatalf("the conflict must name who pulled the other index.html in, got %v", err)
	}
}
