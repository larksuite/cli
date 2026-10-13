// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/stretchr/testify/require"
)

func TestRangeCopyAliasesPreserveSourceDestinationAndPasteType(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, targetFlag := range []string{"copy-to-range", "destination-range", "dest-range", "dst-range", "destination", "to", "paste-to", "paste-range", "target-start-cell"} {
		t.Run(targetFlag, func(t *testing.T) {
			body := parseDryRunBody(t, shortcutFromRegistry(t, "+range-copy"), []string{
				"--url", testURL, "--source-sheet-name", "Source", "--range", "B2:C4",
				"--" + targetFlag, "G7", "--target-sheet-id", "other", "--paste-type", "formats",
			})
			assertInputEquals(t, decodeToolInput(t, body, "transform_range"), map[string]interface{}{
				"excel_id": testToken, "sheet_name": "Source", "operation": "copy",
				"range": "B2:C4", "destination_range": "G7", "destination_sheet_id": "other", "paste_type": "format_only",
			})
		})
	}
	for _, sourceKey := range []string{"range", "src_range"} {
		t.Run("batch "+sourceKey, func(t *testing.T) {
			input := map[string]interface{}{
				"source_sheet_id": "source", sourceKey: "B2:C4", "copy_to_range": "G7",
				"target_sheet_id": "other", "paste_type": "formats",
			}
			translated, err := translateBatchOp(subOp("+range-copy", input), testToken, 0)
			require.NoError(t, err)
			require.Equal(t, "transform_range", translated["tool_name"])
			assertInputEquals(t, translated["input"].(map[string]interface{}), map[string]interface{}{
				"excel_id": testToken, "sheet_id": "source", "operation": "copy",
				"range": "B2:C4", "destination_range": "G7", "destination_sheet_id": "other", "paste_type": "format_only",
			})
		})
	}
}

func TestRangeCopyBatchAliasesKeepConflictsVisible(t *testing.T) {
	_, err := translateBatchOp(subOp("+range-copy", map[string]interface{}{
		"sheet_name": "S", "range": "A1", "source_range": "B1", "copy_to_range": "C1",
	}), testToken, 0)
	var validation *errs.ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, errs.SubtypeInvalidArgument, validation.Subtype)
	require.Equal(t, "--operations", validation.Param)
}
