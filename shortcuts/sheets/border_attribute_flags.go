// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// Attribute flags refine explicitly selected border sides; they must not
// choose new sides or discard attributes from an explicit composite spec.
func borderAttributeFlags(flags interface {
	Str(string) string
	Changed(string) bool
}) (string, bool, error) {
	source := ""
	for _, name := range []string{"border-color", "border-weight"} {
		if flags.Changed(name) {
			source = name
			break
		}
	}
	if source == "" {
		return "", false, nil
	}
	if !flags.Changed("border-styles") {
		return "", true, sheetsValidationForFlag(source, "--%s needs an explicit border side selection", source).WithHint(`pass --border-styles '{"all":{"style":"solid"}}' or an explicit per-side spec`)
	}
	fv := newMapFlagViewForCommand("+cells-set-style", map[string]interface{}{"border-styles": flags.Str("border-styles")})
	value, err := parseJSONFlag(fv, "border-styles")
	if err != nil {
		return "", true, err
	}
	border, ok := value.(map[string]interface{})
	if !ok || len(border) == 0 {
		return "", true, sheetsValidationForFlag("border-styles", "--border-styles must select at least one border side")
	}
	if err := validateWorkbookBorderStyles(border, "--border-styles"); err != nil {
		var validation *errs.ValidationError
		if errors.As(err, &validation) {
			return "", true, validation.WithParam("--border-styles")
		}
		return "", true, err
	}
	for _, name := range []string{"border-color", "border-weight"} {
		if !flags.Changed(name) {
			continue
		}
		attr := "color"
		if name == "border-weight" {
			attr = "weight"
		}
		probe := map[string]interface{}{attr: flags.Str(name)}
		normalizeBorderSideVocab(probe)
		if err := validateWorkbookBorderStyles(map[string]interface{}{"top": probe}, "--"+name); err != nil {
			var validation *errs.ValidationError
			if errors.As(err, &validation) {
				return "", true, validation.WithParam("--" + name)
			}
			return "", true, err
		}
		for _, side := range sortedKeys(border) {
			spec, ok := border[side].(map[string]interface{})
			if !ok {
				return "", true, sheetsValidationForFlag("border-styles", "--border-styles.%s must be a JSON object", side)
			}
			if given, present := spec[attr]; present && given != probe[attr] {
				return "", true, sheetsValidationForFlag(name, "--%s conflicts with --border-styles.%s.%s; keep one value", name, side, attr)
			}
			spec[attr] = probe[attr]
		}
	}
	encoded, err := json.Marshal(border)
	if err != nil {
		return "", true, errs.NewInternalError(errs.SubtypeSDKError, "cannot encode normalized border attributes").WithCause(err)
	}
	return string(encoded), true, nil
}

func normalizeBorderAttributeFlags(_ context.Context, flags *common.FlagContext) error {
	border, changed, err := borderAttributeFlags(flags)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}
	return flags.SetCanonical("border-styles", border)
}
