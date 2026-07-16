// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	internaltransport "github.com/larksuite/cli/internal/transport"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestPrepareLocalDocResourcesXMLImageAndSource(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, map[string]string{
		"diagram.png": localDocResourcePNG(t, 100, 80),
		"report.pdf":  "pdf-data",
	})
	content := `<p>before</p><img path="@diagram.png" alt="diagram" width="50" height="40" align="right" scale="0.5"/><source path="@report.pdf" name="report.pdf"></source>`

	got, resources, err := prepareLocalDocResources(runtime, "xml", content)
	if err != nil {
		t.Fatalf("prepareLocalDocResources() error: %v", err)
	}
	if len(resources) != 2 {
		t.Fatalf("resources len = %d, want 2: %#v", len(resources), resources)
	}
	if resources[0].Kind != localDocResourceImage || resources[0].Path != "diagram.png" {
		t.Fatalf("image resource = %#v", resources[0])
	}
	if resources[1].Kind != localDocResourceFile || resources[1].Path != "report.pdf" {
		t.Fatalf("file resource = %#v", resources[1])
	}
	if strings.Contains(got, "@diagram.png") || strings.Contains(got, "@report.pdf") {
		t.Fatalf("rewritten content leaks local path: %s", got)
	}
	for _, want := range []string{resources[0].Marker, resources[1].Marker, `width="100"`, `height="80"`, `align="right"`, `scale="0.500000"`} {
		if !strings.Contains(got, want) {
			t.Fatalf("rewritten content missing %q: %s", want, got)
		}
	}
}

func TestPrepareLocalDocResourcesRejectsUndecodableLocalImage(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, map[string]string{"fake.png": "<html>not an image</html>"})
	_, _, err := prepareLocalDocResources(runtime, "xml", `<img path="@fake.png"/>`)
	problem, ok := errs.ProblemOf(err)
	var validationErr *errs.ValidationError
	if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument ||
		!errors.As(err, &validationErr) || validationErr.Param != "path" {
		t.Fatalf("error=%T %v problem=%#v validation=%#v", err, err, problem, validationErr)
	}
}

func TestPrepareLocalDocResourcesXMLRemoteImage(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, nil)
	got, resources, err := prepareLocalDocResources(runtime, "xml", `<p>before</p><img href="http://93.184.216.34/photo.png" alt="remote" width="50"/>`)
	if err != nil {
		t.Fatalf("prepareLocalDocResources() error: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("resources = %d, want 1", len(resources))
	}
	resource := resources[0]
	if resource.RemoteURL != "http://93.184.216.34/photo.png" {
		t.Fatalf("RemoteURL = %q", resource.RemoteURL)
	}
	if resource.Path != "" || resource.Size != 0 {
		t.Fatalf("remote resource materialized during preparation: %#v", resource)
	}
	if strings.Contains(got, `href=`) || !strings.Contains(got, `path="`+resource.Marker+`"`) || !strings.Contains(got, `caption="remote"`) {
		t.Fatalf("prepared content = %q", got)
	}
}

func TestPrepareLocalDocResourcesXMLRemoteImageAcceptsBareAmpersandsInHref(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, nil)
	content := `<img href="https://93.184.216.34/photo.png?x=1&image_size=large&token=a%26b" caption="remote"/>`
	got, resources, err := prepareLocalDocResources(runtime, "xml", content)
	if err != nil {
		t.Fatalf("prepareLocalDocResources() error: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("resources = %d, want 1", len(resources))
	}
	if want := "https://93.184.216.34/photo.png?x=1&image_size=large&token=a%26b"; resources[0].RemoteURL != want {
		t.Fatalf("RemoteURL = %q, want %q", resources[0].RemoteURL, want)
	}
	if !strings.Contains(got, `path="`+resources[0].Marker+`"`) || strings.Contains(got, `href=`) {
		t.Fatalf("prepared content = %q", got)
	}
}

func TestPrepareLocalDocResourcesXMLRemoteImagePreservesValidEntities(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, nil)
	got, resources, err := prepareLocalDocResources(
		runtime,
		"xml",
		`<img href="https://93.184.216.34/photo.png?x=1&amp;y=2&#38;z=3" caption="A &amp; B"/>`,
	)
	if err != nil {
		t.Fatalf("prepareLocalDocResources() error: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("resources = %d, want 1", len(resources))
	}
	if want := "https://93.184.216.34/photo.png?x=1&y=2&z=3"; resources[0].RemoteURL != want {
		t.Fatalf("RemoteURL = %q, want %q", resources[0].RemoteURL, want)
	}
	if !strings.Contains(got, `caption="A &amp; B"`) {
		t.Fatalf("prepared content = %q", got)
	}
}

func TestPrepareLocalDocResourcesXMLRemoteImageDoesNotRelaxOtherAttributes(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, nil)
	_, _, err := prepareLocalDocResources(runtime, "xml", `<img href="https://93.184.216.34/photo.png?x=1&y=2" caption="A & B"/>`)
	if err == nil {
		t.Fatal("prepareLocalDocResources() error = nil")
	}
	if !strings.Contains(err.Error(), "invalid character entity") {
		t.Fatalf("prepareLocalDocResources() error = %v", err)
	}
}

func TestPrepareLocalDocResourcesRemoteImageRejectsConflicts(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, nil)
	for _, content := range []string{
		`<img href="https://93.184.216.34/photo.png" src="token"/>`,
		`<img href="file:///tmp/photo.png"/>`,
		`<img href="https://user:secret@93.184.216.34/photo.png"/>`,
	} {
		if _, _, err := prepareLocalDocResources(runtime, "xml", content); err == nil {
			t.Fatalf("prepareLocalDocResources(%q) error = nil", content)
		}
	}
}

func TestUploadRemoteDocImagesRecordsIndividualDownloadFailure(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	factory, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("remote-image-partial-download"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, docsTestConfigWithAppID("remote-image-partial-download"), factory, core.AsUser)
	_, resources, err := prepareLocalDocResources(
		runtime,
		"xml",
		`<img href="https://93.184.216.34/one.png"/><img href="https://93.184.216.34/two.png"/>`,
	)
	if err != nil {
		t.Fatalf("prepareLocalDocResources: %v", err)
	}

	originalDownload := downloadRemoteDocImage
	t.Cleanup(func() { downloadRemoteDocImage = originalDownload })
	var downloadMu sync.Mutex
	downloadCalls := make(map[int]int)
	downloadRemoteDocImage = func(_ *common.RuntimeContext, _ string, occurrence int) (remoteDocImageDownload, error) {
		downloadMu.Lock()
		downloadCalls[occurrence]++
		downloadMu.Unlock()
		if occurrence == 2 {
			return remoteDocImageDownload{}, errors.New("second download failed")
		}
		payload := []byte(localDocResourcePNG(t, 20, 10))
		return remoteDocImageDownload{Content: payload, FileName: "image.png", Width: 20, Height: 10}, nil
	}
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"file_token": "file_one"}},
	})
	outcomes := []*localDocResourceOutcome{
		{Resource: resources[0], BlockID: "blk_one", Status: "pending"},
		{Resource: resources[1], BlockID: "blk_two", Status: "pending"},
	}
	uploadLocalDocResources(runtime, "doxcn_partial_download", outcomes)
	if outcomes[0].Status != "uploaded" || outcomes[0].FileToken != "file_one" {
		t.Fatalf("first outcome = %#v", outcomes[0])
	}
	if outcomes[1].Status != "upload_failed" || outcomes[1].Err == nil || !strings.Contains(outcomes[1].Err.Error(), "second download failed") {
		t.Fatalf("second outcome = %#v", outcomes[1])
	}
	downloadMu.Lock()
	secondDownloadCalls := downloadCalls[2]
	downloadMu.Unlock()
	if secondDownloadCalls != 1 {
		t.Fatalf("non-retryable second download calls = %d, want 1", secondDownloadCalls)
	}
}

func TestUploadRemoteDocImageRetriesRetryableDownload(t *testing.T) {
	factory, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("remote-image-download-retry"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, docsTestConfigWithAppID("remote-image-download-retry"), factory, core.AsUser)
	payload := []byte(localDocResourcePNG(t, 20, 10))
	originalDownload := downloadRemoteDocImage
	downloadCalls := 0
	downloadRemoteDocImage = func(_ *common.RuntimeContext, _ string, _ int) (remoteDocImageDownload, error) {
		downloadCalls++
		if downloadCalls < remoteDocImageDownloadAttempts {
			return remoteDocImageDownload{}, errs.NewNetworkError(errs.SubtypeNetworkTransport, "temporary download failure").WithRetryable()
		}
		return remoteDocImageDownload{Content: payload, FileName: "image.png", Width: 20, Height: 10}, nil
	}
	t.Cleanup(func() { downloadRemoteDocImage = originalDownload })

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"file_token": "file_after_download_retry"}},
	})
	var waits []time.Duration
	originalWait := waitLocalDocResourceRequest
	waitLocalDocResourceRequest = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}
	t.Cleanup(func() { waitLocalDocResourceRequest = originalWait })

	outcome := &localDocResourceOutcome{
		Resource: localDocResource{Occurrence: 1, Kind: localDocResourceImage, RemoteURL: "https://93.184.216.34/retry.png"},
		BlockID:  "blk_download_retry",
		Status:   "pending",
	}
	uploadLocalDocResources(runtime, "doxcn_download_retry", []*localDocResourceOutcome{outcome})
	if outcome.Status != "uploaded" || outcome.FileToken != "file_after_download_retry" {
		t.Fatalf("outcome = %#v", outcome)
	}
	if downloadCalls != remoteDocImageDownloadAttempts {
		t.Fatalf("download calls = %d, want %d", downloadCalls, remoteDocImageDownloadAttempts)
	}
	assertLocalDocResourceRetryWaits(t, waits, 2)
}

