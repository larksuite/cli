// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// reorderItem is the interface for items that have an ID.
type reorderItem interface {
	GetID() string
}

// reorderOp represents a single reorder operation.
type reorderOp struct {
	ID       string `json:"id"`
	Position int32  `json:"position"`
}

// parseReorderOps parses and validates the --ops JSON array.
func parseReorderOps(opsStr string) ([]reorderOp, error) {
	var ops []reorderOp
	if err := json.Unmarshal([]byte(opsStr), &ops); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--ops must be valid JSON array: %s", err).WithParam("--ops").WithCause(err)
	}
	if len(ops) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--ops must contain at least one operation").WithParam("--ops")
	}

	seen := make(map[string]bool)
	seenPos := make(map[int32]bool)
	for i, op := range ops {
		if strings.TrimSpace(op.ID) == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "ops[%d].id is required and cannot be empty", i).WithParam("--ops")
		}
		if op.Position <= 0 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "ops[%d].position must be a positive integer", i).WithParam("--ops")
		}
		if seen[op.ID] {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "duplicate id %q in --ops", op.ID).WithParam("--ops")
		}
		if seenPos[op.Position] {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "duplicate position %d in --ops", op.Position).WithParam("--ops")
		}
		seen[op.ID] = true
		seenPos[op.Position] = true
	}
	return ops, nil
}

// fetchObjectives fetches all objectives in a cycle.
func fetchObjectives(ctx context.Context, runtime *common.RuntimeContext, cycleID string) ([]Objective, error) {
	queryParams := map[string]interface{}{"page_size": "100"}
	var objectives []Objective
	page := 0

	for {
		if page > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
		page++

		path := fmt.Sprintf("/open-apis/okr/v2/cycles/%s/objectives", cycleID)
		data, err := runtime.CallAPITyped("GET", path, queryParams, nil)
		if err != nil {
			return nil, wrapOkrNetworkErr(err, "failed to fetch objectives")
		}

		itemsRaw, _ := data["items"].([]interface{})
		for _, item := range itemsRaw {
			raw, err := json.Marshal(item)
			if err != nil {
				continue
			}
			var obj Objective
			if err := json.Unmarshal(raw, &obj); err != nil {
				continue
			}
			objectives = append(objectives, obj)
		}

		hasMore, pageToken := common.PaginationMeta(data)
		if !hasMore || pageToken == "" {
			break
		}
		queryParams["page_token"] = pageToken
	}

	// Sort objectives by position
	sort.Slice(objectives, func(i, j int) bool {
		pi := int32(0)
		if objectives[i].Position != nil {
			pi = *objectives[i].Position
		}
		pj := int32(0)
		if objectives[j].Position != nil {
			pj = *objectives[j].Position
		}
		return pi < pj
	})

	return objectives, nil
}

// fetchKeyResults fetches all key results for an objective.
func fetchKeyResults(ctx context.Context, runtime *common.RuntimeContext, objectiveID string) ([]KeyResult, error) {
	queryParams := map[string]interface{}{"page_size": "100"}
	var keyResults []KeyResult
	page := 0

	for {
		if page > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
		}
		page++

		path := fmt.Sprintf("/open-apis/okr/v2/objectives/%s/key_results", objectiveID)
		data, err := runtime.CallAPITyped("GET", path, queryParams, nil)
		if err != nil {
			return nil, wrapOkrNetworkErr(err, "failed to fetch key results")
		}

		itemsRaw, _ := data["items"].([]interface{})
		for _, item := range itemsRaw {
			raw, err := json.Marshal(item)
			if err != nil {
				continue
			}
			var kr KeyResult
			if err := json.Unmarshal(raw, &kr); err != nil {
				continue
			}
			keyResults = append(keyResults, kr)
		}

		hasMore, pageToken := common.PaginationMeta(data)
		if !hasMore || pageToken == "" {
			break
		}
		queryParams["page_token"] = pageToken
	}

	// Sort key results by position
	sort.Slice(keyResults, func(i, j int) bool {
		pi := int32(0)
		if keyResults[i].Position != nil {
			pi = *keyResults[i].Position
		}
		pj := int32(0)
		if keyResults[j].Position != nil {
			pj = *keyResults[j].Position
		}
		return pi < pj
	})

	return keyResults, nil
}

