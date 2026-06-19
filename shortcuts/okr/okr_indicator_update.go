// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"context"
	"fmt"
	"io"
	"math"
	"strconv"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// parseIndicatorValue parses and validates the indicator value.
func parseIndicatorValue(valueStr string) (float64, error) {
	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return 0, errs.NewValidationError(errs.SubtypeInvalidArgument, "--value must be a number between -99999999999 and 99999999999").WithParam("--value").WithCause(err)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) || value < -99999999999 || value > 99999999999 {
		return 0, errs.NewValidationError(errs.SubtypeInvalidArgument, "--value must be a number between -99999999999 and 99999999999").WithParam("--value")
	}
	return value, nil
}

// fetchIndicatorID fetches the indicator ID for an objective or key result.
// The indicators.list API returns a single indicator object (not a list),
// which always exists (may be a default empty indicator).
func fetchIndicatorID(ctx context.Context, runtime *common.RuntimeContext, level string, id string) (string, error) {
	var path string
	var params map[string]interface{}

	if level == "objective" {
		path = fmt.Sprintf("/open-apis/okr/v2/objectives/%s/indicators", id)
		params = map[string]interface{}{"page_size": 100}
	} else {
		path = fmt.Sprintf("/open-apis/okr/v2/key_results/%s/indicators", id)
		params = map[string]interface{}{"page_size": 100}
	}

	data, err := runtime.CallAPITyped("GET", path, params, nil)
	if err != nil {
		return "", wrapOkrNetworkErr(err, "failed to fetch indicators")
	}

	// Parse response to get indicator ID
	// Response format: {"indicator": {"id": "...", ...}} (single object, not a list)
	indicator, ok := data["indicator"].(map[string]interface{})
	if !ok {
		return "", errs.NewInternalError(errs.SubtypeUnknown, "indicator field not found in response")
	}

	indicatorID, ok := indicator["id"].(string)
	if !ok || indicatorID == "" {
		return "", errs.NewInternalError(errs.SubtypeUnknown, "indicator ID not found or empty")
	}

	return indicatorID, nil
}

// OKRIndicatorUpdate updates the current value of an indicator for an objective or key result.
var OKRIndicatorUpdate = common.Shortcut{
	Service:     "okr",
	Command:     "+indicator-update",
	Description: "Update the indicator current value for an objective or key result",
	Risk:        "write",
	Scopes:      []string{"okr:okr.content:writeonly"},
	AuthTypes:   []string{"user", "bot"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "level", Desc: "level to update: objective | key-result, Required.", Enum: []string{"objective", "key-result"}},
		{Name: "id", Desc: "objective or key result ID (int64), Required."},
		{Name: "value", Desc: "new current value for the indicator (number), Required."},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		level := runtime.Str("level")
		if level == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--level is required").WithParam("--level")
		}
		if level != "objective" && level != "key-result" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--level must be one of: objective | key-result").WithParam("--level")
		}

		id := runtime.Str("id")
		if id == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--id is required").WithParam("--id")
		}
		if _, err := strconv.ParseInt(id, 10, 64); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--id must be a valid int64").WithParam("--id")
		}
		if runtime.Str("value") == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--value is required").WithParam("--value")
		}
		if _, err := parseIndicatorValue(runtime.Str("value")); err != nil {
			return err
		}

		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		level := runtime.Str("level")
		id := runtime.Str("id")
		value, _ := parseIndicatorValue(runtime.Str("value"))

		apis := common.NewDryRunAPI()

		var listPath string
		if level == "objective" {
			listPath = fmt.Sprintf("/open-apis/okr/v2/objectives/%s/indicators", id)
		} else {
			listPath = fmt.Sprintf("/open-apis/okr/v2/key_results/%s/indicators", id)
		}

		// First API: fetch indicator list
		apis = apis.
			GET(listPath).
			Params(map[string]interface{}{"page_size": 100}).
			Desc(fmt.Sprintf("Fetch indicators for the %s to get indicator ID", level))

		// Second API: patch indicator value
		patchPath := "/open-apis/okr/v2/indicators/:indicator_id"
		patchBody := map[string]interface{}{
			"current_value": value,
		}
		apis = apis.
			PATCH(patchPath).
			Body(patchBody).
			Set("indicator_id", "<indicator_id_from_list>").
			Desc("Update indicator current value")

		return apis
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		level := runtime.Str("level")
		id := runtime.Str("id")
		value, err := parseIndicatorValue(runtime.Str("value"))
		if err != nil {
			return err
		}

		// Step 1: Fetch indicator ID
		indicatorID, err := fetchIndicatorID(ctx, runtime, level, id)
		if err != nil {
			return err
		}

		// Step 2: Update indicator value
		patchPath := fmt.Sprintf("/open-apis/okr/v2/indicators/%s", indicatorID)
		patchBody := map[string]interface{}{
			"current_value": value,
		}

		_, err = runtime.CallAPITyped("PATCH", patchPath, nil, patchBody)
		if err != nil {
			return wrapOkrNetworkErr(err, "failed to update indicator value")
		}

		// Build response
		result := map[string]interface{}{
			"indicator_id":  indicatorID,
			"current_value": value,
			"level":         level,
			"target_id":     id,
		}

		runtime.OutFormat(result, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Updated Indicator [%s]\n", indicatorID)
			fmt.Fprintf(w, "  Level: %s\n", level)
			fmt.Fprintf(w, "  Target ID: %s\n", id)
			fmt.Fprintf(w, "  Current Value: %v\n", value)
		})

		return nil
	},
}
