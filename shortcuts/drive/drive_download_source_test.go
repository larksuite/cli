// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package drive

import (
	"errors"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestDriveDownloadResolvesEntityToken(t *testing.T) {
	for _, tc := range []struct {
		name, flag, input   string
		isWiki, defaultName bool
		status              int
	}{
		{name: "wiki in file-token", flag: "--file-token", input: "input_token", isWiki: true},
		{name: "ordinary file", flag: "--file-token", input: "input_token"},
		{name: "explicit wiki", flag: "--wiki-token", input: "input_token", isWiki: true},
		{name: "file in wiki-token", flag: "--wiki-token", input: "input_token"},
		{name: "wiki URL", flag: "--url", input: "https://example.com/wiki/input_token", isWiki: true},
		{name: "file URL", flag: "--url", input: "https://example.com/file/input_token"},
		{name: "default filename", flag: "--file-token", input: "input_token", isWiki: true, defaultName: true},
		{name: "recycled node status", flag: "--file-token", input: "input_token", isWiki: true, status: 1},
		{name: "deleted node status", flag: "--file-token", input: "input_token", isWiki: true, status: 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())
			// Neither metadata nor Wiki permission is a new precondition for
			// explicit-output downloads. Default naming still requires metadata.
			scopes := "drive:file:download " + common.DrivePermissionMemberAuthScope
			if tc.defaultName {
				scopes += " " + driveMetadataReadScope
			}
			f.Credential = credential.NewCredentialProvider(nil, nil, &driveStatusScopedTokenResolver{scopes: scopes}, nil)
			withDriveWorkingDir(t, t.TempDir())
			var calls []string
			reg.Register(&httpmock.Stub{
				Method: http.MethodGet, URL: driveQueryByTokenPath + "?token=input_token",
				OnMatch: func(r *http.Request) { calls = append(calls, "resolve") },
				Body: map[string]any{"code": 0, "data": map[string]any{
					"obj_token": "resolved_file", "obj_type": "file", "is_wiki_token": tc.isWiki, "status": tc.status,
				}},
			})
			auth := registerDriveDownloadExportAuth(reg, "resolved_file", true)
			auth.OnMatch = func(r *http.Request) {
				calls = append(calls, "auth")
				if r.URL.Query().Get("type") != "file" || r.URL.Query().Get("action") != "export" {
					t.Fatalf("incorrect auth query: %s", r.URL)
				}
			}
			wantCalls := []string{"resolve", "auth"}
			if tc.defaultName {
				reg.Register(&httpmock.Stub{
					Method: http.MethodPost, URL: "/open-apis/drive/v1/metas/batch_query",
					OnMatch: func(r *http.Request) { calls = append(calls, "metadata") },
					BodyFilter: func(body []byte) bool {
						return strings.Contains(string(body), `"doc_token":"resolved_file"`) && strings.Contains(string(body), `"doc_type":"file"`)
					},
					Body: map[string]any{"code": 0, "data": map[string]any{"metas": []map[string]any{{"doc_token": "resolved_file", "doc_type": "file", "title": "download.bin"}}}},
				})
				wantCalls = append(wantCalls, "metadata")
			}
			reg.Register(&httpmock.Stub{
				Method: http.MethodGet, URL: "/open-apis/drive/v1/files/resolved_file/download",
				OnMatch: func(r *http.Request) { calls = append(calls, "download") },
				RawBody: []byte("image bytes"), ContentType: "application/octet-stream",
			})
			wantCalls = append(wantCalls, "download")
			args := []string{"+download", tc.flag, tc.input, "--as", "bot"}
			if !tc.defaultName {
				args = append(args, "--output", "download.bin")
			}
			if err := mountAndRunDrive(t, DriveDownload, args, f, stdout); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile("download.bin")
			if err != nil || string(got) != "image bytes" {
				t.Fatalf("download = %q, err = %v", got, err)
			}
			if !reflect.DeepEqual(calls, wantCalls) {
				t.Fatalf("calls = %v, want %v", calls, wantCalls)
			}
			if stderr.Len() != 0 {
				t.Fatalf("unexpected warning: %s", stderr)
			}
			data := decodeDriveEnvelope(t, stdout)
			if tc.isWiki {
				node, _ := data["wiki_node"].(map[string]any)
				if data["wiki_token"] != "input_token" || node["obj_token"] != "resolved_file" || node["obj_type"] != "file" {
					t.Fatalf("incorrect resolution output: %#v", data)
				}
			} else if _, ok := data["wiki_token"]; ok {
				t.Fatalf("ordinary file labeled as wiki: %#v", data)
			}
			reg.Verify(t)
		})
	}
}

