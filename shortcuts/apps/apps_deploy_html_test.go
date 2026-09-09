// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/shortcuts/common"
)

func TestIsHTMLDeployMode(t *testing.T) {
	if !isHTMLDeployMode("a.html", "") || !isHTMLDeployMode("", "./site") {
		t.Error("either flag should select html deploy mode")
	}
	if isHTMLDeployMode("", "") {
		t.Error("neither flag should keep the existing project mode")
	}
}

func TestValidateHTMLDeployFlags(t *testing.T) {
	cases := []struct {
		name                 string
		filePath, dir, entry string
		skipBuild, noVerify  bool
		wantErr              string
	}{
		{name: "两者同时给", filePath: "a.html", dir: "./site", wantErr: "mutually exclusive"},
		{name: "entry-file 无 dir", filePath: "a.html", entry: "p.html", wantErr: "--entry-file"},
		{name: "skip-build 误用", filePath: "a.html", skipBuild: true, wantErr: "--skip-build"},
		{name: "no-verify 误用", dir: "./site", noVerify: true, wantErr: "--no-verify"},
		{name: "file-path 非 html", filePath: "a.txt", wantErr: "--file-path"},
		{name: "entry-file 带路径分隔符", dir: "./site", entry: "sub/p.html", wantErr: "--entry-file"},
		{name: "合法单文件", filePath: "a.html"},
		{name: "合法目录带入口", dir: "./site", entry: "p.html"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateHTMLDeployFlags(tc.filePath, tc.dir, tc.entry, tc.skipBuild, tc.noVerify)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("got %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

// htmlDeployRuntime builds a RuntimeContext carrying only the bare-HTML flags,
// so validateHTMLDeploy can be exercised without the spark.json project setup.
func htmlDeployRuntime(t *testing.T, filePath, dir, entry string, allowSensitive bool) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "+deploy"}
	cmd.Flags().String("file-path", filePath, "")
	cmd.Flags().String("dir", dir, "")
	cmd.Flags().String("entry-file", entry, "")
	cmd.Flags().Bool("allow-sensitive", allowSensitive, "")
	cmd.Flags().Bool("skip-build", false, "")
	cmd.Flags().Bool("no-verify", false, "")
	return common.TestNewRuntimeContext(cmd, nil)
}

// chdirHTMLPayload writes files (relative names -> content) under a temp dir and
// chdirs into it, since the publish flags only accept cwd-relative paths.
func chdirHTMLPayload(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	return root
}

func TestValidateHTMLDeploy(t *testing.T) {
	t.Run("单文件通过", func(t *testing.T) {
		chdirHTMLPayload(t, map[string]string{"report.html": "<h1>hi</h1>"})
		if err := validateHTMLDeploy(htmlDeployRuntime(t, "report.html", "", "", false)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("目录默认入口通过", func(t *testing.T) {
		chdirHTMLPayload(t, map[string]string{
			"site/index.html":     "<h1>hi</h1>",
			"site/assets/app.css": "body{}",
		})
		if err := validateHTMLDeploy(htmlDeployRuntime(t, "", "site", "", false)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("目录缺入口报错", func(t *testing.T) {
		chdirHTMLPayload(t, map[string]string{"site/page.html": "<h1>hi</h1>"})
		err := validateHTMLDeploy(htmlDeployRuntime(t, "", "site", "", false))
		if err == nil || !strings.Contains(err.Error(), "no entry file") {
			t.Fatalf("got %v, want a missing-entry error", err)
		}
	})
	t.Run("凭证文件拦截", func(t *testing.T) {
		chdirHTMLPayload(t, map[string]string{
			"site/index.html": "<h1>hi</h1>",
			"site/.env":       "TOKEN=x",
		})
		err := validateHTMLDeploy(htmlDeployRuntime(t, "", "site", "", false))
		if err == nil || !strings.Contains(err.Error(), "credential file") {
			t.Fatalf("got %v, want the credential scan to reject the payload", err)
		}
	})
	t.Run("allow-sensitive 放行", func(t *testing.T) {
		chdirHTMLPayload(t, map[string]string{
			"site/index.html": "<h1>hi</h1>",
			"site/.env":       "TOKEN=x",
		})
		if err := validateHTMLDeploy(htmlDeployRuntime(t, "", "site", "", true)); err != nil {
			t.Fatalf("--allow-sensitive should waive the scan: %v", err)
		}
	})
	t.Run("flag 组合先于文件系统检查", func(t *testing.T) {
		chdirHTMLPayload(t, nil)
		err := validateHTMLDeploy(htmlDeployRuntime(t, "a.html", "site", "", false))
		if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Fatalf("got %v, want the flag conflict reported before any stat", err)
		}
	})
}
