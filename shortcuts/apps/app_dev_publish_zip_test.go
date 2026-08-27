// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"archive/zip"
	"bytes"
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
