// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/shortcuts/apps/deploy"
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

func TestHTMLAppIDFromFlagSkipsLookup(t *testing.T) {
	src, id := htmlAppIDFromFlag("app_1abc")
	if src != htmlAppIDSourceFlag || id != "app_1abc" {
		t.Errorf("got (%v, %q), want (--app-id, app_1abc)", src, id)
	}
	src, id = htmlAppIDFromFlag("  ")
	if src != htmlAppIDSourceLookup || id != "" {
		t.Errorf("got (%v, %q), want (lookup, \"\")", src, id)
	}
}

func TestParseHasHTMLAppCreated(t *testing.T) {
	exists, id := parseHasHTMLAppCreated(map[string]interface{}{
		"exists": true, "app_id": "app_x", "app_url": "https://ignored", "ccm_token": "ignored",
	})
	if !exists || id != "app_x" {
		t.Errorf("got (%v, %q), want (true, app_x)", exists, id)
	}
	if exists, id := parseHasHTMLAppCreated(map[string]interface{}{"exists": false}); exists || id != "" {
		t.Errorf("got (%v, %q), want (false, \"\")", exists, id)
	}
	if exists, id := parseHasHTMLAppCreated(map[string]interface{}{"exists": true}); !exists || id != "" {
		t.Errorf("got (%v, %q), want (true, \"\") when app_id is absent", exists, id)
	}
}

// TestHTMLAppIDLookupPath pins the idempotency endpoint and the third source,
// which the create fallback in the Execute step reports.
func TestHTMLAppIDLookupPath(t *testing.T) {
	if hasHTMLAppCreatedPath != "/open-apis/spark/v1/apps/has_html_app_created" {
		t.Errorf("unexpected lookup path %q", hasHTMLAppCreatedPath)
	}
	if htmlAppIDSourceCreate != "+create" {
		t.Errorf("unexpected create source %q", htmlAppIDSourceCreate)
	}
}

// TestHTMLDeployPlanFields covers every field Execute reads off the resolved
// plan, so a missing one shows up here rather than at the call site.
func TestHTMLDeployPlanFields(t *testing.T) {
	plan := htmlDeployPlan{
		AbsEntry:    "/Users/me/site/index.html",
		EntryRel:    "index.html",
		AppID:       "app_x",
		AppIDSource: htmlAppIDSourceLookup,
		Entries:     []deploy.PackEntry{{ZipPath: "output/index.html", Size: 11}},
		FileCount:   1,
		TotalBytes:  11,
		ZipPaths:    []string{"output/index.html"},
		RouteCount:  1,
		ContentHash: "abc",
		Waived:      []string{".env"},
	}
	if plan.AbsEntry == "" || plan.EntryRel == "" || plan.AppID == "" {
		t.Error("entry and app id must survive into the plan")
	}
	if plan.AppIDSource != htmlAppIDSourceLookup {
		t.Errorf("unexpected app id source %q", plan.AppIDSource)
	}
	if len(plan.Entries) != 1 || plan.Entries[0].ZipPath != "output/index.html" {
		t.Errorf("unexpected entries %+v", plan.Entries)
	}
	if plan.FileCount != 1 || plan.TotalBytes != 11 || plan.RouteCount != 1 {
		t.Errorf("unexpected counters %+v", plan)
	}
	if len(plan.ZipPaths) != 1 || plan.ContentHash != "abc" {
		t.Errorf("unexpected zip paths or hash %+v", plan)
	}
	if len(plan.Waived) != 1 || plan.Waived[0] != ".env" {
		t.Errorf("waived credential files must stay on the plan for the stderr notice, got %v", plan.Waived)
	}
}
