// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestDocResourceDownloadCoverDownloadsImageContent(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-cover-download-app"))
	documentID := "doxcnCoverDownload1"
	coverToken := "cover_token_download_123"
	reg.Register(docCoverMetadataStub(documentID, map[string]interface{}{
		"token":          coverToken,
		"offset_ratio_x": 0.25,
		"offset_ratio_y": 0.75,
	}))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/drive/v1/medias/" + coverToken + "/download",
		Status: 200,
		Body:   []byte("png-data"),
		Headers: http.Header{
			"Content-Type": []string{"image/png"},
		},
	})

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)

	err := mountAndRunDocs(t, DocResourceDownload, []string{
		"+resource-download",
		"--doc", documentID,
		"--type", "cover",
		"--output", "cover",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := decodeDocResourceOutput(t, stdout)
	data := out.Data
	if data["type"] != "cover" {
		t.Fatalf("type = %v, want cover", data["type"])
	}
	if data["content_type"] != "image/png" {
		t.Fatalf("content_type = %v, want image/png", data["content_type"])
	}
	if int(data["size_bytes"].(float64)) != len("png-data") {
		t.Fatalf("size_bytes = %v", data["size_bytes"])
	}
	savedPath, _ := data["saved_path"].(string)
	if !strings.HasSuffix(savedPath, "cover.png") {
		t.Fatalf("saved_path = %q, want cover.png suffix", savedPath)
	}
	content, err := os.ReadFile(filepath.Join(tmpDir, "cover.png"))
	if err != nil {
		t.Fatalf("ReadFile(cover.png) error: %v", err)
	}
	if string(content) != "png-data" {
		t.Fatalf("downloaded content = %q", string(content))
	}
	cover := data["cover"].(map[string]interface{})
	if cover["token"] != coverToken {
		t.Fatalf("cover.token = %v, want %s", cover["token"], coverToken)
	}
}

func TestDocResourceDownloadCoverEmptyReturnsErrorWithoutDownload(t *testing.T) {
	f, _, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-cover-empty-download-app"))
	documentID := "doxcnCoverEmptyDownload1"
	reg.Register(docCoverMetadataStub(documentID, map[string]interface{}{}))

	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)

	err := mountAndRunDocs(t, DocResourceDownload, []string{
		"+resource-download",
		"--doc", documentID,
		"--type", "cover",
		"--output", "cover.png",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected empty cover error, got nil")
	}
	assertValidationContract(t, err, errs.SubtypeFailedPrecondition, "--type")
	if _, statErr := os.Stat(filepath.Join(tmpDir, "cover.png")); !os.IsNotExist(statErr) {
		t.Fatalf("cover.png should not be created, statErr=%v", statErr)
	}
}

func TestDocResourceDeleteCoverEmptyIsIdempotent(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-cover-empty-delete-app"))
	documentID := "doxcnCoverEmptyDelete1"
	reg.Register(docCoverMetadataStub(documentID, map[string]interface{}{}))

	err := mountAndRunDocs(t, DocResourceDelete, []string{
		"+resource-delete",
		"--doc", documentID,
		"--type", "cover",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data := decodeDocResourceOutput(t, stdout).Data
	if data["deleted"] != false {
		t.Fatalf("deleted = %v, want false", data["deleted"])
	}
	if data["already_empty"] != true {
		t.Fatalf("already_empty = %v, want true", data["already_empty"])
	}
}

func TestDocResourceDeleteCoverClearsExistingCover(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-cover-delete-app"))
	documentID := "doxcnCoverDelete1"
	reg.Register(docCoverMetadataStub(documentID, map[string]interface{}{"token": "cover_token_delete_123"}))
	patchStub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/docx/v1/documents/" + documentID,
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	}
	reg.Register(patchStub)

	err := mountAndRunDocs(t, DocResourceDelete, []string{
		"+resource-delete",
		"--doc", documentID,
		"--type", "cover",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body := string(patchStub.CapturedBody)
	if !strings.Contains(body, `"update_cover"`) || !strings.Contains(body, `"cover":null`) {
		t.Fatalf("PATCH body = %s, want update_cover.cover=null", body)
	}
	data := decodeDocResourceOutput(t, stdout).Data
	if data["deleted"] != true {
		t.Fatalf("deleted = %v, want true", data["deleted"])
	}
	if data["already_empty"] != false {
		t.Fatalf("already_empty = %v, want false", data["already_empty"])
	}
}

func TestDocResourceUpdateCoverUploadsFileAndReturnsFullTokenOnlyOnStdout(t *testing.T) {
	f, stdout, stderr, reg := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-cover-update-app"))
	documentID := "doxcnCoverUpdate1"
	fileToken := "file_cover_uploaded_token_12345"
	tmpDir := t.TempDir()
	withDocsWorkingDir(t, tmpDir)
	if err := os.WriteFile("cover.png", []byte("png-data"), 0644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	uploadStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/drive/v1/medias/upload_all",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"file_token": fileToken},
		},
	}
	reg.Register(uploadStub)
	patchStub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/docx/v1/documents/" + documentID,
		Body:   map[string]interface{}{"code": 0, "msg": "ok"},
	}
	reg.Register(patchStub)

	err := mountAndRunDocs(t, DocResourceUpdate, []string{
		"+resource-update",
		"--doc", documentID,
		"--type", "cover",
		"--file", "cover.png",
		"--offset-ratio-x", "0.2",
		"--offset-ratio-y", "0.8",
		"--as", "bot",
	}, f, stdout)
	if err != nil {
		t.Fatalf("unexpected error: %v — stderr: %s", err, stderr.String())
	}

	if !bytes.Contains(uploadStub.CapturedBody, []byte("png-data")) {
		t.Fatalf("upload body does not contain file bytes")
	}
	uploadBody := string(uploadStub.CapturedBody)
	if !strings.Contains(uploadBody, `name="parent_type"`) || !strings.Contains(uploadBody, "docx_image") {
		t.Fatalf("upload body missing docx_image parent type: %s", uploadBody)
	}
	if !strings.Contains(uploadBody, "drive_route_token") || !strings.Contains(uploadBody, documentID) {
		t.Fatalf("upload body missing drive_route_token extra: %s", uploadBody)
	}
	patchBody := string(patchStub.CapturedBody)
	for _, want := range []string{`"update_cover"`, `"token":"` + fileToken + `"`, `"offset_ratio_x":0.2`, `"offset_ratio_y":0.8`} {
		if !strings.Contains(patchBody, want) {
			t.Fatalf("PATCH body = %s, missing %s", patchBody, want)
		}
	}

	if strings.Contains(stderr.String(), fileToken) {
		t.Fatalf("stderr leaked full file_token: %s", stderr.String())
	}
	data := decodeDocResourceOutput(t, stdout).Data
	if data["file_token"] != fileToken {
		t.Fatalf("stdout file_token = %v, want %s", data["file_token"], fileToken)
	}
	cover := data["cover"].(map[string]interface{})
	if cover["token"] != fileToken {
		t.Fatalf("stdout cover.token = %v, want %s", cover["token"], fileToken)
	}
}

func TestDocResourceUpdateCoverRejectsMultipleSources(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-cover-source-validation-app"))

	err := mountAndRunDocs(t, DocResourceUpdate, []string{
		"+resource-update",
		"--doc", "doxcnCoverValidate1",
		"--type", "cover",
		"--file", "cover.png",
		"--from-clipboard",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected mutual exclusion error, got nil")
	}
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "", "--file", "--from-clipboard")
}

func TestDocResourceUpdateCoverRejectsMissingSource(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-cover-source-required-app"))

	err := mountAndRunDocs(t, DocResourceUpdate, []string{
		"+resource-update",
		"--doc", "doxcnCoverValidateRequired1",
		"--type", "cover",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected missing source error, got nil")
	}
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "", "--file", "--from-clipboard", "--url")
}

func TestDocResourceUpdateCoverRejectsUnsafeURLSource(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-cover-url-validation-app"))

	err := mountAndRunDocs(t, DocResourceUpdate, []string{
		"+resource-update",
		"--doc", "doxcnCoverURLValidate1",
		"--type", "cover",
		"--url", "https://127.0.0.1/cover.png",
		"--as", "bot",
	}, f, nil)
	if err == nil {
		t.Fatal("expected unsafe URL error, got nil")
	}
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--url")
}

func TestDocCoverURLSyntaxValidation(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		ok   bool
	}{
		{name: "https", raw: " https://example.com/cover.png ", ok: true},
		{name: "http", raw: "http://example.com/cover.png"},
		{name: "userinfo", raw: "https://user:pass@example.com/cover.png"},
		{name: "empty host", raw: "https:///cover.png"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u, err := parseDocCoverURLSyntax(tc.raw)
			if tc.ok {
				if err != nil {
					t.Fatalf("parseDocCoverURLSyntax() error: %v", err)
				}
				if u.String() != "https://example.com/cover.png" {
					t.Fatalf("URL = %q, want normalized https URL", u.String())
				}
				return
			}
			assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--url")
		})
	}
}

