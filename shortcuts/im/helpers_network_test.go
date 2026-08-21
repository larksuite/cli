// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/imcontract"
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

func TestMediaHelperMarksSendAndReplyPreuploadAsNonReplayable(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "image.png"), []byte("image-bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	cmdutil.TestChdir(t, tmp)

	for _, key := range []imcontract.ContractKey{"im +messages-send", "im +messages-reply"} {
		t.Run(string(key), func(t *testing.T) {
			runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/open-apis/im/v1/images") {
					return shortcutJSONResponse(200, map[string]any{
						"code": 0,
						"data": map[string]any{"image_key": "img_uploaded"},
					}), nil
				}
				return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
			}))
			contract, _ := imcontract.Lookup(key)
			session := imcontract.NewSession(contract)
			setRuntimeField(t, runtime, "contractSession", session)

			got, err := resolveOneMedia(context.Background(), runtime, mediaSpec{
				value: "image.png", flagName: "--image", mediaType: "image",
				msgType: "image", kind: mediaKindImage, maxSize: maxImageUploadSize, resultKey: "image_key",
			})
			if err != nil || got != "img_uploaded" {
				t.Fatalf("resolveOneMedia() = (%q, %v)", got, err)
			}
			session.ObserveRequest(map[string]any{"uuid": "stable-key"})
			session.RecordFact(imcontract.Fact{Kind: imcontract.FactWriteAttempted})
			unknown := errs.NewNetworkError(errs.SubtypeNetworkTransport, "send result unknown").WithRetryable()
			problem, _ := errs.ProblemOf(session.FinalizeError(unknown))
			if problem.Retryable ||
				problem.Hint != "The write result is unknown. Do not replay the original request." {
				t.Fatalf("problem = %#v", problem)
			}
		})
	}
}

func TestIMContractJSON5xxUsesHTTPStatusForReplayPolicy(t *testing.T) {
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return shortcutJSONResponse(http.StatusServiceUnavailable, map[string]any{
			"code": 123456,
			"msg":  "unclassified business error",
		}), nil
	}))
	contract, _ := imcontract.Lookup("im +messages-send")
	session := imcontract.NewSession(contract)
	setRuntimeField(t, runtime, "contractSession", session)

	_, err := runtime.DoWriteAPIJSONTyped(
		http.MethodPost,
		"/open-apis/im/v1/messages",
		nil,
		map[string]any{"uuid": "stable-key"},
	)
	if err == nil {
		t.Fatal("expected HTTP 503 error")
	}
	got := session.FinalizeError(err)
	problem, ok := errs.ProblemOf(got)
	if !ok || problem.Category != errs.CategoryNetwork ||
		problem.Subtype != errs.SubtypeNetworkServer ||
		problem.Code != http.StatusServiceUnavailable ||
		!problem.Retryable ||
		problem.Hint != "The write result is unknown. Retry only with the same idempotency key." {
		t.Fatalf("problem = %#v, err=%T %v", problem, got, got)
	}
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
	if gotHeaders.Get("Range") != "bytes=0-33554431" {
		t.Fatalf("initial Range header = %q, want bounded probe", gotHeaders.Get("Range"))
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

func TestDownloadIMResourceToPathResumesInterruptedPartWithETag(t *testing.T) {
	payload := []byte("abcdefgh")
	requests := 0
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		switch requests {
		case 1:
			if got := req.Header.Get("Range"); got != "bytes=0-33554431" {
				t.Fatalf("initial Range = %q, want bounded probe", got)
			}
			return &http.Response{
				StatusCode: http.StatusPartialContent,
				Header: http.Header{
					"Content-Type":  {"application/octet-stream"},
					"Content-Range": {"bytes 0-7/8"},
					"Etag":          {`"im-v1"`},
				},
				Body:          &imInterruptedBody{payload: payload[:3], err: io.ErrUnexpectedEOF},
				ContentLength: int64(len(payload)),
			}, nil
		case 2:
			if got := req.Header.Get("Range"); got != "bytes=3-7" {
				t.Fatalf("resume Range = %q, want bytes=3-7", got)
			}
			if got := req.Header.Get("If-Range"); got != `"im-v1"` {
				t.Fatalf("resume If-Range = %q", got)
			}
			return &http.Response{
				StatusCode: http.StatusPartialContent,
				Header: http.Header{
					"Content-Type":  {"application/octet-stream"},
					"Content-Range": {"bytes 3-7/8"},
					"Etag":          {`"im-v1"`},
				},
				Body:          io.NopCloser(bytes.NewReader(payload[3:])),
				ContentLength: 5,
			}, nil
		default:
			return nil, fmt.Errorf("unexpected request %d", requests)
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())
	_, size, err := downloadIMResourceToPath(context.Background(), runtime, "om_resume", "file_resume", "file", "resume.bin", true)
	if err != nil {
		t.Fatalf("downloadIMResourceToPath() error = %v", err)
	}
	got, readErr := os.ReadFile("resume.bin")
	if readErr != nil || !bytes.Equal(got, payload) || size != int64(len(payload)) {
		t.Fatalf("saved = %q, %d, %v; want %q", got, size, readErr, payload)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want initial part plus one resume", requests)
	}
}

func TestDownloadIMResourceToPathContinuesWithoutValidator(t *testing.T) {
	payload := []byte("abcdefgh")
	requests := 0
	runtime := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		switch requests {
		case 1:
			if got := req.Header.Get("Range"); got != "bytes=0-33554431" {
				t.Fatalf("initial Range = %q", got)
			}
			return shortcutRawResponse(http.StatusPartialContent, payload[:4], http.Header{
				"Content-Type":  {"application/octet-stream"},
				"Content-Range": {"bytes 0-3/8"},
			}), nil
		case 2:
			if got := req.Header.Get("Range"); got != "bytes=4-7" {
				t.Fatalf("second Range = %q, want bytes=4-7", got)
			}
			return shortcutRawResponse(http.StatusPartialContent, payload[4:], http.Header{
				"Content-Type":  {"application/octet-stream"},
				"Content-Range": {"bytes 4-7/8"},
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request %d", requests)
		}
	}))

	cmdutil.TestChdir(t, t.TempDir())
	if err := os.WriteFile("atomic.bin", []byte("original"), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	_, size, err := downloadIMResourceToPath(context.Background(), runtime, "om_atomic", "file_atomic", "file", "atomic.bin", true)
	if err != nil || size != int64(len(payload)) {
		t.Fatalf("download = %d bytes, %v", size, err)
	}
	got, readErr := os.ReadFile("atomic.bin")
	if readErr != nil || !bytes.Equal(got, payload) {
		t.Fatalf("target = %q, %v; want %q", got, readErr, payload)
	}
	if requests != 2 {
		t.Fatalf("requests = %d, want two validated parts", requests)
	}
}

type imInterruptedBody struct {
	payload []byte
	err     error
}

func (b *imInterruptedBody) Read(p []byte) (int, error) {
	if len(b.payload) == 0 {
		err := b.err
		b.err = nil
		if err == nil {
			err = io.EOF
		}
		return 0, err
	}
	n := copy(p, b.payload)
	b.payload = b.payload[n:]
	if len(b.payload) == 0 && b.err != nil {
		err := b.err
		b.err = nil
		return n, err
	}
	return n, nil
}

func (b *imInterruptedBody) Close() error { return nil }

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