// buildReorderedIDs builds the complete ordered ID list from current items and reorder ops.
// Positions are treated as 1-indexed placement keys stored in a map (safe for large values).
// Items are first placed at user-specified positions, remaining items fill empty slots
// in original order starting from position 1, and final output is sorted by position.
func buildReorderedIDs[T reorderItem](items []T, ops []reorderOp, total int) ([]string, error) {
	// Create a map of ID to current position
	idToPos := make(map[string]int)
	for i, item := range items {
		idToPos[item.GetID()] = i
	}

	// Validate all ops IDs exist
	for _, op := range ops {
		if _, ok := idToPos[op.ID]; !ok {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "id %q not found in current list", op.ID).WithParam("--ops")
		}
	}

	// Use map to store position -> ID (1-indexed, safe for large position values)
	posToID := make(map[int]string)
	used := make(map[string]bool)
	for _, op := range ops {
		posToID[int(op.Position)] = op.ID
		used[op.ID] = true
	}

	// Collect unused items in original order
	var unused []string
	for _, item := range items {
		id := item.GetID()
		if !used[id] {
			unused = append(unused, id)
		}
	}

	// Fill empty slots starting from position 1, in original order
	unusedIdx := 0
	for pos := 1; unusedIdx < len(unused); pos++ {
		if _, occupied := posToID[pos]; !occupied {
			posToID[pos] = unused[unusedIdx]
			unusedIdx++
		}
	}

	// Collect all positions, sort them, and build result in position order
	positions := make([]int, 0, len(posToID))
	for pos := range posToID {
		positions = append(positions, pos)
	}
	sort.Ints(positions)

	result := make([]string, 0, len(positions))
	for _, pos := range positions {
		result = append(result, posToID[pos])
	}

	return result, nil
}

// GetID implements the interface for Objective.
func (o Objective) GetID() string { return o.ID }

// GetID implements the interface for KeyResult.
func (k KeyResult) GetID() string { return k.ID }

