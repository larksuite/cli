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

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
)

// permissiveFIO delegates to os without the cwd sandbox so the scanner can be
// driven with absolute t.TempDir paths. Production goes through the cwd-bounded
// LocalFileIO, which these tests deliberately do not exercise.
type permissiveFIO struct{}

func (permissiveFIO) Open(name string) (fileio.File, error)     { return os.Open(name) }
func (permissiveFIO) Stat(name string) (fileio.FileInfo, error) { return os.Stat(name) }
func (permissiveFIO) ResolvePath(p string) (string, error)      { return p, nil }
func (permissiveFIO) Save(string, fileio.SaveOptions, io.Reader) (fileio.SaveResult, error) {
	panic("Save not used in deploy unit tests")
}

// collectRels runs CollectFile on root/entry and returns the published paths
// plus the rendered skip notes.
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

// collectErr runs CollectFile expecting the publish to stop.
func collectErr(t *testing.T, root, entry string) error {
	t.Helper()
	cands, _, _, err := CollectFile(permissiveFIO{}, filepath.Join(root, entry))
	if err == nil {
		t.Fatalf("expected the publish to stop, got %d files", len(cands))
	}
	return err
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

// A reference the browser resolves against the document rather than against the
// script -- fetch, Worker, XHR -- must resolve from the payload root even when
// the script sits in a subdirectory. Getting this backwards puts a file at the
// wrong path, which is a file set the GUI cannot reproduce.
func TestCollectFileResolvesRuntimeReferencesAgainstTheDocument(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "index.html"), `<script src="js/app.js"></script>`)
	mustWrite(t, filepath.Join(root, "js", "app.js"), `
fetch('./data.json');
window.fetch('./ignored.json');
new Worker('./worker.js');
new URL('./sibling.js', import.meta.url);
`)
	mustWrite(t, filepath.Join(root, "data.json"), `{}`)
	mustWrite(t, filepath.Join(root, "worker.js"), ``)
	mustWrite(t, filepath.Join(root, "js", "sibling.js"), ``)
	// Same names one directory down: picked up only if the rewrite were skipped.
	mustWrite(t, filepath.Join(root, "js", "data.json"), `{}`)
	mustWrite(t, filepath.Join(root, "ignored.json"), `{}`)

	rels, _ := collectRels(t, root, "index.html")
	want := []string{"data.json", "index.html", "js/app.js", "js/sibling.js", "worker.js"}
	if strings.Join(rels, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", rels, want)
	}
}

func TestCollectFileIgnoresExternalAndInertReferences(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "page.html"), `
<link rel="stylesheet" href="https://cdn.example.com/a.css">
<link rel="stylesheet" href="//cdn.example.com/b.css">
<script src="http://cdn.example.com/c.js"></script>
<img src="data:image/png;base64,AAAA">
<a href="other.html">nav</a>
<a href="#section">anchor</a>
<img src="#">
<!-- <link rel="stylesheet" href="ghost.css"> -->
<form action="submit.php"></form>
<link rel="canonical" href="canonical.html">`)
	for _, f := range []string{"other.html", "ghost.css", "submit.php", "canonical.html"} {
		mustWrite(t, filepath.Join(root, f), "x")
	}

	rels, skipped := collectRels(t, root, "page.html")
	if strings.Join(rels, ",") != "page.html" {
		t.Fatalf("only the entry should be published, got %v", rels)
	}
	if len(skipped) != 0 {
		t.Fatalf("external and navigation references must not be reported as skips, got %v", skipped)
	}
}

