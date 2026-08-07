// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	internaltransport "github.com/larksuite/cli/internal/transport"
	"github.com/larksuite/cli/shortcuts/common"
)

type staticShortcutTokenResolver struct{}

func (s *staticShortcutTokenResolver) ResolveToken(_ context.Context, _ credential.TokenSpec) (*credential.TokenResult, error) {
	return &credential.TokenResult{Token: "tenant-token"}, nil
}

type shortcutRoundTripFunc func(*http.Request) (*http.Response, error)

func (f shortcutRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type shortcutPolicyDecorator struct {
	base http.RoundTripper
	fn   shortcutRoundTripFunc
}

func (t *shortcutPolicyDecorator) BaseRoundTripper() http.RoundTripper {
	return t.base
}

func (t *shortcutPolicyDecorator) WithBaseRoundTripper(base http.RoundTripper) http.RoundTripper {
	cloned := *t
	cloned.base = base
	return &cloned
}

func (t *shortcutPolicyDecorator) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.fn(req)
}

func shortcutJSONResponse(status int, body interface{}) *http.Response {
	b, _ := json.Marshal(body)
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(b)),
	}
}

func shortcutRawResponse(status int, body []byte, headers http.Header) *http.Response {
	if headers == nil {
		headers = make(http.Header)
	}
	return &http.Response{
		StatusCode:    status,
		Header:        headers,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
	}
}

func setRuntimeField(t *testing.T, runtime *common.RuntimeContext, field string, value interface{}) {
	t.Helper()

	rv := reflect.ValueOf(runtime).Elem().FieldByName(field)
	if !rv.IsValid() {
		t.Fatalf("field %q not found", field)
	}
	reflect.NewAt(rv.Type(), unsafe.Pointer(rv.UnsafeAddr())).Elem().Set(reflect.ValueOf(value))
}

func newBotShortcutRuntime(t *testing.T, rt http.RoundTripper) *common.RuntimeContext {
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
	testCred := credential.NewCredentialProvider(nil, nil, &staticShortcutTokenResolver{}, nil)
	runtime := &common.RuntimeContext{
		Config: cfg,
		Factory: &cmdutil.Factory{
			Config:         func() (*core.CliConfig, error) { return cfg, nil },
			HttpClient:     func() (*http.Client, error) { return httpClient, nil },
			LarkClient:     func() (*lark.Client, error) { return sdk, nil },
			Credential:     testCred,
			FileIOProvider: fileio.GetProvider(),
			IOStreams: &cmdutil.IOStreams{
				Out:    &bytes.Buffer{},
				ErrOut: &bytes.Buffer{},
			},
		},
	}
	setRuntimeField(t, runtime, "ctx", cmdutil.ContextWithShortcut(context.Background(), "im.test", "exec-123"))
	setRuntimeField(t, runtime, "resolvedAs", core.AsBot)
	setRuntimeField(t, runtime, "larkSDK", sdk)
	return runtime
}

func newUserShortcutRuntime(t *testing.T, rt http.RoundTripper) *common.RuntimeContext {
	t.Helper()
	runtime := newBotShortcutRuntime(t, rt)
	setRuntimeField(t, runtime, "resolvedAs", core.AsUser)
	return runtime
}

func TestResolveP2PChatID(t *testing.T) {
	runtime := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/chat_p2p/batch_query"):
			return shortcutJSONResponse(200, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"p2p_chats": []interface{}{
						map[string]interface{}{"chat_id": "oc_123"},
					},
				},
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	got, err := resolveP2PChatID(runtime, "ou_123")
	if err != nil {
		t.Fatalf("resolveP2PChatID() error = %v", err)
	}
	if got != "oc_123" {
		t.Fatalf("resolveP2PChatID() = %q, want %q", got, "oc_123")
	}
}

func TestResolveP2PChatIDNotFound(t *testing.T) {
	runtime := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/chat_p2p/batch_query"):
			return shortcutJSONResponse(200, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"p2p_chats": []interface{}{},
				},
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	_, err := resolveP2PChatID(runtime, "ou_404")
	if err == nil || !strings.Contains(err.Error(), "P2P chat not found") {
		t.Fatalf("resolveP2PChatID() error = %v", err)
	}
}

func TestResolveP2PChatIDRejectsBot(t *testing.T) {
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
	}))

	_, err := resolveP2PChatID(runtime, "ou_123")
	if err == nil || !strings.Contains(err.Error(), "requires user identity") {
		t.Fatalf("resolveP2PChatID() error = %v, want requires user identity", err)
	}
}

func TestResolveThreadID(t *testing.T) {
	t.Run("thread id passthrough", func(t *testing.T) {
		got, err := resolveThreadID(newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		})), "omt_123")
		if err != nil {
			t.Fatalf("resolveThreadID() error = %v", err)
		}
		if got != "omt_123" {
			t.Fatalf("resolveThreadID() = %q, want %q", got, "omt_123")
		}
	})

	t.Run("invalid id", func(t *testing.T) {
		_, err := resolveThreadID(newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		})), "bad_123")
		if err == nil || !strings.Contains(err.Error(), "must start with om_ or omt_") {
			t.Fatalf("resolveThreadID() error = %v", err)
		}
	})

	t.Run("message lookup success", func(t *testing.T) {
		runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_123"):
				return shortcutJSONResponse(200, map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"items": []interface{}{
							map[string]interface{}{"thread_id": "omt_resolved"},
						},
					},
				}), nil
			default:
				return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
			}
		}))

		got, err := resolveThreadID(runtime, "om_123")
		if err != nil {
			t.Fatalf("resolveThreadID() error = %v", err)
		}
		if got != "omt_resolved" {
			t.Fatalf("resolveThreadID() = %q, want %q", got, "omt_resolved")
		}
	})

	t.Run("message lookup not found", func(t *testing.T) {
		runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_404"):
				return shortcutJSONResponse(200, map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{
						"items": []interface{}{
							map[string]interface{}{},
						},
					},
				}), nil
			default:
				return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
			}
		}))

		_, err := resolveThreadID(runtime, "om_404")
		if err == nil || !strings.Contains(err.Error(), "thread ID not found") {
			t.Fatalf("resolveThreadID() error = %v", err)
		}
	})
}

