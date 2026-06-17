// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import "github.com/larksuite/cli/internal/apicatalog"

// EmbeddedCatalog returns a navigation catalog over the embedded (overlay-free)
// metadata — deterministic across machines, for `lark-cli schema`, golden tests
// and schema lint.
func EmbeddedCatalog() apicatalog.Catalog {
	return apicatalog.New(apicatalog.SourceEmbedded, EmbeddedServicesTyped())
}

// RuntimeCatalog returns a navigation catalog over the merged (embedded + remote
// overlay) metadata — for service command registration and scope discovery,
// where overlay methods must be reachable.
func RuntimeCatalog() apicatalog.Catalog {
	return apicatalog.New(apicatalog.SourceRuntime, ServicesTyped())
}
