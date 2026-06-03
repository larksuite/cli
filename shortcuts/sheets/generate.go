// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

// flag_defs_gen.go and flag_schemas_gen.go are generated from the canonical
// data/*.json spec artifacts (synced from sheet-skill-spec). After the sync
// script updates data/flag-defs.json or data/flag-schemas.json, regenerate
// the compiled Go with:
//
//	go generate ./shortcuts/sheets/...
//
//go:generate go run ./internal/gen