func TestDownloadIMResourceToPathSuccess(t *testing.T) {
	var gotHeaders http.Header
	payload := []byte("hello download")
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_123/resources/file_123"):
			gotHeaders = req.Header.Clone()
			return shortcutRawResponse(200, payload, http.Header{"Content-Type": []string{"application/octet-stream"}}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())

	target := filepath.Join("nested", "resource.bin")
	_, size, err := downloadIMResourceToPath(context.Background(), runtime, "om_123", "file_123", "file", target, true)
	if err != nil {
		t.Fatalf("downloadIMResourceToPath() error = %v", err)
	}
	if size != int64(len(payload)) {
		t.Fatalf("downloadIMResourceToPath() size = %d, want %d", size, len(payload))
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("downloaded payload = %q, want %q", string(data), string(payload))
	}
	if gotHeaders.Get("Authorization") != "Bearer tenant-token" {
		t.Fatalf("Authorization header = %q, want %q", gotHeaders.Get("Authorization"), "Bearer tenant-token")
	}
	if gotHeaders.Get(cmdutil.HeaderSource) != cmdutil.SourceValue {
		t.Fatalf("%s = %q, want %q", cmdutil.HeaderSource, gotHeaders.Get(cmdutil.HeaderSource), cmdutil.SourceValue)
	}
	if gotHeaders.Get(cmdutil.HeaderShortcut) != "im.test" {
		t.Fatalf("%s = %q, want %q", cmdutil.HeaderShortcut, gotHeaders.Get(cmdutil.HeaderShortcut), "im.test")
	}
	if gotHeaders.Get(cmdutil.HeaderExecutionId) != "exec-123" {
		t.Fatalf("%s = %q, want %q", cmdutil.HeaderExecutionId, gotHeaders.Get(cmdutil.HeaderExecutionId), "exec-123")
	}
	if gotHeaders.Get("Range") != fmt.Sprintf("bytes=0-%d", probeChunkSize-1) {
		t.Fatalf("Range header = %q, want %q", gotHeaders.Get("Range"), fmt.Sprintf("bytes=0-%d", probeChunkSize-1))
	}
}

func TestDownloadIMResourceToPathImageUsesSingleRequestWithoutRange(t *testing.T) {
	var gotHeaders http.Header
	payload := []byte("image download")
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_img/resources/img_123"):
			gotHeaders = req.Header.Clone()
			return shortcutRawResponse(200, payload, http.Header{"Content-Type": []string{"image/png"}}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())

	gotPath, size, err := downloadIMResourceToPath(context.Background(), runtime, "om_img", "img_123", "image", "image", true)
	if err != nil {
		t.Fatalf("downloadIMResourceToPath() error = %v", err)
	}
	if size != int64(len(payload)) {
		t.Fatalf("downloadIMResourceToPath() size = %d, want %d", size, len(payload))
	}
	if gotHeaders.Get("Range") != "" {
		t.Fatalf("Range header = %q, want empty", gotHeaders.Get("Range"))
	}
	if !strings.HasSuffix(gotPath, "image.png") {
		t.Fatalf("saved path = %q, want suffix %q", gotPath, "image.png")
	}
	data, err := os.ReadFile("image.png")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(data) != string(payload) {
		t.Fatalf("downloaded payload = %q, want %q", string(data), string(payload))
	}
}

func TestDownloadIMResourceToPathHTTPErrorBody(t *testing.T) {
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_403/resources/file_403"):
			return shortcutRawResponse(403, []byte("denied"), http.Header{"Content-Type": []string{"text/plain"}}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())

	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_403", "file_403", "file", "out.bin", true)
	if err == nil || !strings.Contains(err.Error(), "HTTP 403: denied") {
		t.Fatalf("downloadIMResourceToPath() error = %v", err)
	}
}

func TestDownloadIMResourceToPathRetriesNetworkError(t *testing.T) {
	attempts := 0
	payload := []byte("retry success")
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "tenant_access_token"):
			return shortcutJSONResponse(200, map[string]interface{}{
				"code":                0,
				"tenant_access_token": "tenant-token",
				"expire":              7200,
			}), nil
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_retry/resources/file_retry"):
			attempts++
			if attempts < 3 {
				return nil, fmt.Errorf("temporary network failure")
			}
			return shortcutRawResponse(200, payload, http.Header{"Content-Type": []string{"application/octet-stream"}}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())
	target := "out.bin"
	_, size, err := downloadIMResourceToPath(context.Background(), runtime, "om_retry", "file_retry", "file", target, true)
	if err != nil {
		t.Fatalf("downloadIMResourceToPath() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("download attempts = %d, want 3", attempts)
	}
	if size != int64(len(payload)) {
		t.Fatalf("downloadIMResourceToPath() size = %d, want %d", size, len(payload))
	}
}

func TestDownloadIMResourceToPathRetrySecondAttemptSuccess(t *testing.T) {
	attempts := 0
	payload := []byte("second retry success")
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "tenant_access_token"):
			return shortcutJSONResponse(200, map[string]interface{}{
				"code":                0,
				"tenant_access_token": "tenant-token",
				"expire":              7200,
			}), nil
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_retry2/resources/file_retry2"):
			attempts++
			if attempts < 2 {
				return nil, fmt.Errorf("temporary network failure")
			}
			return shortcutRawResponse(200, payload, http.Header{"Content-Type": []string{"application/octet-stream"}}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())
	target := "out.bin"
	_, size, err := downloadIMResourceToPath(context.Background(), runtime, "om_retry2", "file_retry2", "file", target, true)
	if err != nil {
		t.Fatalf("downloadIMResourceToPath() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("download attempts = %d, want 2", attempts)
	}
	if size != int64(len(payload)) {
		t.Fatalf("downloadIMResourceToPath() size = %d, want %d", size, len(payload))
	}
}

func TestDownloadIMResourceToPathRetryContextCanceled(t *testing.T) {
	attempts := 0
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "tenant_access_token"):
			return shortcutJSONResponse(200, map[string]interface{}{
				"code":                0,
				"tenant_access_token": "tenant-token",
				"expire":              7200,
			}), nil
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_cancel/resources/file_cancel"):
			attempts++
			return nil, fmt.Errorf("temporary network failure")
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel context immediately to trigger context error on first retry
	cancel()

	cmdutil.TestChdir(t, t.TempDir())
	target := "out.bin"
	_, _, err := downloadIMResourceToPath(ctx, runtime, "om_cancel", "file_cancel", "file", target, true)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("downloadIMResourceToPath() error = %v, want errors.Is(context.Canceled)", err)
	}
	var ne *errs.NetworkError
	if !errors.As(err, &ne) {
		t.Fatalf("downloadIMResourceToPath() error = %T, want *errs.NetworkError", err)
	}
	if ne.Subtype != errs.SubtypeNetworkTransport {
		t.Fatalf("network subtype = %q, want %q", ne.Subtype, errs.SubtypeNetworkTransport)
	}
	// First attempt is made, then retry checks ctx.Err() and returns
	if attempts != 1 {
		t.Fatalf("download attempts = %d, want 1", attempts)
	}
}