func TestUploadRemoteDocImageRetriesRetryableUploadWithoutRedownloading(t *testing.T) {
	factory, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("remote-image-upload-retry"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, docsTestConfigWithAppID("remote-image-upload-retry"), factory, core.AsUser)
	payload := []byte(localDocResourcePNG(t, 20, 10))
	originalDownload := downloadRemoteDocImage
	downloadCalls := 0
	downloadRemoteDocImage = func(_ *common.RuntimeContext, _ string, _ int) (remoteDocImageDownload, error) {
		downloadCalls++
		return remoteDocImageDownload{Content: payload, FileName: "image.png", Width: 20, Height: 10}, nil
	}
	t.Cleanup(func() { downloadRemoteDocImage = originalDownload })

	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Status: http.StatusServiceUnavailable,
		Body:   map[string]interface{}{"code": 0, "msg": "temporary upstream failure"},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"file_token": "file_after_upload_retry"}},
	})
	var waits []time.Duration
	originalWait := waitLocalDocResourceRequest
	waitLocalDocResourceRequest = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}
	t.Cleanup(func() { waitLocalDocResourceRequest = originalWait })

	outcome := &localDocResourceOutcome{
		Resource: localDocResource{Occurrence: 1, Kind: localDocResourceImage, RemoteURL: "https://93.184.216.34/retry.png"},
		BlockID:  "blk_upload_retry",
		Status:   "pending",
	}
	uploadLocalDocResources(runtime, "doxcn_upload_retry", []*localDocResourceOutcome{outcome})
	if outcome.Status != "uploaded" || outcome.FileToken != "file_after_upload_retry" {
		t.Fatalf("outcome = %#v", outcome)
	}
	if downloadCalls != 1 {
		t.Fatalf("download calls = %d, want 1 when only upload is retried", downloadCalls)
	}
	assertLocalDocResourceRetryWaits(t, waits, 1)
}

func TestApplyRemoteDocImageDownloadNormalizesPresentationWithoutRetainingContent(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, nil)
	content, resources, err := prepareLocalDocResources(runtime, "xml", `<img href="https://93.184.216.34/photo.png" width="50" align="right"/>`)
	if err != nil {
		t.Fatalf("prepareLocalDocResources: %v", err)
	}
	payload := []byte(localDocResourcePNG(t, 200, 100))
	resource := resources[0]
	if err := applyRemoteDocImageDownload(&resource, remoteDocImageDownload{Content: payload, FileName: "image.png", Width: 200, Height: 100}); err != nil {
		t.Fatalf("applyRemoteDocImageDownload: %v", err)
	}
	if resource.ImageWidth != 200 || resource.ImageHeight != 100 || !resource.HasScale || resource.ImageScale != 0.25 {
		t.Fatalf("downloaded resource = %#v", resource)
	}
	if resource.FileName != "image.png" || resource.Size != int64(len(payload)) || len(resource.Content) != 0 {
		t.Fatalf("downloaded resource metadata = %#v", resource)
	}
	if !strings.Contains(content, resource.Marker) || strings.Contains(content, `width=`) || strings.Contains(content, `align=`) {
		t.Fatalf("placeholder content = %q, want presentation deferred until upload", content)
	}
}

func TestDownloadRemoteDocImageContentBuffersSupportedImage(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, nil)
	payload := []byte(localDocResourcePNG(t, 30, 20))

	originalDo := doRemoteDocImageRequest
	t.Cleanup(func() { doRemoteDocImageRequest = originalDo })
	doRemoteDocImageRequest = func(_ remoteDocImageHTTPDoer, req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"image/png"}},
			Body:          io.NopCloser(bytes.NewReader(payload)),
			ContentLength: int64(len(payload)),
			Request:       req,
		}, nil
	}

	download, err := downloadRemoteDocImageContent(runtime, "https://93.184.216.34/photo.png", 1)
	if err != nil {
		t.Fatalf("downloadRemoteDocImageContent: %v", err)
	}
	if download.FileName != "image.png" || download.Width != 30 || download.Height != 20 || !bytes.Equal(download.Content, payload) {
		t.Fatalf("download result = %#v", download)
	}
}

type remoteDocImageProbeBody struct {
	reads  int
	closed bool
}

func (b *remoteDocImageProbeBody) Read([]byte) (int, error) {
	b.reads++
	return 0, io.EOF
}

func (b *remoteDocImageProbeBody) Close() error {
	b.closed = true
	return nil
}

func TestProbeRemoteDocImageDownloadDoesNotReadImageBody(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, nil)
	originalDo := doRemoteDocImageRequest
	t.Cleanup(func() { doRemoteDocImageRequest = originalDo })
	body := &remoteDocImageProbeBody{}
	method := ""
	requestRange := ""
	doRemoteDocImageRequest = func(_ remoteDocImageHTTPDoer, req *http.Request) (*http.Response, error) {
		method = req.Method
		requestRange = req.Header.Get("Range")
		return &http.Response{
			StatusCode:    http.StatusOK,
			ContentLength: 1024,
			Header:        http.Header{"Content-Type": []string{"image/png"}},
			Body:          body,
		}, nil
	}

	if err := probeRemoteDocImageDownload(runtime, "https://93.184.216.34/image.png", 1); err != nil {
		t.Fatalf("probeRemoteDocImageDownload: %v", err)
	}
	if method != http.MethodGet || requestRange != "bytes=0-0" || body.reads != 0 || !body.closed {
		t.Fatalf("probe method=%q range=%q reads=%d closed=%v, want ranged GET with closed unread body", method, requestRange, body.reads, body.closed)
	}
}

func TestDownloadRemoteDocImageContentRejectsNonImage(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, nil)

	originalDo := doRemoteDocImageRequest
	t.Cleanup(func() { doRemoteDocImageRequest = originalDo })
	doRemoteDocImageRequest = func(_ remoteDocImageHTTPDoer, req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader("not an image")),
			Request:    req,
		}, nil
	}

	_, err := downloadRemoteDocImageContent(runtime, "http://93.184.216.34/not-image", 1)
	if err == nil {
		t.Fatal("downloadRemoteDocImageContent() error = nil")
	}
	problem, ok := errs.ProblemOf(err)
	var validationErr *errs.ValidationError
	if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument ||
		!errors.As(err, &validationErr) || validationErr.Param != "href" {
		t.Fatalf("problem = %#v, validation = %#v, %v", problem, validationErr, ok)
	}
}

func TestDownloadRemoteDocImageContentRejectsDeclaredOversize(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, nil)

	originalDo := doRemoteDocImageRequest
	t.Cleanup(func() { doRemoteDocImageRequest = originalDo })
	doRemoteDocImageRequest = func(_ remoteDocImageHTTPDoer, req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"image/png"}},
			Body:          io.NopCloser(strings.NewReader("")),
			ContentLength: remoteDocImageMaxBytes + 1,
			Request:       req,
		}, nil
	}

	_, err := downloadRemoteDocImageContent(runtime, "https://93.184.216.34/oversize.png", 1)
	if err == nil || !strings.Contains(err.Error(), "20MiB") {
		t.Fatalf("downloadRemoteDocImageContent() error = %v", err)
	}
	problem, ok := errs.ProblemOf(err)
	var validationErr *errs.ValidationError
	if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument ||
		!errors.As(err, &validationErr) || validationErr.Param != "href" {
		t.Fatalf("problem = %#v, validation = %#v, ok = %v", problem, validationErr, ok)
	}
}

func TestDownloadRemoteDocImageHTTPStatusRetryMetadata(t *testing.T) {
	tests := []struct {
		status        int
		wantSubtype   errs.Subtype
		wantRetryable bool
	}{
		{status: http.StatusTooManyRequests, wantSubtype: errs.SubtypeRateLimit, wantRetryable: true},
		{status: http.StatusServiceUnavailable, wantSubtype: errs.SubtypeNetworkServer, wantRetryable: true},
		{status: http.StatusNotFound, wantSubtype: errs.SubtypeNetworkTransport},
	}
	for _, test := range tests {
		t.Run(http.StatusText(test.status), func(t *testing.T) {
			runtime := newLocalDocResourceTestRuntime(t, nil)

			originalDo := doRemoteDocImageRequest
			t.Cleanup(func() { doRemoteDocImageRequest = originalDo })
			doRemoteDocImageRequest = func(_ remoteDocImageHTTPDoer, req *http.Request) (*http.Response, error) {
				return &http.Response{
					StatusCode: test.status,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("temporary")),
					Request:    req,
				}, nil
			}

			_, err := downloadRemoteDocImageContent(runtime, "https://93.184.216.34/image.png", 1)
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != test.wantSubtype ||
				problem.Code != test.status || problem.Retryable != test.wantRetryable {
				t.Fatalf("error=%T %v problem=%#v", err, err, problem)
			}
		})
	}
}

type cannedRemoteImageTransport struct {
	base    http.RoundTripper
	payload []byte
}

func (t *cannedRemoteImageTransport) BaseRoundTripper() http.RoundTripper {
	return t.base
}

func (t *cannedRemoteImageTransport) WithBaseRoundTripper(base http.RoundTripper) http.RoundTripper {
	return &cannedRemoteImageTransport{base: base, payload: t.payload}
}

func (t *cannedRemoteImageTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode:    http.StatusOK,
		Header:        http.Header{"Content-Type": []string{"image/png"}},
		Body:          io.NopCloser(bytes.NewReader(t.payload)),
		ContentLength: int64(len(t.payload)),
		Request:       req,
	}, nil
}

func TestDownloadRemoteDocImageContentUsesExternalPolicyBranch(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, nil)
	runtime.Factory.HttpClient = func() (*http.Client, error) {
		external := &cannedRemoteImageTransport{base: http.DefaultTransport, payload: []byte(localDocResourcePNG(t, 2, 1))}
		return &http.Client{Transport: internaltransport.NewHTTPPolicyRouter(http.DefaultTransport, external)}, nil
	}
	download, err := downloadRemoteDocImageContent(runtime, "https://93.184.216.34/image.png", 1)
	if err != nil {
		t.Fatalf("downloadRemoteDocImageContent() error = %v", err)
	}
	if download.FileName != "image.png" || len(download.Content) <= 3 {
		t.Fatalf("download result = %#v", download)
	}
}

func TestDownloadRemoteDocImageContentRejectsInvalidImageBody(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, nil)

	originalDo := doRemoteDocImageRequest
	t.Cleanup(func() { doRemoteDocImageRequest = originalDo })
	doRemoteDocImageRequest = func(_ remoteDocImageHTTPDoer, req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"image/png"}},
			Body:          io.NopCloser(strings.NewReader("<html>not an image</html>")),
			ContentLength: 25,
			Request:       req,
		}, nil
	}

	_, err := downloadRemoteDocImageContent(runtime, "https://93.184.216.34/not-really.png", 1)
	problem, ok := errs.ProblemOf(err)
	var validationErr *errs.ValidationError
	if !ok || problem.Category != errs.CategoryValidation || problem.Subtype != errs.SubtypeInvalidArgument ||
		!errors.As(err, &validationErr) || validationErr.Param != "href" {
		t.Fatalf("error = %T %v, problem=%#v validation=%#v", err, err, problem, validationErr)
	}
}

