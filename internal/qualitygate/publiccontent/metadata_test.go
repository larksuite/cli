// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package publiccontent

import (
	"path/filepath"
	"testing"
)

func TestLoadMetadataReadsTitleAndBody(t *testing.T) {
	path := filepath.Join(t.TempDir(), "metadata.json")
	writeFile(t, path, `{"title":"public change","body":"pass`+`word = \"example-password\""}`)

	got, err := LoadMetadata(path)
	if err != nil {
		t.Fatalf("LoadMetadata() error = %v", err)
	}
	if got.Title != "public change" || got.Body == "" {
		t.Fatalf("metadata = %#v", got)
	}
}