func TestDownloadIMResourceToPathRangeDownload(t *testing.T) {
	cases := []struct {
		name       string
		payloadLen int64
		wantRanges []string
	}{
		{
			name:       "single small chunk",
			payloadLen: 16,
			wantRanges: []string{"bytes=0-131071"},
		},
		{
			name:       "exact probe chunk",
			payloadLen: probeChunkSize,
			wantRanges: []string{"bytes=0-131071"},
		},
		{
			name:       "multiple chunks with tail",
			payloadLen: probeChunkSize + normalChunkSize + 1234,
			wantRanges: []string{
				"bytes=0-131071",
				fmt.Sprintf("bytes=%d-%d", probeChunkSize, probeChunkSize+normalChunkSize-1),
				fmt.Sprintf("bytes=%d-%d", probeChunkSize+normalChunkSize, probeChunkSize+normalChunkSize+1233),
			},
		},
		{
			name:       "multiple chunks exact 8mb tail",
			payloadLen: probeChunkSize + 2*normalChunkSize,
			wantRanges: []string{
				"bytes=0-131071",
				fmt.Sprintf("bytes=%d-%d", probeChunkSize, probeChunkSize+normalChunkSize-1),
				fmt.Sprintf("bytes=%d-%d", probeChunkSize+normalChunkSize, probeChunkSize+2*normalChunkSize-1),
			},
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			payload := bytes.Repeat([]byte("range-download-"), int(tt.payloadLen/15)+1)
			payload = payload[:tt.payloadLen]

			var gotRanges []string
			runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch {
				case strings.Contains(req.URL.Path, "tenant_access_token"):
					return shortcutJSONResponse(200, map[string]interface{}{
						"code":                0,
						"tenant_access_token": "tenant-token",
						"expire":              7200,
					}), nil
				case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_range/resources/file_range"):
					rangeHeader := req.Header.Get("Range")
					gotRanges = append(gotRanges, rangeHeader)
					if req.Header.Get("Authorization") != "Bearer tenant-token" {
						return nil, fmt.Errorf("missing authorization header")
					}
					start, end, err := parseRangeHeader(rangeHeader, int64(len(payload)))
					if err != nil {
						return nil, err
					}
					return shortcutRawResponse(http.StatusPartialContent, payload[start:end+1], http.Header{
						"Content-Type":  []string{"application/octet-stream"},
						"Content-Range": []string{fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload))},
						"Etag":          []string{`"v1"`},
					}), nil
				default:
					return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
				}
			}))

			cmdutil.TestChdir(t, t.TempDir())
			target := filepath.Join("nested", "resource.bin")
			_, size, err := downloadIMResourceToPath(context.Background(), runtime, "om_range", "file_range", "file", target, true)
			if err != nil {
				t.Fatalf("downloadIMResourceToPath() error = %v", err)
			}
			if size != int64(len(payload)) {
				t.Fatalf("downloadIMResourceToPath() size = %d, want %d", size, len(payload))
			}
			if !reflect.DeepEqual(gotRanges, tt.wantRanges) {
				t.Fatalf("Range requests = %#v, want %#v", gotRanges, tt.wantRanges)
			}

			got, err := os.ReadFile(target)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}
			if md5.Sum(got) != md5.Sum(payload) {
				t.Fatalf("downloaded payload MD5 = %x, want %x", md5.Sum(got), md5.Sum(payload))
			}
		})
	}
}

func TestDownloadIMResourceToPathInvalidContentRange(t *testing.T) {
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "tenant_access_token"):
			return shortcutJSONResponse(200, map[string]interface{}{
				"code":                0,
				"tenant_access_token": "tenant-token",
				"expire":              7200,
			}), nil
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_bad/resources/file_bad"):
			return shortcutRawResponse(http.StatusPartialContent, []byte("bad"), http.Header{
				"Content-Type":  []string{"application/octet-stream"},
				"Content-Range": []string{"bytes 0-2/not-a-number"},
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())
	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_bad", "file_bad", "file", "out.bin", true)
	requireDownloadProblem(t, err, "invalid Content-Range header", errs.SubtypeNetworkProtocol, false)
}

func TestDownloadIMResourceToPathRangeChunkFailureCleansOutput(t *testing.T) {
	payload := bytes.Repeat([]byte("range-download-"), int((probeChunkSize+1024)/15)+1)
	payload = payload[:probeChunkSize+1024]

	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_miderr/resources/file_miderr"):
			rangeHeader := req.Header.Get("Range")
			if rangeHeader == fmt.Sprintf("bytes=0-%d", probeChunkSize-1) {
				return shortcutRawResponse(http.StatusPartialContent, payload[:probeChunkSize], http.Header{
					"Content-Type":  []string{"application/octet-stream"},
					"Content-Range": []string{fmt.Sprintf("bytes 0-%d/%d", probeChunkSize-1, len(payload))},
					"Etag":          []string{`"v1"`},
				}), nil
			}
			return shortcutRawResponse(http.StatusInternalServerError, []byte("chunk failed"), http.Header{"Content-Type": []string{"text/plain"}}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())

	target := "out.bin"
	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_miderr", "file_miderr", "file", target, true)
	if err == nil || !strings.Contains(err.Error(), "HTTP 500: chunk failed") {
		t.Fatalf("downloadIMResourceToPath() error = %v", err)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("downloadIMResourceToPath() error = %T, want typed problem", err)
	}
	if p.Category != errs.CategoryNetwork || p.Subtype != errs.SubtypeNetworkServer || p.Code != http.StatusInternalServerError {
		t.Fatalf("network problem = subtype %q code %d, want subtype %q code %d",
			p.Subtype, p.Code, errs.SubtypeNetworkServer, http.StatusInternalServerError)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("output file exists after failed download, stat error = %v", statErr)
	}
}

func TestDownloadIMResourceToPathRangeOverflowCleansOutput(t *testing.T) {
	payload := []byte("overflow-payload")
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_overflow/resources/file_overflow"):
			return shortcutRawResponse(http.StatusPartialContent, payload, http.Header{
				"Content-Type":  []string{"application/octet-stream"},
				"Content-Range": []string{"bytes 0-3/4"},
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())

	target := "out.bin"
	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_overflow", "file_overflow", "file", target, true)
	requireDownloadProblem(t, err, "chunk overflow", errs.SubtypeNetworkProtocol, false)
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("output file exists after overflow, stat error = %v", statErr)
	}
}

