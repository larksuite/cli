// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

// exportURL builds the archive endpoint for a locator. The locator is a path
// segment shared by --app-id and --meta-token, so the URL varies per case.
func exportURL(lookup string) string {
	return "/open-apis/spark/v1/apps/" + lookup + "/code-archive"
}

// archiveStub serves a raw zip body the way the gateway does for this endpoint.
// lookup is the path segment the request is expected to target.
func archiveStub(lookup string, status int, body []byte, contentType, disposition string) *httpmock.Stub {
	headers := http.Header{}
	headers.Set("Content-Type", contentType)
	if disposition != "" {
		headers.Set("Content-Disposition", disposition)
	}
	return &httpmock.Stub{
		Method: "GET", URL: exportURL(lookup), Status: status, RawBody: body, Headers: headers,
	}
}

// TestAppsExport_RequiresExactlyOneSource pins the --app-id / --meta-token XOR.
func TestAppsExport_RequiresExactlyOneSource(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"neither", []string{"+export", "--as", "user"}},
		{"both", []string{"+export", "--app-id", "app_x", "--meta-token", "tok", "--as", "user"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			factory, stdout, _ := newAppsExecuteFactory(t)
			err := runAppsShortcut(t, AppsExport, c.args, factory, stdout)
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("err = %T %v, want *errs.ValidationError", err, err)
			}
		})
	}
}

// TestAppsExport_RejectsOutputTraversal keeps writes inside the working directory.
func TestAppsExport_RejectsOutputTraversal(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsExport,
		[]string{"+export", "--app-id", "app_x", "--output", "../escape.zip", "--as", "user"}, factory, stdout)
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T %v, want *errs.ValidationError", err, err)
	}
	if ve.Param != "--output" {
		t.Fatalf("Param = %q, want --output", ve.Param)
	}
}

// TestAppsExport_DryRun asserts the method, URL and params without a real call.
func TestAppsExport_DryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsExport,
		[]string{"+export", "--app-id", "app_x", "--checkpoint-id", "42", "--dry-run", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	var env dryRunAPIEnvelope
	_ = json.Unmarshal([]byte(stdout.String()), &env)
	if env.API[0].Method != "GET" || env.API[0].URL != exportURL("app_x") {
		t.Fatalf("dry-run = %s %s, want GET %s", env.API[0].Method, env.API[0].URL, exportURL("app_x"))
	}
	out := stdout.String()
	for _, want := range []string{"app_x", "42"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q\n%s", want, out)
		}
	}
}

// TestAppsExport_StreamsArchiveToDisk is the happy path: raw body lands on disk.
func TestAppsExport_StreamsArchiveToDisk(t *testing.T) {
	dir := chdirTemp(t)
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(archiveStub("app_x", 200, []byte("ZIPDATA"), "application/octet-stream", ""))

	if err := runAppsShortcut(t, AppsExport,
		[]string{"+export", "--app-id", "app_x", "--output", "src.zip", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "src.zip"))
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	if string(b) != "ZIPDATA" {
		t.Fatalf("archive content = %q, want ZIPDATA", b)
	}
	if !strings.Contains(stdout.String(), `"size_bytes": 7`) {
		t.Errorf("output json missing size_bytes:7\n%s", stdout.String())
	}
}

// TestAppsExport_RejectsJSONEnvelopeBody pins the gateway's HTTP 200 + JSON
// error envelope: the stream client only intercepts status >= 400, so without a
// content-type gate the envelope is written to disk as the "archive" and the
// command reports success. The caller then holds an unopenable .zip.
func TestAppsExport_RejectsJSONEnvelopeBody(t *testing.T) {
	dir := chdirTemp(t)
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(archiveStub("app_x", 200,
		[]byte(`{"code":40400,"msg":"app not found"}`), "application/json; charset=utf-8", ""))

	err := runAppsShortcut(t, AppsExport,
		[]string{"+export", "--app-id", "app_x", "--output", "src.zip", "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatal("execute err = nil, want the envelope surfaced as an error")
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err = %T %v, want *errs.APIError carrying the envelope code", err, err)
	}
	if apiErr.Code != 40400 {
		t.Errorf("code = %d, want 40400 from the envelope", apiErr.Code)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "src.zip")); !os.IsNotExist(statErr) {
		t.Error("src.zip was written; a JSON error envelope must never become a product")
	}
}

// TestAppsExport_RejectsEmptyContentType covers the same gate for a response
// that omits Content-Type: the repo treats an absent type as JSON-suspect
// (see client.HandleResponse), so it must not stream straight to disk either.
func TestAppsExport_RejectsEmptyContentType(t *testing.T) {
	dir := chdirTemp(t)
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(archiveStub("app_x", 200, []byte(`{"code":40400,"msg":"app not found"}`), "", ""))

	if err := runAppsShortcut(t, AppsExport,
		[]string{"+export", "--app-id", "app_x", "--output", "src.zip", "--as", "user"}, factory, stdout); err == nil {
		t.Fatal("execute err = nil, want the envelope surfaced as an error")
	}
	if _, statErr := os.Stat(filepath.Join(dir, "src.zip")); !os.IsNotExist(statErr) {
		t.Error("src.zip was written despite an untyped JSON body")
	}
}

