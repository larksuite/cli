// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestBuildManifestRenamesEntry(t *testing.T) {
	cands := []Candidate{
		{RelPath: "page.html", AbsPath: "/site/page.html", Size: 3},
		{RelPath: "assets/x.css", AbsPath: "/site/assets/x.css", Size: 4},
		{RelPath: "other.html", AbsPath: "/site/other.html", Size: 2},
	}
	entries, htmlRels, err := BuildManifest(cands, "page.html")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := map[string]string{}
	for _, e := range entries {
		got[e.ZipPath] = e.AbsPath
	}
	if got["output/index.html"] != "/site/page.html" {
		t.Errorf("entry not renamed to output/index.html: %v", got)
	}
	if got["output/assets/x.css"] != "/site/assets/x.css" {
		t.Errorf("non-entry file lost its relative path: %v", got)
	}
	if got["output/other.html"] != "/site/other.html" {
		t.Errorf("non-entry html must still be published: %v", got)
	}
	htmlRels = sortPathsUTF16(htmlRels)
	if len(htmlRels) != 2 || htmlRels[0] != "index.html" || htmlRels[1] != "other.html" {
		t.Errorf("htmlRels = %v, want [index.html other.html]", htmlRels)
	}
}

func TestBuildManifestMissingEntry(t *testing.T) {
	_, _, err := BuildManifest([]Candidate{{RelPath: "a.css"}}, "index.html")
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("got %v, want a missing-entry error", err)
	}
}

// A closure that pulls in a sibling index.html while the entry carries another
// name would put two files at output/index.html; which one survives unpacking
// is undefined, so the publish must stop instead.
func TestBuildManifestRejectsIndexCollision(t *testing.T) {
	_, _, err := BuildManifest([]Candidate{
		{RelPath: "page.html"},
		{RelPath: "index.html"},
	}, "page.html")
	ve := requireValidation(t, err, errs.SubtypeFailedPrecondition)
	if !strings.Contains(ve.Message, "entry conflict") {
		t.Errorf("message should name the conflict, got %q", ve.Message)
	}
}