func TestDownloadRemoteDocImageContentSupportsWebPDimensions(t *testing.T) {
	const encodedWebP = "UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA=="
	payload, err := base64.StdEncoding.DecodeString(encodedWebP)
	if err != nil {
		t.Fatalf("decode WebP fixture: %v", err)
	}
	runtime := newLocalDocResourceTestRuntime(t, nil)

	originalDo := doRemoteDocImageRequest
	t.Cleanup(func() { doRemoteDocImageRequest = originalDo })
	doRemoteDocImageRequest = func(_ remoteDocImageHTTPDoer, req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"image/webp"}},
			Body:          io.NopCloser(bytes.NewReader(payload)),
			ContentLength: int64(len(payload)),
			Request:       req,
		}, nil
	}

	download, err := downloadRemoteDocImageContent(runtime, "https://93.184.216.34/image.webp", 1)
	if err != nil {
		t.Fatalf("downloadRemoteDocImageContent() error = %v", err)
	}
	if download.Width <= 0 || download.Height <= 0 || download.FileName != "image.webp" || !bytes.Equal(download.Content, payload) {
		t.Fatalf("WebP download = %#v", download)
	}
}

type remoteImageErrorReader struct{ err error }

func (r remoteImageErrorReader) Read([]byte) (int, error) { return 0, r.err }

func TestDownloadRemoteDocImageContentClassifiesResponseReadFailureAsNetwork(t *testing.T) {
	readErr := errors.New("connection reset while streaming")
	runtime := newLocalDocResourceTestRuntime(t, nil)

	originalDo := doRemoteDocImageRequest
	t.Cleanup(func() { doRemoteDocImageRequest = originalDo })
	doRemoteDocImageRequest = func(_ remoteDocImageHTTPDoer, req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode:    http.StatusOK,
			Header:        http.Header{"Content-Type": []string{"image/png"}},
			Body:          io.NopCloser(remoteImageErrorReader{err: readErr}),
			ContentLength: -1,
			Request:       req,
		}, nil
	}

	_, err := downloadRemoteDocImageContent(runtime, "https://93.184.216.34/image.png", 1)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Subtype != errs.SubtypeNetworkTransport || !problem.Retryable || !errors.Is(err, readErr) {
		t.Fatalf("error = %T %v, problem=%#v", err, err, problem)
	}
	var validationErr *errs.ValidationError
	if errors.As(err, &validationErr) {
		t.Fatalf("response read failure was classified as validation: %#v", validationErr)
	}
}

func TestRemoteDocImageNetworkErrorDoesNotRetryCanceledContext(t *testing.T) {
	err := remoteDocImageNetworkError(context.Canceled, 1, "was interrupted")
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryNetwork || problem.Retryable || !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %T %v, problem=%#v", err, err, problem)
	}
}

func TestRemoteImageErrorsAndDryRunRedactURLCredentials(t *testing.T) {
	const rawURL = "https://alice-sensitive:password@93.184.216.34/image.png?token=secret-value#frag-sensitive"
	requestErr := &url.Error{Op: "Get", URL: rawURL, Err: errors.New("dial failed")}
	got := remoteDocImageNetworkError(requestErr, 1, "request failed")
	for _, secret := range []string{"alice-sensitive", "password", "secret-value", "frag-sensitive"} {
		if strings.Contains(got.Error(), secret) {
			t.Errorf("network error leaks %q: %v", secret, got)
		}
	}

	dry := appendRemoteDocImageDownloadsDryRun(common.NewDryRunAPI(), []localDocResource{{Occurrence: 1, RemoteURL: rawURL}})
	raw, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}
	for _, secret := range []string{"alice-sensitive", "password", "secret-value", "frag-sensitive"} {
		if strings.Contains(string(raw), secret) {
			t.Errorf("dry run leaks %q: %s", secret, raw)
		}
	}
	if !strings.Contains(string(raw), `https://93.184.216.34/image.png`) {
		t.Fatalf("dry run lost redacted endpoint identity: %s", raw)
	}
}

func TestPrepareLocalDocResourcesNormalizesImageDimensionsAndScale(t *testing.T) {
	tests := []struct {
		name          string
		nativeWidth   int
		nativeHeight  int
		attrs         string
		wantWidth     int
		wantHeight    int
		wantScale     float64
		wantHasScale  bool
		wantScaleText string
	}{
		{
			name:         "intrinsic dimensions without model display size",
			nativeWidth:  100,
			nativeHeight: 80,
			wantWidth:    100,
			wantHeight:   80,
		},
		{
			name:          "model width becomes scale",
			nativeWidth:   100,
			nativeHeight:  80,
			attrs:         ` width="50"`,
			wantWidth:     100,
			wantHeight:    80,
			wantScale:     0.5,
			wantHasScale:  true,
			wantScaleText: `scale="0.500000"`,
		},
		{
			name:          "model height becomes scale",
			nativeWidth:   100,
			nativeHeight:  80,
			attrs:         ` height="20"`,
			wantWidth:     100,
			wantHeight:    80,
			wantScale:     0.25,
			wantHasScale:  true,
			wantScaleText: `scale="0.250000"`,
		},
		{
			name:          "model width wins when both dimensions exist",
			nativeWidth:   100,
			nativeHeight:  80,
			attrs:         ` width="25" height="70"`,
			wantWidth:     100,
			wantHeight:    80,
			wantScale:     0.25,
			wantHasScale:  true,
			wantScaleText: `scale="0.250000"`,
		},
		{
			name:          "explicit scale wins over model dimensions",
			nativeWidth:   100,
			nativeHeight:  80,
			attrs:         ` width="50" scale="0.75"`,
			wantWidth:     100,
			wantHeight:    80,
			wantScale:     0.75,
			wantHasScale:  true,
			wantScaleText: `scale="0.750000"`,
		},
		{
			name:          "percentage width becomes scale",
			nativeWidth:   100,
			nativeHeight:  80,
			attrs:         ` width="80%"`,
			wantWidth:     100,
			wantHeight:    80,
			wantScale:     0.8,
			wantHasScale:  true,
			wantScaleText: `scale="0.800000"`,
		},
		{
			name:         "invalid model dimensions are ignored",
			nativeWidth:  100,
			nativeHeight: 80,
			attrs:        ` width="invalid" height="0" scale="-1"`,
			wantWidth:    100,
			wantHeight:   80,
		},
		{
			name:          "wide image is capped below page width",
			nativeWidth:   1200,
			nativeHeight:  800,
			wantWidth:     1200,
			wantHeight:    800,
			wantScale:     0.849999,
			wantHasScale:  true,
			wantScaleText: `scale="0.849999"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := newLocalDocResourceTestRuntime(t, map[string]string{
				"diagram.png": localDocResourcePNG(t, tt.nativeWidth, tt.nativeHeight),
			})
			content := `<img path="@diagram.png"` + tt.attrs + `/>`

			got, resources, err := prepareLocalDocResources(runtime, "xml", content)
			if err != nil {
				t.Fatalf("prepareLocalDocResources() error: %v", err)
			}
			if len(resources) != 1 {
				t.Fatalf("resources len = %d, want 1: %#v", len(resources), resources)
			}
			resource := resources[0]
			if resource.ImageWidth != tt.wantWidth || resource.ImageHeight != tt.wantHeight {
				t.Fatalf("intrinsic dimensions = %dx%d, want %dx%d; content=%s", resource.ImageWidth, resource.ImageHeight, tt.wantWidth, tt.wantHeight, got)
			}
			if resource.HasScale != tt.wantHasScale || resource.ImageScale != tt.wantScale {
				t.Fatalf("scale = %v (present=%v), want %v (present=%v); content=%s", resource.ImageScale, resource.HasScale, tt.wantScale, tt.wantHasScale, got)
			}
			for _, want := range []string{
				fmt.Sprintf(`width="%d"`, tt.wantWidth),
				fmt.Sprintf(`height="%d"`, tt.wantHeight),
			} {
				if !strings.Contains(got, want) {
					t.Fatalf("rewritten content missing %q: %s", want, got)
				}
			}
			if tt.wantHasScale {
				if !strings.Contains(got, tt.wantScaleText) {
					t.Fatalf("rewritten content missing %q: %s", tt.wantScaleText, got)
				}
			} else if strings.Contains(got, ` scale=`) {
				t.Fatalf("rewritten content unexpectedly contains scale: %s", got)
			}
		})
	}
}

// BUG_MAP #1: Markdown alt is persisted by docx_engine through caption, then
// exported back as Markdown alt. Sending only the SDK-only alt attribute loses
// the text when the placeholder reaches the engine.
func TestPrepareLocalDocResourcesMarkdownAltUsesCaption(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, map[string]string{"diagram.png": localDocResourcePNG(t, 2, 1)})

	got, resources, err := prepareLocalDocResources(runtime, "markdown", `![architecture diagram](@diagram.png)`)
	if err != nil {
		t.Fatalf("prepareLocalDocResources() error: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("resources len = %d, want 1", len(resources))
	}
	if !strings.Contains(got, `caption="architecture diagram"`) {
		t.Fatalf("Markdown alt must be mapped to engine caption, got: %s", got)
	}
	if strings.Contains(got, ` alt=`) {
		t.Fatalf("rewritten Markdown image must not rely on non-persisted alt: %s", got)
	}
}

func TestPrepareLocalDocResourcesMarkdownTitleConsumesQuotedClosingParen(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, map[string]string{"diagram.png": localDocResourcePNG(t, 2, 1)})
	content := `before ![architecture diagram](@diagram.png "v2 (final)") after`

	got, resources, err := prepareLocalDocResources(runtime, "markdown", content)
	if err != nil {
		t.Fatalf("prepareLocalDocResources() error: %v", err)
	}
	if len(resources) != 1 {
		t.Fatalf("resources len = %d, want 1", len(resources))
	}
	want := `before <img path="` + resources[0].Marker + `" caption="architecture diagram" title="v2 (final)"/> after`
	if got != want {
		t.Fatalf("rewritten Markdown image = %q, want %q", got, want)
	}
}

func TestParseMarkdownImageDestinationRejectsUnclosedTitle(t *testing.T) {
	content := `![diagram](@diagram.png "v2 (final)) trailing`
	if image, ok := parseMarkdownImageAt(content, 0); ok {
		t.Fatalf("parseMarkdownImageAt() = %#v, true; want invalid unclosed title", image)
	}
}

// BUG_MAP #2: image-looking text in inert Markdown contexts must remain text;
// otherwise the CLI plans an upload for a block the Markdown parser never
// creates and reports a partial failure after the document write succeeded.
func TestPrepareLocalDocResourcesMarkdownIgnoresInertContexts(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, map[string]string{"diagram.png": localDocResourcePNG(t, 2, 1)})
	content := "<!-- ![comment](@diagram.png) -->\n    ![indented code](@diagram.png)\n"

	got, resources, err := prepareLocalDocResources(runtime, "markdown", content)
	if err != nil {
		t.Fatalf("prepareLocalDocResources() error: %v", err)
	}
	if got != content {
		t.Fatalf("inert Markdown was rewritten:\n got: %q\nwant: %q", got, content)
	}
	if len(resources) != 0 {
		t.Fatalf("inert Markdown planned %d resources, want 0: %#v", len(resources), resources)
	}
}

func TestPrepareLocalDocResourcesMarkdownListIndentIsRelativeToList(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, map[string]string{"diagram.png": localDocResourcePNG(t, 2, 1)})
	content := "- report:\n\n    ![chart](@diagram.png)\n"

	got, resources, err := prepareLocalDocResources(runtime, "markdown", content)
	if err != nil {
		t.Fatalf("prepareLocalDocResources() error: %v", err)
	}
	if len(resources) != 1 || !strings.Contains(got, resources[0].Marker) {
		t.Fatalf("four-space list content was not rewritten: got=%q resources=%#v", got, resources)
	}
	if strings.Contains(got, "@diagram.png") {
		t.Fatalf("rewritten list content leaked local path: %q", got)
	}
}

func TestPrepareLocalDocResourcesMarkdownListIndentedCodeRemainsInert(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, map[string]string{"diagram.png": localDocResourcePNG(t, 2, 1)})
	content := "- report:\n\n      ![code](@diagram.png)\n"

	got, resources, err := prepareLocalDocResources(runtime, "markdown", content)
	if err != nil {
		t.Fatalf("prepareLocalDocResources() error: %v", err)
	}
	if got != content || len(resources) != 0 {
		t.Fatalf("six-space list code was rewritten: got=%q resources=%#v", got, resources)
	}
}

