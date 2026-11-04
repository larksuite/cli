// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/util"
	"github.com/spf13/cobra"
)

// Check the effective params after typed flags override --params, before URL
// construction consumes path params or the query builder drops empty values.
func validateParameterConstraints(constraints *meta.ParameterConstraints, params map[string]interface{}) error {
	if constraints == nil || len(constraints.ExactlyOneOf) == 0 {
		return nil
	}
	provided := 0
	for _, name := range constraints.ExactlyOneOf {
		if !util.IsEmptyValue(params[name]) {
			provided++
		}
	}
	if provided == 1 {
		return nil
	}
	names := strings.Join(constraints.ExactlyOneOf, ", ")
	message := "exactly one of %s is required; provide one non-empty value"
	if provided > 1 {
		message = "%s are mutually exclusive; provide exactly one non-empty value"
	}
	reason := "provide one non-empty value"
	if provided > 1 {
		reason = "mutually exclusive; provide only one non-empty value"
	}
	invalid := make([]errs.InvalidParam, 0, len(constraints.ExactlyOneOf))
	for _, name := range constraints.ExactlyOneOf {
		invalid = append(invalid, errs.InvalidParam{Name: "--" + (meta.Field{Name: name}).FlagName(), Reason: reason})
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument, message, names).
		WithParams(invalid...).WithHint("Provide exactly one of " + names + " using its flag or --params; omit the other parameters.")
}

// Append after description truncation so the executable constraint is always
// shown in full on each participating flag's help line.
func (b *paramFlagBinder) addParameterConstraintHelp(cmd *cobra.Command, constraints *meta.ParameterConstraints) {
	if constraints == nil || len(constraints.ExactlyOneOf) == 0 {
		return
	}
	flags := make([]string, len(constraints.ExactlyOneOf))
	for i, name := range constraints.ExactlyOneOf {
		flags[i] = "--" + (meta.Field{Name: name}).FlagName()
	}
	note := "; mutually exclusive; exactly one required: " + strings.Join(flags, ", ")
	for _, name := range constraints.ExactlyOneOf {
		if !b.hasTypedFlag(name) {
			continue
		}
		if flag := cmd.Flags().Lookup((meta.Field{Name: name}).FlagName()); flag != nil {
			flag.Usage += note
		}
	}
}
