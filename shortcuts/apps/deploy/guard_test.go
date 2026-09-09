// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"strings"
	"testing"
)

func TestSensitiveNameCoverage(t *testing.T) {
	// 本包自有的凭证名单契约。+html-publish 长期要下线，
	// 这里不跟随它的后续变更。
	hit := []string{
		".env", ".env.local", ".env.production", ".env.html",
		".npmrc", "id_rsa", ".git-credentials",
	}
	for _, n := range hit {
		if !isSensitiveName(n) {
			t.Errorf("isSensitiveName(%q) = false, want true", n)
		}
	}
	miss := []string{"index.html", "environment.html", "readme.md", "env.js"}
	for _, n := range miss {
		if isSensitiveName(n) {
			t.Errorf("isSensitiveName(%q) = true, want false", n)
		}
	}
}

func TestGuardRejectsSensitiveUnlessWaived(t *testing.T) {
	cands := []Candidate{{RelPath: "index.html", Size: 10}, {RelPath: ".env", Size: 5}}
	if _, err := Guard(cands, false, DefaultLimits()); err == nil ||
		!strings.Contains(err.Error(), "credential file") {
		t.Fatalf("got %v, want a credential-file rejection", err)
	}
	waived, err := Guard(cands, true, DefaultLimits())
	if err != nil {
		t.Fatalf("--allow-sensitive should waive the scan: %v", err)
	}
	if len(waived) != 1 || waived[0] != ".env" {
		t.Errorf("waived = %v, want [.env]", waived)
	}
}

func TestGuardRejectsOversize(t *testing.T) {
	lim := Limits{SingleHTMLBytes: 20, RawTotalBytes: 100, ZipBytes: 50}
	if _, err := Guard([]Candidate{{RelPath: "a.html", Size: 25}}, false, lim); err == nil ||
		!strings.Contains(err.Error(), "per-file limit") {
		t.Fatalf("got %v, want a per-file size rejection", err)
	}
	big := []Candidate{{RelPath: "a.html", Size: 10}, {RelPath: "b.png", Size: 200}}
	if _, err := Guard(big, false, lim); err == nil ||
		!strings.Contains(err.Error(), "total size") {
		t.Fatalf("got %v, want a total-size rejection", err)
	}
}

func TestDefaultLimits(t *testing.T) {
	lim := DefaultLimits()
	if lim.SingleHTMLBytes != 20*1024*1024 {
		t.Errorf("SingleHTMLBytes = %d, want 20 MiB", lim.SingleHTMLBytes)
	}
	if lim.ZipBytes != 50*1024*1024 {
		t.Errorf("ZipBytes = %d, want 50 MiB", lim.ZipBytes)
	}
	if lim.RawTotalBytes != 200*1024*1024 {
		t.Errorf("RawTotalBytes = %d, want 200 MiB", lim.RawTotalBytes)
	}
}
