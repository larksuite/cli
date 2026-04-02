// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/json"
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

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
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
	_, size, err := downloadIMResourceToPath(context.Background(), runtime, "om_123", "file_123", "file", target)
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

	gotPath, size, err := downloadIMResourceToPath(context.Background(), runtime, "om_img", "img_123", "image", "image")
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

	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_403", "file_403", "file", "out.bin")
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
	_, size, err := downloadIMResourceToPath(context.Background(), runtime, "om_retry", "file_retry", "file", target)
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
	_, size, err := downloadIMResourceToPath(context.Background(), runtime, "om_retry2", "file_retry2", "file", target)
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
	_, _, err := downloadIMResourceToPath(ctx, runtime, "om_cancel", "file_cancel", "file", target)
	if err != context.Canceled {
		t.Fatalf("downloadIMResourceToPath() error = %v, want context.Canceled", err)
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
					}), nil
				default:
					return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
				}
			}))

			cmdutil.TestChdir(t, t.TempDir())
			target := filepath.Join("nested", "resource.bin")
			_, size, err := downloadIMResourceToPath(context.Background(), runtime, "om_range", "file_range", "file", target)
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
	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_bad", "file_bad", "file", "out.bin")
	if err == nil || !strings.Contains(err.Error(), "invalid Content-Range header") {
		t.Fatalf("downloadIMResourceToPath() error = %v", err)
	}
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
				}), nil
			}
			return shortcutRawResponse(http.StatusInternalServerError, []byte("chunk failed"), http.Header{"Content-Type": []string{"text/plain"}}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())

	target := "out.bin"
	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_miderr", "file_miderr", "file", target)
	if err == nil || !strings.Contains(err.Error(), "HTTP 500: chunk failed") {
		t.Fatalf("downloadIMResourceToPath() error = %v", err)
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
	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_overflow", "file_overflow", "file", target)
	if err == nil || !strings.Contains(err.Error(), "chunk overflow") {
		t.Fatalf("downloadIMResourceToPath() error = %v", err)
	}
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
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.String())
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())

	_, _, err := downloadIMResourceToPath(context.Background(), runtime, "om_short", "file_short", "file", "out.bin")
	if err == nil || !strings.Contains(err.Error(), "file size mismatch") {
		t.Fatalf("downloadIMResourceToPath() error = %v", err)
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

	got, err := uploadImageToIM(context.Background(), runtime, path, "message")
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

	got, err := uploadFileToIM(context.Background(), runtime, path, "stream", "1200")
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
	_, err = uploadImageToIM(context.Background(), rt, path, "message")
	if err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("uploadImageToIM() error = %v", err)
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
	_, err = uploadFileToIM(context.Background(), rt, path, "stream", "")
	if err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("uploadFileToIM() error = %v", err)
	}
}

func TestResolveMediaContentWrapsUploadError(t *testing.T) {
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
	if err == nil || !strings.Contains(err.Error(), "image upload failed") {
		t.Fatalf("resolveMediaContent() error = %v", err)
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