func TestDocResourceCoverDryRunsPlanAPIs(t *testing.T) {
	downloadRT := docValidateRuntime(t, map[string]string{
		"doc":    "doxcnCoverDryRunDownload",
		"output": "cover",
	}, nil, nil)
	download := decodeDocDryRun(t, DocResourceDownload.DryRun(context.Background(), downloadRT))
	if len(download.API) != 2 {
		t.Fatalf("download dry-run API count = %d, want 2", len(download.API))
	}
	if got := download.API[0].URL; got != "/open-apis/docx/v1/documents/doxcnCoverDryRunDownload" {
		t.Fatalf("download metadata URL = %q", got)
	}
	if got := download.API[1].URL; got != "/open-apis/drive/v1/medias/%3Ccover.token%3E/download" {
		t.Fatalf("download media URL = %q", got)
	}

	updateRT := docValidateRuntime(t, map[string]string{
		"doc": "https://example.larksuite.com/wiki/wikcnCoverDryRunUpdate",
		"url": "https://example.com/cover.png",
	}, nil, nil)
	update := decodeDocDryRun(t, DocResourceUpdate.DryRun(context.Background(), updateRT))
	if len(update.API) != 3 {
		t.Fatalf("update dry-run API count = %d, want 3", len(update.API))
	}
	if got := update.API[0].URL; got != "/open-apis/wiki/v2/spaces/get_node" {
		t.Fatalf("wiki resolve URL = %q", got)
	}
	if got := update.API[1].URL; got != "/open-apis/drive/v1/medias/upload_all" {
		t.Fatalf("upload URL = %q", got)
	}
	if got := update.API[2].URL; got != "/open-apis/docx/v1/documents/%3Cresolved_docx_token%3E" {
		t.Fatalf("patch URL = %q", got)
	}
	if got := update.API[1].Body["file"]; got != "https://example.com/cover.png" {
		t.Fatalf("upload source = %#v", got)
	}

	deleteRT := docValidateRuntime(t, map[string]string{"doc": "doxcnCoverDryRunDelete"}, nil, nil)
	deleteDry := decodeDocDryRun(t, DocResourceDelete.DryRun(context.Background(), deleteRT))
	if len(deleteDry.API) != 2 {
		t.Fatalf("delete dry-run API count = %d, want 2", len(deleteDry.API))
	}
	if got := deleteDry.API[1].URL; got != "/open-apis/docx/v1/documents/doxcnCoverDryRunDelete" {
		t.Fatalf("delete patch URL = %q", got)
	}
}

