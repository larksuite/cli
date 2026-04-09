// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"testing"
	"unsafe"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/shortcuts/common"
	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

type staticConvertlibTokenResolver struct{}

func (s *staticConvertlibTokenResolver) ResolveToken(_ context.Context, _ credential.TokenSpec) (*credential.TokenResult, error) {
	return &credential.TokenResult{Token: "test-token"}, nil
}

type convertlibRoundTripFunc func(*http.Request) (*http.Response, error)

func (f convertlibRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func convertlibJSONResponse(status int, body interface{}) *http.Response {
	b, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(b)),
	}
}

func setConvertlibRuntimeField(t *testing.T, runtime *common.RuntimeContext, field string, value interface{}) {
	t.Helper()

	rv := reflect.ValueOf(runtime).Elem().FieldByName(field)
	if !rv.IsValid() {
		t.Fatalf("field %q not found", field)
	}
	reflect.NewAt(rv.Type(), unsafe.Pointer(rv.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
}

func newBotConvertlibRuntime(t *testing.T, rt http.RoundTripper) *common.RuntimeContext {
	t.Helper()

	httpClient := &http.Client{Transport: rt}
	sdk := lark.NewClient(
		"test-app",
		"test-secret",
		lark.WithEnableTokenCache(false),
		lark.WithLogLevel(larkcore.LogLevelError),
		lark.WithHttpClient(httpClient),
	)
	cfg := &core.CliConfig{
		AppID:     "test-app",
		AppSecret: "test-secret",
		Brand:     core.BrandFeishu,
	}
	testCred := credential.NewCredentialProvider(nil, nil, &staticConvertlibTokenResolver{}, nil)
	runtime := &common.RuntimeContext{
		Config: cfg,
		Factory: &cmdutil.Factory{
			Config:     func() (*core.CliConfig, error) { return cfg, nil },
			HttpClient: func() (*http.Client, error) { return httpClient, nil },
			LarkClient: func() (*lark.Client, error) { return sdk, nil },
			Credential: testCred,
			IOStreams: &cmdutil.IOStreams{
				Out:    &bytes.Buffer{},
				ErrOut: &bytes.Buffer{},
			},
		},
	}
	setConvertlibRuntimeField(t, runtime, "ctx", context.Background())
	setConvertlibRuntimeField(t, runtime, "resolvedAs", core.AsBot)
	setConvertlibRuntimeField(t, runtime, "larkSDK", sdk)
	return runtime
}
