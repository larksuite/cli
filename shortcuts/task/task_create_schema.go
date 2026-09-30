// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"encoding/json"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/registry"
	metaschema "github.com/larksuite/cli/internal/schema"
)

const taskCreateMethodPath = "task.tasks.create"

func printTaskCreateDataFlagSchema(flagName string) ([]byte, error) {
	flagName = strings.TrimSpace(flagName)
	if flagName == "" {
		return json.MarshalIndent(map[string]any{
			"method":               taskCreateMethodPath,
			"introspectable_flags": []string{"data"},
			"hint":                 "run again with --flag-name data to dump its JSON Schema; append a dotted property path to inspect one nested field",
		}, "", "  ")
	}

	path := strings.Split(flagName, ".")
	if path[0] != "data" {
		return nil, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"schema inspection is available for --data, not %q",
			flagName,
		).WithParam("--flag-name")
	}

	snapshot, err := registry.OpenSnapshot()
	if err != nil {
		return nil, err
	}
	target, err := snapshot.Catalog().Resolve(apicatalog.ParsePath([]string{taskCreateMethodPath}))
	if err != nil || target.Kind != apicatalog.TargetMethod || target.Method == nil {
		validationErr := errs.NewValidationError(
			errs.SubtypeFailedPrecondition,
			"API schema source %s is unavailable",
			taskCreateMethodPath,
		).WithHint("run lark-cli schema " + taskCreateMethodPath + " to verify the installed API metadata")
		if err != nil {
			validationErr.WithCause(err)
		}
		return nil, validationErr
	}

	input := metaschema.EnvelopeOf(nil, *target.Method).InputSchema
	if input == nil || input.Properties == nil {
		return nil, errs.NewValidationError(
			errs.SubtypeFailedPrecondition,
			"API schema source %s has no input fields",
			taskCreateMethodPath,
		)
	}
	property, ok := input.Properties.Map["data"]
	if !ok {
		return nil, errs.NewValidationError(
			errs.SubtypeFailedPrecondition,
			"API schema source %s has no data field",
			taskCreateMethodPath,
		)
	}
	for _, segment := range path[1:] {
		for property.Properties == nil && property.Items != nil {
			property = *property.Items
		}
		if property.Properties == nil {
			ok = false
			break
		}
		property, ok = property.Properties.Map[segment]
		if !ok {
			break
		}
	}
	if !ok {
		return nil, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"JSON Schema path %s is unavailable in %s",
			flagName,
			taskCreateMethodPath,
		).WithParam("--flag-name")
	}
	if len(path) == 1 {
		// A named --summary flag can supply this API-required field, so the
		// shortcut's --data object does not require it.
		required := make([]string, 0, len(property.Required))
		for _, field := range property.Required {
			if field != "summary" {
				required = append(required, field)
			}
		}
		property.Required = required
	}
	return json.MarshalIndent(property, "", "  ")
}
