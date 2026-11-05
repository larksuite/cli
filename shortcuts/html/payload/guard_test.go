// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package payload

import (
	"strings"
	"testing"
)

func TestSensitiveNameCoverage(t *testing.T) {
	// HTML publication owns its credential-file rules independently.
	hit := []string{
		".env", ".env.local", ".env.production", ".env.html",
		".npmrc", "id_rsa", ".git-credentials",
		// 大小写不敏感：macOS/Windows 文件系统同名同文件，不能绕过扫描。
		".ENV", ".Env.Local", "ID_RSA", "key.PEM", "Credentials",
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
	lim := Limits{SingleHTMLBytes: 20, RawTotalBytes: 100}
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
	if lim.SingleHTMLBytes != 20_000_000 || lim.RawTotalBytes != 20_000_000 {
		t.Fatalf("HTML limits must be 20,000,000 bytes: %+v", lim)
	}
}

func TestIsSensitiveRelCoversParentAnchoredPairs(t *testing.T) {
	// 这些文件的 basename 太通用（config.json / config），只看叶子名必然漏过，
	// 必须按父目录锚定。前两项含 registry auth token 与集群证书。
	hit := []string{
		".docker/config.json", ".kube/config", ".aws/credentials", ".aws/config",
		"nested/.docker/config.json", ".DOCKER/CONFIG.JSON",
		".ssh/known_hosts", ".ssh/id_rsa", ".gnupg/secring.gpg",
		// .aws 整目录纳管：sso 缓存的 token 藏在多层子目录里，
		// 只锚定 .aws/credentials 会漏掉 .aws/sso/cache/*.json。
		".aws/sso/cache/abc.json", ".aws/cli/cache/x.json",
		"assets/.env",
	}
	for _, rel := range hit {
		if !isSensitiveRel(rel) {
			t.Errorf("isSensitiveRel(%q) = false, want true", rel)
		}
	}
	miss := []string{
		"config.json", "config", "assets/config.json",
		"docker/config.json", "index.html", "docs/kube/config.md",
	}
	for _, rel := range miss {
		if isSensitiveRel(rel) {
			t.Errorf("isSensitiveRel(%q) = true, want false", rel)
		}
	}
}

func TestGuardBlocksParentAnchoredCredentials(t *testing.T) {
	cands := []Candidate{
		{RelPath: "index.html", Size: 10},
		{RelPath: ".docker/config.json", Size: 5},
	}
	if _, err := Guard(cands, false, DefaultLimits()); err == nil ||
		!strings.Contains(err.Error(), ".docker/config.json") {
		t.Fatalf("got %v, want the payload rejected naming .docker/config.json", err)
	}
}
