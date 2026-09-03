// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"testing"

	"github.com/larksuite/cli/internal/registry"
)

func testCatalogServiceNames(t *testing.T) []string {
	t.Helper()
	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	services := snapshot.Catalog().Services()
	names := make([]string, 0, len(services))
	for _, service := range services {
		names = append(names, service.Name)
	}
	return names
}
