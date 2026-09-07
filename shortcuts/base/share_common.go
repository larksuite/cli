// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

var shareAccessScopeEnums = []string{"invite", "tenant", "anyone"}

func validateSingleShareUpdate(runtime *common.RuntimeContext, flagNames ...string) error {
	changedNames := make([]string, 0, len(flagNames))
	for _, name := range flagNames {
		if runtime.Changed(name) {
			changedNames = append(changedNames, name)
		}
	}
	switch len(changedNames) {
	case 1:
		return nil
	case 0:
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "exactly one share field must be provided").
			WithHint("use one of: %s", shareFlagNames(flagNames))
	default:
		return baseFlagErrorf("share update accepts exactly one field; do not combine %s", shareFlagNames(changedNames))
	}
}

func shareFlagNames(flagNames []string) string {
	names := make([]string, 0, len(flagNames))
	for _, name := range flagNames {
		names = append(names, "--"+name)
	}
	return strings.Join(names, ", ")
}

func addCommonShareUpdateFields(runtime *common.RuntimeContext, body map[string]interface{}) {
	if runtime.Changed("enabled") {
		body["enabled"] = runtime.Bool("enabled")
	}
	if runtime.Changed("access-scope") {
		body["access_scope"] = runtime.Str("access-scope")
	}
}
