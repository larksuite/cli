// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestFetchTenantResourceURL_ReturnsMetaURL(t *testing.T) {
	t.Parallel()

	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test", AppSecret: "secret", Brand: core.BrandFeishu})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"metas": []interface{}{
					map[string]interface{}{
						"doc_token": "doxcnABC",
						"doc_type":  "docx",
						"url":       "https://tenant.feishu.cn/docx/doxcnABC",
					},
				},
			},
		},
	})

	rt := newTestRuntimeForTenantURL(t, f)
	got := FetchTenantResourceURL(rt, "docx", "doxcnABC")
	want := "https://tenant.feishu.cn/docx/doxcnABC"
	if got != want {
		t.Fatalf("FetchTenantResourceURL = %q, want %q", got, want)
	}
}

func TestFetchTenantResourceURL_FallsBackToBuildResourceURL(t *testing.T) {
	t.Parallel()

	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test", AppSecret: "secret", Brand: core.BrandFeishu})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"metas": []interface{}{},
			},
		},
	})

	rt := newTestRuntimeForTenantURL(t, f)
	got := FetchTenantResourceURL(rt, "docx", "doxcnABC")
	want := "https://www.feishu.cn/docx/doxcnABC"
	if got != want {
		t.Fatalf("FetchTenantResourceURL = %q, want %q", got, want)
	}
}

func TestFetchTenantResourceURL_FallsBackWhenMetaAPIFails(t *testing.T) {
	t.Parallel()

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test", AppSecret: "secret", Brand: core.BrandLark})

	rt := newTestRuntimeForTenantURL(t, f)
	got := FetchTenantResourceURL(rt, "sheet", "shtcnABC")
	want := "https://www.larksuite.com/sheets/shtcnABC"
	if got != want {
		t.Fatalf("FetchTenantResourceURL = %q, want %q", got, want)
	}
}

func TestFetchTenantResourceURL_EmptyToken(t *testing.T) {
	t.Parallel()

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test", AppSecret: "secret", Brand: core.BrandFeishu})

	rt := newTestRuntimeForTenantURL(t, f)
	if got := FetchTenantResourceURL(rt, "docx", ""); got != "" {
		t.Fatalf("FetchTenantResourceURL(empty token) = %q, want empty", got)
	}
}

func TestFetchTenantResourceURL_UnknownKind(t *testing.T) {
	t.Parallel()

	f, _, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test", AppSecret: "secret", Brand: core.BrandFeishu})

	rt := newTestRuntimeForTenantURL(t, f)
	if got := FetchTenantResourceURL(rt, "calendar", "calABC"); got != "" {
		t.Fatalf("FetchTenantResourceURL(unknown kind) = %q, want empty", got)
	}
}

func TestFetchTenantResourceURL_TrimsWhitespaceURL(t *testing.T) {
	t.Parallel()

	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test", AppSecret: "secret", Brand: core.BrandFeishu})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"metas": []interface{}{
					map[string]interface{}{
						"doc_token": "doxcnABC",
						"doc_type":  "docx",
						"url":       "  https://tenant.feishu.cn/docx/doxcnABC  ",
					},
				},
			},
		},
	})

	rt := newTestRuntimeForTenantURL(t, f)
	got := FetchTenantResourceURL(rt, "docx", "doxcnABC")
	want := "https://tenant.feishu.cn/docx/doxcnABC"
	if got != want {
		t.Fatalf("FetchTenantResourceURL = %q, want %q", got, want)
	}
}

func TestFetchTenantResourceURL_FallsBackWhenMetaURLIsWhitespace(t *testing.T) {
	t.Parallel()

	f, _, _, reg := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test", AppSecret: "secret", Brand: core.BrandFeishu})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"metas": []interface{}{
					map[string]interface{}{
						"doc_token": "doxcnABC",
						"doc_type":  "docx",
						"url":       "   ",
					},
				},
			},
		},
	})

	rt := newTestRuntimeForTenantURL(t, f)
	got := FetchTenantResourceURL(rt, "docx", "doxcnABC")
	want := "https://www.feishu.cn/docx/doxcnABC"
	if got != want {
		t.Fatalf("FetchTenantResourceURL = %q, want %q", got, want)
	}
}

func TestKindToDocType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		kind string
		want string
	}{
		{"docx", "docx"},
		{"doc", "docx"},
		{"sheet", "sheet"},
		{"bitable", "bitable"},
		{"wiki", "wiki"},
		{"file", "file"},
		{"folder", "folder"},
		{"mindnote", "mindnote"},
		{"slides", "slides"},
		{"DOCX", "docx"},
		{"  sheet  ", "sheet"},
		{"calendar", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got := kindToDocType(tt.kind)
			if got != tt.want {
				t.Fatalf("kindToDocType(%q) = %q, want %q", tt.kind, got, tt.want)
			}
		})
	}
}

// newTestRuntimeForTenantURL creates a RuntimeContext backed by a test factory
// with a context that carries shortcut headers so CallAPI works.
func newTestRuntimeForTenantURL(t *testing.T, f *cmdutil.Factory) *RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(context.Background())
	cfg, _ := f.Config()
	return TestNewRuntimeContextForAPI(context.Background(), cmd, cfg, f, core.AsBot)
}
