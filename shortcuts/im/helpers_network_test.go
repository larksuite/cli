// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
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

func TestResolveP2PChatID(t *testing.T) {
	var gotAuth string
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/chat_p2p/batch_query"):
			gotAuth = req.Header.Get("Authorization")
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
	if gotAuth != "Bearer tenant-token" {
		t.Fatalf("Authorization header = %q, want %q", gotAuth, "Bearer tenant-token")
	}
}

func TestResolveP2PChatIDNotFound(t *testing.T) {
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
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