func TestDocResourceCoverDryRunReportsInvalidDoc(t *testing.T) {
	rt := docValidateRuntime(t, map[string]string{"doc": "https://example.com/sheets/shtxxx"}, nil, nil)
	dry := DocResourceDownload.DryRun(context.Background(), rt)
	if got := dry.Format(); !strings.Contains(got, "error:") {
		t.Fatalf("dry-run error output = %q, want error field", got)
	}
}

func TestParseAndValidateDocCoverURLRejectsUnsafeIP(t *testing.T) {
	_, err := parseAndValidateDocCoverURL(context.Background(), "https://127.0.0.1/cover.png")
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--url")
}

func TestValidateDocCoverURLHost(t *testing.T) {
	for _, host := range []string{"", "localhost", "service.localhost", "127.0.0.1"} {
		t.Run(host, func(t *testing.T) {
			assertValidationContract(t, validateDocCoverURLHost(context.Background(), host), errs.SubtypeInvalidArgument, "--url")
		})
	}
	if err := validateDocCoverURLHost(context.Background(), "1.1.1.1"); err != nil {
		t.Fatalf("validateDocCoverURLHost(public IP) error: %v", err)
	}
}

func TestDocCoverIPSafetyBlocksSpecialRanges(t *testing.T) {
	for _, rawIP := range []string{
		"10.0.0.1",
		"127.0.0.1",
		"169.254.1.1",
		"172.16.0.1",
		"192.168.0.1",
		"100.64.0.1",
		"198.18.0.1",
		"240.0.0.1",
	} {
		t.Run(rawIP, func(t *testing.T) {
			if !isUnsafeDocCoverIP(net.ParseIP(rawIP)) {
				t.Fatalf("%s was classified as safe", rawIP)
			}
		})
	}
	if isUnsafeDocCoverIP(net.ParseIP("1.1.1.1")) {
		t.Fatal("public IPv4 address was classified as unsafe")
	}
}

