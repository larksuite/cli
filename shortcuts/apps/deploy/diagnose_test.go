// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"path/filepath"
	"strings"
	"testing"
)

// --dir follows no references by design, so nothing else notices that a page
// points at a file outside the directory. Publishing that quietly is how a
// caller ends up with a live URL that renders unstyled and no reason why --
// and it is where every "use --dir instead" suggestion sends them.
func TestDiagnoseDirReportsReferencesItCannotSatisfy(t *testing.T) {
	root := t.TempDir()
	site := filepath.Join(root, "site")
	mustWrite(t, filepath.Join(site, "index.html"), `
<link rel="stylesheet" href="../shared/theme.css">
<link rel="stylesheet" href="local.css">
<img src="missing.png">
<img src="https://cdn.example.com/remote.png">
<a href="gone.html">nav</a>`)
	mustWrite(t, filepath.Join(site, "local.css"), ".a{}")
	mustWrite(t, filepath.Join(root, "shared", "theme.css"), ".b{}")

	cands, _, _, err := CollectDir(permissiveFIO{}, site)
	if err != nil {
		t.Fatalf("CollectDir: %v", err)
	}
	skips := DiagnoseDir(permissiveFIO{}, site, cands)

	var got []string
	for _, s := range skips {
		got = append(got, s.String())
	}
	joined := strings.Join(got, "\n")
	if len(skips) != 2 {
		t.Fatalf("expected exactly the two unsatisfiable references, got:\n%s", joined)
	}
	if !strings.Contains(joined, "../shared/theme.css") || !strings.Contains(joined, "missing.png") {
		t.Errorf("both the out-of-directory and the missing reference should be named:\n%s", joined)
	}
	// An external URL is meant to stay external, and a navigation link is not a
	// subresource; reporting either would train the caller to ignore the list.
	if strings.Contains(joined, "remote.png") || strings.Contains(joined, "gone.html") {
		t.Errorf("external and navigation references must not be reported:\n%s", joined)
	}
}

// The same bad reference has to be explained the same way whichever flag the
// caller used. Three acceptance rounds were lost to this drifting: --file-path
// was corrected each time and --dir kept the wording from the round before,
// which is how it came to advise moving /etc/passwd into a directory about to
// be published. Both sides now read their verdict from resolveReference, and
// this pins that.
func TestDirAndFilePathExplainTheSameReferenceIdentically(t *testing.T) {
	refs := map[string]string{
		"file scheme":   "file:///etc/hosts",
		"windows drive": `c:\boot.css`,
		"escapes root":  "../shared/theme.css",
		"bad percent":   "a%ZZb.css",
	}
	for name, ref := range refs {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			site := filepath.Join(root, "site")
			mustWrite(t, filepath.Join(site, "index.html"),
				`<link rel="stylesheet" href="`+ref+`">`)
			mustWrite(t, filepath.Join(root, "shared", "theme.css"), ".a{}")

			// --file-path refuses outright.
			_, _, _, ferr := CollectFile(permissiveFIO{}, filepath.Join(site, "index.html"))
			if ferr == nil {
				t.Fatalf("--file-path should refuse %q", ref)
			}
			wantWhy, wantAdvice := referenceProblem(ferr, ref, "index.html")

			// --dir publishes anyway, but has to say the same thing about it.
			cands, _, _, err := CollectDir(permissiveFIO{}, site)
			if err != nil {
				t.Fatalf("CollectDir: %v", err)
			}
			skips := DiagnoseDir(permissiveFIO{}, site, cands)
			if len(skips) != 1 {
				t.Fatalf("expected one report for %q, got %+v", ref, skips)
			}
			if skips[0].Why != wantWhy {
				t.Errorf("reason differs between modes\n --dir %q\n --file-path %q", skips[0].Why, wantWhy)
			}
			if skips[0].Advice != wantAdvice {
				t.Errorf("advice differs between modes\n --dir %q\n --file-path %q", skips[0].Advice, wantAdvice)
			}
			// Never tell anyone to move a system path into what they publish.
			if strings.Contains(skips[0].Advice, "move the file") && name != "escapes root" {
				t.Errorf("a malformed reference must not be answered with move-the-file: %q", skips[0].Advice)
			}
		})
	}
}
