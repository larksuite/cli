// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
)

// swapEmbeddedMeta replaces the compiled-in metadata bytes for one test and
// restores them (with a full state reset) on cleanup.
func swapEmbeddedMeta(t *testing.T, data []byte) {
	t.Helper()
	resetInit()
	orig := embeddedMetaJSON
	embeddedMetaJSON = data
	t.Cleanup(func() {
		waitBackgroundRefresh()
		embeddedMetaJSON = orig
		resetInit()
	})
}

func TestSchemaCatalog_EmbeddedWhenCompiledIn(t *testing.T) {
	swapEmbeddedMeta(t, testCacheJSON("embedded_svc"))
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_REMOTE_META", "off")

	c := SchemaCatalog()

	if c.Source() != apicatalog.SourceEmbedded {
		t.Fatalf("Source = %q, want %q", c.Source(), apicatalog.SourceEmbedded)
	}
	if _, ok := c.Service("embedded_svc"); !ok {
		t.Fatal("expected embedded_svc from embedded metadata")
	}
}

// TestSchemaCatalog_FallsBackToRuntimeWhenNoEmbedded simulates a binary built
// from the bare Go module (plugin builds): only the empty meta_data_default.json
// stub is compiled in, so SchemaCatalog must serve the merged runtime view that
// Init seeds via sync fetch.
func TestSchemaCatalog_FallsBackToRuntimeWhenNoEmbedded(t *testing.T) {
	swapEmbeddedMeta(t, embeddedMetaDataDefaultJSON)
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	t.Setenv("LARKSUITE_CLI_REMOTE_META", "on")

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write(testEnvelopeJSON("remote_svc"))
	}))
	defer ts.Close()
	testMetaURL = ts.URL

	c := SchemaCatalog()

	if c.Source() != apicatalog.SourceRuntime {
		t.Fatalf("Source = %q, want %q", c.Source(), apicatalog.SourceRuntime)
	}
	if _, ok := c.Service("remote_svc"); !ok {
		t.Fatal("expected remote_svc from runtime fallback")
	}
}

func TestEmbeddedCatalogIncludesMailAutoReply(t *testing.T) {
	c := EmbeddedCatalog()

	tests := []struct {
		method string
		scope  string
		risk   string
		danger bool
	}{
		{method: "get", scope: "mail:user_mailbox.message:readonly", risk: "read"},
		{method: "update", scope: "mail:user_mailbox.message:modify", risk: "write", danger: true},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			target, err := c.Resolve([]string{"mail", "user_mailbox.auto_reply", tt.method})
			if err != nil {
				t.Fatalf("Resolve() error = %v", err)
			}
			if target.Kind != apicatalog.TargetMethod {
				t.Fatalf("Resolve() kind = %q, want %q", target.Kind, apicatalog.TargetMethod)
			}
			gotPath := strings.Join(target.Method.CommandPath(), " ")
			wantPath := "mail user_mailbox.auto_reply " + tt.method
			if gotPath != wantPath {
				t.Fatalf("CommandPath() = %q, want %q", gotPath, wantPath)
			}
			method := target.Method.Method
			if method.ID != "user_mailbox.auto_reply."+tt.method {
				t.Fatalf("Method.ID = %q", method.ID)
			}
			if len(method.Scopes) != 1 || method.Scopes[0] != tt.scope {
				t.Fatalf("Scopes = %v, want [%s]", method.Scopes, tt.scope)
			}
			if len(method.RequiredScopes) != 1 || method.RequiredScopes[0] != tt.scope {
				t.Fatalf("RequiredScopes = %v, want [%s]", method.RequiredScopes, tt.scope)
			}
			if method.Risk != tt.risk {
				t.Fatalf("Risk = %q, want %q", method.Risk, tt.risk)
			}
			if method.Danger != tt.danger {
				t.Fatalf("Danger = %v, want %v", method.Danger, tt.danger)
			}
		})
	}
}
