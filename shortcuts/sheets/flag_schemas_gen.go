// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Code generated from data/flag-schemas.json; DO NOT EDIT.

package sheets

// commandsWithSchema is the set of shortcut commands that have at least one
// introspectable composite flag in data/flag-schemas.json. Codegen'd so the
// registration loop (shortcuts.go) and the validate fast-path can gate on it
// without parsing the 256KB schema blob at startup (that parse used to run on
// every CLI invocation, sheets or not). The 256KB is now only unmarshaled
// on --print-schema or when validating a command that is in this set. Do not
// hand-edit; regenerate with `go generate ./shortcuts/sheets/...`.
var commandsWithSchema = map[string]struct{}{
	"+batch-update":          {},
	"+cells-batch-set-style": {},
	"+cells-set":             {},
	"+cells-set-style":       {},
	"+chart-create":          {},
	"+chart-update":          {},
	"+cond-format-create":    {},
	"+cond-format-update":    {},
	"+dropdown-set":          {},
	"+dropdown-update":       {},
	"+filter-create":         {},
	"+filter-update":         {},
	"+filter-view-create":    {},
	"+filter-view-update":    {},
	"+pivot-create":          {},
	"+pivot-update":          {},
	"+range-sort":            {},
	"+sparkline-create":      {},
	"+sparkline-update":      {},
	"+table-put":             {},
	"+workbook-create":       {},
}