func TestPrepareLocalDocResourcesMarkdownThematicBreakDoesNotOpenList(t *testing.T) {
	for _, thematicBreak := range []string{"* * *", "- - -"} {
		t.Run(thematicBreak, func(t *testing.T) {
			runtime := newLocalDocResourceTestRuntime(t, map[string]string{"diagram.png": localDocResourcePNG(t, 2, 1)})
			content := thematicBreak + "\n\n    ![code](@diagram.png)\n"

			got, resources, err := prepareLocalDocResources(runtime, "markdown", content)
			if err != nil {
				t.Fatalf("prepareLocalDocResources() error: %v", err)
			}
			if got != content || len(resources) != 0 {
				t.Fatalf("indented code after thematic break was rewritten: got=%q resources=%#v", got, resources)
			}
		})
	}
}

// BUG_MAP #3: internal correlation markers are an implementation detail and
// must never escape in a partial-failure result, even if no document ID was
// returned and cleanup therefore cannot run.
func TestFinalizeLocalDocResourcesScrubsMarkerWhenDocumentIDMissing(t *testing.T) {
	marker := "@lcli_img_0123456789abcdef0123456789abcdef"
	factory, stdout, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("local-resource-marker-scrub"))
	runtime := common.TestNewRuntimeContextForAPI(
		context.Background(),
		&cobra.Command{Use: "docs +create"},
		docsTestConfigWithAppID("local-resource-marker-scrub"),
		factory,
		core.AsUser,
	)
	block := map[string]interface{}{
		"block_id":    "blk_placeholder",
		"block_type":  "image",
		"block_token": marker,
	}
	data := map[string]interface{}{
		"document": map[string]interface{}{
			"new_blocks": []interface{}{block},
		},
	}

	err := finalizeLocalDocResources(runtime, "", data, []localDocResource{{
		Occurrence: 1,
		Kind:       localDocResourceImage,
		Marker:     marker,
	}})
	if err == nil {
		t.Fatal("finalizeLocalDocResources() error = nil, want partial failure")
	}
	if token := common.GetString(block, "block_token"); token != "" {
		t.Fatalf("partial-failure response leaked marker %q", token)
	}
	var envelope struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal partial-failure response: %v\nstdout: %s", err, stdout.String())
	}
	if envelope.OK {
		t.Fatalf("partial failure reported ok:true: %s", stdout.String())
	}
	for _, internalField := range []string{"summary", "items"} {
		if _, exposed := envelope.Data[internalField]; exposed {
			t.Fatalf("partial-failure response exposed internal field %q: %s", internalField, stdout.String())
		}
	}
}

func TestNewLocalDocResourceRejectsTraversalAndSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(work, 0o700); err != nil {
		t.Fatalf("MkdirAll(work): %v", err)
	}
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatalf("MkdirAll(outside): %v", err)
	}
	if err := os.WriteFile(filepath.Join(outside, "secret.png"), []byte("secret"), 0o600); err != nil {
		t.Fatalf("WriteFile(secret): %v", err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.png"), filepath.Join(work, "escape.png")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	cmdutil.TestChdir(t, work)
	factory, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("local-resource-path"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, docsTestConfigWithAppID("local-resource-path"), factory, core.AsUser)

	for _, path := range []string{"@../outside/secret.png", "@escape.png"} {
		if _, err := newLocalDocResource(runtime, localDocResourceImage, path, 1); err == nil {
			t.Fatalf("newLocalDocResource(%q) error = nil, want unsafe path rejection", path)
		}
	}
}

func TestCorrelateLocalDocResourcesMatchesTypeAndBlockID(t *testing.T) {
	marker := "@lcli_file_0123456789abcdef0123456789abcdef"
	block := map[string]interface{}{
		"block_id":    "blk_file",
		"block_type":  "file",
		"block_token": marker,
	}
	data := map[string]interface{}{
		"document": map[string]interface{}{"new_blocks": []interface{}{block}},
	}

	outcomes := correlateLocalDocResources(data, []localDocResource{{Occurrence: 1, Kind: localDocResourceFile, Marker: marker}})
	if len(outcomes) != 1 {
		t.Fatalf("outcomes len = %d, want 1", len(outcomes))
	}
	if outcomes[0].Status != "pending" || outcomes[0].BlockID != "blk_file" || !outcomes[0].SafeToCleanup {
		t.Fatalf("outcome = %#v", outcomes[0])
	}
}

func TestCorrelateLocalDocResourcesTypeMismatchIsNotSafeToCleanup(t *testing.T) {
	marker := "@lcli_img_0123456789abcdef0123456789abcdef"
	outcomes := correlateLocalDocResources(map[string]interface{}{
		"document": map[string]interface{}{"new_blocks": []interface{}{
			map[string]interface{}{"block_id": "blk_mismatch", "block_type": "file", "block_token": marker},
		}},
	}, []localDocResource{{Occurrence: 1, Kind: localDocResourceImage, Marker: marker}})

	if len(outcomes) != 1 || outcomes[0].Status != "correlation_failed" || outcomes[0].SafeToCleanup {
		t.Fatalf("mismatched block outcome = %#v", outcomes)
	}
	if len(outcomes[0].CleanupBlockIDs) != 0 || outcomes[0].CleanupStatus != "skipped_ambiguous" {
		t.Fatalf("mismatched block was scheduled for cleanup: %#v", outcomes[0])
	}
}

func TestCorrelateLocalDocResourcesUnknownMarkerDisablesCleanup(t *testing.T) {
	expectedMarker := "@lcli_img_0123456789abcdef0123456789abcdef"
	unknownMarker := "@lcli_file_fedcba9876543210fedcba9876543210"
	outcomes := correlateLocalDocResources(map[string]interface{}{
		"document": map[string]interface{}{"new_blocks": []interface{}{
			map[string]interface{}{"block_id": "blk_expected", "block_type": "image", "block_token": expectedMarker},
			map[string]interface{}{"block_id": "blk_unknown", "block_type": "file", "block_token": unknownMarker},
		}},
	}, []localDocResource{{Occurrence: 1, Kind: localDocResourceImage, Marker: expectedMarker}})

	if len(outcomes) != 1 || outcomes[0].Status != "correlation_failed" || outcomes[0].SafeToCleanup {
		t.Fatalf("unknown marker outcome = %#v", outcomes)
	}
	if outcomes[0].CleanupStatus != "skipped_ambiguous" {
		t.Fatalf("unknown marker cleanup status = %q, want skipped_ambiguous", outcomes[0].CleanupStatus)
	}
	for _, blockID := range outcomes[0].CleanupBlockIDs {
		if blockID == "blk_unknown" {
			t.Fatalf("unknown marker block was scheduled for cleanup: %#v", outcomes[0])
		}
	}
}

func TestBuildLocalDocResourceBatchUpdatePreservesImagePresentation(t *testing.T) {
	image := &localDocResourceOutcome{
		Resource: localDocResource{
			Kind:        localDocResourceImage,
			ImageWidth:  640,
			ImageHeight: 480,
			ImageAlign:  "right",
			ImageScale:  0.5,
			HasScale:    true,
		},
		BlockID:   "blk_image",
		FileToken: "file_image",
	}
	file := &localDocResourceOutcome{
		Resource:  localDocResource{Kind: localDocResourceFile},
		BlockID:   "blk_file",
		FileToken: "file_attachment",
	}

	body := buildLocalDocResourceBatchUpdate([]*localDocResourceOutcome{image, file})
	requests, _ := body["requests"].([]interface{})
	if len(requests) != 2 {
		t.Fatalf("requests len = %d, want 2: %#v", len(requests), body)
	}
	imageReq, _ := requests[0].(map[string]interface{})
	replaceImage, _ := imageReq["replace_image"].(map[string]interface{})
	for key, want := range map[string]interface{}{
		"token":  "file_image",
		"width":  640,
		"height": 480,
		"align":  alignMap["right"],
		"scale":  0.5,
	} {
		if got := replaceImage[key]; got != want {
			t.Fatalf("replace_image[%s] = %#v, want %#v; body=%#v", key, got, want, body)
		}
	}
	fileReq, _ := requests[1].(map[string]interface{})
	replaceFile, _ := fileReq["replace_file"].(map[string]interface{})
	if got := replaceFile["token"]; got != "file_attachment" {
		t.Fatalf("replace_file token = %#v, want file_attachment", got)
	}
}

func TestPrepareLocalDocResourcesSourceUsesExplicitUploadName(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, map[string]string{"report.pdf": "pdf-data"})

	got, resources, err := prepareLocalDocResources(runtime, "xml", `<source path="@report.pdf" name="  自定义报告.pdf  "/>`)
	if err != nil {
		t.Fatalf("prepareLocalDocResources() error: %v", err)
	}
	if len(resources) != 1 || resources[0].FileName != "自定义报告.pdf" {
		t.Fatalf("resources = %#v, want trimmed explicit file name", resources)
	}
	if !strings.Contains(got, `name="自定义报告.pdf"`) {
		t.Fatalf("rewritten source did not preserve trimmed name: %s", got)
	}
}

