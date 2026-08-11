// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/larksuite/cli/errs"
)

// bindTypedMap binds already-resolved values used by batch/internal callers.
// Unlike the CLI entry point it never interprets @file or stdin markers: map
// values are final content unless a caller explicitly runs source resolution.
func bindTypedMap(command *compiledCommand, values map[string]any) (*boundArgs, error) {
	args := command.hooks.newArgs()
	root := reflect.ValueOf(args).Elem()
	provided := make([]bool, len(command.fields))
	known := make(map[string]struct{}, len(command.fields))
	for i, field := range command.fields {
		known[field.name] = struct{}{}
		canonical, canonicalSet := values[field.name]
		value, set := canonical, canonicalSet
		sourceName := field.name
		sourceSet := canonicalSet
		for _, alias := range field.cli.Aliases {
			known[alias.Name] = struct{}{}
			aliasValue, aliasSet := values[alias.Name]
			if !aliasSet {
				continue
			}
			switch alias.Mode {
			case AliasNormalize:
				value, set = aliasValue, true
			case AliasIndependent:
				switch alias.Conflict {
				case AliasCanonicalWins:
					if !sourceSet {
						value, set = aliasValue, true
						sourceName, sourceSet = alias.Name, true
					}
				case AliasErrorIfBoth:
					if sourceSet {
						return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
							"--%s cannot be used together with --%s", sourceName, alias.Name).WithParam("--" + alias.Name)
					}
					value, set = aliasValue, true
					sourceName, sourceSet = alias.Name, true
				case AliasTrimmedEqualOrError:
					if sourceSet {
						if strings.TrimSpace(fmt.Sprint(value)) != strings.TrimSpace(fmt.Sprint(aliasValue)) {
							return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--%s and --%s are both set with different values", sourceName, alias.Name).WithParam("--" + alias.Name)
						}
						value = strings.TrimSpace(fmt.Sprint(value))
					} else {
						value, set = aliasValue, true
						sourceName, sourceSet = alias.Name, true
					}
				}
			}
		}
		provided[i] = set
		if !set && field.defaultValue.Set {
			value = field.defaultValue.Value
		}
		if !set && !field.defaultValue.Set {
			if field.required {
				return nil, typedRequiredFieldValidation(field)
			}
			continue
		}
		decoded, err := decodeCompiledMapValue(value, field)
		if err != nil {
			return nil, typedFieldValidation(field, "%v", err).WithCause(err)
		}
		if err := validateCompiledValue(decoded, field); err != nil {
			return nil, err
		}
		if err := assignCompiledField(root, field, decoded, set); err != nil {
			return nil, errs.NewInternalError(errs.SubtypeUnknown, "failed to bind map value %s: %v", field.name, err).WithCause(err)
		}
	}
	for name := range values {
		if _, ok := known[name]; !ok {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "unknown parameter %q", name).WithParam("--" + name)
		}
	}
	if err := validateCompiledRelations(command, args, provided, StageSourcePreRun); err != nil {
		return nil, err
	}
	return &boundArgs{value: args, provided: provided}, nil
}

func decodeCompiledMapValue(value any, field compiledInputField) (any, error) {
	if field.cli.Encoding == EncodingJSON {
		if text, ok := value.(string); ok {
			return decodeCompiledValue(text, field)
		}
		return convertReflectValue(value, field.valueType)
	}
	return convertReflectValue(value, field.valueType)
}