func TestDocCoverHTTPClientDoesNotUseProxy(t *testing.T) {
	baseTransport := &http.Transport{Proxy: http.ProxyFromEnvironment}
	baseClient := &http.Client{Transport: baseTransport}

	client := newDocCoverHTTPClient(baseClient)
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("client transport = %T, want *http.Transport", client.Transport)
	}
	if transport.Proxy != nil {
		t.Fatal("cover URL downloader must not inherit proxy settings")
	}
	if baseTransport.Proxy == nil {
		t.Fatal("base transport proxy was mutated")
	}
}

func TestDocCoverHTTPClientRedirectValidation(t *testing.T) {
	client := newDocCoverHTTPClient(&http.Client{})
	req, err := http.NewRequest(http.MethodGet, "https://1.1.1.1/cover.png", nil)
	if err != nil {
		t.Fatalf("NewRequest() error: %v", err)
	}
	if err := client.CheckRedirect(req, []*http.Request{{}, {}, {}}); err == nil {
		t.Fatal("expected too many redirects error")
	}

	prev, err := http.NewRequest(http.MethodGet, "https://1.1.1.1/start", nil)
	if err != nil {
		t.Fatalf("NewRequest(prev) error: %v", err)
	}
	downgrade, err := http.NewRequest(http.MethodGet, "http://1.1.1.1/cover.png", nil)
	if err != nil {
		t.Fatalf("NewRequest(downgrade) error: %v", err)
	}
	if err := client.CheckRedirect(downgrade, []*http.Request{prev}); err == nil {
		t.Fatal("expected https-to-http redirect error")
	}
}

func TestDocCoverConnRemoteIPValidation(t *testing.T) {
	if err := validateDocCoverConnRemoteIP(nil); err == nil {
		t.Fatal("expected nil connection error")
	}
	if err := validateDocCoverConnRemoteIP(docCoverRemoteAddrConn{}); err == nil {
		t.Fatal("expected missing remote address error")
	}
	if err := validateDocCoverConnRemoteIP(docCoverRemoteAddrConn{addr: testAddr("not-ip")}); err == nil {
		t.Fatal("expected invalid remote IP error")
	}
	if err := validateDocCoverConnRemoteIP(docCoverRemoteAddrConn{addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 443}}); err == nil {
		t.Fatal("expected local remote IP error")
	}
}