func TestDownloadIMResourceToPathRangeShortChunkSizeMismatch(t *testing.T) {
	payload := bytes.Repeat([]byte("range-download-"), int((probeChunkSize+1024)/15)+1)
	payload = payload[:probeChunkSize+1024]

	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_short/resources/file_short"):
			rangeHeader := req.Header.Get("Range")
			start, end, err := parseRangeHeader(rangeHeader, int64(len(payload)))
			if err != nil {
				return nil, err
			}
			body := payload[start : end+1]
			if start == probeChunkSize {
				body = body[:len(body)-10]
			}
			return shortcutRawResponse(http.StatusPartialContent, body, http.Header{
				"Content-Type":  []string{"application/octet-stream"},
				"Content-Range": []string{fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload))},
				"Etag":          []string{`"v1"`},
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())

	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_short", "file_short", "file", "out.bin", true)
	requireDownloadProblem(t, err, "range response delivered", errs.SubtypeNetworkProtocol, false)
}

// imRangeServer is a fake resource endpoint whose range behaviour each test
// controls: slice picks how much of the requested range it actually serves, so
// a test can hand back the whole file, a shorter slice, or the wrong offset.
type imRangeServer struct {
	payload   []byte
	etag      string
	requests  []string
	ifRanges  []string
	slice     func(start, end int64) (gotStart, gotEnd int64)
	statusFor func(attempt int) int
}

func (s *imRangeServer) handler(t *testing.T, path string) shortcutRoundTripFunc {
	t.Helper()
	return shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "tenant_access_token"):
			return shortcutJSONResponse(200, map[string]interface{}{
				"code":                0,
				"tenant_access_token": "tenant-token",
				"expire":              7200,
			}), nil
		case strings.Contains(req.URL.Path, path):
			s.requests = append(s.requests, req.Header.Get("Range"))
			s.ifRanges = append(s.ifRanges, req.Header.Get("If-Range"))
			total := int64(len(s.payload))
			start, end, err := parseRangeHeader(req.Header.Get("Range"), total)
			if err != nil {
				return nil, err
			}
			if s.statusFor != nil {
				if status := s.statusFor(len(s.requests)); status == http.StatusOK {
					return shortcutRawResponse(http.StatusOK, s.payload, http.Header{
						"Content-Type": []string{"application/octet-stream"},
					}), nil
				}
			}
			if s.slice != nil {
				start, end = s.slice(start, end)
			}
			header := http.Header{
				"Content-Type":  []string{"application/octet-stream"},
				"Content-Range": []string{fmt.Sprintf("bytes %d-%d/%d", start, end, total)},
			}
			if s.etag != "" {
				header.Set("ETag", s.etag)
			}
			return shortcutRawResponse(http.StatusPartialContent, s.payload[start:end+1], header), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	})
}

func imRangePayload(size int64) []byte {
	payload := bytes.Repeat([]byte("range-payload-"), int(size/14)+1)
	return payload[:size]
}

// A server may answer a range request with more than was asked for — object
// storage gateways commonly return the whole object in one 206. The data is
// intact, so the download must complete instead of failing on the mismatch.
func TestDownloadIMResourceToPathAcceptsWholeFileInOneRangeResponse(t *testing.T) {
	payload := imRangePayload(probeChunkSize + 4096)
	server := &imRangeServer{
		payload: payload,
		etag:    `"v1"`,
		slice: func(start, end int64) (int64, int64) {
			return start, int64(len(payload)) - 1
		},
	}
	runtime := newBotShortcutRuntime(t, server.handler(t, "/open-apis/im/v1/messages/om_whole/resources/file_whole"))

	cmdutil.TestChdir(t, t.TempDir())
	_, size, err := downloadIMResourceToPath(context.Background(), runtime, "om_whole", "file_whole", "file", "out.bin", true)
	if err != nil {
		t.Fatalf("downloadIMResourceToPath() error = %v", err)
	}
	if size != int64(len(payload)) {
		t.Fatalf("size = %d, want %d", size, len(payload))
	}
	if len(server.requests) != 1 {
		t.Fatalf("resource requests = %#v, want a single request", server.requests)
	}
	got, err := os.ReadFile("out.bin")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if md5.Sum(got) != md5.Sum(payload) {
		t.Fatalf("payload MD5 = %x, want %x", md5.Sum(got), md5.Sum(payload))
	}
}

// A server may also answer with less than was asked for. Resuming from the end
// it reported keeps the download going instead of skipping the bytes it held
// back.
func TestDownloadIMResourceToPathResumesFromShorterSlices(t *testing.T) {
	payload := imRangePayload(probeChunkSize * 3)
	server := &imRangeServer{
		payload: payload,
		etag:    `"v1"`,
		slice: func(start, end int64) (int64, int64) {
			// Serve at most a quarter of the probe chunk per response.
			if capped := start + probeChunkSize/4 - 1; capped < end {
				return start, capped
			}
			return start, end
		},
	}
	runtime := newBotShortcutRuntime(t, server.handler(t, "/open-apis/im/v1/messages/om_slices/resources/file_slices"))

	cmdutil.TestChdir(t, t.TempDir())
	_, size, err := downloadIMResourceToPath(context.Background(), runtime, "om_slices", "file_slices", "file", "out.bin", true)
	if err != nil {
		t.Fatalf("downloadIMResourceToPath() error = %v", err)
	}
	if size != int64(len(payload)) {
		t.Fatalf("size = %d, want %d", size, len(payload))
	}
	got, err := os.ReadFile("out.bin")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if md5.Sum(got) != md5.Sum(payload) {
		t.Fatalf("payload MD5 = %x, want %x", md5.Sum(got), md5.Sum(payload))
	}
	if len(server.requests) < 4 {
		t.Fatalf("resource requests = %d, want the download to continue across several slices", len(server.requests))
	}
	// Every follow-up request must resume exactly where the previous response
	// ended, so no byte is skipped or fetched twice.
	wantNext := int64(0)
	for i, r := range server.requests {
		start, _, err := parseRangeHeader(r, int64(len(payload)))
		if err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
		if start != wantNext {
			t.Fatalf("request %d = %q, want it to resume at byte %d", i, r, wantNext)
		}
		wantNext = start + probeChunkSize/4
		if i == 0 {
			wantNext = probeChunkSize / 4
		}
	}
}