func TestPrepareLocalDocResourcesRejectsInvalidSourceName(t *testing.T) {
	for _, name := range []string{"   ", "../secret.pdf", `folder\secret.pdf`} {
		t.Run(strings.ReplaceAll(name, "/", "_"), func(t *testing.T) {
			runtime := newLocalDocResourceTestRuntime(t, map[string]string{"report.pdf": "pdf-data"})
			_, _, err := prepareLocalDocResources(runtime, "xml", `<source path="@report.pdf" name="`+name+`"/>`)
			if err == nil {
				t.Fatalf("source name %q was accepted", name)
			}
		})
	}
}

func TestUploadLocalDocResourceUsesExplicitSourceName(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile("report.pdf", []byte("pdf-data"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	factory, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("local-source-name"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, docsTestConfigWithAppID("local-source-name"), factory, core.AsUser)
	_, resources, err := prepareLocalDocResources(runtime, "xml", `<source path="@report.pdf" name="自定义报告.pdf"/>`)
	if err != nil {
		t.Fatalf("prepareLocalDocResources: %v", err)
	}
	upload := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"file_token": "file_custom_name"},
		},
	}
	reg.Register(upload)
	outcome := &localDocResourceOutcome{Resource: resources[0], BlockID: "blk_file", Status: "pending"}
	uploadLocalDocResources(runtime, "doxcn_source_name", []*localDocResourceOutcome{outcome})
	if outcome.Status != "uploaded" || outcome.FileToken != "file_custom_name" {
		t.Fatalf("outcome = %#v", outcome)
	}
	body := string(upload.CapturedBody)
	if !strings.Contains(body, "自定义报告.pdf") || strings.Contains(body, "\r\n\r\nreport.pdf\r\n") {
		t.Fatalf("upload body did not use explicit source name: %s", body)
	}
}

func TestUploadRemoteDocImagesUsesBoundedConcurrency(t *testing.T) {
	factory, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("remote-image-concurrent-upload"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, docsTestConfigWithAppID("remote-image-concurrent-upload"), factory, core.AsUser)
	payload := []byte(localDocResourcePNG(t, 20, 10))
	originalDownload := downloadRemoteDocImage
	t.Cleanup(func() { downloadRemoteDocImage = originalDownload })
	var downloadMu sync.Mutex
	downloads := 0
	downloadRemoteDocImage = func(_ *common.RuntimeContext, _ string, _ int) (remoteDocImageDownload, error) {
		downloadMu.Lock()
		downloads++
		downloadMu.Unlock()
		return remoteDocImageDownload{Content: payload, FileName: "remote.png", Width: 20, Height: 10}, nil
	}

	releaseUploads := make(chan struct{})
	reachedConcurrentUpload := make(chan struct{})
	var concurrentOnce sync.Once
	var uploadMu sync.Mutex
	activeUploads := 0
	maxActiveUploads := 0
	upload := &httpmock.Stub{
		Method:   "POST",
		URL:      "/open-apis/drive/v1/medias/upload_all",
		Reusable: true,
		OnMatch: func(*http.Request) {
			uploadMu.Lock()
			activeUploads++
			if activeUploads > maxActiveUploads {
				maxActiveUploads = activeUploads
			}
			if activeUploads >= 2 {
				concurrentOnce.Do(func() { close(reachedConcurrentUpload) })
			}
			uploadMu.Unlock()
			<-releaseUploads
			uploadMu.Lock()
			activeUploads--
			uploadMu.Unlock()
		},
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"file_token": "file_remote_image"},
		},
	}
	reg.Register(upload)
	outcomes := make([]*localDocResourceOutcome, remoteDocImageUploadConcurrency+2)
	for i := range outcomes {
		occurrence := i + 1
		outcomes[i] = &localDocResourceOutcome{
			Resource: localDocResource{
				Occurrence: occurrence,
				Kind:       localDocResourceImage,
				RemoteURL:  fmt.Sprintf("https://93.184.216.34/%d.png", occurrence),
				Content:    payload,
			},
			BlockID: fmt.Sprintf("blk_remote_image_%d", occurrence),
			Status:  "pending",
		}
	}

	done := make(chan struct{})
	go func() {
		uploadLocalDocResources(runtime, "doxcn_remote_image", outcomes)
		close(done)
	}()
	select {
	case <-reachedConcurrentUpload:
		close(releaseUploads)
	case <-time.After(2 * time.Second):
		close(releaseUploads)
		<-done
		t.Fatalf("remote image uploads did not overlap; max active uploads = %d", maxActiveUploads)
	}
	<-done
	downloadMu.Lock()
	gotDownloads := downloads
	downloadMu.Unlock()
	if gotDownloads != len(outcomes) {
		t.Fatalf("remote image downloads = %d, want %d", gotDownloads, len(outcomes))
	}
	if maxActiveUploads < 2 {
		t.Fatalf("max active remote image uploads = %d, want at least 2", maxActiveUploads)
	}
	if maxActiveUploads > remoteDocImageUploadConcurrency {
		t.Fatalf("max active remote image uploads = %d, concurrency limit = %d", maxActiveUploads, remoteDocImageUploadConcurrency)
	}
	for _, outcome := range outcomes {
		if outcome.Status != "uploaded" || outcome.FileToken != "file_remote_image" {
			t.Fatalf("outcome = %#v", outcome)
		}
		if len(outcome.Resource.Content) != 0 {
			t.Fatalf("remote image #%d retained %d buffered bytes after upload", outcome.Resource.Occurrence, len(outcome.Resource.Content))
		}
	}
}

func TestUploadLocalDocResourcesRetriesConflictAndSerializes(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	for _, name := range []string{"first.png", "second.png"} {
		if err := os.WriteFile(name, []byte(name), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}
	factory, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("local-upload-serial"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, docsTestConfigWithAppID("local-upload-serial"), factory, core.AsUser)
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/drive/v1/medias/upload_all", Body: map[string]interface{}{"code": localDocResourceUploadConflictCode, "msg": "material transaction conflict"}})
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/drive/v1/medias/upload_all", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"file_token": "file_first"}}})
	reg.Register(&httpmock.Stub{Method: "POST", URL: "/open-apis/drive/v1/medias/upload_all", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"file_token": "file_second"}}})
	outcomes := []*localDocResourceOutcome{
		{Resource: localDocResource{Occurrence: 1, Kind: localDocResourceImage, Path: "first.png", FileName: "first.png", Size: int64(len("first.png"))}, BlockID: "blk_first", Status: "pending"},
		{Resource: localDocResource{Occurrence: 2, Kind: localDocResourceImage, Path: "second.png", FileName: "second.png", Size: int64(len("second.png"))}, BlockID: "blk_second", Status: "pending"},
	}
	var waits []time.Duration
	originalWait := waitLocalDocResourceRequest
	waitLocalDocResourceRequest = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}
	t.Cleanup(func() { waitLocalDocResourceRequest = originalWait })
	uploadLocalDocResources(runtime, "doxcn_upload_serial", outcomes)
	if outcomes[0].FileToken != "file_first" || outcomes[1].FileToken != "file_second" {
		t.Fatalf("outcomes = %#v", outcomes)
	}
	if len(waits) != 2 {
		t.Fatalf("upload pacing waits = %#v", waits)
	}
	retryWaits := waits[:1]
	assertLocalDocResourceRetryWaits(t, retryWaits, 1)
	if waits[1] != localDocResourceUploadInterval {
		t.Fatalf("serial upload pacing wait = %v, want %v", waits[1], localDocResourceUploadInterval)
	}
}

func assertLocalDocResourceRetryWaits(t *testing.T, waits []time.Duration, want int) {
	t.Helper()
	if len(waits) != want {
		t.Fatalf("retry waits = %#v, want %d", waits, want)
	}
	for attempt, got := range waits {
		base := localDocResourceUploadInterval * time.Duration(1<<attempt)
		max := base + base/4
		if got < base || got > max {
			t.Fatalf("retry wait[%d] = %v, want in [%v, %v]", attempt, got, base, max)
		}
	}
}

func TestPrepareLocalDocResourcesRejectsDuplicateAttributes(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, map[string]string{
		"diagram.png": localDocResourcePNG(t, 2, 1),
		"secret.png":  localDocResourcePNG(t, 3, 1),
	})
	_, resources, err := prepareLocalDocResources(runtime, "xml", `<img path="@diagram.png" PATH="@secret.png"/>`)
	if err == nil {
		t.Fatal("duplicate path attributes were accepted")
	}
	if len(resources) != 0 {
		t.Fatalf("duplicate attributes planned resources: %#v", resources)
	}
	if strings.Contains(err.Error(), "secret.png") {
		t.Fatalf("duplicate-attribute error leaked the second local path: %v", err)
	}
}

func TestCorrelateLocalDocResourcesRejectsDuplicateBlockID(t *testing.T) {
	markers := []string{
		"@lcli_img_0123456789abcdef0123456789abcdef",
		"@lcli_img_fedcba9876543210fedcba9876543210",
	}
	blocks := []interface{}{
		map[string]interface{}{"block_id": "blk_shared", "block_type": "image", "block_token": markers[0]},
		map[string]interface{}{"block_id": "blk_shared", "block_type": "image", "block_token": markers[1]},
	}
	outcomes := correlateLocalDocResources(map[string]interface{}{
		"document": map[string]interface{}{"new_blocks": blocks},
	}, []localDocResource{
		{Occurrence: 1, Kind: localDocResourceImage, Marker: markers[0]},
		{Occurrence: 2, Kind: localDocResourceImage, Marker: markers[1]},
	})
	cleanupCount := 0
	for _, outcome := range outcomes {
		if outcome.Status != "correlation_failed" {
			t.Fatalf("duplicate block outcome = %#v", outcome)
		}
		cleanupCount += len(outcome.CleanupBlockIDs)
	}
	if cleanupCount != 1 {
		t.Fatalf("cleanup block IDs = %d, want one deduplicated delete", cleanupCount)
	}
}

