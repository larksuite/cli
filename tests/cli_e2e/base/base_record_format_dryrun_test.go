// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestBaseRecordReadsDryRunDefaultNDJSON(t *testing.T) {
	for _, command := range []string{"+record-list", "+record-search", "+record-get"} {
		t.Run(command, func(t *testing.T) {
			args := []string{"base", command, "--base-token", "app_x", "--table-id", "tbl_x"}
			if command == "+record-search" {
				args = append(args, "--keyword", "Alice", "--search-field", "Name")
			}
			if command == "+record-get" {
				args = append(args, "--record-id", "rec_x")
			}
			result := runBaseDryRun(t, 0, args...)
			out := result.Stdout
			require.Equal(t, "ndjson", gjson.Get(out, "data.export_format").String(), out)
			if command != "+record-get" {
				require.Equal(t, int64(2000), gjson.Get(out, "data.requested_limit").Int(), out)
				if command == "+record-list" {
					require.Contains(t, gjson.Get(out, "data.api.0.url").String(), "limit=2000&offset=0", out)
				} else {
					require.Equal(t, int64(2000), gjson.Get(out, "data.api.0.body.limit").Int(), out)
				}
			}
		})
	}
}
func TestBaseRecordReadsDryRunRejectMarkdown(t *testing.T) {
	for _, command := range []string{"+record-list", "+record-search", "+record-get"} {
		t.Run(command, func(t *testing.T) {
			result := runBaseDryRun(t, 2, "base", command, "--base-token", "app_x", "--table-id", "tbl_x", "--format", "markdown")
			require.Empty(t, result.Stdout)
			require.Contains(t, result.Stderr, "deprecated format")
			require.Equal(t, "--format", gjson.Get(result.Stderr, "error.param").String(), result.Stderr)
			require.Contains(t, gjson.Get(result.Stderr, "error.hint").String(), "--format ndjson")
		})
	}
}
