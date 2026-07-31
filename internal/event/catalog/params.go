// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package catalog

import (
	"sort"
	"strings"

	"github.com/larksuite/cli/errs"
)

// ValidateParams applies declared defaults into params, then rejects missing
// required and undeclared parameters. It is the single implementation every
// layer validates against, so a decision can never accept parameters the
// runtime host would refuse.
func ValidateParams(def *KeyDefinition, params map[string]string) error {
	for _, p := range def.Params {
		if _, ok := params[p.Name]; !ok && p.Default != "" {
			params[p.Name] = p.Default
		}
	}
	for _, p := range def.Params {
		if p.Required {
			if _, ok := params[p.Name]; !ok {
				return errs.NewValidationError(errs.SubtypeInvalidArgument,
					"required param %q missing for EventKey %s", p.Name, def.Key).
					WithParam("--param").
					WithHint("pass it as --param %s=<value>; run `lark-cli event schema %s` for details", p.Name, def.Key)
			}
		}
	}
	known := make(map[string]bool, len(def.Params))
	validNames := make([]string, 0, len(def.Params))
	for _, p := range def.Params {
		known[p.Name] = true
		validNames = append(validNames, p.Name)
	}
	sort.Strings(validNames)
	for k := range params {
		if known[k] {
			continue
		}
		if len(validNames) == 0 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument,
				"unknown param %q: EventKey %s accepts no params", k, def.Key).
				WithParam("--param").
				WithHint("run `lark-cli event schema %s` for details", def.Key)
		}
		return errs.NewValidationError(errs.SubtypeInvalidArgument,
			"unknown param %q for EventKey %s. valid params: %s", k, def.Key, strings.Join(validNames, ", ")).
			WithParam("--param").
			WithHint("run `lark-cli event schema %s` for details", def.Key)
	}
	return nil
}