func TestPrepareLocalDocResourcesMarkdownIgnoresNestedFencesAndRawHTML(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, map[string]string{"diagram.png": localDocResourcePNG(t, 2, 1)})
	content := strings.Join([]string{
		"> ```xml",
		"> <img path=\"@diagram.png\"/>",
		"> ```",
		"- ```markdown",
		"  ![list code](@diagram.png)",
		"  ```",
		"<pre>",
		"```",
		"<img path=\"@diagram.png\"/>",
		"```",
		"</pre>",
		"<script>const sample = '<img path=\"@diagram.png\"/>';</script>",
		"<style>/* ![style](@diagram.png) */</style>",
		"<textarea><source path=\"@diagram.png\"/></textarea>",
		`\<img path="@diagram.png"/>`,
	}, "\n")

	got, resources, err := prepareLocalDocResources(runtime, "markdown", content)
	if err != nil {
		t.Fatalf("prepareLocalDocResources: %v", err)
	}
	if got != content || len(resources) != 0 {
		t.Fatalf("inert Markdown changed: resources=%#v\n got=%q\nwant=%q", resources, got, content)
	}
}

func TestPrepareLocalDocResourcesXMLBackticksAreNotInert(t *testing.T) {
	runtime := newLocalDocResourceTestRuntime(t, map[string]string{"diagram.png": localDocResourcePNG(t, 2, 1)})
	got, resources, err := prepareLocalDocResources(runtime, "xml", "`<img path=\"@diagram.png\"/>`")
	if err != nil {
		t.Fatalf("prepareLocalDocResources: %v", err)
	}
	if len(resources) != 1 || !strings.Contains(got, resources[0].Marker) {
		t.Fatalf("XML backticks incorrectly hid resource: got=%q resources=%#v", got, resources)
	}
}

