// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package examples

import (
	"testing"

	"github.com/larksuite/cli/internal/qualitygate/manifest"
)

func TestHarvestManifestExamples(t *testing.T) {
	m := manifest.Manifest{Commands: []manifest.Command{{
		Path:    "root",
		Example: "lark-cli calendar +agenda\nlark-cli api GET /open-apis/test",
	}}}
	got := FromManifest(m)
	if len(got) != 2 {
		t.Fatalf("got %d examples, want 2", len(got))
	}
	if got[0].SourceFile != "command-manifest" {
		t.Fatalf("source = %q", got[0].SourceFile)
	}
}
