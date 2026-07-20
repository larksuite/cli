// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

type serviceFailOnWriteWriter struct {
	buf    bytes.Buffer
	writes int
	failAt int
	err    error
}

func (w *serviceFailOnWriteWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failAt {
		return 0, w.err
	}
	return w.buf.Write(p)
}

func newServicePaginateTestHarness(t *testing.T) (*client.APIClient, *bytes.Buffer, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	previousNotice := output.PendingNotice
	output.PendingNotice = nil
	t.Cleanup(func() { output.PendingNotice = previousNotice })

	config := &core.CliConfig{
		AppID:     "test-app",
		AppSecret: "test-secret",
		Brand:     core.BrandFeishu,
	}
	f, out, errOut, reg := cmdutil.TestFactory(t, config)
	ac, err := f.NewAPIClientWithConfig(config)
	if err != nil {
		t.Fatalf("NewAPIClientWithConfig() error = %v", err)
	}
	ac.ErrOut = io.Discard
	return ac, out, errOut, reg
}

func servicePaginateRequest() client.RawApiRequest {
	return client.RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/test/v1/items",
		As:     core.AsBot,
	}
}

func assertServicePaginateJSONBytes(t *testing.T, got []byte, want interface{}) {
	t.Helper()
	wantBytes, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatalf("marshal expected JSON: %v", err)
	}
	wantBytes = append(wantBytes, '\n')
	if !bytes.Equal(got, wantBytes) {
		t.Fatalf("stdout bytes mismatch\ngot:\n%s\nwant:\n%s", got, wantBytes)
	}
}

func TestServicePaginate_DefaultAggregatesAllPages(t *testing.T) {
	ac, out, errOut, reg := newServicePaginateTestHarness(t)
	calls := 0
	wantTokens := []string{"", "next-1", "next-2"}
	for i, wantToken := range wantTokens {
		page := i + 1
		hasMore := page < len(wantTokens)
		data := map[string]interface{}{
			"items":    []interface{}{map[string]interface{}{"id": string(rune('0' + page))}},
			"has_more": hasMore,
		}
		if hasMore {
			data["page_token"] = wantTokens[page]
		}
		reg.Register(&httpmock.Stub{
			URL: "/open-apis/test/v1/items",
			OnMatch: func(req *http.Request) {
				calls++
				if got := req.URL.Query().Get("page_token"); got != wantToken {
					t.Errorf("request %d page_token = %q, want %q", page, got, wantToken)
				}
			},
			Body: map[string]interface{}{
				"code": 0,
				"msg":  "ok",
				"data": data,
			},
		})
	}

	err := servicePaginate(context.Background(), ac, servicePaginateRequest(),
		output.FormatJSON, "", out, errOut, "lark-cli test items list", client.PaginationOptions{
			PageLimit: 10,
			PageDelay: -1,
		}, ac.CheckResponse)

	if err != nil {
		t.Fatalf("servicePaginate() error = %v, want nil", err)
	}
	if calls != 3 {
		t.Fatalf("pagination requests = %d, want 3", calls)
	}
	assertServicePaginateJSONBytes(t, out.Bytes(), output.Envelope{
		OK:       true,
		Identity: "bot",
		Data: map[string]interface{}{
			"items": []interface{}{
				map[string]interface{}{"id": "1"},
				map[string]interface{}{"id": "2"},
				map[string]interface{}{"id": "3"},
			},
			"has_more": false,
		},
	})
	if got := errOut.String(); got != "" {
		t.Fatalf("stderr bytes = %q, want empty", got)
	}
}

