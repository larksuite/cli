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
)

func TestDrivePreviewResolvesEntityToken(t *testing.T) {
	for _, source := range []struct {
		name, flag, input string
		wiki              bool
	}{
		{"file", "--file-token", "input_token", false},
		{"wiki in file-token", "--file-token", "input_token", true},
		{"wiki", "--wiki-token", "input_token", true},
		{"file in wiki-token", "--wiki-token", "input_token", false},
		{"file URL", "--url", "https://example.com/file/input_token", false},
		{"wiki URL", "--url", "https://example.com/wiki/input_token", true},
	} {
		for _, mode := range []string{"list", "source_file", "pdf"} {
			t.Run(source.name+"/"+mode, func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				f, stdout, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())
				// Successful entity lookup must not introduce a mandatory
				// metadata or Wiki scope check in any preview mode.
				f.Credential = credential.NewCredentialProvider(nil, nil, &driveStatusScopedTokenResolver{scopes: "drive:file:download"}, nil)
				withDriveWorkingDir(t, t.TempDir())
				var calls []string
				reg.Register(&httpmock.Stub{
					Method: http.MethodGet, URL: driveQueryByTokenPath + "?token=input_token",
					OnMatch: func(r *http.Request) { calls = append(calls, "resolve") },
					Body: map[string]any{"code": 0, "data": map[string]any{
						"obj_token": "resolved_file", "obj_type": "file", "is_wiki_token": source.wiki,
					}},
				})
				wantCalls := []string{"resolve"}
				if mode != "source_file" {
					reg.Register(&httpmock.Stub{
						Method: http.MethodPost, URL: "/open-apis/drive/v1/medias/resolved_file/preview_result",
						OnMatch: func(r *http.Request) { calls = append(calls, "candidates") },
						Body: map[string]any{"code": 0, "data": map[string]any{
							"version": "12", "preview_results": []map[string]any{{"preview_type": 0, "preview_status": 0}},
						}},
					})
					wantCalls = append(wantCalls, "candidates")
				}
				if mode != "list" {
					reg.Register(&httpmock.Stub{
						Method: http.MethodGet, URL: "/open-apis/drive/v1/medias/resolved_file/preview_download",
						OnMatch: func(r *http.Request) {
							calls = append(calls, "artifact")
							wantType, wantVersion := "0", "12"
							if mode == "source_file" {
								wantType, wantVersion = "16", "7"
							}
							if r.URL.Query().Get("preview_type") != wantType || r.URL.Query().Get("version") != wantVersion {
								t.Fatalf("incorrect preview query: %s", r.URL)
							}
						},
						RawBody: []byte("preview bytes"), ContentType: "application/pdf",
					})
					wantCalls = append(wantCalls, "artifact")
				}
				args := []string{"+preview", source.flag, source.input, "--as", "bot"}
				if mode == "list" {
					args = append(args, "--list-only")
				} else {
					args = append(args, "--type", mode, "--output", "preview.pdf")
					if mode == "source_file" {
						args = append(args, "--version", "7")
					}
				}
				if err := mountAndRunDrive(t, DrivePreview, args, f, stdout); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(calls, wantCalls) || stderr.Len() != 0 {
					t.Fatalf("calls=%v want=%v stderr=%s", calls, wantCalls, stderr)
				}
				data := decodeDriveEnvelope(t, stdout)
				if data["file_token"] != "resolved_file" {
					t.Fatalf("incorrect entity token: %#v", data)
				}
				if source.wiki {
					node, _ := data["wiki_node"].(map[string]any)
					if data["wiki_token"] != "input_token" || node["obj_token"] != "resolved_file" || node["obj_type"] != "file" {
						t.Fatalf("incorrect Wiki output: %#v", data)
					}
				} else if _, ok := data["wiki_token"]; ok {
					t.Fatalf("ordinary file labeled as Wiki: %#v", data)
				}
				if mode == "list" {
					if data["mode"] != "list" {
						t.Fatalf("incorrect list output: %#v", data)
					}
					if _, err := os.Stat("preview.pdf"); !os.IsNotExist(err) {
						t.Fatalf("list mode wrote a file: %v", err)
					}
				} else {
					content, err := os.ReadFile("preview.pdf")
					if err != nil || string(content) != "preview bytes" || data["selected_type"] != mode {
						t.Fatalf("artifact=%q err=%v output=%#v", content, err, data)
					}
				}
				reg.Verify(t)
			})
		}
	}
}

