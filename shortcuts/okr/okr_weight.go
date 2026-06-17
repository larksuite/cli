// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// weightItem is the interface for items that have ID and weight.
type weightItem interface {
	GetID() string
	GetWeight() float64
}

// weightOp represents a single weight assignment.
type weightOp struct {
	ID     string  `json:"id"`
	Weight float64 `json:"weight"`
}

// parseWeightOps parses and validates the --weights JSON array.
func parseWeightOps(weightsStr string) ([]weightOp, error) {
	var ops []weightOp
	if err := json.Unmarshal([]byte(weightsStr), &ops); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--weights must be valid JSON array: %s", err).WithParam("--weights").WithCause(err)
	}
	if len(ops) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--weights must contain at least one weight assignment").WithParam("--weights")
	}

	seen := make(map[string]bool)
	var sum float64
	for i, op := range ops {
		if strings.TrimSpace(op.ID) == "" {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "weights[%d].id is required and cannot be empty", i).WithParam("--weights")
		}
		if op.Weight < 0 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "weights[%d].weight must be non-negative", i).WithParam("--weights")
		}
		if op.Weight > 1 {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "weights[%d].weight must be <= 1", i).WithParam("--weights")
		}
		// Check for at most 3 decimal places
		if math.Round(op.Weight*1000)/1000 != op.Weight {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "weights[%d].weight must have at most 3 decimal places", i).WithParam("--weights")
		}
		if seen[op.ID] {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "duplicate id %q in --weights", op.ID).WithParam("--weights")
		}
		seen[op.ID] = true
		sum += op.Weight
	}

	// Sum must be <= 1
	if sum > 1+1e-9 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "sum of weights must be <= 1, got %.6f", sum).WithParam("--weights")
	}

	return ops, nil
}

// formatWeight formats a fixed-point weight value as a json.Number with exactly 3 decimal places.
// This ensures precise JSON serialization and avoids float64 precision issues.
func formatWeight(fp int64) json.Number {
	return json.Number(fmt.Sprintf("%d.%03d", fp/1000, fp%1000))
}

// normalizeWeights normalizes weights using fixed-point arithmetic (×1000).
//   - Specified weights are used as-is (already validated to 3 decimal places).
//   - Remaining weight (1 - sum_specified) is distributed to unspecified items
//     proportionally based on their original weights.
//   - Fixed-point arithmetic ensures exact sum = 1, with residual added to the last item.
//   - Weights are returned as json.Number to avoid float64 precision issues in JSON serialization.
func normalizeWeights[T weightItem](
	items []T,
	ops []weightOp,
) ([]map[string]interface{}, error) {
	const scale = 1000 // fixed-point scale for 3 decimal places

	// Build map of specified weights (as fixed-point integers)
	specified := make(map[string]int64)
	var specifiedSum int64
	for _, op := range ops {
		fp := int64(math.Round(op.Weight * scale))
		specified[op.ID] = fp
		specifiedSum += fp
	}

	// Calculate remaining weight to distribute (as fixed-point)
	remaining := scale - specifiedSum
	if remaining < 0 {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "weight calculation error: remaining weight is negative")
	}

	// Collect unspecified items and their original weights
	type itemWithWeight struct {
		item T
		fp   int64 // original weight as fixed-point
	}
	var unspecified []itemWithWeight
	var originalUnspecifiedSum int64

	for _, item := range items {
		id := item.GetID()
		if _, ok := specified[id]; ok {
			continue
		}
		origWeight := item.GetWeight()
		if origWeight < 0 {
			origWeight = 0
		}
		fp := int64(math.Round(origWeight * scale))
		unspecified = append(unspecified, itemWithWeight{item: item, fp: fp})
		originalUnspecifiedSum += fp
	}

	// Distribute remaining weight proportionally
	result := make([]map[string]interface{}, 0, len(items))
	var resultSum int64

	// First add specified items in original order
	for _, item := range items {
		id := item.GetID()
		if fp, ok := specified[id]; ok {
			result = append(result, map[string]interface{}{
				"id":     id,
				"weight": formatWeight(fp),
			})
			resultSum += fp
		}
	}

	// Then distribute to unspecified items
	if len(unspecified) > 0 && remaining > 0 {
		if originalUnspecifiedSum == 0 {
			// All original weights are zero, distribute evenly
			perItem := remaining / int64(len(unspecified))
			residual := remaining - perItem*int64(len(unspecified))

			for i, uw := range unspecified {
				fp := perItem
				// Add residual to the last unspecified item
				if i == len(unspecified)-1 {
					fp += residual
				}
				result = append(result, map[string]interface{}{
					"id":     uw.item.GetID(),
					"weight": formatWeight(fp),
				})
				resultSum += fp
			}
		} else {
			// Distribute proportionally based on original weights
			var distributed int64
			for i, uw := range unspecified {
				var fp int64
				if i == len(unspecified)-1 {
					// Last item gets the remainder to ensure exact sum
					fp = remaining - distributed
				} else {
					// Proportional distribution
					fp = int64(float64(remaining) * float64(uw.fp) / float64(originalUnspecifiedSum))
					distributed += fp
				}
				result = append(result, map[string]interface{}{
					"id":     uw.item.GetID(),
					"weight": formatWeight(fp),
				})
				resultSum += fp
			}
		}
	} else if remaining > 0 {
		// All items were specified, add residual to the last item
		if len(result) > 0 {
			lastIdx := len(result) - 1
			// Parse current weight as fixed-point and add residual
			var lastFP int64
			if lastWeight, ok := result[lastIdx]["weight"].(json.Number); ok {
				if f, err := lastWeight.Float64(); err == nil {
					lastFP = int64(math.Round(f * scale))
				}
			}
			result[lastIdx]["weight"] = formatWeight(lastFP + remaining)
			resultSum += remaining
		}
	}

	// Verify sum is exactly 1.0
	if resultSum != scale {
		return nil, errs.NewInternalError(errs.SubtypeUnknown,
			"weight normalization error: sum is %.6f, expected 1.0", float64(resultSum)/scale)
	}

	return result, nil
}