// The regression this whole change exists for: a chunk that carries the right
// number of bytes for the wrong offset used to be written at the requested
// offset, leaving a file of the correct size with corrupt contents and an exit
// code of 0.
func TestDownloadIMResourceToPathRejectsChunkAtWrongOffset(t *testing.T) {
	payload := imRangePayload(probeChunkSize + 512)
	server := &imRangeServer{
		payload: payload,
		etag:    `"v1"`,
		slice: func(start, end int64) (int64, int64) {
			if start == 0 {
				return start, end
			}
			// Same length, wrong place: the bytes come from the head of the file.
			return 0, end - start
		},
	}
	runtime := newBotShortcutRuntime(t, server.handler(t, "/open-apis/im/v1/messages/om_wrong/resources/file_wrong"))

	cmdutil.TestChdir(t, t.TempDir())
	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_wrong", "file_wrong", "file", "out.bin", true)
	requireDownloadProblem(t, err, "want it to resume at byte", errs.SubtypeNetworkProtocol, false)
	if _, statErr := os.Stat("out.bin"); !os.IsNotExist(statErr) {
		t.Fatalf("output file exists after a rejected download, stat error = %v", statErr)
	}
}

func TestDownloadIMResourceToPathPinsLaterChunksWithIfRange(t *testing.T) {
	payload := imRangePayload(probeChunkSize + 2048)
	server := &imRangeServer{payload: payload, etag: `"v1"`}
	runtime := newBotShortcutRuntime(t, server.handler(t, "/open-apis/im/v1/messages/om_pin/resources/file_pin"))

	cmdutil.TestChdir(t, t.TempDir())
	if _, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_pin", "file_pin", "file", "out.bin", true); err != nil {
		t.Fatalf("downloadIMResourceToPath() error = %v", err)
	}
	if len(server.ifRanges) < 2 {
		t.Fatalf("If-Range headers = %#v, want at least two requests", server.ifRanges)
	}
	if server.ifRanges[0] != "" {
		t.Fatalf("probe If-Range = %q, want empty (there is nothing to pin to yet)", server.ifRanges[0])
	}
	for i, got := range server.ifRanges[1:] {
		if got != `"v1"` {
			t.Fatalf("If-Range on chunk %d = %q, want %q", i+1, got, `"v1"`)
		}
	}
}

// If-Range did not match, so the server sent the whole current representation
// with 200 instead of a 206. Continuing would splice two versions of the file
// together.
func TestDownloadIMResourceToPathRejectsResourceChangedMidDownload(t *testing.T) {
	payload := imRangePayload(probeChunkSize + 2048)
	server := &imRangeServer{
		payload: payload,
		etag:    `"v1"`,
		statusFor: func(attempt int) int {
			return map[bool]int{true: http.StatusOK, false: http.StatusPartialContent}[attempt > 1]
		},
	}
	runtime := newBotShortcutRuntime(t, server.handler(t, "/open-apis/im/v1/messages/om_changed/resources/file_changed"))

	cmdutil.TestChdir(t, t.TempDir())
	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_changed", "file_changed", "file", "out.bin", true)
	requireDownloadProblem(t, err, "resource changed while downloading", errs.SubtypeRepresentationChanged, true)
	if _, statErr := os.Stat("out.bin"); !os.IsNotExist(statErr) {
		t.Fatalf("output file exists after a rejected download, stat error = %v", statErr)
	}
}

func TestDownloadIMResourceToPathRejectsTotalSizeChangeMidDownload(t *testing.T) {
	payload := imRangePayload(probeChunkSize + 2048)
	var requests int
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_grew/resources/file_grew"):
			requests++
			start, end, err := parseRangeHeader(req.Header.Get("Range"), int64(len(payload)))
			if err != nil {
				return nil, err
			}
			total := int64(len(payload))
			if requests > 1 {
				total += 4096
			}
			// No validator on purpose: the total-size check is the guarantee that
			// still holds when the server offers nothing to pin the transfer to.
			return shortcutRawResponse(http.StatusPartialContent, payload[start:end+1], http.Header{
				"Content-Type":  []string{"application/octet-stream"},
				"Content-Range": []string{fmt.Sprintf("bytes %d-%d/%d", start, end, total)},
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())
	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_grew", "file_grew", "file", "out.bin", true)
	requireDownloadProblem(t, err, "resource size changed while downloading", errs.SubtypeRepresentationChanged, true)
}

// With If-Range on the wire a changed resource has to come back as 200, so a 206
// describing a different total means the server ignored the condition. Asking
// again the same way gets the same answer, so this one is not retryable.
func TestDownloadIMResourceToPathRejectsTotalSizeChangeDespiteIfRange(t *testing.T) {
	payload := imRangePayload(probeChunkSize + 2048)
	var requests int
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_ignored/resources/file_ignored"):
			requests++
			start, end, err := parseRangeHeader(req.Header.Get("Range"), int64(len(payload)))
			if err != nil {
				return nil, err
			}
			total := int64(len(payload))
			if requests > 1 {
				if got := req.Header.Get("If-Range"); got != `"v1"` {
					return nil, fmt.Errorf("If-Range = %q, want %q", got, `"v1"`)
				}
				total += 4096
			}
			header := http.Header{
				"Content-Type":  []string{"application/octet-stream"},
				"Content-Range": []string{fmt.Sprintf("bytes %d-%d/%d", start, end, total)},
			}
			header.Set("ETag", `"v1"`)
			return shortcutRawResponse(http.StatusPartialContent, payload[start:end+1], header), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())
	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_ignored", "file_ignored", "file", "out.bin", true)
	requireDownloadProblem(t, err, "server ignored If-Range", errs.SubtypeNetworkProtocol, false)
}

// A response whose body is longer than the Content-Range it came with
// contradicts itself; the extra bytes have no place to go.
func TestDownloadIMResourceToPathRejectsChunkLongerThanContentRange(t *testing.T) {
	payload := imRangePayload(probeChunkSize + 64)
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_long/resources/file_long"):
			start, end, err := parseRangeHeader(req.Header.Get("Range"), int64(len(payload)))
			if err != nil {
				return nil, err
			}
			body := payload[start : end+1]
			header := http.Header{
				"Content-Type":  []string{"application/octet-stream"},
				"Content-Range": []string{fmt.Sprintf("bytes %d-%d/%d", start, end-1, len(payload))},
				"Etag":          []string{`"v1"`},
			}
			return shortcutRawResponse(http.StatusPartialContent, body, header), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())
	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_long", "file_long", "file", "out.bin", true)
	requireDownloadProblem(t, err, "range response delivered", errs.SubtypeNetworkProtocol, false)
}

// requireDownloadProblem asserts the message and the structured fields agents
// branch on. Checking only the message text lets a regression to
// network/transport pass unnoticed.
func requireDownloadProblem(t *testing.T, err error, wantMsg string, wantSubtype errs.Subtype, wantRetryable bool) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), wantMsg) {
		t.Fatalf("error = %v, want a message containing %q", err, wantMsg)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %T, want a typed problem", err)
	}
	if p.Category != errs.CategoryNetwork || p.Subtype != wantSubtype {
		t.Fatalf("problem = %s/%s, want %s/%s", p.Category, p.Subtype, errs.CategoryNetwork, wantSubtype)
	}
	if p.Retryable != wantRetryable {
		t.Fatalf("problem retryable = %v, want %v", p.Retryable, wantRetryable)
	}
}