func TestCollectFileStripsQueryAndFragment(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "page.html"), `
<link rel="stylesheet" href="style.css?v=2">
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
	mustWrite(t, filepath.Join(root, "page.html"), `<link rel="stylesheet" href="style.css"><img src="gone.png">`)
	mustWrite(t, filepath.Join(root, "style.css"), ".a{}")

	rels, skipped := collectRels(t, root, "page.html")
	if strings.Join(rels, ",") != "page.html,style.css" {
		t.Fatalf("a missing dependency must not drop the rest: %v", rels)
	}
	if len(skipped) != 1 || !strings.Contains(skipped[0], "gone.png") ||
		!strings.Contains(skipped[0], "does not exist") {
		t.Fatalf("skip note should name the file and the reason: %v", skipped)
	}
}

// The publish stops rather than shipping a payload the web client would have
// refused: a reference above the payload root cannot be expressed in the
// published layout at all.
func TestCollectFileRejectsReferenceAboveEntryDirectory(t *testing.T) {
	root := t.TempDir()
	site := filepath.Join(root, "site")
	mustWrite(t, filepath.Join(site, "page.html"), `<link rel="stylesheet" href="../shared/theme.css">`)
	mustWrite(t, filepath.Join(root, "shared", "theme.css"), ".a{}")

	ve := requireValidation(t, collectErr(t, site, "page.html"), errs.SubtypeFailedPrecondition)
	if !strings.Contains(ve.Message, "above the entry file") {
		t.Errorf("message should say the reference points above the payload: %v", ve.Message)
	}
	if ve.Hint != hintReferenceEscapes {
		t.Errorf("the way out must be the one written for this cause, got %q", ve.Hint)
	}
}

func TestCollectFileRejectsDangerousReferences(t *testing.T) {
	for name, ref := range map[string]string{
		"file scheme":   "file:///etc/hosts",
		"windows drive": `c:\secrets.css`,
		"colon decoded": "a%3Ab.css",
		"bad percent":   "a%ZZ.css",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			mustWrite(t, filepath.Join(root, "page.html"),
				`<link rel="stylesheet" href="`+ref+`">`)
			ve := requireValidation(t, collectErr(t, root, "page.html"), errs.SubtypeFailedPrecondition)
			if ve.Hint != hintFixReferenceText {
				t.Errorf("a malformed reference must be answered by fixing the text, got %q", ve.Hint)
			}
		})
	}
}

// A root-absolute reference is read as site-root-absolute, which for a
// single-file publish means the entry's own directory.
func TestCollectFileResolvesRootAbsoluteAgainstEntryDirectory(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "page.html"), `<link rel="stylesheet" href="/style.css">`)
	mustWrite(t, filepath.Join(root, "style.css"), ".a{}")

	rels, skipped := collectRels(t, root, "page.html")
	if strings.Join(rels, ",") != "page.html,style.css" {
		t.Fatalf("got %v", rels)
	}
	if len(skipped) != 0 {
		t.Fatalf("unexpected skips: %v", skipped)
	}
}

// Any symbolic link stops the publish, not only one pointing out of the
// payload. Following a link publishes a file the caller did not name, and the
// web client refuses them outright -- a payload accepted here has to be one the
// GUI can reproduce.
func TestCollectFileRejectsSymlink(t *testing.T) {
	for name, target := range map[string]string{
		"inside the payload":  "real.css",
		"outside the payload": "../outside.css",
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			site := filepath.Join(root, "site")
			mustWrite(t, filepath.Join(site, "page.html"), `<link rel="stylesheet" href="linked.css">`)
			mustWrite(t, filepath.Join(site, "real.css"), ".a{}")
			mustWrite(t, filepath.Join(root, "outside.css"), ".b{}")
			if err := os.Symlink(target, filepath.Join(site, "linked.css")); err != nil {
				t.Skipf("symlink unsupported: %v", err)
			}
			ve := requireValidation(t, collectErr(t, site, "page.html"), errs.SubtypeFailedPrecondition)
			if !strings.Contains(ve.Message, "symbolic link") {
				t.Errorf("message should name the symlink: %v", ve.Message)
			}
		})
	}
}

func TestCollectFileTerminatesOnReferenceCycle(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "page.html"), `<iframe src="b.html"></iframe>`)
	mustWrite(t, filepath.Join(root, "b.html"), `<iframe src="page.html"></iframe><link rel="stylesheet" href="c.css">`)
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
		name := "a/f" + itoa(i) + ".css"
		mustWrite(t, filepath.Join(root, filepath.FromSlash(name)), ".a{}")
		refs.WriteString(`<link rel="stylesheet" href="` + name + `">`)
	}
	mustWrite(t, filepath.Join(root, "page.html"), refs.String())

	ve := requireValidation(t, collectErr(t, root, "page.html"), errs.SubtypeFailedPrecondition)
	// The count is named so the caller can see how far past the limit they are;
	// the way out (--dir) rides on the hint, which the message does not carry.
	if !strings.Contains(ve.Message, "200-file limit") || !strings.Contains(ve.Message, "reach 211 files") {
		t.Errorf("hitting the cap should name the limit and the actual count: %v", ve.Message)
	}
	if !strings.Contains(ve.Hint, "--dir") {
		t.Errorf("the way out belongs in the hint, got %q", ve.Hint)
	}
}

// The depth limit is checked the way the file limit is: the chain is built at
// run time rather than committed, because none of its files carries an
// assertion of its own -- seventeen stylesheets exist only to be seventeen
// levels. Both implementations read the limit from the same written contract,
// so pinning it against a fixture would cost twenty files to confirm a constant.
func TestCollectFileStopsAtDepthLimit(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "page.html"), `<link rel="stylesheet" href="c0.css">`)
	for i := 0; i <= maxDepDepth+1; i++ {
		mustWrite(t, filepath.Join(root, "c"+itoa(i)+".css"), `@import "c`+itoa(i+1)+`.css";`)
	}
	mustWrite(t, filepath.Join(root, "c"+itoa(maxDepDepth+2)+".css"), ".end{}")

	ve := requireValidation(t, collectErr(t, root, "page.html"), errs.SubtypeFailedPrecondition)
	// The message names the file and the reference that overflowed, so the
	// caller can cut the chain instead of guessing where it runs deep.
	if !strings.Contains(ve.Message, "nest more than 16 levels deep") ||
		!strings.Contains(ve.Message, "references") {
		t.Errorf("exceeding the depth limit should name the reference that did it: %v", ve.Message)
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

// A dry-run that returns a green light and a real publish that fails is worse
// than either alone, so the collision has to be detectable before any write.
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
	ve := requireValidation(t, err, errs.SubtypeFailedPrecondition)
	if !strings.Contains(ve.Message, "report.html references it") {
		t.Errorf("the conflict must name who pulled the other index.html in, got %q", ve.Message)
	}
}

func TestResolveReferenceClassification(t *testing.T) {
	type want struct {
		rel  string
		skip bool
		err  bool
	}
	cases := map[string]struct {
		from, ref string
		want      want
	}{
		"sibling":        {"index.html", "style.css", want{rel: "style.css"}},
		"up one":         {"a/b.html", "../c.css", want{rel: "c.css"}},
		"same dir":       {"a/b.html", "c.css", want{rel: "a/c.css"}},
		"root absolute":  {"index.html", "/deep/x.css", want{rel: "deep/x.css"}},
		"bare specifier": {"js/app.js", "lodash", want{rel: "js/lodash"}},
		"above root":     {"a/b.html", "../../escape.css", want{err: true}},
		"backslash":      {"index.html", `a\b.css`, want{err: true}},
		"file scheme":    {"index.html", "file:///x", want{err: true}},
		"windows drive":  {"index.html", `C:\x`, want{err: true}},
		"decoded colon":  {"index.html", "a%3Ab.css", want{err: true}},
		"https":          {"index.html", "https://x/y.css", want{skip: true}},
		"protocol rel":   {"index.html", "//x/y.css", want{skip: true}},
		"data uri":       {"index.html", "data:text/css,a", want{skip: true}},
		"mailto":         {"index.html", "mailto:a@b.c", want{skip: true}},
		"fragment":       {"index.html", "#top", want{skip: true}},
		"query only":     {"index.html", "?v=1", want{skip: true}},
		"blank":          {"index.html", "  ", want{skip: true}},
		"trailing slash": {"index.html", "assets/", want{rel: "assets"}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			rel, skip, err := resolveReference(c.from, c.ref)
			switch {
			case c.want.err:
				requireValidation(t, err, errs.SubtypeFailedPrecondition)
			case c.want.skip:
				if err != nil || !skip {
					t.Fatalf("expected a skip, got rel=%q skip=%v err=%v", rel, skip, err)
				}
			default:
				if err != nil || skip || rel != c.want.rel {
					t.Fatalf("got rel=%q skip=%v err=%v, want %q", rel, skip, err, c.want.rel)
				}
			}
		})
	}
}
