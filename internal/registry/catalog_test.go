// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import (
	"net/http"
	"net/http/httptest"
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
