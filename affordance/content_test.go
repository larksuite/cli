// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"io/fs"
	"testing"
)

func TestDefaultFSContainsDomainGuidance(t *testing.T) {
	if _, err := fs.ReadFile(DefaultFS(), "im.md"); err != nil {
		t.Fatalf("read im.md: %v", err)
	}
}