func TestDrivePreviewEntityLookupFallsBack(t *testing.T) {
	for _, flag := range []string{"--file-token", "--wiki-token"} {
		for _, mode := range []string{"list", "source_file", "pdf"} {
			t.Run(flag+"/"+mode, func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				f, stdout, stderr, reg := cmdutil.TestFactory(t, driveTestConfig())
				withDriveWorkingDir(t, t.TempDir())
				reg.Register(&httpmock.Stub{Method: http.MethodGet, URL: driveQueryByTokenPath + "?token=input_token", Body: map[string]any{"code": 1061001, "msg": "lookup failed for input_token"}})
				token := "input_token"
				if flag == "--wiki-token" {
					token = "legacy_file"
					reg.Register(&httpmock.Stub{
						Method: http.MethodGet, URL: "/open-apis/wiki/v2/spaces/get_node?token=input_token",
						Body: map[string]any{"code": 0, "data": map[string]any{"node": map[string]any{"obj_token": token, "obj_type": "file"}}},
					})
				}
				if mode != "source_file" {
					reg.Register(&httpmock.Stub{
						Method: http.MethodPost, URL: "/open-apis/drive/v1/medias/" + token + "/preview_result",
						Body: map[string]any{"code": 0, "data": map[string]any{"preview_results": []map[string]any{{"preview_type": 0, "preview_status": 0}}}},
					})
				}
				args := []string{"+preview", flag, "input_token", "--as", "bot"}
				if mode == "list" {
					args = append(args, "--list-only")
				} else {
					args = append(args, "--type", mode, "--output", "fallback.pdf")
					reg.Register(&httpmock.Stub{
						Method: http.MethodGet, URL: "/open-apis/drive/v1/medias/" + token + "/preview_download",
						RawBody: []byte("fallback bytes"), ContentType: "application/pdf",
					})
				}
				if err := mountAndRunDrive(t, DrivePreview, args, f, stdout); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(stderr.String(), "token lookup failed") {
					t.Fatalf("missing fallback warning: %s", stderr)
				}
				if strings.Contains(stderr.String(), "input_token") || strings.Contains(stderr.String(), driveQueryByTokenPath) {
					t.Fatalf("fallback warning exposed the resource token or request URL: %s", stderr)
				}
				if data := decodeDriveEnvelope(t, stdout); data["file_token"] != token {
					t.Fatalf("incorrect fallback token: %#v", data)
				}
				if mode != "list" {
					content, err := os.ReadFile("fallback.pdf")
					if err != nil || string(content) != "fallback bytes" {
						t.Fatalf("artifact=%q err=%v", content, err)
					}
				}
				reg.Verify(t)
			})
		}
	}
}

func TestDrivePreviewEntityTypeGuard(t *testing.T) {
	for _, mode := range []string{"list", "source_file", "pdf"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, stdout, _, reg := cmdutil.TestFactory(t, driveTestConfig())
			withDriveWorkingDir(t, t.TempDir())
			reg.Register(&httpmock.Stub{
				Method: http.MethodGet, URL: driveQueryByTokenPath,
				Body: map[string]any{"code": 0, "data": map[string]any{"obj_token": "document", "obj_type": "docx", "is_wiki_token": true}},
			})
			args := []string{"+preview", "--file-token", "wiki_input", "--as", "bot"}
			if mode == "list" {
				args = append(args, "--list-only")
			} else {
				args = append(args, "--type", mode, "--output", "never.pdf")
			}
			err := mountAndRunDrive(t, DrivePreview, args, f, stdout)
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) || validationErr.Param != "--file-token" || validationErr.Subtype != errs.SubtypeInvalidArgument || !strings.Contains(validationErr.Hint, "+export") {
				t.Fatalf("incorrect non-file rejection: %v", err)
			}
			if stdout.Len() != 0 {
				t.Fatalf("unexpected success output: %s", stdout)
			}
			reg.Verify(t)
		})
	}
}