func TestDriveDownloadEntityLookupFallsBack(t *testing.T) {
	for _, tc := range []struct {
		name        string
		body        any
		raw         []byte
		status      int
		err         error
		headers     http.Header
		wantWarning []string
	}{
		{
			name:        "server error",
			body:        map[string]any{"code": 1061001, "msg": "lookup failed for input_token"},
			headers:     http.Header{"X-Tt-Logid": []string{"lookup-log-id"}},
			wantWarning: []string{"code=1061001", `log_id="lookup-log-id"`},
		},
		{name: "not found", body: map[string]any{"code": 1061003}},
		{name: "forbidden", body: map[string]any{"code": 1061004}},
		{name: "missing scope", body: map[string]any{"code": 99991679}},
		{name: "http failure", status: 503, raw: []byte("unavailable")},
		{name: "transport failure", err: errors.New("network unavailable"), wantWarning: []string{"type=network", "subtype=transport"}},
		{name: "incomplete object", body: map[string]any{"code": 0, "data": map[string]any{"obj_type": "file"}}},
		{name: "wrong field type", body: map[string]any{"code": 0, "data": map[string]any{"obj_token": 123, "obj_type": "file"}}},
		{name: "invalid token", body: map[string]any{"code": 0, "data": map[string]any{"obj_token": "../bad", "obj_type": "file"}}},
		{name: "invalid json", raw: []byte("{")},
	} {
		for _, wiki := range []bool{false, true} {
			name := tc.name + "/file"
			if wiki {
				name = tc.name + "/wiki"
			}
			t.Run(name, func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				f, stdout, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())
				withDriveWorkingDir(t, t.TempDir())
				reg.Register(&httpmock.Stub{
					Method: http.MethodGet, URL: driveQueryByTokenPath + "?token=input_token",
					Body: tc.body, RawBody: tc.raw, Status: tc.status, Error: tc.err, Headers: tc.headers,
				})
				token, flag := "input_token", "--file-token"
				if wiki {
					token, flag = "legacy_file", "--wiki-token"
					reg.Register(&httpmock.Stub{
						Method: http.MethodGet, URL: "/open-apis/wiki/v2/spaces/get_node?token=input_token",
						Body: map[string]any{"code": 0, "data": map[string]any{"node": map[string]any{"obj_token": token, "obj_type": "file"}}},
					})
				}
				registerDriveDownloadExportAuth(reg, token, true)
				reg.Register(&httpmock.Stub{
					Method: http.MethodGet, URL: "/open-apis/drive/v1/files/" + token + "/download",
					RawBody: []byte("fallback bytes"), ContentType: "application/octet-stream",
				})
				err := mountAndRunDrive(t, DriveDownload, []string{"+download", flag, "input_token", "--output", "fallback.bin", "--as", "bot"}, f, stdout)
				if err != nil {
					t.Fatal(err)
				}
				got, readErr := os.ReadFile("fallback.bin")
				if readErr != nil || string(got) != "fallback bytes" {
					t.Fatalf("download = %q, err = %v", got, readErr)
				}
				if !strings.Contains(stderr.String(), "token lookup failed") {
					t.Fatalf("missing fallback warning: %s", stderr)
				}
				if strings.Contains(stderr.String(), "input_token") || strings.Contains(stderr.String(), driveQueryByTokenPath) {
					t.Fatalf("fallback warning exposed the resource token or request URL: %s", stderr)
				}
				for _, want := range tc.wantWarning {
					if !strings.Contains(stderr.String(), want) {
						t.Errorf("fallback warning missing diagnostic %q: %s", want, stderr)
					}
				}
				reg.Verify(t)
			})
		}
	}
}

func TestDriveDownloadEntityTypeAndPermissionGuards(t *testing.T) {
	for _, typ := range []string{"docx", "folder", "file"} {
		t.Run(typ, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
			withDriveWorkingDir(t, t.TempDir())
			reg.Register(&httpmock.Stub{
				Method: http.MethodGet, URL: driveQueryByTokenPath,
				Body: map[string]any{"code": 0, "data": map[string]any{"obj_token": "resolved_object", "obj_type": typ, "is_wiki_token": true}},
			})
			if typ == "file" {
				registerDriveDownloadExportAuth(reg, "resolved_object", false)
			}
			err := mountAndRunDrive(t, DriveDownload, []string{"+download", "--file-token", "wiki_input", "--output", "never.bin", "--as", "bot"}, f, stdout)
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected typed failure, got %v", err)
			}
			if typ == "file" {
				if p.Category != errs.CategoryAuthorization {
					t.Fatalf("expected permission denial, got %+v", p)
				}
			} else {
				var validationErr *errs.ValidationError
				if !errors.As(err, &validationErr) || validationErr.Param != "--file-token" || p.Subtype != errs.SubtypeInvalidArgument || !strings.Contains(p.Hint, "+export") {
					t.Fatalf("incorrect non-file rejection: %v", err)
				}
			}
			if stdout.Len() != 0 {
				t.Fatalf("unexpected success output: %s", stdout)
			}
			if _, err := os.Stat("never.bin"); !os.IsNotExist(err) {
				t.Fatalf("download wrote a file: %v", err)
			}
			reg.Verify(t)
		})
	}
}

func TestDriveFileSourceFallbackRequiresWikiScope(t *testing.T) {
	for _, shortcut := range []common.Shortcut{DriveDownload, DrivePreview} {
		t.Run(shortcut.Command, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())
			f.Credential = credential.NewCredentialProvider(nil, nil, &driveStatusScopedTokenResolver{scopes: "drive:file:download"}, nil)
			withDriveWorkingDir(t, t.TempDir())
			reg.Register(&httpmock.Stub{
				Method: http.MethodGet, URL: driveQueryByTokenPath + "?token=wiki_input",
				Body: map[string]any{"code": 1061001},
			})
			args := []string{shortcut.Command, "--wiki-token", "wiki_input", "--output", "never.bin", "--as", "bot"}
			if shortcut.Command == "+preview" {
				args = append(args, "--type", "source_file")
			}
			err := mountAndRunDrive(t, shortcut, args, f, stdout)
			var permissionErr *errs.PermissionError
			if !errors.As(err, &permissionErr) || permissionErr.Subtype != errs.SubtypeMissingScope || !reflect.DeepEqual(permissionErr.MissingScopes, []string{driveWikiNodeRetrieveScope}) {
				t.Fatalf("expected legacy Wiki scope failure, got %v", err)
			}
			if !strings.Contains(stderr.String(), "token lookup failed") || stdout.Len() != 0 {
				t.Fatalf("stdout=%s stderr=%s", stdout, stderr)
			}
			reg.Verify(t)
		})
	}
}
