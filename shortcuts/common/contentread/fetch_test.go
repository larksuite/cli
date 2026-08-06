// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentread

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func newFetchTestRuntime(t *testing.T) (*common.RuntimeContext, *httpmock.Registry) {
	t.Helper()
	cfg := &core.CliConfig{Brand: core.BrandFeishu, AppID: "cli_x"}
	f, _, _, reg := cmdutil.TestFactory(t, cfg)
	rt := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "+fetch"}, cfg, f, core.AsUser)
	return rt, reg
}

func TestFetchDocInfoDecodesContract(t *testing.T) {
	rt, reg := newFetchTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    Path,
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"title":        "Doc",
			"full_content": "# hi",
			"url":          "https://x",
			"update_time":  float64(123),
			"qa_image_meta_map": map[string]interface{}{
				"t1": map[string]interface{}{
					"image_key": "img_key_1",
					"caption":   "图",
				},
			},
		}},
	})

	resp, err := FetchDocInfo(context.Background(), rt, Request{URL: "https://doc"})
	if err != nil {
		t.Fatalf("FetchDocInfo: %v", err)
	}
	if resp.Title != "Doc" || resp.FullContent != "# hi" || resp.UpdateTime != 123 {
		t.Errorf("decode mismatch: %+v", resp)
	}
	if resp.URL != "https://x" {
		t.Errorf("url decode mismatch: %q", resp.URL)
	}
	m := resp.ImageMetaMap["t1"]
	if m == nil || m.ImageKey != "img_key_1" || m.Caption != "图" {
		t.Errorf("image meta decode mismatch: %+v", m)
	}
}

func TestFetchBlockIDRoundTrip(t *testing.T) {
	rt, reg := newFetchTestRuntime(t)
	const xml = `<h1 id="b1">hi</h1>`
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    Path,
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"title":        "Doc",
			"full_content": xml,
		}},
	}
	reg.Register(stub)

	req := NewRequest("https://doc")
	req.WithBlockID = true
	resp, err := FetchDocInfo(context.Background(), rt, req)
	if err != nil {
		t.Fatalf("FetchDocInfo: %v", err)
	}
	if !strings.Contains(string(stub.CapturedBody), `"with_block_id":true`) {
		t.Errorf("request body missing with_block_id: %s", stub.CapturedBody)
	}
	if resp.FullContent != xml {
		t.Errorf("FullContent decode mismatch: %q", resp.FullContent)
	}
}

func TestFetchPaginationRoundTrip(t *testing.T) {
	rt, reg := newFetchTestRuntime(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    Path,
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"title":           "Doc",
			"full_content":    "# page 1",
			"has_more":        true,
			"next_page_token": "tok-2",
		}},
	}
	reg.Register(stub)

	req := NewRequest("https://doc")
	req.EnablePagination = true
	req.PageToken = "tok-1"
	req.PageSize = 4000
	resp, err := FetchDocInfo(context.Background(), rt, req)
	if err != nil {
		t.Fatalf("FetchDocInfo: %v", err)
	}
	body := string(stub.CapturedBody)
	for _, want := range []string{`"enable_pagination":true`, `"page_token":"tok-1"`, `"page_size":4000`} {
		if !strings.Contains(body, want) {
			t.Errorf("request body missing %s: %s", want, body)
		}
	}
	if !resp.HasMore || resp.NextPageToken != "tok-2" {
		t.Errorf("pagination decode mismatch: HasMore=%v NextPageToken=%q", resp.HasMore, resp.NextPageToken)
	}
}

func TestFetchAnchoredMarkdownClassifiesEmptyContent(t *testing.T) {
	rt, reg := newFetchTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    Path,
		Body:   map[string]interface{}{"code": float64(0), "data": map[string]interface{}{}},
	})

	_, err := FetchAnchoredMarkdown(context.Background(), rt, "https://www.feishu.cn/doc/doccnLegacy", FetchOptions{})
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("error = %T %v, want internal/invalid_response", err, err)
	}
}

func TestNewRequestOmitsPagination(t *testing.T) {
	t.Parallel()
	req := NewRequest("https://doc")
	if req.EnablePagination || req.PageToken != "" || req.PageSize != 0 {
		t.Fatalf("NewRequest should leave pagination unset, got %+v", req)
	}
	raw, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, bad := range []string{"with_block_id", "enable_pagination", "page_token", "page_size"} {
		if strings.Contains(string(raw), bad) {
			t.Errorf("wire shape leaked %s: %s", bad, raw)
		}
	}
}

func TestFetchNonZeroCodePropagates(t *testing.T) {
	rt, reg := newFetchTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    Path,
		Body:   map[string]interface{}{"code": float64(1061044), "msg": "doc not found", "log_id": "lz"},
	})

	_, err := FetchDocInfo(context.Background(), rt, Request{URL: "https://doc"})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed errs.* error for non-zero code, got %T: %v", err, err)
	}
	if p.Code != 1061044 {
		t.Errorf("code = %d, want 1061044", p.Code)
	}
	if p.LogID != "lz" {
		t.Errorf("LogID = %q, want lz", p.LogID)
	}
}

func TestFetchHTTPErrorPropagates(t *testing.T) {
	rt, reg := newFetchTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     Path,
		Status:  500,
		RawBody: []byte(`{"error":"boom"}`),
	})

	if _, err := FetchDocInfo(context.Background(), rt, Request{URL: "x"}); err == nil {
		t.Fatal("expected error on HTTP 500")
	}
}

func TestFetchDocInfoRejectsMalformedDataAsInvalidResponse(t *testing.T) {
	rt, reg := newFetchTestRuntime(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    Path,
		Body: map[string]interface{}{"code": float64(0), "data": map[string]interface{}{
			"update_time": "not-an-integer",
		}},
	})

	_, err := FetchDocInfo(context.Background(), rt, Request{URL: "https://doc"})
	p, ok := errs.ProblemOf(err)
	if !ok || p.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("error = %T %v, want invalid_response", err, err)
	}
	if errors.Unwrap(err) == nil {
		t.Fatal("decode error must preserve its cause")
	}
}