// This endpoint answers 206 with no ETag at all, so requiring a strong validator
// before combining ranges would disable ranged downloads outright and put every
// file behind a single request with one shared timeout. Ranges continue without
// one; what is lost is only the ability to notice a replacement of exactly the
// same length, and an IM attachment is fixed once its message is sent.
func TestDownloadIMResourceToPathChunksWithoutStrongValidator(t *testing.T) {
	payload := imRangePayload(probeChunkSize + 4096)
	var ranges, ifRanges []string
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_noval/resources/file_noval"):
			ranges = append(ranges, req.Header.Get("Range"))
			ifRanges = append(ifRanges, req.Header.Get("If-Range"))
			start, end, err := parseRangeHeader(req.Header.Get("Range"), int64(len(payload)))
			if err != nil {
				return nil, err
			}
			// Serves ranges, but offers only a weak entity-tag and a date — neither
			// may be sent in If-Range.
			return shortcutRawResponse(http.StatusPartialContent, payload[start:end+1], http.Header{
				"Content-Type":  []string{"application/octet-stream"},
				"Content-Range": []string{fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload))},
				"Etag":          []string{`W/"weak"`},
				"Last-Modified": []string{"Wed, 21 Oct 2015 07:28:00 GMT"},
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())
	_, size, err := downloadIMResourceToPath(context.Background(), runtime, "om_noval", "file_noval", "file", "out.bin", true)
	if err != nil {
		t.Fatalf("downloadIMResourceToPath() error = %v", err)
	}
	if size != int64(len(payload)) {
		t.Fatalf("size = %d, want %d", size, len(payload))
	}
	if len(ranges) != 2 {
		t.Fatalf("requests = %#v, want the probe plus one more range", ranges)
	}
	for i, r := range ranges {
		if r == "" {
			t.Fatalf("request %d went out without a Range header, so ranges were abandoned", i)
		}
	}
	for i, r := range ifRanges {
		if r != "" {
			t.Fatalf("request %d sent If-Range %q; neither a weak tag nor a date may be sent there", i, r)
		}
	}
	got, err := os.ReadFile("out.bin")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if md5.Sum(got) != md5.Sum(payload) {
		t.Fatalf("payload MD5 = %x, want %x", md5.Sum(got), md5.Sum(payload))
	}
}

func TestDownloadIMResourceToPathRejectsValidatorChangeOnLaterChunk(t *testing.T) {
	payload := imRangePayload(probeChunkSize + 2048)
	var requests int
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_etag/resources/file_etag"):
			requests++
			start, end, err := parseRangeHeader(req.Header.Get("Range"), int64(len(payload)))
			if err != nil {
				return nil, err
			}
			header := http.Header{
				"Content-Type":  []string{"application/octet-stream"},
				"Content-Range": []string{fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload))},
			}
			// The server ignores If-Range and serves a different version anyway.
			if requests > 1 {
				header.Set("ETag", `"v2"`)
			} else {
				header.Set("ETag", `"v1"`)
			}
			return shortcutRawResponse(http.StatusPartialContent, payload[start:end+1], header), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())
	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_etag", "file_etag", "file", "out.bin", true)
	requireDownloadProblem(t, err, `carries validator "\"v2\"", want "\"v1\""`, errs.SubtypeRepresentationChanged, true)
}

func TestDownloadIMResourceToPathRejectsMissingValidatorOnLaterChunk(t *testing.T) {
	payload := imRangePayload(probeChunkSize + 2048)
	var requests int
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_noetag/resources/file_noetag"):
			requests++
			start, end, err := parseRangeHeader(req.Header.Get("Range"), int64(len(payload)))
			if err != nil {
				return nil, err
			}
			header := http.Header{
				"Content-Type":  []string{"application/octet-stream"},
				"Content-Range": []string{fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload))},
			}
			if requests == 1 {
				header.Set("ETag", `"v1"`)
			}
			return shortcutRawResponse(http.StatusPartialContent, payload[start:end+1], header), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())
	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_noetag", "file_noetag", "file", "out.bin", true)
	requireDownloadProblem(t, err, "carries no usable validator", errs.SubtypeNetworkProtocol, false)
}

// One byte per response must not turn a download into one request per byte.
func TestDownloadIMResourceToPathCapsRangeResponseCount(t *testing.T) {
	total := int64(2048)
	payload := imRangePayload(total)
	var requests int
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_cap/resources/file_cap"):
			requests++
			start, _, err := parseRangeHeader(req.Header.Get("Range"), total)
			if err != nil {
				return nil, err
			}
			return shortcutRawResponse(http.StatusPartialContent, payload[start:start+1], http.Header{
				"Content-Type":  []string{"application/octet-stream"},
				"Content-Range": []string{fmt.Sprintf("bytes %d-%d/%d", start, start, total)},
				"Etag":          []string{`"v1"`},
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())
	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_cap", "file_cap", "file", "out.bin", true)
	requireDownloadProblem(t, err, "range responses; giving up", errs.SubtypeNetworkProtocol, false)
	if want := maxRangeResponses(total); requests != want {
		t.Fatalf("resource requests = %d, want exactly the ceiling %d", requests, want)
	}
	if _, statErr := os.Stat("out.bin"); !os.IsNotExist(statErr) {
		t.Fatalf("output file exists after a capped download, stat error = %v", statErr)
	}
}

func TestMaxRangeResponses(t *testing.T) {
	tests := []struct {
		name  string
		total int64
		want  int
	}{
		{name: "small file uses the floor", total: 1024, want: 64},
		{name: "exactly one chunk uses the floor", total: normalChunkSize, want: 64},
		{name: "large file scales with chunk count", total: 400 * normalChunkSize, want: 4 * 401},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := maxRangeResponses(tt.total); got != tt.want {
				t.Fatalf("maxRangeResponses(%d) = %d, want %d", tt.total, got, tt.want)
			}
		})
	}
}

// eofWithBytesBody hands back its payload together with io.EOF in one Read,
// which io.Reader permits and net/http bodies do when Content-Length is known,
// and fails on Close. It is the only shape that reaches the branch where a
// close error could mask the integrity error that ended the transfer.
type eofWithBytesBody struct {
	payload   []byte
	delivered bool
	closeErr  error
}

func (b *eofWithBytesBody) Read(p []byte) (int, error) {
	if b.delivered {
		return 0, io.EOF
	}
	b.delivered = true
	return copy(p, b.payload), io.EOF
}

