// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import "github.com/larksuite/cli/internal/apicatalog"

// EmbeddedCatalog returns a navigation catalog over the embedded (overlay-free)
// metadata — deterministic across machines, for golden tests and schema lint.
func EmbeddedCatalog() apicatalog.Catalog {
	return apicatalog.New(apicatalog.SourceEmbedded, EmbeddedServicesTyped())
}

// RuntimeCatalog returns a navigation catalog over the merged (embedded + remote
// overlay) metadata — for service command registration and scope discovery,
// where overlay methods must be reachable.
func RuntimeCatalog() apicatalog.Catalog {
	return apicatalog.New(apicatalog.SourceRuntime, ServicesTyped())
}

// SchemaCatalog returns the embedded catalog when metadata is compiled in,
// otherwise the merged runtime catalog. Binaries built from the bare Go module
// embed only the empty meta_data_default.json stub, so the embedded view has
// nothing to resolve; the merged view is the only data such binaries have.
func SchemaCatalog() apicatalog.Catalog {
	if len(EmbeddedServicesTyped()) > 0 {
		return EmbeddedCatalog()
	}
	return RuntimeCatalog()
}