func TestDocCoverURLFileName(t *testing.T) {
	cases := []struct {
		raw  string
		ext  string
		want string
	}{
		{raw: "https://example.com/images/cover", ext: ".png", want: "cover.png"},
		{raw: "https://example.com/images/cover.jpeg", ext: ".png", want: "cover.jpeg"},
		{raw: "https://example.com/", ext: ".webp", want: "cover.webp"},
		{raw: "https://example.com/images/%2Fescaped", ext: ".gif", want: "escaped.gif"},
	}
	for _, tc := range cases {
		t.Run(tc.want, func(t *testing.T) {
			u, err := url.Parse(tc.raw)
			if err != nil {
				t.Fatalf("url.Parse(%q): %v", tc.raw, err)
			}
			if got := docCoverURLFileName(u, tc.ext); got != tc.want {
				t.Fatalf("docCoverURLFileName() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDownloadDocCoverURLSuccess(t *testing.T) {
	runtime, rawURL := newDocCoverURLTestRuntime(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("png-data"))
	}))

	content, fileName, err := downloadDocCoverURL(context.Background(), runtime, rawURL)
	if err != nil {
		t.Fatalf("downloadDocCoverURL() error: %v", err)
	}
	if string(content) != "png-data" {
		t.Fatalf("content = %q, want png-data", string(content))
	}
	if fileName != "cover.png" {
		t.Fatalf("fileName = %q, want cover.png", fileName)
	}
}

func TestDownloadDocCoverURLRejectsHTTPStatus(t *testing.T) {
	runtime, rawURL := newDocCoverURLTestRuntime(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("unavailable"))
	}))

	_, _, err := downloadDocCoverURL(context.Background(), runtime, rawURL)
	if err == nil {
		t.Fatal("expected HTTP status error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("error = %T %v, want typed problem", err, err)
	}
	if p.Category != errs.CategoryNetwork {
		t.Fatalf("problem category = %v, want %v", p.Category, errs.CategoryNetwork)
	}
	if p.Subtype != errs.SubtypeNetworkServer {
		t.Fatalf("problem subtype = %v, want %v", p.Subtype, errs.SubtypeNetworkServer)
	}
	if p.Code != http.StatusServiceUnavailable {
		t.Fatalf("problem code = %v, want %v", p.Code, http.StatusServiceUnavailable)
	}
	var networkErr *errs.NetworkError
	if !errors.As(err, &networkErr) {
		t.Fatalf("error = %T %v, want *errs.NetworkError", err, err)
	}
	if networkErr.Cause == nil {
		t.Fatal("expected preserved underlying cause, got nil")
	}
}

func TestDownloadDocCoverURLRejectsUnsupportedContentType(t *testing.T) {
	runtime, rawURL := newDocCoverURLTestRuntime(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("not-image"))
	}))

	_, _, err := downloadDocCoverURL(context.Background(), runtime, rawURL)
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--url")
}

func TestDownloadDocCoverURLRejectsOversizeResponse(t *testing.T) {
	runtime, rawURL := newDocCoverURLTestRuntime(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = io.CopyN(w, repeatByteReader('x'), docCoverURLMaxBytes+1)
	}))

	_, _, err := downloadDocCoverURL(context.Background(), runtime, rawURL)
	assertValidationContract(t, err, errs.SubtypeInvalidArgument, "--url")
}

