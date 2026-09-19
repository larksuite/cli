// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package deploy

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/larksuite/cli/errs"
)

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestIsUnsafeRel(t *testing.T) {
	unsafe := []string{"/abs", "..", "../x", "a/../../b", "a/..", "a\x00b", `a\\b.html`}
	for _, in := range unsafe {
		if !isUnsafeRel(in) {
			t.Errorf("isUnsafeRel(%q) = false, want true", in)
		}
	}
	safe := []string{"index.html", "a/b.css", "archive.tar..bak"}
	for _, in := range safe {
		if isUnsafeRel(in) {
			t.Errorf("isUnsafeRel(%q) = true, want false", in)
		}
	}
}

func TestCollectDirSkipsGitAndNonRegular(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "index.html"), "hi")
	mustWrite(t, filepath.Join(root, "assets", "x.css"), "c")
	mustWrite(t, filepath.Join(root, ".git", "config"), "g")
	if err := os.Symlink(filepath.Join(root, "index.html"), filepath.Join(root, "link.html")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	cands, rootNames, _, err := collectDirAt(root)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := map[string]bool{}
	for _, c := range cands {
		got[c.RelPath] = true
	}
	if !got["index.html"] || !got["assets/x.css"] {
		t.Errorf("missing expected files: %v", got)
	}
	if got[".git/config"] {
		t.Error(".git subtree must be skipped")
	}
	if got["link.html"] {
		t.Error("symlinks must not be followed")
	}
	if len(rootNames) == 0 {
		t.Error("rootNames must list the directory's top-level file names")
	}
}

func TestCanonicalAbsResolvesSymlinks(t *testing.T) {
	real := t.TempDir()
	mustWrite(t, filepath.Join(real, "index.html"), "hi")

	linkDir := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(real, linkDir); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	viaReal, err := canonicalAbs(filepath.Join(real, "index.html"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	viaLink, err := canonicalAbs(filepath.Join(linkDir, "index.html"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 同一个文件经由符号链接与真实路径进入，必须得到同一个 key，
	// 否则会为同一份内容建出两个应用。
	if viaReal != viaLink {
		t.Errorf("canonicalAbs diverged: %q vs %q", viaReal, viaLink)
	}
}

func TestInputPathErrorOnlyClaimsOutOfBoundsWhenTrue(t *testing.T) {
	// 核心属性：只有输入本身确实越界时，才可以说「必须相对且在当前目录内」。
	// 对一个 ./ 开头、就在 cwd 内的路径说这句话，会把调用方引向无效的 cd 重试。
	const outOfBounds = "outside the current directory"

	cases := []struct {
		name           string
		path           string
		cause          error
		wantContains   string
		mustNotMention bool // 不得出现越界断言
	}{
		{"不存在", "./nope.html", fs.ErrNotExist, "does not exist", true},
		{"无权限", "./locked/page.html", fs.ErrPermission, "permission denied", true},
		{"路径拼接错(ENOTDIR)", "./page.html/deeper.html", syscall.ENOTDIR, "cannot be used", true},
		{"软链环(ELOOP)", "./loop.html", syscall.ELOOP, "cannot be used", true},
		{"绝对路径", "/etc/passwd", errors.New("resolves outside"), outOfBounds, false},
		{"向上穿透", "../outside.html", errors.New("resolves outside"), outOfBounds, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := inputPathError("--file-path", tc.path, tc.cause)
			ve := requireValidation(t, err, errs.SubtypeFailedPrecondition)
			if ve.Param != "--file-path" {
				t.Errorf("Param = %q, want --file-path", ve.Param)
			}
			// The wrapped failure has to stay reachable: it is what tells a
			// caller reading the envelope whether this was ENOENT or EACCES,
			// which the re-framed message deliberately no longer spells out.
			requireCause(t, err, tc.cause)
			if !strings.Contains(ve.Message, tc.wantContains) {
				t.Errorf("error = %q, want it to contain %q", ve.Message, tc.wantContains)
			}
			if tc.mustNotMention && strings.Contains(ve.Message, outOfBounds) {
				t.Errorf("a path inside the working directory must not be reported as out of bounds: %q", ve.Message)
			}
		})
	}
}

func TestEscapesWorkingDir(t *testing.T) {
	for _, p := range []string{"/abs/x.html", "../up.html", "..", "a/../../up.html"} {
		if !escapesWorkingDir(p) {
			t.Errorf("escapesWorkingDir(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"./x.html", "a/b.html", "a/../b.html", "."} {
		if escapesWorkingDir(p) {
			t.Errorf("escapesWorkingDir(%q) = true, want false", p)
		}
	}
}
