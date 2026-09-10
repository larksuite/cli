// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build scopeexport

package auth

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/registry"
)

// escUnicode returns the literal 6-character text of a JSON \uXXXX escape
// (e.g. escUnicode(0x0026) == the text backslash-u-0-0-2-6), built at
// runtime so it survives copy/paste and doc rendering without ever
// appearing as a literal escape sequence in this source file.
func escUnicode(r rune) string {
	return fmt.Sprintf("\\u%04x", r)
}

// TestBuildBrandScopesDoc_ContractInvariants 锁契约形状,不锁具体 scope 值
// (线上"不发版得新 scope"使真实值本就漂移)。两 brand 各验一遍。
func TestBuildBrandScopesDoc_ContractInvariants(t *testing.T) {
	for _, brand := range []string{"feishu", "lark"} {
		t.Run(brand, func(t *testing.T) {
			doc, err := buildBrandScopesDoc(brand, "1.0.0-test")
			if err != nil {
				t.Fatalf("buildBrandScopesDoc(%q) error: %v", brand, err)
			}

			// #1 schema
			if doc.Version != "1.0.0-test" {
				t.Errorf("version: got %q, want %q", doc.Version, "1.0.0-test")
			}
			if len(doc.Scopes) == 0 {
				t.Fatal("scopes is empty")
			}

			for d, entry := range doc.Scopes {
				// #2 每 domain i18n_name 非空;两 slice 非 nil
				if entry.I18nName.ZhCn == "" || entry.I18nName.EnUs == "" {
					t.Errorf("domain %q: i18n_name has empty locale: %+v", d, entry.I18nName)
				}
				if entry.TenantScopes == nil || entry.UserScopes == nil {
					t.Errorf("domain %q: scope slice is nil (must be non-nil for []-marshal)", d)
				}
				// #3 空域跳过
				if len(entry.TenantScopes) == 0 && len(entry.UserScopes) == 0 {
					t.Errorf("domain %q: both scope lists empty — should have been skipped", d)
				}
				// #4 send_as_user 过滤
				for _, s := range append(append([]string{}, entry.TenantScopes...), entry.UserScopes...) {
					if s == "im:message.send_as_user" {
						t.Errorf("domain %q: send_as_user not filtered", d)
					}
				}
				// #5 排序去重
				assertSortedUnique(t, d, "tenant", entry.TenantScopes)
				assertSortedUnique(t, d, "user", entry.UserScopes)
				// #6 i18n 一致:已知域标题等于 registry getter
				if want := registry.GetServiceTitle(d, "zh"); want != "" && entry.I18nName.ZhCn != want {
					t.Errorf("domain %q: i18n_name.zh_cn=%q, want %q", d, entry.I18nName.ZhCn, want)
				}
			}

			// #7 确定性:同 brand 连续构造两次,经真实 marshalBrandScopesDoc 编码后逐字节相同
			// (直接比较 json.MarshalIndent 只验证了数据结构确定性,没验证实际交付的字节)
			doc2, _ := buildBrandScopesDoc(brand, "1.0.0-test")
			b1, err := marshalBrandScopesDoc(doc)
			if err != nil {
				t.Fatalf("marshal doc: %v", err)
			}
			b2, err := marshalBrandScopesDoc(doc2)
			if err != nil {
				t.Fatalf("marshal doc2: %v", err)
			}
			if string(b1) != string(b2) {
				t.Error("non-deterministic output across two builds")
			}

			// #8 序列化格式:2 空格缩进、尾 \n、空列表为 []、不转义 HTML 字符
			out, err := marshalBrandScopesDoc(doc)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			if !strings.HasSuffix(string(out), "}\n") {
				t.Error("output must end with newline")
			}
			if strings.Contains(string(out), ": null") {
				t.Error("empty list marshaled as null, want []")
			}
			// Go's json encoder, with SetEscapeHTML(true) (the default this fix
			// disables), rewrites '&', '<', '>' in string values as the
			// 6-character unicode-escape sequences below. Assert those
			// sequences are absent — a JSON.stringify-encoded publisher never
			// produces them, so their presence would break the byte-exact
			// drop-in contract. (This does not check for the literal
			// characters themselves: after the fix, a literal '&' etc. is the
			// CORRECT output.)
			for _, esc := range []string{escUnicode(0x0026), escUnicode(0x003c), escUnicode(0x003e)} {
				if strings.Contains(string(out), esc) {
					t.Errorf("output contains HTML-escaped sequence %s; a JSON.stringify publisher does not escape these", esc)
				}
			}
		})
	}
}

// #6 未知域回退:service_descriptions 里不存在的域,i18n_name 回退为域名本身。
func TestTitleOrDomain_UnknownFallsBackToDomain(t *testing.T) {
	const unknown = "definitely_not_a_real_service_domain_xyz"
	if got := titleOrDomain(unknown, "zh"); got != unknown {
		t.Errorf("unknown domain title: got %q, want %q", got, unknown)
	}
}

