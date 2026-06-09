// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

var driveMetaTestSeq atomic.Int64

func TestFetchDriveMetaTitle(t *testing.T) {
	t.Run("returns title from batch_query response", func(t *testing.T) {
		runtime, reg := newDriveMetaTestRuntime(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/drive/v1/metas/batch_query",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"metas": []map[string]interface{}{
						{"doc_token": "doxcnABC", "doc_type": "docx", "title": "My Document"},
					},
				},
			},
		})

		title, err := FetchDriveMetaTitle(runtime, "doxcnABC", "docx")
		if err != nil {
			t.Fatalf("FetchDriveMetaTitle() error: %v", err)
		}
		if title != "My Document" {
			t.Errorf("title = %q, want %q", title, "My Document")
		}
	})

	t.Run("returns empty string when metas is empty", func(t *testing.T) {
		runtime, reg := newDriveMetaTestRuntime(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/drive/v1/metas/batch_query",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"metas": []map[string]interface{}{},
				},
			},
		})

		title, err := FetchDriveMetaTitle(runtime, "doxcnABC", "docx")
		if err != nil {
			t.Fatalf("FetchDriveMetaTitle() error: %v", err)
		}
		if title != "" {
			t.Errorf("title = %q, want empty string", title)
		}
	})

	t.Run("returns empty string when meta has no title", func(t *testing.T) {
		runtime, reg := newDriveMetaTestRuntime(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/drive/v1/metas/batch_query",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"metas": []map[string]interface{}{
						{"doc_token": "doxcnABC", "doc_type": "docx"},
					},
				},
			},
		})

		title, err := FetchDriveMetaTitle(runtime, "doxcnABC", "docx")
		if err != nil {
			t.Fatalf("FetchDriveMetaTitle() error: %v", err)
		}
		if title != "" {
			t.Errorf("title = %q, want empty string", title)
		}
	})

	t.Run("propagates API error", func(t *testing.T) {
		runtime, reg := newDriveMetaTestRuntime(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/drive/v1/metas/batch_query",
			Body: map[string]interface{}{
				"code": 99991668,
				"msg":  "permission denied",
			},
		})

		_, err := FetchDriveMetaTitle(runtime, "doxcnABC", "docx")
		if err == nil {
			t.Fatal("FetchDriveMetaTitle() expected error, got nil")
		}
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("expected typed error, got %T", err)
		}
		if p.Code != 99991668 {
			t.Fatalf("code = %d, want 99991668", p.Code)
		}
	})
}

func TestFetchDriveMetaURL(t *testing.T) {
	runtime, reg := newDriveMetaTestRuntime(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/metas/batch_query",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"metas": []map[string]interface{}{
					{
						"doc_token": "boxcnABC",
						"doc_type":  "file",
						"title":     "report.pdf",
						"url":       "https://tenant.example.com/file/boxcnABC",
					},
				},
			},
		},
	}
	reg.Register(stub)

	got, err := FetchDriveMetaURL(runtime, "boxcnABC", "file")
	if err != nil {
		t.Fatalf("FetchDriveMetaURL() error: %v", err)
	}
	if got != "https://tenant.example.com/file/boxcnABC" {
		t.Fatalf("url = %q, want tenant URL", got)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("decode captured body: %v", err)
	}
	if body["with_url"] != true {
		t.Fatalf("with_url = %#v, want true", body["with_url"])
	}
}

func newDriveMetaTestRuntime(t *testing.T) (*RuntimeContext, *httpmock.Registry) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	cfg := &core.CliConfig{
		AppID: fmt.Sprintf("drive-meta-test-%d", driveMetaTestSeq.Add(1)), AppSecret: "test-secret", Brand: core.BrandFeishu,
	}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	runtime := &RuntimeContext{
		ctx:        context.Background(),
		Config:     cfg,
		Factory:    f,
		resolvedAs: core.AsBot,
	}
	return runtime, reg
}