func TestDocCoverMetadataOutputAndOptionalFloats(t *testing.T) {
	x := 0.25
	y := 1.0
	out := docCoverMetadata{
		Token:        "cover_token",
		OffsetRatioX: &x,
		OffsetRatioY: &y,
	}.toOutput()
	if out["token"] != "cover_token" || out["offset_ratio_x"] != x || out["offset_ratio_y"] != y {
		t.Fatalf("cover output = %#v", out)
	}

	data := map[string]interface{}{
		"float": float64(1.5),
		"int":   2,
		"int64": int64(3),
		"text":  "4",
	}
	if got, ok := getOptionalFloat(data, "float"); !ok || got != 1.5 {
		t.Fatalf("float optional = %v/%v, want 1.5/true", got, ok)
	}
	if got, ok := getOptionalFloat(data, "int"); !ok || got != 2 {
		t.Fatalf("int optional = %v/%v, want 2/true", got, ok)
	}
	if got, ok := getOptionalFloat(data, "int64"); !ok || got != 3 {
		t.Fatalf("int64 optional = %v/%v, want 3/true", got, ok)
	}
	if _, ok := getOptionalFloat(data, "text"); ok {
		t.Fatal("string optional unexpectedly parsed as float")
	}
	if _, ok := getOptionalFloat(nil, "missing"); ok {
		t.Fatal("nil map optional unexpectedly returned a value")
	}
}

func TestDocCoverDryRunSource(t *testing.T) {
	cases := []struct {
		name  string
		str   map[string]string
		bools map[string]bool
		want  string
	}{
		{name: "clipboard", bools: map[string]bool{"from-clipboard": true}, want: "<clipboard image>"},
		{name: "url", str: map[string]string{"url": "https://example.com/cover.png"}, want: "https://example.com/cover.png"},
		{name: "file", str: map[string]string{"file": "cover.png"}, want: "@cover.png"},
		{name: "empty", want: "<cover image>"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := docValidateRuntime(t, tc.str, tc.bools, nil)
			if got := docCoverDryRunSource(rt); got != tc.want {
				t.Fatalf("docCoverDryRunSource() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDocShortcutsIncludeCoverResourceCommands(t *testing.T) {
	got := map[string]bool{}
	for _, shortcut := range Shortcuts() {
		got[shortcut.Command] = true
	}
	for _, want := range []string{"+resource-download", "+resource-update", "+resource-delete"} {
		if !got[want] {
			t.Fatalf("Shortcuts() missing %s", want)
		}
	}
}

func newDocCoverURLTestRuntime(t *testing.T, handler http.Handler) (*common.RuntimeContext, string) {
	t.Helper()

	server := httptest.NewTLSServer(handler)
	t.Cleanup(server.Close)

	f, _, _, _ := cmdutil.TestFactory(t, docsTestConfigWithAppID("docs-cover-url-download-app"))
	targetAddr := server.Listener.Addr().String()
	f.HttpClient = func() (*http.Client, error) {
		return &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					var d net.Dialer
					conn, err := d.DialContext(ctx, network, targetAddr)
					if err != nil {
						return nil, err
					}
					return docCoverRemoteAddrConn{
						Conn: conn,
						addr: &net.TCPAddr{IP: net.ParseIP("1.1.1.1"), Port: 443},
					}, nil
				},
			},
		}, nil
	}
	return &common.RuntimeContext{Factory: f}, "https://1.1.1.1/assets/cover"
}

type docCoverRemoteAddrConn struct {
	net.Conn
	addr net.Addr
}

func (c docCoverRemoteAddrConn) RemoteAddr() net.Addr {
	return c.addr
}

type testAddr string

func (a testAddr) Network() string {
	return "test"
}

func (a testAddr) String() string {
	return string(a)
}

type repeatByteReader byte

func (r repeatByteReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(r)
	}
	return len(p), nil
}

func docCoverMetadataStub(documentID string, cover map[string]interface{}) *httpmock.Stub {
	return &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/docx/v1/documents/" + documentID,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"document": map[string]interface{}{
					"cover": cover,
				},
			},
		},
	}
}

type docResourceOutput struct {
	OK   bool                   `json:"ok"`
	Data map[string]interface{} `json:"data"`
}

func decodeDocResourceOutput(t *testing.T, stdout *bytes.Buffer) docResourceOutput {
	t.Helper()
	var out docResourceOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		t.Fatalf("decode resource output: %v; output=%s", err, stdout.String())
	}
	return out
}