func TestNewCmdAuthExportScopes_InvalidBrand(t *testing.T) {
	_, err := buildBrandScopesDoc("nope", "1.0.0")
	if err == nil {
		t.Fatal("expected error for invalid brand, got nil")
	}
	if !strings.Contains(err.Error(), "must be feishu or lark") {
		t.Errorf("error message: got %q", err.Error())
	}
}

// TestNewCmdAuthExportScopes_RunE drives the cobra command's RunE directly,
// covering the IO paths (stdout, --output file, version-warning, invalid
// input) that have no other automated coverage — E2E does not apply to a
// scopeexport-build-tag-gated command. The factory argument is unused by
// RunE, so nil is fine here.
func TestNewCmdAuthExportScopes_RunE(t *testing.T) {
	t.Run("stdout_valid_json", func(t *testing.T) {
		cmd := newCmdAuthExportScopes(nil)
		var outBuf, errBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&errBuf)
		cmd.SetArgs([]string{"--brand", "feishu", "--version", "1.0.0-test"})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		out := outBuf.String()
		if !strings.Contains(out, `"version"`) || !strings.Contains(out, `"scopes"`) {
			t.Errorf("stdout does not look like the scopes doc: %s", out)
		}
	})

	t.Run("output_file", func(t *testing.T) {
		// validate.SafeOutputPath only allows cwd, /tmp, or ~/files; t.TempDir()
		// resolves under macOS's /var/folders/... and would be rejected, so use
		// a throwaway directory under the package's own working directory.
		tmpDir, err := os.MkdirTemp(".", "export-scopes-test-")
		if err != nil {
			t.Fatalf("MkdirTemp: %v", err)
		}
		t.Cleanup(func() { os.RemoveAll(tmpDir) })
		outPath := filepath.Join(tmpDir, "scopes.json")
		cmd := newCmdAuthExportScopes(nil)
		var outBuf, errBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&errBuf)
		cmd.SetArgs([]string{"--brand", "feishu", "--version", "1.0.0-test", "--output", outPath})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if outBuf.Len() != 0 {
			t.Errorf("stdout should be empty when --output is set, got: %q", outBuf.String())
		}
		fileBytes, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("reading output file: %v", err)
		}

		// Same args without --output must produce identical bytes on stdout.
		cmd2 := newCmdAuthExportScopes(nil)
		var stdoutOnly bytes.Buffer
		cmd2.SetOut(&stdoutOnly)
		cmd2.SetErr(&bytes.Buffer{})
		cmd2.SetArgs([]string{"--brand", "feishu", "--version", "1.0.0-test"})
		if err := cmd2.Execute(); err != nil {
			t.Fatalf("Execute (stdout compare): %v", err)
		}
		if string(fileBytes) != stdoutOnly.String() {
			t.Error("--output file bytes differ from stdout bytes for identical args")
		}
	})

	t.Run("dev_version_warning", func(t *testing.T) {
		cmd := newCmdAuthExportScopes(nil)
		var outBuf, errBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&errBuf)
		cmd.SetArgs([]string{"--brand", "feishu"}) // no --version: falls back to build.Version, DEV in `go test`
		if err := cmd.Execute(); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.Contains(errBuf.String(), `warning: exporting with version "DEV"`) {
			t.Errorf("stderr missing DEV warning: %q", errBuf.String())
		}
	})

	t.Run("invalid_output_path_rejected", func(t *testing.T) {
		cmd := newCmdAuthExportScopes(nil)
		var outBuf, errBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&errBuf)
		// /etc/passwd sits outside the allowlist (cwd/tmp/~) that
		// validate.SafeOutputPath enforces, so it must be rejected.
		cmd.SetArgs([]string{"--brand", "feishu", "--output", "/etc/passwd"})
		if err := cmd.Execute(); err == nil {
			t.Fatal("expected error for --output outside the allowed roots, got nil")
		}
	})

	t.Run("invalid_brand_errors", func(t *testing.T) {
		cmd := newCmdAuthExportScopes(nil)
		var outBuf, errBuf bytes.Buffer
		cmd.SetOut(&outBuf)
		cmd.SetErr(&errBuf)
		cmd.SetArgs([]string{"--brand", "xyz"})
		if err := cmd.Execute(); err == nil {
			t.Fatal("expected error for invalid --brand, got nil")
		}
	})
}

func assertSortedUnique(t *testing.T, domain, kind string, xs []string) {
	t.Helper()
	if !sort.StringsAreSorted(xs) {
		t.Errorf("domain %q %s scopes not sorted: %v", domain, kind, xs)
	}
	for i := 1; i < len(xs); i++ {
		if xs[i] == xs[i-1] {
			t.Errorf("domain %q %s scopes has duplicate %q", domain, kind, xs[i])
		}
	}
}