func TestServicePaginate_StreamingFormatsEmitExactMultiPageBytes(t *testing.T) {
	tests := []struct {
		name   string
		format output.Format
		want   string
	}{
		{
			name:   "ndjson",
			format: output.FormatNDJSON,
			want:   "{\"id\":\"1\",\"name\":\"Alice\"}\n{\"id\":\"2\",\"name\":\"Carol\",\"page_only\":\"ignored\"}\n",
		},
		{
			name:   "table",
			format: output.FormatTable,
			want:   "id  name \n──  ─────\n1   Alice\n2   Carol\n",
		},
		{
			name:   "csv",
			format: output.FormatCSV,
			want:   "id,name\n1,Alice\n2,Carol\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ac, out, errOut, reg := newServicePaginateTestHarness(t)
			reg.Register(&httpmock.Stub{
				URL: "/open-apis/test/v1/items",
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "ok",
					"data": map[string]interface{}{
						"items": []interface{}{
							map[string]interface{}{"id": "1", "name": "Alice"},
						},
						"has_more":   true,
						"page_token": "next-1",
					},
				},
			})
			reg.Register(&httpmock.Stub{
				URL: "/open-apis/test/v1/items",
				Body: map[string]interface{}{
					"code": 0,
					"msg":  "ok",
					"data": map[string]interface{}{
						"items": []interface{}{
							map[string]interface{}{"id": "2", "name": "Carol", "page_only": "ignored"},
						},
						"has_more": false,
					},
				},
			})

			err := servicePaginate(context.Background(), ac, servicePaginateRequest(),
				tt.format, "", out, errOut, "lark-cli test items list", client.PaginationOptions{
					PageLimit: 10,
					PageDelay: -1,
				}, ac.CheckResponse)

			if err != nil {
				t.Fatalf("servicePaginate() error = %v, want nil", err)
			}
			if got := out.String(); got != tt.want {
				t.Fatalf("stdout byte mismatch\ngot (%d bytes):\n%q\nwant (%d bytes):\n%q", len(got), got, len(tt.want), tt.want)
			}
			if got := errOut.String(); got != "" {
				t.Fatalf("stderr bytes = %q, want empty", got)
			}
		})
	}
}

func TestServicePaginate_StreamingWriteFailureStopsFurtherPages(t *testing.T) {
	ac, _, errOut, reg := newServicePaginateTestHarness(t)
	sentinel := errors.New("page write failed")
	out := &serviceFailOnWriteWriter{failAt: 2, err: sentinel}
	calls := 0
	for page := 1; page <= 2; page++ {
		hasMore := true
		data := map[string]interface{}{
			"items":    []interface{}{map[string]interface{}{"id": page}},
			"has_more": hasMore,
		}
		if hasMore {
			data["page_token"] = fmt.Sprintf("next-%d", page)
		}
		reg.Register(&httpmock.Stub{
			URL: "/open-apis/test/v1/items",
			OnMatch: func(*http.Request) {
				calls++
			},
			Body: map[string]interface{}{
				"code": 0,
				"msg":  "ok",
				"data": data,
			},
		})
	}

	err := servicePaginate(context.Background(), ac, servicePaginateRequest(),
		output.FormatNDJSON, "", out, errOut, "lark-cli test items list",
		client.PaginationOptions{PageLimit: 10, PageDelay: -1}, ac.CheckResponse)

	if !errors.Is(err, sentinel) {
		t.Fatalf("servicePaginate() error = %v, want preserved writer cause", err)
	}
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal {
		t.Fatalf("servicePaginate() problem = %#v, %v; want internal typed error", problem, ok)
	}
	if calls != 2 {
		t.Fatalf("pagination requests = %d, want 2", calls)
	}
	if got, want := out.buf.String(), "{\"id\":1}\n"; got != want {
		t.Fatalf("stdout bytes = %q, want %q", got, want)
	}
}

func TestServicePaginate_StreamingFormatFallsBackToJSONWithoutList(t *testing.T) {
	ac, out, errOut, reg := newServicePaginateTestHarness(t)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/test/v1/items",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"name":    "Test User",
				"user_id": "u123",
			},
		},
	})

	err := servicePaginate(context.Background(), ac, servicePaginateRequest(),
		output.FormatNDJSON, "", out, errOut, "lark-cli test items get",
		client.PaginationOptions{PageDelay: -1}, ac.CheckResponse)

	if err != nil {
		t.Fatalf("servicePaginate() error = %v, want nil", err)
	}
	assertServicePaginateJSONBytes(t, out.Bytes(), output.Envelope{
		OK:       true,
		Identity: "bot",
		Data: map[string]interface{}{
			"name":    "Test User",
			"user_id": "u123",
		},
	})
	wantWarning := "warning: this API does not return a list, format \"ndjson\" is not supported, falling back to json\n"
	if got := errOut.String(); got != wantWarning {
		t.Fatalf("stderr bytes = %q, want %q", got, wantWarning)
	}
}

