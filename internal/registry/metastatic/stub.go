// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package metastatic

import "github.com/larksuite/cli/internal/registry/metaschema"

// Registry is the command spec as static Go data. It is declared here (zero
// value) so the package always compiles, and populated by meta_data_gen.go's
// init() when that generated file is present. On a fresh checkout the generated
// file is absent — it is gitignored and produced at build time by
// `make gen_meta` — so Registry stays empty. This keeps the "heavy spec is
// never committed, only generated" model, now without a build tag: the
// generated file augments this one rather than replacing it under a tag.
var Registry = metaschema.Registry{}