func TestMarkdownUnescapePreservesNonPunctuationBackslashes(t *testing.T) {
	if got, want := unescapeMarkdownText(`C:\temp\photo \*draft\* \\`), `C:\temp\photo *draft* \`; got != want {
		t.Fatalf("unescapeMarkdownText() = %q, want %q", got, want)
	}
	image, ok := parseMarkdownImageAt(`![x](@images\photo.png)`, 0)
	if !ok || image.Destination != `@images\photo.png` {
		t.Fatalf("parsed destination = %q, ok=%v", image.Destination, ok)
	}
}

func TestBindLocalDocResourcesBatchesTwentyAndPaces(t *testing.T) {
	factory, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("local-bind-batch"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, docsTestConfigWithAppID("local-bind-batch"), factory, core.AsUser)
	first := &httpmock.Stub{Method: "PATCH", URL: "/open-apis/docx/v1/documents/doxcn_batch/blocks/batch_update", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"document_revision_id": 1}}}
	second := &httpmock.Stub{Method: "PATCH", URL: "/open-apis/docx/v1/documents/doxcn_batch/blocks/batch_update", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"document_revision_id": 2}}}
	reg.Register(first)
	reg.Register(second)
	outcomes := make([]*localDocResourceOutcome, 21)
	for i := range outcomes {
		outcomes[i] = &localDocResourceOutcome{
			Resource:  localDocResource{Occurrence: i + 1, Kind: localDocResourceImage},
			BlockID:   fmt.Sprintf("blk_%d", i),
			FileToken: fmt.Sprintf("file_%d", i),
			Status:    "uploaded",
		}
	}
	var waits []time.Duration
	originalWait := waitLocalDocResourceRequest
	waitLocalDocResourceRequest = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}
	t.Cleanup(func() { waitLocalDocResourceRequest = originalWait })

	revision, revisionKnown := bindLocalDocResources(runtime, "doxcn_batch", outcomes)
	if !revisionKnown || revision != int64(2) {
		t.Fatalf("revision = %#v, want 2", revision)
	}
	if got := requestCountFromLocalDocBatchBody(t, first.CapturedBody); got != 20 {
		t.Fatalf("first batch size = %d, want 20", got)
	}
	if got := requestCountFromLocalDocBatchBody(t, second.CapturedBody); got != 1 {
		t.Fatalf("second batch size = %d, want 1", got)
	}
	if len(waits) != 1 || waits[0] != localDocResourceBindInterval {
		t.Fatalf("bind pacing waits = %#v", waits)
	}
}

func TestNormalizeLocalDocResourceRevision(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value interface{}
		want  interface{}
	}{
		{name: "integer", value: 7, want: int64(7)},
		{name: "float integer", value: float64(8), want: int64(8)},
		{name: "json number", value: json.Number("9"), want: int64(9)},
		{name: "numeric string", value: "10", want: int64(10)},
		{name: "negative sentinel", value: -1},
		{name: "fraction", value: 1.5},
		{name: "invalid string", value: "latest"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeLocalDocResourceRevision(tt.value); got != tt.want {
				t.Fatalf("normalizeLocalDocResourceRevision(%#v) = %#v, want %#v", tt.value, got, tt.want)
			}
		})
	}
}

func TestBindLocalDocResourceRetryUsesStableTokenAndBackoff(t *testing.T) {
	factory, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("local-bind-retry"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, docsTestConfigWithAppID("local-bind-retry"), factory, core.AsUser)
	var clientTokens []string
	for _, stub := range []*httpmock.Stub{
		{Method: "PATCH", URL: "/open-apis/docx/v1/documents/doxcn_retry/blocks/batch_update", Body: map[string]interface{}{"code": localDocResourceUploadRateLimitCode, "msg": "rate limited"}, OnMatch: func(req *http.Request) { clientTokens = append(clientTokens, req.URL.Query().Get("client_token")) }},
		{Method: "GET", URL: "/open-apis/docx/v1/documents/doxcn_retry/blocks/blk_retry", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"block": map[string]interface{}{"image": map[string]interface{}{"token": ""}}}}},
		{Method: "PATCH", URL: "/open-apis/docx/v1/documents/doxcn_retry/blocks/batch_update", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{}}, OnMatch: func(req *http.Request) { clientTokens = append(clientTokens, req.URL.Query().Get("client_token")) }},
	} {
		reg.Register(stub)
	}
	var waits []time.Duration
	originalWait := waitLocalDocResourceRequest
	waitLocalDocResourceRequest = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}
	t.Cleanup(func() { waitLocalDocResourceRequest = originalWait })
	outcome := &localDocResourceOutcome{Resource: localDocResource{Kind: localDocResourceImage}, BlockID: "blk_retry", FileToken: "file_retry", Status: "uploaded"}
	_, _ = bindLocalDocResourceChunk(runtime, "doxcn_retry", []*localDocResourceOutcome{outcome})
	if outcome.Status != "bound" || len(clientTokens) != 2 || clientTokens[0] == "" || clientTokens[0] != clientTokens[1] {
		t.Fatalf("outcome=%#v client_tokens=%#v", outcome, clientTokens)
	}
	if len(waits) != 1 || waits[0] != localDocResourceBindInterval {
		t.Fatalf("retry waits = %#v", waits)
	}
}

func TestBindLocalDocResourceAmbiguousSuccessDropsStaleRevision(t *testing.T) {
	factory, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("local-bind-ambiguous-success"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, docsTestConfigWithAppID("local-bind-ambiguous-success"), factory, core.AsUser)
	reg.Register(&httpmock.Stub{Method: "PATCH", URL: "/open-apis/docx/v1/documents/doxcn_ambiguous/blocks/batch_update", Body: map[string]interface{}{"code": localDocResourceUploadRateLimitCode, "msg": "rate limited"}})
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/docx/v1/documents/doxcn_ambiguous/blocks/blk_ambiguous", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"block": map[string]interface{}{"image": map[string]interface{}{"token": "file_ambiguous"}}}}})
	outcome := &localDocResourceOutcome{Resource: localDocResource{Kind: localDocResourceImage}, BlockID: "blk_ambiguous", FileToken: "file_ambiguous", Status: "uploaded"}
	revision, revisionKnown := bindLocalDocResourceChunk(runtime, "doxcn_ambiguous", []*localDocResourceOutcome{outcome})
	if revision != nil || revisionKnown || outcome.Status != "bound" {
		t.Fatalf("revision=%#v known=%v outcome=%#v", revision, revisionKnown, outcome)
	}
}

func TestCleanupLocalDocResourcePlaceholdersRevalidatesToken(t *testing.T) {
	tests := []struct {
		name         string
		getStatus    int
		block        map[string]interface{}
		wantDelete   bool
		wantStatus   string
		wantCleanup  string
		wantKnown    bool
		wantRevision interface{}
	}{
		{name: "empty", block: map[string]interface{}{"block_type": 27, "image": map[string]interface{}{"token": ""}}, wantDelete: true, wantStatus: "bind_failed", wantCleanup: "succeeded", wantKnown: true, wantRevision: int64(8)},
		{name: "ours", block: map[string]interface{}{"block_type": 27, "image": map[string]interface{}{"token": "file_ours"}}, wantStatus: "bound", wantCleanup: "not_needed"},
		{name: "other", block: map[string]interface{}{"block_type": 27, "image": map[string]interface{}{"token": "file_other"}}, wantStatus: "bind_conflict", wantCleanup: "skipped_conflict"},
		{name: "other_kind_token", block: map[string]interface{}{"block_type": 27, "image": map[string]interface{}{"token": ""}, "file": map[string]interface{}{"token": "file_hidden"}}, wantStatus: "bind_conflict", wantCleanup: "skipped_conflict"},
		{name: "type_mismatch", block: map[string]interface{}{"block_type": 23, "file": map[string]interface{}{"token": ""}}, wantStatus: "bind_ambiguous", wantCleanup: "skipped_ambiguous"},
		{name: "type_unknown", block: map[string]interface{}{"block_type": 999, "image": map[string]interface{}{"token": ""}}, wantStatus: "bind_ambiguous", wantCleanup: "skipped_ambiguous"},
		{name: "get_fail", getStatus: 503, wantStatus: "bind_ambiguous", wantCleanup: "skipped_ambiguous"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			factory, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("local-cleanup-"+tt.name))
			runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, docsTestConfigWithAppID("local-cleanup-"+tt.name), factory, core.AsUser)
			getStub := &httpmock.Stub{Method: "GET", URL: "/open-apis/docx/v1/documents/doxcn_cleanup/blocks/blk_cleanup"}
			if tt.getStatus != 0 {
				getStub.Status = tt.getStatus
				getStub.RawBody = []byte("temporary")
			} else {
				getStub.Body = map[string]interface{}{"code": 0, "data": map[string]interface{}{"block": tt.block}}
			}
			reg.Register(getStub)
			deleteCalls := 0
			var deleteStub *httpmock.Stub
			if tt.wantDelete {
				deleteStub = &httpmock.Stub{Method: "PUT", URL: "/open-apis/docs_ai/v1/documents/doxcn_cleanup", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"document": map[string]interface{}{"revision_id": 8}}}, OnMatch: func(*http.Request) { deleteCalls++ }}
				reg.Register(deleteStub)
			}
			outcome := &localDocResourceOutcome{
				Resource:        localDocResource{Occurrence: 1, Kind: localDocResourceImage},
				BlockID:         "blk_cleanup",
				CleanupBlockIDs: []string{"blk_cleanup"},
				FileToken:       "file_ours",
				Status:          "bind_failed",
				CleanupStatus:   "pending",
				SafeToCleanup:   true,
			}
			revision, revisionKnown := cleanupLocalDocResourcePlaceholders(runtime, "doxcn_cleanup", []*localDocResourceOutcome{outcome}, float64(7))
			if got := deleteCalls > 0; got != tt.wantDelete {
				t.Fatalf("delete called=%v, want %v", got, tt.wantDelete)
			}
			if revisionKnown != tt.wantKnown || revision != tt.wantRevision {
				t.Fatalf("revision=%#v known=%v, want revision=%#v known=%v", revision, revisionKnown, tt.wantRevision, tt.wantKnown)
			}
			if outcome.Status != tt.wantStatus || outcome.CleanupStatus != tt.wantCleanup || outcome.SafeToCleanup {
				t.Fatalf("outcome=%#v, want status=%s cleanup=%s safe=false", outcome, tt.wantStatus, tt.wantCleanup)
			}
			if tt.wantDelete {
				var body map[string]interface{}
				if err := json.Unmarshal(deleteStub.CapturedBody, &body); err != nil {
					t.Fatalf("decode cleanup body: %v", err)
				}
				if body["revision_id"] != float64(7) {
					t.Fatalf("cleanup revision_id=%#v, want 7", body["revision_id"])
				}
			}
		})
	}
}

func TestCleanupLocalDocResourcePlaceholdersRequiresKnownRevision(t *testing.T) {
	factory, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("local-cleanup-no-revision"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, docsTestConfigWithAppID("local-cleanup-no-revision"), factory, core.AsUser)
	outcome := &localDocResourceOutcome{Resource: localDocResource{Kind: localDocResourceImage}, CleanupBlockIDs: []string{"blk_cleanup"}, Status: "upload_failed", CleanupStatus: "pending", SafeToCleanup: true}
	revision, revisionKnown := cleanupLocalDocResourcePlaceholders(runtime, "doxcn_cleanup", []*localDocResourceOutcome{outcome}, nil)
	if revision != nil || revisionKnown || outcome.CleanupStatus != "skipped_ambiguous" || outcome.SafeToCleanup {
		t.Fatalf("revision=%#v known=%v outcome=%#v", revision, revisionKnown, outcome)
	}
}

func TestCleanupLocalDocResourcePlaceholdersPreservesServiceFailure(t *testing.T) {
	factory, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("local-cleanup-service-failure"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, docsTestConfigWithAppID("local-cleanup-service-failure"), factory, core.AsUser)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docx/v1/documents/doxcn_cleanup/blocks/blk_cleanup",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"block": map[string]interface{}{
			"block_type": 27,
			"image":      map[string]interface{}{"token": ""},
		}}},
	})
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/docs_ai/v1/documents/doxcn_cleanup",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"result":   "failed",
			"warnings": []interface{}{"target block cannot be deleted"},
		}},
	})
	outcome := &localDocResourceOutcome{
		Resource:        localDocResource{Occurrence: 1, Kind: localDocResourceImage},
		BlockID:         "blk_cleanup",
		CleanupBlockIDs: []string{"blk_cleanup"},
		Status:          "upload_failed",
		CleanupStatus:   "pending",
		SafeToCleanup:   true,
	}

	revision, revisionKnown := cleanupLocalDocResourcePlaceholders(runtime, "doxcn_cleanup", []*localDocResourceOutcome{outcome}, float64(7))

	if revision != nil || revisionKnown || outcome.CleanupStatus != "failed" || outcome.SafeToCleanup {
		t.Fatalf("revision=%#v known=%v outcome=%#v", revision, revisionKnown, outcome)
	}
	if len(outcome.ServerWarnings) != 1 || outcome.ServerWarnings[0] != "target block cannot be deleted" {
		t.Fatalf("server warnings = %#v", outcome.ServerWarnings)
	}
	problem, ok := errs.ProblemOf(outcome.Err)
	if !ok || problem.Category != errs.CategoryAPI {
		t.Fatalf("cleanup error = %T %v, problem=%#v", outcome.Err, outcome.Err, problem)
	}
}

func TestCleanupLocalDocResourceFileDeletesFigureParent(t *testing.T) {
	factory, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("local-cleanup-file-parent"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, docsTestConfigWithAppID("local-cleanup-file-parent"), factory, core.AsUser)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docx/v1/documents/doxcn_cleanup/blocks/blk_file",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"block": map[string]interface{}{
			"block_id":   "blk_file",
			"block_type": 23,
			"parent_id":  "blk_figure",
			"file":       map[string]interface{}{"token": ""},
		}}},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docx/v1/documents/doxcn_cleanup/blocks/blk_figure",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"block": map[string]interface{}{
			"block_id":   "blk_figure",
			"block_type": 33,
			"children":   []interface{}{"blk_file"},
			"view":       map[string]interface{}{"view_type": 2},
		}}},
	})
	deleteStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/docs_ai/v1/documents/doxcn_cleanup",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{"document": map[string]interface{}{"revision_id": 8}}},
	}
	reg.Register(deleteStub)
	outcome := &localDocResourceOutcome{
		Resource:        localDocResource{Occurrence: 1, Kind: localDocResourceFile},
		BlockID:         "blk_file",
		CleanupBlockIDs: []string{"blk_file"},
		Status:          "upload_failed",
		CleanupStatus:   "pending",
		SafeToCleanup:   true,
	}

	revision, revisionKnown := cleanupLocalDocResourcePlaceholders(runtime, "doxcn_cleanup", []*localDocResourceOutcome{outcome}, float64(7))

	if !revisionKnown || revision != int64(8) || outcome.CleanupStatus != "succeeded" {
		t.Fatalf("revision=%#v known=%v outcome=%#v", revision, revisionKnown, outcome)
	}
	var body map[string]interface{}
	if err := json.Unmarshal(deleteStub.CapturedBody, &body); err != nil {
		t.Fatalf("decode cleanup body: %v", err)
	}
	if body["block_id"] != "blk_figure" {
		t.Fatalf("cleanup block_id=%#v, want figure parent", body["block_id"])
	}
}

func TestCleanupLocalDocResourceFileDoesNotDeleteInlineParent(t *testing.T) {
	factory, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("local-cleanup-inline-file"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, docsTestConfigWithAppID("local-cleanup-inline-file"), factory, core.AsUser)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docx/v1/documents/doxcn_cleanup/blocks/blk_file",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"block": map[string]interface{}{
			"block_id":   "blk_file",
			"block_type": 23,
			"parent_id":  "blk_paragraph",
			"file":       map[string]interface{}{"token": ""},
		}}},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docx/v1/documents/doxcn_cleanup/blocks/blk_paragraph",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"block": map[string]interface{}{
			"block_id":   "blk_paragraph",
			"block_type": 2,
			"children":   []interface{}{"blk_file"},
			"text":       map[string]interface{}{"elements": []interface{}{map[string]interface{}{"text_run": map[string]interface{}{"content": "keep me"}}}},
		}}},
	})
	outcome := &localDocResourceOutcome{
		Resource:        localDocResource{Occurrence: 1, Kind: localDocResourceFile},
		BlockID:         "blk_file",
		CleanupBlockIDs: []string{"blk_file"},
		Status:          "upload_failed",
		CleanupStatus:   "pending",
		SafeToCleanup:   true,
	}

	revision, revisionKnown := cleanupLocalDocResourcePlaceholders(runtime, "doxcn_cleanup", []*localDocResourceOutcome{outcome}, float64(7))

	if revision != nil || revisionKnown || outcome.Status != "bind_ambiguous" || outcome.CleanupStatus != "skipped_ambiguous" || outcome.SafeToCleanup {
		t.Fatalf("revision=%#v known=%v outcome=%#v", revision, revisionKnown, outcome)
	}
	if outcome.Err == nil || !strings.Contains(outcome.Err.Error(), "sole-source figure") {
		t.Fatalf("outcome error = %v, want parent verification failure", outcome.Err)
	}
}

func TestCleanupLocalDocResourceFileWithoutFigureParentIsPreserved(t *testing.T) {
	factory, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("local-cleanup-file-no-parent"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, docsTestConfigWithAppID("local-cleanup-file-no-parent"), factory, core.AsUser)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docx/v1/documents/doxcn_cleanup/blocks/blk_file",
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"block": map[string]interface{}{
			"block_id":   "blk_file",
			"block_type": 23,
			"file":       map[string]interface{}{"token": ""},
		}}},
	})
	outcome := &localDocResourceOutcome{
		Resource:        localDocResource{Occurrence: 1, Kind: localDocResourceFile},
		BlockID:         "blk_file",
		CleanupBlockIDs: []string{"blk_file"},
		Status:          "upload_failed",
		CleanupStatus:   "pending",
		SafeToCleanup:   true,
	}

	revision, revisionKnown := cleanupLocalDocResourcePlaceholders(runtime, "doxcn_cleanup", []*localDocResourceOutcome{outcome}, float64(7))

	if revision != nil || revisionKnown || outcome.Status != "bind_ambiguous" || outcome.CleanupStatus != "skipped_ambiguous" || outcome.SafeToCleanup {
		t.Fatalf("revision=%#v known=%v outcome=%#v", revision, revisionKnown, outcome)
	}
}

func TestDocAPINullDataIsTypedInvalidResponse(t *testing.T) {
	factory, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("local-null-data"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, docsTestConfigWithAppID("local-null-data"), factory, core.AsUser)
	reg.Register(&httpmock.Stub{Method: "PUT", URL: "/open-apis/docs_ai/v1/documents/doxcn_null", Body: map[string]interface{}{"code": 0, "data": nil}})
	_, err := doDocAPI(runtime, "PUT", "/open-apis/docs_ai/v1/documents/doxcn_null", map[string]interface{}{})
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("error = %T %v, want typed invalid_response", err, err)
	}
}

func TestFinalizeLocalDocResourcesTOCTOUErrorDoesNotLeakCWD(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile("vanished.png", []byte(localDocResourcePNG(t, 2, 1)), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	factory, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("local-toctou"))
	runtime := common.TestNewRuntimeContextForAPI(context.Background(), &cobra.Command{Use: "test"}, docsTestConfigWithAppID("local-toctou"), factory, core.AsUser)
	_, resources, err := prepareLocalDocResources(runtime, "xml", `<img path="@vanished.png"/>`)
	if err != nil {
		t.Fatalf("prepareLocalDocResources: %v", err)
	}
	if err := os.Remove("vanished.png"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	block := map[string]interface{}{"block_id": "blk_vanished", "block_type": "image", "block_token": resources[0].Marker}
	data := map[string]interface{}{"document": map[string]interface{}{"revision_id": 1, "new_blocks": []interface{}{block}}}
	reg.Register(&httpmock.Stub{Method: "GET", URL: "/open-apis/docx/v1/documents/doxcn_toctou/blocks/blk_vanished", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"block": map[string]interface{}{"block_type": 27, "image": map[string]interface{}{"token": ""}}}}})
	reg.Register(&httpmock.Stub{Method: "PUT", URL: "/open-apis/docs_ai/v1/documents/doxcn_toctou", Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{"document": map[string]interface{}{"revision_id": 2}}}})
	if err := finalizeLocalDocResources(runtime, "doxcn_toctou", data, resources); err == nil {
		t.Fatal("finalizeLocalDocResources error = nil, want partial failure")
	}
	public, _ := json.Marshal(data)
	public = append(public, stdout.Bytes()...)
	for _, secret := range []string{dir, "vanished.png", resources[0].Marker} {
		if strings.Contains(string(public), secret) {
			t.Fatalf("partial response leaked %q: %s", secret, public)
		}
	}
	var envelope struct {
		Data struct {
			Failures []struct {
				Occurrence int    `json:"occurrence"`
				Kind       string `json:"kind"`
				Status     string `json:"status"`
				Cleanup    string `json:"cleanup_status"`
				Error      struct {
					Type    string `json:"type"`
					Subtype string `json:"subtype"`
				} `json:"error"`
			} `json:"local_resource_failures"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("decode partial failure: %v", err)
	}
	if len(envelope.Data.Failures) != 1 || envelope.Data.Failures[0].Occurrence != 1 ||
		envelope.Data.Failures[0].Kind != "image" || envelope.Data.Failures[0].Status != "upload_failed" ||
		envelope.Data.Failures[0].Error.Type == "" || envelope.Data.Failures[0].Error.Subtype == "" {
		t.Fatalf("failure details = %#v", envelope.Data.Failures)
	}
}

