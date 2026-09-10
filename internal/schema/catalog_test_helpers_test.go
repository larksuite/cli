// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/registry"
)

func testFullCatalog(t *testing.T) apicatalog.Catalog {
	t.Helper()
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	return snapshot.Catalog()
}
