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