// GetWeight implements the interface for Objective.
func (o Objective) GetWeight() float64 {
	if o.Weight == nil {
		return 0
	}
	return *o.Weight
}

// GetWeight implements the interface for KeyResult.
func (k KeyResult) GetWeight() float64 {
	if k.Weight == nil {
		return 0
	}
	return *k.Weight
}

// OKRWeight adjusts the weight of objectives or key results.
var OKRWeight = common.Shortcut{
	Service:     "okr",
	Command:     "+weight",
	Description: "Adjust the weight of OKR objectives or key results",
	Risk:        "write",
	Scopes:      []string{"okr:okr.content:writeonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "level", Desc: "level to adjust: objective | key-result", Enum: []string{"objective", "key-result"}, Required: true},
		{Name: "cycle-id", Desc: "OKR cycle ID (int64)", Required: true},
		{Name: "objective-id", Desc: "objective ID (required when --level=key-result)"},
		{Name: "weights", Desc: "JSON array of weight assignments: [{\"id\":\"...\",\"weight\":0.5}]", Input: []string{common.File, common.Stdin}, Required: true},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		level := runtime.Str("level")
		if level != "objective" && level != "key-result" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--level must be one of: objective | key-result").WithParam("--level")
		}

		cycleID := runtime.Str("cycle-id")
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

		weightsStr := runtime.Str("weights")
		if err := common.RejectDangerousCharsTyped("--weights", weightsStr); err != nil {
			return err
		}
		if _, err := parseWeightOps(weightsStr); err != nil {
			return err
		}

		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		level := runtime.Str("level")
		cycleID := runtime.Str("cycle-id")
		objectiveID := runtime.Str("objective-id")
		ops, _ := parseWeightOps(runtime.Str("weights"))

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
				Desc("Fetch all objectives in the cycle to get current weights for normalization")

			// Then update weights
			weightParams := map[string]interface{}{
				"cycle_id": cycleID,
			}
			// Build sample body
			objectiveWeights := make([]map[string]interface{}, 0, len(ops))
			for _, op := range ops {
				objectiveWeights = append(objectiveWeights, map[string]interface{}{
					"objective_id": op.ID,
					"weight":       op.Weight,
				})
			}
			weightBody := map[string]interface{}{
				"objective_weights": objectiveWeights,
			}
			weightPath := fmt.Sprintf("/open-apis/okr/v2/cycles/%s/objectives_weight", cycleID)
			apis = apis.
				PUT(weightPath).
				Params(weightParams).
				Body(weightBody).
				Desc("Update objective weights (full list sent after normalization)")
		} else {
			// key-result level
			listParams := map[string]interface{}{
				"page_size": 100,
			}
			listPath := fmt.Sprintf("/open-apis/okr/v2/objectives/%s/key_results", objectiveID)
			apis = apis.
				GET(listPath).
				Params(listParams).
				Desc("Fetch all key results for the objective to get current weights for normalization")

			weightParams := map[string]interface{}{
				"objective_id": objectiveID,
			}
			// Build sample body
			keyResultWeights := make([]map[string]interface{}, 0, len(ops))
			for _, op := range ops {
				keyResultWeights = append(keyResultWeights, map[string]interface{}{
					"key_result_id": op.ID,
					"weight":        op.Weight,
				})
			}
			weightBody := map[string]interface{}{
				"key_result_weights": keyResultWeights,
			}
			weightPath := fmt.Sprintf("/open-apis/okr/v2/objectives/%s/key_results_weight", objectiveID)
			apis = apis.
				PUT(weightPath).
				Params(weightParams).
				Body(weightBody).
				Desc("Update key result weights (full list sent after normalization)")
		}

		return apis
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		level := runtime.Str("level")
		cycleID := runtime.Str("cycle-id")
		objectiveID := runtime.Str("objective-id")
		ops, err := parseWeightOps(runtime.Str("weights"))
		if err != nil {
			return err
		}

		var normalizedWeights []map[string]interface{}
		var total int

		if level == "objective" {
			objectives, err := fetchObjectives(ctx, runtime, cycleID)
			if err != nil {
				return err
			}
			total = len(objectives)

			// Validate all specified IDs exist
			objIDs := make(map[string]bool)
			for _, obj := range objectives {
				objIDs[obj.ID] = true
			}
			for _, op := range ops {
				if !objIDs[op.ID] {
					return errs.NewValidationError(errs.SubtypeInvalidArgument, "objective id %q not found in cycle", op.ID).WithParam("--weights")
				}
			}

			normalizedWeights, err = normalizeWeights(objectives, ops)
			if err != nil {
				return err
			}

			// Build position map for sorting
			posMap := make(map[string]int32)
			for _, obj := range objectives {
				if obj.Position != nil {
					posMap[obj.ID] = *obj.Position
				}
			}

			// Submit weight update
			params := map[string]interface{}{
				"cycle_id": cycleID,
			}
			objectiveWeights := make([]map[string]interface{}, 0, len(normalizedWeights))
			for _, w := range normalizedWeights {
				objectiveWeights = append(objectiveWeights, map[string]interface{}{
					"objective_id": w["id"],
					"weight":       w["weight"],
				})
			}
			// Sort by position to match API requirements
			sort.Slice(objectiveWeights, func(i, j int) bool {
				idI := objectiveWeights[i]["objective_id"].(string)
				idJ := objectiveWeights[j]["objective_id"].(string)
				return posMap[idI] < posMap[idJ]
			})
			body := map[string]interface{}{
				"objective_weights": objectiveWeights,
			}
			path := fmt.Sprintf("/open-apis/okr/v2/cycles/%s/objectives_weight", cycleID)
			_, err = runtime.CallAPITyped("PUT", path, params, body)
			if err != nil {
				return wrapOkrNetworkErr(err, "failed to update objective weights")
			}
		} else {
			// key-result level
			keyResults, err := fetchKeyResults(ctx, runtime, objectiveID)
			if err != nil {
				return err
			}
			total = len(keyResults)

			// Validate all specified IDs exist
			krIDs := make(map[string]bool)
			for _, kr := range keyResults {
				krIDs[kr.ID] = true
			}
			for _, op := range ops {
				if !krIDs[op.ID] {
					return errs.NewValidationError(errs.SubtypeInvalidArgument, "key_result id %q not found in objective", op.ID).WithParam("--weights")
				}
			}

			normalizedWeights, err = normalizeWeights(keyResults, ops)
			if err != nil {
				return err
			}

			// Build position map for sorting
			posMap := make(map[string]int32)
			for _, kr := range keyResults {
				if kr.Position != nil {
					posMap[kr.ID] = *kr.Position
				}
			}

			// Submit weight update
			params := map[string]interface{}{
				"objective_id": objectiveID,
			}
			keyResultWeights := make([]map[string]interface{}, 0, len(normalizedWeights))
			for _, w := range normalizedWeights {
				keyResultWeights = append(keyResultWeights, map[string]interface{}{
					"key_result_id": w["id"],
					"weight":        w["weight"],
				})
			}
			// Sort by position to match API requirements
			sort.Slice(keyResultWeights, func(i, j int) bool {
				idI := keyResultWeights[i]["key_result_id"].(string)
				idJ := keyResultWeights[j]["key_result_id"].(string)
				return posMap[idI] < posMap[idJ]
			})
			body := map[string]interface{}{
				"key_result_weights": keyResultWeights,
			}
			path := fmt.Sprintf("/open-apis/okr/v2/objectives/%s/key_results_weight", objectiveID)
			_, err = runtime.CallAPITyped("PUT", path, params, body)
			if err != nil {
				return wrapOkrNetworkErr(err, "failed to update key result weights")
			}
		}

		// Build response
		result := map[string]interface{}{
			"level":    level,
			"cycle_id": cycleID,
			"total":    total,
			"weights":  normalizedWeights,
		}

		runtime.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Successfully updated weights for %d %s(s)\n", total, level)
			fmt.Fprintln(w, "Weights:")
			for _, weightEntry := range normalizedWeights {
				fmt.Fprintf(w, "  %s: %v\n", weightEntry["id"], weightEntry["weight"])
			}
		})

		return nil
	},
}