func TestLocalDocResourceDryRunIncludesRouteExtraPartSizeAndTwentyBatch(t *testing.T) {
	resources := make([]localDocResource, 21)
	for i := range resources {
		resources[i] = localDocResource{Occurrence: i + 1, Kind: localDocResourceImage, Size: common.MaxDriveMediaUploadSinglePartSize + 1}
	}
	dry := appendLocalDocResourcesDryRun(common.NewDryRunAPI(), "doc/with space", resources)
	decoded := decodeDocDryRun(t, dry)
	patches := 0
	for _, api := range decoded.API {
		switch {
		case api.URL == "/open-apis/drive/v1/medias/upload_prepare":
			if !strings.Contains(fmt.Sprint(api.Body["extra"]), "drive_route_token") || api.Body["size"] == nil {
				t.Fatalf("upload_prepare body = %#v", api.Body)
			}
		case api.URL == "/open-apis/drive/v1/medias/upload_part":
			if api.Body["size"] == nil {
				t.Fatalf("upload_part body missing size: %#v", api.Body)
			}
		case strings.Contains(api.URL, "/blocks/batch_update"):
			patches++
			if !strings.Contains(api.URL, "doc%2Fwith%20space") {
				t.Fatalf("batch URL is not encoded: %s", api.URL)
			}
		}
	}
	if patches != 2 {
		t.Fatalf("batch PATCH count = %d, want 2", patches)
	}
}

func TestDocsUpdateLocalResourceWikiDryRunResolvesDocxFirst(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile("diagram.png", []byte(localDocResourcePNG(t, 2, 1)), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	runtime := newUpdateShortcutTestRuntime(t, "", map[string]string{
		"doc":     "https://example.larksuite.com/wiki/wikcn_local",
		"command": "append",
		"content": `<img path="@diagram.png"/>`,
	})
	dry := decodeDocDryRun(t, dryRunUpdateV2(context.Background(), runtime))
	if len(dry.API) < 2 || dry.API[0].URL != "/open-apis/wiki/v2/spaces/get_node" {
		t.Fatalf("dry-run must resolve wiki first: %#v", dry.API)
	}
	if got := dry.API[1].URL; got != "/open-apis/docs_ai/v1/documents/%3Cresolved_docx_token%3E" {
		t.Fatalf("docs_ai URL = %q", got)
	}
	for _, api := range dry.API[2:] {
		if strings.Contains(api.URL, "wikcn_local") {
			t.Fatalf("post-resolve API still uses wiki node token: %s", api.URL)
		}
	}
}

func TestDocsUpdateLocalResourceRejectsLegacyDocBeforeWrite(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile("diagram.png", []byte(localDocResourcePNG(t, 2, 1)), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	runtime := newUpdateShortcutTestRuntime(t, "", map[string]string{
		"doc":     "https://example.larksuite.com/doc/docc_legacy",
		"command": "append",
		"content": `<img path="@diagram.png"/>`,
	})
	err := validateUpdateV2(context.Background(), runtime)
	problem, ok := errs.ProblemOf(err)
	var validationErr *errs.ValidationError
	if !ok || problem.Subtype != errs.SubtypeInvalidArgument ||
		!errors.As(err, &validationErr) || validationErr.Param != "--doc" {
		t.Fatalf("error=%T %v problem=%#v validation=%#v", err, err, problem, validationErr)
	}
}

func TestDocsUpdateRemoteImageDryRunDownloadsAfterDocumentWrite(t *testing.T) {
	runtime := newUpdateShortcutTestRuntime(t, "", map[string]string{
		"doc":     "doxcn_remote_image",
		"command": "append",
		"content": `<img href="https://93.184.216.34/photo.png"/>`,
	})
	dry := decodeDocDryRun(t, dryRunUpdateV2(context.Background(), runtime))
	if len(dry.API) != 6 {
		t.Fatalf("dry-run API calls = %d, want 6: %#v", len(dry.API), dry.API)
	}
	if got := dry.API[0].URL; got != "/open-apis/docs_ai/v1/documents/doxcn_remote_image" {
		t.Fatalf("document update URL = %q", got)
	}
	if got := dry.API[1].URL; got != "https://93.184.216.34/photo.png" {
		t.Fatalf("download URL = %q", got)
	}
	if got := dry.API[2].URL; got != "/open-apis/drive/v1/medias/upload_all" {
		t.Fatalf("upload URL = %q", got)
	}
	preparedContent := fmt.Sprint(dry.API[0].Body["content"])
	if strings.Contains(preparedContent, `href=`) || !strings.Contains(preparedContent, "@lcli_img_") {
		t.Fatalf("prepared content = %q", preparedContent)
	}
}

func TestLocalDocResourceUpdateCommands(t *testing.T) {
	resources := []localDocResource{{Kind: localDocResourceImage}}
	for _, command := range []string{"str_replace"} {
		if err := validateLocalDocResourceUpdateCommand(command, resources); err == nil {
			t.Fatalf("command %s accepted local resources", command)
		}
	}
	for _, command := range []string{"append", "block_insert_after", "block_replace", "overwrite"} {
		if err := validateLocalDocResourceUpdateCommand(command, resources); err != nil {
			t.Fatalf("command %s rejected: %v", command, err)
		}
	}
}

func TestDocsUpdateLocalResourceBlockReplaceDryRun(t *testing.T) {
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile("replacement.png", []byte(localDocResourcePNG(t, 2, 1)), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	runtime := newUpdateShortcutTestRuntime(t, "", map[string]string{
		"doc":      "doxcn_block_replace",
		"command":  "block_replace",
		"block-id": "blk_target",
		"content":  `<img path="@replacement.png" caption="replacement"/>`,
	})

	dry := decodeDocDryRun(t, dryRunUpdateV2(context.Background(), runtime))
	if len(dry.API) != 5 {
		t.Fatalf("dry-run API calls = %d, want 5: %#v", len(dry.API), dry.API)
	}
	update := dry.API[0]
	if got := update.Body["command"]; got != "block_replace" {
		t.Fatalf("update command = %#v, want block_replace", got)
	}
	if got := update.Body["block_id"]; got != "blk_target" {
		t.Fatalf("update block_id = %#v, want blk_target", got)
	}
	preparedContent := fmt.Sprint(update.Body["content"])
	if strings.Contains(preparedContent, "@replacement.png") || !strings.Contains(preparedContent, "@lcli_img_") {
		t.Fatalf("prepared content = %q, want private marker without local path", preparedContent)
	}
	if got := dry.API[1].URL; got != "/open-apis/drive/v1/medias/upload_all" {
		t.Fatalf("upload URL = %q", got)
	}
	if got := dry.API[2].URL; got != "/open-apis/docx/v1/documents/doxcn_block_replace/blocks/batch_update" {
		t.Fatalf("bind URL = %q", got)
	}
}

func requestCountFromLocalDocBatchBody(t *testing.T, raw []byte) int {
	t.Helper()
	var body struct {
		Requests []interface{} `json:"requests"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("decode batch body: %v; %s", err, raw)
	}
	return len(body.Requests)
}

func newLocalDocResourceTestRuntime(t *testing.T, files map[string]string) *common.RuntimeContext {
	t.Helper()
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	for name, content := range files {
		if err := os.WriteFile(name, []byte(content), 0o600); err != nil {
			t.Fatalf("WriteFile(%s): %v", name, err)
		}
	}
	factory, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("local-resource-test"))
	return common.TestNewRuntimeContextForAPI(
		context.Background(),
		&cobra.Command{Use: "docs local-resource-test"},
		docsTestConfigWithAppID("local-resource-test"),
		factory,
		core.AsUser,
	)
}

func localDocResourcePNG(t *testing.T, width, height int) string {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
		t.Fatalf("encode %dx%d PNG: %v", width, height, err)
	}
	return buf.String()
}