// OKRReorder adjusts the position of objectives or key results.
var OKRReorder = common.Shortcut{
	Service:     "okr",
	Command:     "+reorder",
	Description: "Adjust the position (order) of OKR objectives or key results",
	Risk:        "write",
	Scopes:      []string{"okr:okr.content:writeonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "level", Desc: "level to reorder: objective | key-result, Required.", Enum: []string{"objective", "key-result"}},
		{Name: "cycle-id", Desc: "OKR cycle ID (int64), Required."},
		{Name: "objective-id", Desc: "objective ID (required when --level=key-result)"},
		{Name: "ops", Desc: "JSON array of reorder operations: [{\"id\":\"...\",\"position\":1}], Required.", Input: []string{common.File, common.Stdin}},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		level := runtime.Str("level")
		if strings.TrimSpace(level) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--level is required").WithParam("--level")
		}
		if level != "objective" && level != "key-result" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--level must be one of: objective | key-result").WithParam("--level")
		}

		cycleID := runtime.Str("cycle-id")
		if strings.TrimSpace(cycleID) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--cycle-id is required").WithParam("--cycle-id")
		}
		if id, err := strconv.ParseInt(cycleID, 10, 64); err != nil || id <= 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--cycle-id must be a positive int64").WithParam("--cycle-id")
		}

		if level == "key-result" {
			objID := runtime.Str("objective-id")
			if objID == "" {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--objective-id is required when --level=key-result").WithParam("--objective-id")
			}
			if id, err := strconv.ParseInt(objID, 10, 64); err != nil || id <= 0 {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "--objective-id must be a positive int64").WithParam("--objective-id")
			}
		}

		opsStr := runtime.Str("ops")
		if strings.TrimSpace(opsStr) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--ops is required").WithParam("--ops")
		}
		if err := common.RejectDangerousCharsTyped("--ops", opsStr); err != nil {
			return err
		}
		if _, err := parseReorderOps(opsStr); err != nil {
			return err
		}

		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		level := runtime.Str("level")
		cycleID := runtime.Str("cycle-id")
		objectiveID := runtime.Str("objective-id")
		ops, _ := parseReorderOps(runtime.Str("ops"))

		apis := common.NewDryRunAPI()

		if level == "objective" {
			// First fetch objectives
			listParams := map[string]interface{}{
				"page_size": 100,
			}
			listPath := fmt.Sprintf("/open-apis/okr/v2/cycles/%s/objectives", cycleID)
			apis = apis.
				GET(listPath).
				Params(listParams).
				Desc("Fetch all objectives in the cycle to determine current order")

			// Then reorder
			reorderParams := map[string]interface{}{
				"cycle_id": cycleID,
			}
			// Build sample body with placeholder IDs
			objectiveIDs := make([]string, 0, len(ops))
			for _, op := range ops {
				objectiveIDs = append(objectiveIDs, op.ID)
			}
			reorderBody := map[string]interface{}{
				"objective_ids": objectiveIDs,
			}
			reorderPath := fmt.Sprintf("/open-apis/okr/v2/cycles/%s/objectives_position", cycleID)
			apis = apis.
				PUT(reorderPath).
				Params(reorderParams).
				Body(reorderBody).
				Desc("Update objective positions (full list sent, not just changes)")
		} else {
			// key-result level
			listParams := map[string]interface{}{
				"page_size": 100,
			}
			listPath := fmt.Sprintf("/open-apis/okr/v2/objectives/%s/key_results", objectiveID)
			apis = apis.
				GET(listPath).
				Params(listParams).
				Desc("Fetch all key results for the objective to determine current order")

			reorderParams := map[string]interface{}{
				"objective_id": objectiveID,
			}
			// Build sample body with placeholder IDs
			keyResultIDs := make([]string, 0, len(ops))
			for _, op := range ops {
				keyResultIDs = append(keyResultIDs, op.ID)
			}
			reorderBody := map[string]interface{}{
				"key_result_ids": keyResultIDs,
			}
			reorderPath := fmt.Sprintf("/open-apis/okr/v2/objectives/%s/key_results_position", objectiveID)
			apis = apis.
				PUT(reorderPath).
				Params(reorderParams).
				Body(reorderBody).
				Desc("Update key result positions (full list sent, not just changes)")
		}

		return apis
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		level := runtime.Str("level")
		cycleID := runtime.Str("cycle-id")
		objectiveID := runtime.Str("objective-id")
		ops, err := parseReorderOps(runtime.Str("ops"))
		if err != nil {
			return err
		}

		var reorderedIDs []string
		var total int

		if level == "objective" {
			objectives, err := fetchObjectives(ctx, runtime, cycleID)
			if err != nil {
				return err
			}
			total = len(objectives)

			reorderedIDs, err = buildReorderedIDs(objectives, ops, total)
			if err != nil {
				return err
			}

			// Submit reorder
			params := map[string]interface{}{
				"cycle_id": cycleID,
			}
			body := map[string]interface{}{
				"objective_ids": reorderedIDs,
			}
			path := fmt.Sprintf("/open-apis/okr/v2/cycles/%s/objectives_position", cycleID)
			_, err = runtime.CallAPITyped("PUT", path, params, body)
			if err != nil {
				return wrapOkrNetworkErr(err, "failed to update objective positions")
			}
		} else {
			// key-result level
			keyResults, err := fetchKeyResults(ctx, runtime, objectiveID)
			if err != nil {
				return err
			}
			total = len(keyResults)

			reorderedIDs, err = buildReorderedIDs(keyResults, ops, total)
			if err != nil {
				return err
			}

			// Submit reorder
			params := map[string]interface{}{
				"objective_id": objectiveID,
			}
			body := map[string]interface{}{
				"key_result_ids": reorderedIDs,
			}
			path := fmt.Sprintf("/open-apis/okr/v2/objectives/%s/key_results_position", objectiveID)
			_, err = runtime.CallAPITyped("PUT", path, params, body)
			if err != nil {
				return wrapOkrNetworkErr(err, "failed to update key result positions")
			}
		}

		// Build response
		result := map[string]interface{}{
			"level":    level,
			"cycle_id": cycleID,
			"total":    total,
			"ordered":  reorderedIDs,
		}

		runtime.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Successfully reordered %d %s(s)\n", total, level)
			fmt.Fprintln(w, "New order:")
			for i, id := range reorderedIDs {
				fmt.Fprintf(w, "  Position %d: %s\n", i+1, id)
			}
		})

		return nil
	},
}
