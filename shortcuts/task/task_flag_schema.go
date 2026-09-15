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

const taskUpdateMethodPath = "task.tasks.patch"

func printTaskUpdateDataFlagSchema(flagName string) ([]byte, error) {
	return taskUpdateDataFlagSchema(registry.SchemaCatalog(), flagName)
}

func taskUpdateDataFlagSchema(catalog apicatalog.Catalog, flagName string) ([]byte, error) {
	flagName = strings.TrimSpace(flagName)
	if flagName == "" {
		return json.MarshalIndent(map[string]any{
			"method":               taskUpdateMethodPath,
			"introspectable_flags": []string{"data"},
			"hint":                 "run again with --flag-name data to dump its JSON Schema; append a dotted property path to inspect one nested field",
		}, "", "  ")
	}

	requested := strings.Split(flagName, ".")
	if requested[0] != "data" {
		return nil, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"no JSON Schema registered for --%s; available: [data]",
			requested[0],
		).WithParam("--flag-name")
	}

	target, err := catalog.Resolve(apicatalog.ParsePath([]string{taskUpdateMethodPath}))
	if err != nil || target.Kind != apicatalog.TargetMethod || target.Method == nil {
		validationErr := errs.NewValidationError(
			errs.SubtypeFailedPrecondition,
			"API schema source %s is unavailable",
			taskUpdateMethodPath,
		).WithHint("run lark-cli schema " + taskUpdateMethodPath + " to verify the installed API metadata")
		if err != nil {
			validationErr.WithCause(err)
		}
		return nil, validationErr
	}

	envelope := metaschema.EnvelopeOf(*target.Method)
	path := append([]string{"data", "task"}, requested[1:]...)
	property, ok := taskUpdateInputSchemaProperty(envelope.InputSchema, path)
	if !ok {
		return nil, errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"JSON Schema path %s is unavailable in %s",
			strings.Join(path, "."),
			taskUpdateMethodPath,
		).WithParam("--flag-name")
	}
	return json.MarshalIndent(property, "", "  ")
}

func taskUpdateInputSchemaProperty(input *metaschema.InputSchema, path []string) (metaschema.Property, bool) {
	if input == nil || input.Properties == nil || len(path) == 0 {
		return metaschema.Property{}, false
	}
	property, ok := input.Properties.Map[path[0]]
	if !ok {
		return metaschema.Property{}, false
	}
	for _, segment := range path[1:] {
		property, ok = taskUpdateNestedSchemaProperty(property, segment)
		if !ok {
			return metaschema.Property{}, false
		}
	}
	return property, true
}

func taskUpdateNestedSchemaProperty(property metaschema.Property, segment string) (metaschema.Property, bool) {
	for property.Properties == nil && property.Items != nil {
		property = *property.Items
	}
	if property.Properties == nil {
		return metaschema.Property{}, false
	}
	child, ok := property.Properties.Map[segment]
	return child, ok
}
