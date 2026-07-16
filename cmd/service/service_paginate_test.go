// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
)

type servicePaginateRoundTripFunc func(*http.Request) (*http.Response, error)

func (f servicePaginateRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type servicePaginateTokenResolver struct{}

func (*servicePaginateTokenResolver) ResolveToken(context.Context, credential.TokenSpec) (*credential.TokenResult, error) {
	return &credential.TokenResult{Token: "test-token"}, nil
}

func newServicePaginateTestClient(t *testing.T, rt http.RoundTripper) *client.APIClient {
	t.Helper()
	sdk := lark.NewClient("test-app", "test-secret",
		lark.WithEnableTokenCache(false),
		lark.WithLogLevel(larkcore.LogLevelError),
		lark.WithHttpClient(&http.Client{Transport: rt}),
	)
	return &client.APIClient{
		SDK:        sdk,
		ErrOut:     &bytes.Buffer{},
		Credential: credential.NewCredentialProvider(nil, nil, &servicePaginateTokenResolver{}, nil),
		Config: &core.CliConfig{
			AppID:     "test-app",
			AppSecret: "test-secret",
			Brand:     core.BrandFeishu,
		},
	}
}

func servicePaginateJSONResponse(body interface{}) *http.Response {
	b, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(b)),
	}
}

func servicePaginateRequest() client.RawApiRequest {
	return client.RawApiRequest{
		Method: "GET",
		URL:    "/open-apis/test/v1/items",
		As:     core.AsBot,
	}
}

func prepareServicePaginateOutput(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONTENT_SAFETY_MODE", "off")
	previousNotice := output.PendingNotice
	output.PendingNotice = nil
	t.Cleanup(func() { output.PendingNotice = previousNotice })
	return &bytes.Buffer{}, &bytes.Buffer{}
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
	calls := 0
	rt := servicePaginateRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		wantTokens := []string{"", "next-1", "next-2"}
		if calls >= len(wantTokens) {
			t.Fatalf("unexpected pagination request %d: %s", calls+1, req.URL.String())
		}
		if got := req.URL.Query().Get("page_token"); got != wantTokens[calls] {
			t.Errorf("request %d page_token = %q, want %q", calls+1, got, wantTokens[calls])
		}
		calls++
		hasMore := calls < len(wantTokens)
		data := map[string]interface{}{
			"items":    []interface{}{map[string]interface{}{"id": string(rune('0' + calls))}},
			"has_more": hasMore,
		}
		if hasMore {
			data["page_token"] = wantTokens[calls]
		}
		return servicePaginateJSONResponse(map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": data,
		}), nil
	})
	ac := newServicePaginateTestClient(t, rt)
	out, errOut := prepareServicePaginateOutput(t)

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

func TestServicePaginate_StreamingFormatFallsBackToJSONWithoutList(t *testing.T) {
	rt := servicePaginateRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return servicePaginateJSONResponse(map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"name":    "Test User",
				"user_id": "u123",
			},
		}), nil
	})
	ac := newServicePaginateTestClient(t, rt)
	out, errOut := prepareServicePaginateOutput(t)

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
			rt := servicePaginateRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return servicePaginateJSONResponse(businessResponse), nil
			})
			ac := newServicePaginateTestClient(t, rt)
			out, errOut := prepareServicePaginateOutput(t)

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
			rt := servicePaginateRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, io.ErrUnexpectedEOF
			})
			ac := newServicePaginateTestClient(t, rt)
			out, errOut := prepareServicePaginateOutput(t)

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
	rt := servicePaginateRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return servicePaginateJSONResponse(map[string]interface{}{
			"code": 123456,
			"msg":  "fixture business error",
			"data": map[string]interface{}{},
		}), nil
	})
	ac := newServicePaginateTestClient(t, rt)
	out, errOut := prepareServicePaginateOutput(t)

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
