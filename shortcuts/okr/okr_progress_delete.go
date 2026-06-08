// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"fmt"
	"io"
	"strconv"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// OKRDeleteProgressRecord deletes a progress by ID.
var OKRDeleteProgressRecord = common.Shortcut{
	Service:     "okr",
	Command:     "+progress-delete",
	Description: "Delete an OKR progress by ID",
	Risk:        "high-risk-write",
	Scopes:      []string{"okr:okr.progress:delete"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "progress-id", Desc: "progress ID (int64)", Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		progressID := runtime.Str("progress-id")
		if progressID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--progress-id is required").WithParam("--progress-id")
		}
		if id, err := strconv.ParseInt(progressID, 10, 64); err != nil || id <= 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--progress-id must be a positive int64").WithParam("--progress-id")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		progressID := runtime.Str("progress-id")
		return common.NewDryRunAPI().
			DELETE("/open-apis/okr/v1/progress_records/:progress_id").
			Set("progress_id", progressID).
			Desc("Delete OKR progress")
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		progressID := runtime.Str("progress-id")

		path := fmt.Sprintf("/open-apis/okr/v1/progress_records/%s", progressID)
		_, err := runtime.CallAPITyped("DELETE", path, nil, nil)
		if err != nil {
			return err
		}

		result := map[string]interface{}{
			"deleted":     true,
			"progress_id": progressID,
		}

		runtime.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Deleted progress record %s\n", progressID)
		})
		return nil
	},
}
