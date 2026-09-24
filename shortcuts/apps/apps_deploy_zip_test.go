// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// zipEntryNames opens an in-memory zip and returns its entry names.
func zipEntryNames(t *testing.T, body []byte) []string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	return names
}

func TestBuildAppDevZip_MissingSourceFile(t *testing.T) {
	_, err := buildAppDevZip(permissiveFIO{}, []appDevPackEntry{
		{ZipPath: "output/gone.html", AbsPath: "/nonexistent/gone.html", Size: 1},
	})
	if err == nil {
		t.Fatal("an entry whose source file vanished must fail the pack")
	}
}

// A 301, 302 or 303 turns the artifact PUT into a bodyless GET. If that GET
// answers 2xx the upload reports success while nothing was stored, and the
// release is created against an artifact that does not exist -- a failure with
// no symptom anywhere, since the status code is the only evidence the upload
// leaves. A redirect that keeps the method replays the body and is allowed.
func TestAppDevTransferClientRejectsMethodChangingRedirect(t *testing.T) {
	for name, code := range map[string]int{
		"302 found":     http.StatusFound,
		"301 moved":     http.StatusMovedPermanently,
		"303 see other": http.StatusSeeOther,
		"307 temporary": http.StatusTemporaryRedirect,
	} {
		t.Run(name, func(t *testing.T) {
			var methods []string
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				methods = append(methods, r.Method)
				if r.URL.Path == "/upload" {
					http.Redirect(w, r, "/moved", code)
					return
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer srv.Close()

			client := newAppDevTransferClient()
			client.Transport = srv.Client().Transport
			req, err := http.NewRequest(http.MethodPut, srv.URL+"/upload", bytes.NewReader([]byte("zip-body")))
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			resp, err := client.Do(req)
			if resp != nil {
				resp.Body.Close()
			}

			if code == http.StatusTemporaryRedirect {
				if err != nil {
					t.Fatalf("a redirect that keeps the method must be followed: %v", err)
				}
				if len(methods) != 2 || methods[1] != http.MethodPut {
					t.Errorf("the replayed request should still be a PUT, got %v", methods)
				}
				return
			}
			if err == nil {
				t.Fatalf("the upload must fail rather than report success for a request that carried no body; server saw %v", methods)
			}
			if !strings.Contains(err.Error(), "body would be dropped") {
				t.Errorf("the error should say why the redirect was refused: %v", err)
			}
		})
	}
}