func (b *eofWithBytesBody) Close() error { return b.closeErr }

// A body that stops short of the length it framed reports io.ErrUnexpectedEOF
// rather than a clean io.EOF, so it never reaches the short-body check. Returned
// raw it used to surface as `internal/file_io: cannot create file: unexpected
// EOF`, blaming the local disk for a truncated response.
func TestDownloadIMResourceToPathClassifiesTruncatedBody(t *testing.T) {
	payload := imRangePayload(probeChunkSize + 1024)
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_trunc/resources/file_trunc"):
			start, end, err := parseRangeHeader(req.Header.Get("Range"), int64(len(payload)))
			if err != nil {
				return nil, err
			}
			header := http.Header{
				"Content-Type":  []string{"application/octet-stream"},
				"Content-Range": []string{fmt.Sprintf("bytes %d-%d/%d", start, end, len(payload))},
			}
			header.Set("ETag", `"v1"`)
			// ContentLength promises the full slice; the body delivers less, which
			// is what net/http reports as io.ErrUnexpectedEOF.
			body := payload[start : end+1]
			resp := shortcutRawResponse(http.StatusPartialContent, body[:len(body)-16], header)
			resp.Body = io.NopCloser(&truncatedReader{remaining: body[:len(body)-16]})
			resp.ContentLength = int64(len(body))
			return resp, nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())
	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_trunc", "file_trunc", "file", "out.bin", true)
	requireDownloadProblem(t, err, "ended after", errs.SubtypeNetworkProtocol, false)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncation should be preserved as the cause, got %v", err)
	}
}

// truncatedReader hands back its bytes and then reports the framing violation
// net/http reports when a body is shorter than its Content-Length.
type truncatedReader struct {
	remaining []byte
}

func (r *truncatedReader) Read(p []byte) (int, error) {
	if len(r.remaining) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	n := copy(p, r.remaining)
	r.remaining = r.remaining[n:]
	return n, nil
}

// A failing Close must not replace the protocol error that ended the transfer.
// It used to: the close error propagated instead and the outer save wrapper
// re-classified it as internal/file_io, so the envelope no longer said why the
// download stopped.
func TestDownloadIMResourceToPathKeepsProtocolErrorWhenCloseFails(t *testing.T) {
	closeErr := errors.New("close failed")
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_close/resources/file_close"):
			// Content-Range promises 4 bytes; the body carries 16.
			return &http.Response{
				StatusCode: http.StatusPartialContent,
				Header: http.Header{
					"Content-Type":  []string{"application/octet-stream"},
					"Content-Range": []string{"bytes 0-3/4"},
				},
				Body:          &eofWithBytesBody{payload: []byte("overflow-payload"), closeErr: closeErr},
				ContentLength: 16,
			}, nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())
	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_close", "file_close", "file", "out.bin", true)
	requireDownloadProblem(t, err, "chunk overflow", errs.SubtypeNetworkProtocol, false)
	if !errors.Is(err, closeErr) {
		t.Fatalf("close error should be preserved as the cause, got %v", err)
	}
}

func parseRangeHeader(header string, totalSize int64) (int64, int64, error) {
	if !strings.HasPrefix(header, "bytes=") {
		return 0, 0, fmt.Errorf("unexpected range header: %q", header)
	}
	parts := strings.SplitN(strings.TrimPrefix(header, "bytes="), "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected range header: %q", header)
	}

	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse start: %w", err)
	}
	end, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("parse end: %w", err)
	}
	if start < 0 || end < start || start >= totalSize {
		return 0, 0, fmt.Errorf("invalid range bounds: %d-%d for size %d", start, end, totalSize)
	}
	if end >= totalSize {
		end = totalSize - 1
	}
	return start, end, nil
}

func TestUploadImageToIMSuccess(t *testing.T) {
	var gotBody string
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/images"):
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			gotBody = string(body)
			return shortcutJSONResponse(200, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"image_key": "img_uploaded"},
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())

	path := "demo.png"
	if err := os.WriteFile(path, []byte("png"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := uploadImageToIM(context.Background(), runtime, path, "message", "--image")
	if err != nil {
		t.Fatalf("uploadImageToIM() error = %v", err)
	}
	if got != "img_uploaded" {
		t.Fatalf("uploadImageToIM() = %q, want %q", got, "img_uploaded")
	}
	if !strings.Contains(gotBody, `name="image_type"`) || !strings.Contains(gotBody, "message") {
		t.Fatalf("uploadImageToIM() multipart body = %q, want image_type=message", gotBody)
	}
}

func TestUploadFileToIMSuccess(t *testing.T) {
	var gotBody string
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/files"):
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			gotBody = string(body)
			return shortcutJSONResponse(200, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"file_key": "file_uploaded"},
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())

	path := "demo.txt"
	if err := os.WriteFile(path, []byte("demo"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := uploadFileToIM(context.Background(), runtime, path, "stream", "1200", "--file")
	if err != nil {
		t.Fatalf("uploadFileToIM() error = %v", err)
	}
	if got != "file_uploaded" {
		t.Fatalf("uploadFileToIM() = %q, want %q", got, "file_uploaded")
	}
	if !strings.Contains(gotBody, `name="duration"`) || !strings.Contains(gotBody, "1200") {
		t.Fatalf("uploadFileToIM() multipart body = %q, want duration field", gotBody)
	}
	if !strings.Contains(gotBody, `name="file_type"`) || !strings.Contains(gotBody, "stream") {
		t.Fatalf("uploadFileToIM() multipart body = %q, want file_type field", gotBody)
	}
}

func TestUploadImageToIMSizeLimit(t *testing.T) {
	cmdutil.TestChdir(t, t.TempDir())
	path := "too-large.png"
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := f.Truncate(maxImageUploadSize + 1); err != nil {
		t.Fatalf("Truncate() error = %v", err)
	}
	f.Close()

	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("unexpected")
	}))
	_, err = uploadImageToIM(context.Background(), rt, path, "message", "--image")
	if err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("uploadImageToIM() error = %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--image" {
		t.Fatalf("uploadImageToIM() size error must carry Param=--image, got %T %+v", err, err)
	}
}

func TestUploadFileToIMSizeLimit(t *testing.T) {
	cmdutil.TestChdir(t, t.TempDir())
	path := "too-large.bin"
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := f.Truncate(maxFileUploadSize + 1); err != nil {
		t.Fatalf("Truncate() error = %v", err)
	}
	f.Close()

	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("unexpected")
	}))
	_, err = uploadFileToIM(context.Background(), rt, path, "stream", "", "--file")
	if err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("uploadFileToIM() error = %v", err)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Param != "--file" {
		t.Fatalf("uploadFileToIM() size error must carry Param=--file, got %T %+v", err, err)
	}
}

