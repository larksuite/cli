// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// dashboardChartLinkage is the same-datasource write contract. Keep the empty
// array: it clears linkage, whereas an omitted linkage leaves it unchanged.
type dashboardChartLinkage struct {
	TargetBlockIDs []string `json:"target_block_ids"`
}

func parseDashboardChartLinkage(pc *parseCtx, raw, sourceID string) (*dashboardChartLinkage, error) {
	resolved, err := loadJSONInput(pc, raw, "linkage")
	if err != nil {
		return nil, err
	}
	var linkage *dashboardChartLinkage
	decoder := json.NewDecoder(strings.NewReader(resolved))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&linkage); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--linkage must be a JSON object containing only target_block_ids: %v", err).
			WithParam("--linkage").WithCause(err)
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--linkage must contain exactly one JSON object").WithParam("--linkage").WithCause(err)
	}
	if linkage == nil || linkage.TargetBlockIDs == nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--linkage requires a non-null target_block_ids array; use [] to clear linkage").WithParam("--linkage")
	}
	if len(linkage.TargetBlockIDs) > 100 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--linkage accepts at most 100 target block IDs").WithParam("--linkage")
	}
	seen := make(map[string]struct{}, len(linkage.TargetBlockIDs))
	for _, id := range linkage.TargetBlockIDs {
		if strings.TrimSpace(id) == "" || len(id) > 4096 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--linkage target block IDs must be non-empty strings of at most 4096 bytes").WithParam("--linkage")
		}
		if id == sourceID {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--linkage cannot target the source block itself").WithParam("--linkage")
		}
		if _, exists := seen[id]; exists {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
				"--linkage target block IDs must be unique").WithParam("--linkage")
		}
		seen[id] = struct{}{}
	}
	return linkage, nil
}

// validateDashboardChartLinkage is deliberately offline, including for dry-run.
// Actual datasource identity and component eligibility are checked by the API.
// --no-validate does not bypass the supported linkage input shape.
func validateDashboardChartLinkage(pc *parseCtx, runtime *common.RuntimeContext) error {
	if !runtime.Cmd.Flags().Changed("linkage") {
		return nil
	}
	linkage, err := parseDashboardChartLinkage(pc, runtime.Str("linkage"), runtime.Str("block-id"))
	if err != nil {
		return err
	}
	// Freeze @file input before DryRun/Execute so it is opened exactly once.
	encoded, err := json.Marshal(linkage)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "cannot encode dashboard linkage").WithCause(err)
	}
	return runtime.Cmd.Flags().Set("linkage", string(encoded))
}