// TestAppsExport_DefaultsOutputToContentDisposition prefers the server-provided
// filename so the archive keeps its canonical name.
func TestAppsExport_DefaultsOutputToContentDisposition(t *testing.T) {
	dir := chdirTemp(t)
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(archiveStub("app_x", 200, []byte("ZIP"), "application/octet-stream", `attachment; filename="app_x.zip"`))

	if err := runAppsShortcut(t, AppsExport,
		[]string{"+export", "--app-id", "app_x", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "app_x.zip")); err != nil {
		t.Fatalf("expected app_x.zip from Content-Disposition: %v", err)
	}
}

// TestAppsExport_MetaTokenSource sends the share token instead of an app id.
//
// The token occupies the same path segment an app id would — the server tells
// them apart by the "app_" prefix — so this also pins that neither source is
// ever sent as a query parameter.
func TestAppsExport_MetaTokenSource(t *testing.T) {
	chdirTemp(t)
	factory, stdout, reg := newAppsExecuteFactory(t)
	var gotURL string
	stub := archiveStub("share-tok", 200, []byte("ZIP"), "application/octet-stream", "")
	stub.OnMatch = func(req *http.Request) { gotURL = req.URL.String() }
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsExport,
		[]string{"+export", "--meta-token", "share-tok", "--output", "s.zip", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(gotURL, "/apps/share-tok/code-archive") {
		t.Fatalf("request URL = %q, want the token in the locator path segment", gotURL)
	}
	if strings.Contains(gotURL, "meta_token=") || strings.Contains(gotURL, "app_id=") {
		t.Errorf("request URL = %q, must not carry the locator as a query parameter", gotURL)
	}
}

// errHint pulls the recovery hint off whichever typed error this endpoint returned.
func errHint(err error) string {
	var ae *errs.APIError
	if errors.As(err, &ae) {
		return ae.Hint
	}
	var pe *errs.PermissionError
	if errors.As(err, &pe) {
		return pe.Hint
	}
	var ne *errs.NetworkError
	if errors.As(err, &ne) {
		return ne.Hint
	}
	var authErr *errs.AuthenticationError
	if errors.As(err, &authErr) {
		return authErr.Hint
	}
	return ""
}

// TestAppsExport_ClassifiesFailures asserts the typed error and, for the two
// cases an agent cannot otherwise recover from, that the hint says what to do
// instead. 422 is the static-HTML gate: the code is not in git at all, so the
// hint must point at file storage rather than suggest a retry.
func TestAppsExport_ClassifiesFailures(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		assert   func(error) bool
		wantHint string
	}{
		{"unauthorized", 401, "auth info is empty", func(e error) bool {
			var t *errs.AuthenticationError
			return errors.As(e, &t)
		}, ""},
		{"forbidden", 403, "permission denied", func(e error) bool {
			var t *errs.PermissionError
			return errors.As(e, &t)
		}, "download permission"},
		{"not found", 404, "app not found", func(e error) bool {
			var t *errs.APIError
			return errors.As(e, &t)
		}, ""},
		{"code not in git", 422, "this app type stores code outside git", func(e error) bool {
			var t *errs.APIError
			return errors.As(e, &t)
		}, "file storage"},
		{"too large", 413, "archive too large", func(e error) bool {
			var t *errs.APIError
			return errors.As(e, &t)
		}, "git-credential-init"},
		{"server error", 500, "boom", func(e error) bool {
			var t *errs.NetworkError
			return errors.As(e, &t)
		}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			chdirTemp(t)
			factory, stdout, reg := newAppsExecuteFactory(t)
			reg.Register(archiveStub("app_x", c.status, []byte(c.body), "text/plain", ""))

			err := runAppsShortcut(t, AppsExport,
				[]string{"+export", "--app-id", "app_x", "--output", "o.zip", "--as", "user"}, factory, stdout)
			if err == nil {
				t.Fatalf("HTTP %d: expected an error", c.status)
			}
			if !c.assert(err) {
				t.Fatalf("HTTP %d: err = %T (%v), wrong typed error", c.status, err, err)
			}
			if c.wantHint != "" && !strings.Contains(errHint(err), c.wantHint) {
				t.Errorf("HTTP %d: hint = %q, want it to mention %q", c.status, errHint(err), c.wantHint)
			}
			// A failed export must not leave a partial file behind.
			if _, statErr := os.Stat("o.zip"); statErr == nil {
				t.Errorf("HTTP %d: output file was written despite failure", c.status)
			}
		})
	}
}