// TestResolveMediaContentMissingLocalFileIsValidation pins that a missing local
// media path is a typed validation error (bad --image input), not a network or
// internal error: the file never opened, so there is no transport failure to
// classify as network.
func TestResolveMediaContentMissingLocalFileIsValidation(t *testing.T) {
	runtime := &common.RuntimeContext{
		Factory: &cmdutil.Factory{
			FileIOProvider: fileio.GetProvider(),
			IOStreams: &cmdutil.IOStreams{
				Out:    &bytes.Buffer{},
				ErrOut: &bytes.Buffer{},
			},
		},
	}

	cmdutil.TestChdir(t, t.TempDir())

	missing := "missing.png"
	_, _, err := resolveMediaContent(context.Background(), runtime, "", missing, "", "", "", "")
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("missing local media file must be a validation error, got %T: %v", err, err)
	}
	if ve.Param != "--image" {
		t.Fatalf("missing local media file Param = %q, want --image", ve.Param)
	}
	if !strings.Contains(err.Error(), "cannot read file") {
		t.Fatalf("error should explain the unreadable file, got %v", err)
	}
}

func TestUploadFileToIMMissingLocalFileCarriesParam(t *testing.T) {
	runtime := &common.RuntimeContext{
		Factory: &cmdutil.Factory{
			FileIOProvider: fileio.GetProvider(),
			IOStreams: &cmdutil.IOStreams{
				Out:    &bytes.Buffer{},
				ErrOut: &bytes.Buffer{},
			},
		},
	}

	cmdutil.TestChdir(t, t.TempDir())

	_, err := uploadFileToIM(context.Background(), runtime, "missing.bin", "stream", "", "--file")
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("missing local file must be a validation error, got %T: %v", err, err)
	}
	if ve.Param != "--file" {
		t.Fatalf("missing local file Param = %q, want --file", ve.Param)
	}
}

func TestStartURLDownloadBlockedURLCarriesParam(t *testing.T) {
	_, _, err := startURLDownload(context.Background(), nil, "http://127.0.0.1/image.png", "--image")
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("blocked URL must be a validation error, got %T: %v", err, err)
	}
	if ve.Param != "--image" {
		t.Fatalf("blocked URL Param = %q, want --image", ve.Param)
	}
}

func TestStartURLDownloadUsesExternalRequestClass(t *testing.T) {
	platform := &shortcutPolicyDecorator{
		base: http.DefaultTransport,
		fn: func(req *http.Request) (*http.Response, error) {
			return shortcutRawResponse(http.StatusBadGateway, nil, nil), nil
		},
	}
	external := &shortcutPolicyDecorator{
		base: http.DefaultTransport,
		fn: func(req *http.Request) (*http.Response, error) {
			resp := shortcutRawResponse(http.StatusOK, []byte("image"), nil)
			resp.Request = req
			return resp, nil
		},
	}
	runtime := &common.RuntimeContext{
		Factory: &cmdutil.Factory{
			HttpClient: func() (*http.Client, error) {
				return &http.Client{
					Transport: internaltransport.NewHTTPPolicyRouter(platform, external),
				}, nil
			},
		},
	}

	resp, _, err := startURLDownload(
		context.Background(),
		runtime,
		"https://open.feishu.cn/presigned/image.png",
		"--image",
	)
	if err != nil {
		t.Fatalf("startURLDownload() error = %v, want external route", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(body); got != "image" {
		t.Fatalf("download body = %q, want external payload", got)
	}
}

// TestResolveLocalMediaImage verifies that resolveLocalMedia can upload an image
// via uploadImageToIM without double path validation.
func TestResolveLocalMediaImage(t *testing.T) {
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/open-apis/im/v1/images") {
			return shortcutJSONResponse(200, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"image_key": "img_via_resolve"},
			}), nil
		}
		return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
	}))

	cmdutil.TestChdir(t, t.TempDir())

	if err := os.WriteFile("test.png", []byte("png-data"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := resolveLocalMedia(context.Background(), runtime, mediaSpec{
		value: "./test.png", flagName: "--image", mediaType: "image",
		kind: mediaKindImage, maxSize: maxImageUploadSize, resultKey: "image_key",
	})
	if err != nil {
		t.Fatalf("resolveLocalMedia(image) error = %v", err)
	}
	if got != "img_via_resolve" {
		t.Fatalf("resolveLocalMedia(image) = %q, want %q", got, "img_via_resolve")
	}
}

// TestResolveLocalMediaFile verifies that resolveLocalMedia can upload a file
// via uploadFileToIM without double path validation.
func TestResolveLocalMediaFile(t *testing.T) {
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/open-apis/im/v1/files") {
			return shortcutJSONResponse(200, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"file_key": "file_via_resolve"},
			}), nil
		}
		return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
	}))

	cmdutil.TestChdir(t, t.TempDir())

	if err := os.WriteFile("test.txt", []byte("file-data"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	got, err := resolveLocalMedia(context.Background(), runtime, mediaSpec{
		value: "./test.txt", flagName: "--file", mediaType: "file",
		kind: mediaKindFile, maxSize: maxFileUploadSize, resultKey: "file_key",
	})
	if err != nil {
		t.Fatalf("resolveLocalMedia(file) error = %v", err)
	}
	if got != "file_via_resolve" {
		t.Fatalf("resolveLocalMedia(file) = %q, want %q", got, "file_via_resolve")
	}
}

// TestUploadFileToIMPreservesLocalFileName locks in that local uploads keep
// the basename of the caller-supplied path as the multipart file_name, so the
// URL-side fix for mediaBuffer cannot silently regress the local branch later.
func TestUploadFileToIMPreservesLocalFileName(t *testing.T) {
	var gotBody string
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/open-apis/im/v1/files") {
			body, err := io.ReadAll(req.Body)
			if err != nil {
				return nil, err
			}
			gotBody = string(body)
			return shortcutJSONResponse(200, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{"file_key": "file_uploaded"},
			}), nil
		}
		return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
	}))

	cmdutil.TestChdir(t, t.TempDir())

	localName := "Q1-meeting-notes.pdf"
	if err := os.WriteFile(localName, []byte("pdfdata"), 0600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := uploadFileToIM(context.Background(), runtime, "./"+localName, "pdf", "", "--file"); err != nil {
		t.Fatalf("uploadFileToIM() error = %v", err)
	}
	if !strings.Contains(gotBody, `name="file_name"`) || !strings.Contains(gotBody, localName) {
		t.Fatalf("upload body missing local filename %q; got: %q", localName, gotBody)
	}
}