func TestServicePaginate_BusinessErrorsWriteRawAndRemainUnmarked(t *testing.T) {
	businessResponse := map[string]interface{}{
		"code": 123456,
		"msg":  "fixture business error",
		"data": map[string]interface{}{"detail": "business failed"},
	}
	tests := []struct {
		name   string
		format output.Format
		jqExpr string
	}{
		{name: "jq", format: output.FormatJSON, jqExpr: ".data.items"},
		{name: "default_json", format: output.FormatJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ac, out, errOut, reg := newServicePaginateTestHarness(t)
			reg.Register(&httpmock.Stub{
				URL:  "/open-apis/test/v1/items",
				Body: businessResponse,
			})

			err := servicePaginate(context.Background(), ac, servicePaginateRequest(),
				tt.format, tt.jqExpr, out, errOut, "lark-cli test items list",
				client.PaginationOptions{PageDelay: -1}, ac.CheckResponse)

			if err == nil {
				t.Fatal("servicePaginate() error = nil, want business error")
			}
			if errs.IsRaw(err) {
				t.Fatalf("errs.IsRaw(error) = true, want current servicePaginate pass-through behavior")
			}
			assertServicePaginateJSONBytes(t, out.Bytes(), businessResponse)
			if bytes.Contains(out.Bytes(), []byte(`"ok": true`)) {
				t.Fatalf("business-error stdout contains a success envelope:\n%s", out.Bytes())
			}
			if got := errOut.String(); got != "" {
				t.Fatalf("stderr bytes = %q, want empty", got)
			}
		})
	}
}

func TestServicePaginate_TransportErrorsRemainUnmarked(t *testing.T) {
	tests := []struct {
		name   string
		format output.Format
		jqExpr string
	}{
		{name: "jq_paginate_all", format: output.FormatJSON, jqExpr: ".data.items"},
		{name: "stream_pages", format: output.FormatNDJSON},
		{name: "default_paginate_all", format: output.FormatJSON},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ac, out, errOut, _ := newServicePaginateTestHarness(t)

			err := servicePaginate(context.Background(), ac, servicePaginateRequest(),
				tt.format, tt.jqExpr, out, errOut, "lark-cli test items list",
				client.PaginationOptions{PageDelay: -1}, ac.CheckResponse)

			if err == nil {
				t.Fatal("servicePaginate() error = nil, want transport error")
			}
			if errs.IsRaw(err) {
				t.Fatalf("errs.IsRaw(error) = true, want current servicePaginate pass-through behavior")
			}
			if got := out.String(); got != "" {
				t.Fatalf("stdout bytes = %q, want empty", got)
			}
			if got := errOut.String(); got != "" {
				t.Fatalf("stderr bytes = %q, want empty", got)
			}
		})
	}
}

func TestServicePaginate_StreamBusinessErrorRemainsUnmarked(t *testing.T) {
	ac, out, errOut, reg := newServicePaginateTestHarness(t)
	reg.Register(&httpmock.Stub{
		URL: "/open-apis/test/v1/items",
		Body: map[string]interface{}{
			"code": 123456,
			"msg":  "fixture business error",
			"data": map[string]interface{}{},
		},
	})

	err := servicePaginate(context.Background(), ac, servicePaginateRequest(),
		output.FormatNDJSON, "", out, errOut, "lark-cli test items list",
		client.PaginationOptions{PageDelay: -1}, ac.CheckResponse)

	if err == nil {
		t.Fatal("servicePaginate() error = nil, want business error")
	}
	if errs.IsRaw(err) {
		t.Fatalf("errs.IsRaw(error) = true, want current servicePaginate pass-through behavior")
	}
	if got := out.String(); got != "" {
		t.Fatalf("stdout bytes = %q, want empty", got)
	}
	if got := errOut.String(); got != "" {
		t.Fatalf("stderr bytes = %q, want empty", got)
	}
}
