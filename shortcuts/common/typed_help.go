// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"fmt"

	"github.com/spf13/cobra"
)

func installTypedHelp(cmd *cobra.Command, command *compiledCommand) {
	installTypedGroupedUsage(cmd, typedHelpFacts(command))
}

func typedHelpFacts(command *compiledCommand) typedCommandHelpFacts {
	facts := typedCommandHelpFacts{
		Parameters:  make([]typedParameterHelpFact, 0, len(command.fields)),
		Constraints: make([]typedConstraintHelpFact, 0, len(command.relations)),
		Execution:   []typedHelpFlagRef{{Name: "as"}, {Name: "dry-run"}, {Name: "yes"}},
		OutputFlags: []typedHelpFlagRef{{Name: "format"}, {Name: "json"}, {Name: "jq"}},
		Other:       []typedHelpFlagRef{{Name: "print-schema"}, {Name: "flag-name"}, {Name: "help"}},
	}
	parameterIndex := make(map[int]int, len(command.fields))
	stdinParameters := 0
	for fieldIndex, field := range command.fields {
		if field.cli.Hidden {
			continue
		}
		fact := typedParameterHelpFact{
			Name: field.name, Type: helpType(field.shape, field.cli.Encoding), Description: field.description,
			Required: field.required, DefaultSet: field.defaultValue.Set, Default: field.defaultValue.Value,
			Encoding: string(field.cli.Encoding),
		}
		for _, source := range field.cli.ValueSources {
			fact.Sources = append(fact.Sources, string(source))
			if source == typedSourceStdin {
				stdinParameters++
			}
		}
		for _, alias := range field.cli.Aliases {
			fact.Aliases = append(fact.Aliases, typedAliasHelpFact{Name: alias.Name, Hidden: alias.Hidden, Deprecated: alias.Deprecated})
		}
		applyShapeToHelpFact(&fact, field.shape)
		parameterIndex[fieldIndex] = len(facts.Parameters)
		facts.Parameters = append(facts.Parameters, fact)
	}
	if stdinParameters > 1 {
		facts.Constraints = append(facts.Constraints, typedConstraintHelpFact{Text: "at most one parameter may read stdin in one invocation"})
	}
	for _, relation := range command.relations {
		params := make([]string, 0, len(relation.fields))
		visibleFields := make([]int, 0, len(relation.fields))
		for _, index := range relation.fields {
			if _, visible := parameterIndex[index]; !visible {
				continue
			}
			visibleFields = append(visibleFields, index)
			params = append(params, command.fields[index].name)
		}
		if len(visibleFields) != len(relation.fields) {
			if len(visibleFields) == 1 && (relation.kind == typedRelationExactlyOne || relation.kind == typedRelationAtLeastOne) {
				parameter := &facts.Parameters[parameterIndex[visibleFields[0]]]
				parameter.Required = true
				parameter.Explicit = relation.presence == typedPresenceExplicit
			}
			continue
		}
		facts.Constraints = append(facts.Constraints, typedConstraintHelpFact{Kind: string(relation.kind), Params: params, Presence: string(relation.presence)})
	}
	if command.output.Meta.Pagination {
		text := "pagination metadata reports completion, pages, items, and a resume token when incomplete"
		if command.output.Mode != typedOutputFixedJSON {
			summaryFormats := "table"
			if command.hooks.renderers["pretty"] != nil {
				summaryFormats = "pretty/table"
			}
			text += "; successful " + summaryFormats + " output appends a pagination summary"
		}
		facts.Output = append(facts.Output, typedOutputHelpFact{Text: text})
	}
	identityOrder := command.metadata.Authorization.IdentityOrder
	if len(identityOrder) == 0 {
		identityOrder = []typedIdentity{typedIdentityUser, typedIdentityBot}
	}
	for _, identity := range identityOrder {
		authorization, ok := command.metadata.Authorization.Identities[identity]
		if !ok || len(authorization.RequiredScopes)+len(authorization.ConditionalScopes) == 0 {
			continue
		}
		fact := typedAuthorizationHelpFact{Identity: string(identity), RequiredScopes: append([]string(nil), authorization.RequiredScopes...)}
		for _, conditional := range authorization.ConditionalScopes {
			fact.ConditionalScopes = append(fact.ConditionalScopes, typedConditionalScopeHelpFact{
				Scopes: append([]string(nil), conditional.Scopes...), When: conditional.When,
				Params: append([]string(nil), conditional.Params...), Requirement: conditional.Requirement,
			})
		}
		facts.Authorization = append(facts.Authorization, fact)
	}
	return facts
}

func helpType(shape typedValueShape, encoding typedCLIEncoding) string {
	if encoding == typedEncodingJSON {
		return "json"
	}
	shape = nonNullableShape(shape)
	switch value := shape.(type) {
	case typedBooleanShape:
		return "boolean"
	case typedIntegerShape:
		return "integer"
	case typedNumberShape:
		return "number"
	case typedStringShape:
		return "string"
	case typedArrayShape:
		item := helpType(value.Items, "")
		if item == "boolean" {
			item = "bool"
		}
		if item == "json" || item == "" {
			return "array"
		}
		return item + "[]"
	case typedObjectShape, typedOneOfShape:
		return "json"
	case typedConstShape:
		switch value.Value.(type) {
		case bool:
			return "boolean"
		case string:
			return "string"
		default:
			return "value"
		}
	case typedNullShape:
		return "null"
	default:
		return "value"
	}
}

func nonNullableShape(shape typedValueShape) typedValueShape {
	if oneOf, ok := shape.(typedOneOfShape); ok && len(oneOf.Variants) == 2 {
		if _, null := oneOf.Variants[0].(typedNullShape); null {
			return oneOf.Variants[1]
		}
		if _, null := oneOf.Variants[1].(typedNullShape); null {
			return oneOf.Variants[0]
		}
	}
	return shape
}

func applyShapeToHelpFact(fact *typedParameterHelpFact, shape typedValueShape) {
	shape = nonNullableShape(shape)
	switch value := shape.(type) {
	case typedStringShape:
		fact.Enum = append([]string{}, value.Enum...)
		fact.Format, fact.MinLength, fact.MaxLength = value.Format, cloneInt(value.MinLength), cloneInt(value.MaxLength)
	case typedIntegerShape:
		for _, item := range value.Enum {
			fact.Enum = append(fact.Enum, fmt.Sprint(item))
		}
		fact.Minimum, fact.Maximum = int64AsFloat(value.Minimum), int64AsFloat(value.Maximum)
	case typedNumberShape:
		for _, item := range value.Enum {
			fact.Enum = append(fact.Enum, fmt.Sprintf("%g", item))
		}
		fact.Minimum, fact.Maximum = cloneFloat(value.Minimum), cloneFloat(value.Maximum)
	case typedArrayShape:
		fact.MinItems, fact.MaxItems = cloneInt(value.MinItems), cloneInt(value.MaxItems)
	}
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
func cloneFloat(value *float64) *float64 {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
func int64AsFloat(value *int64) *float64 {
	if value == nil {
		return nil
	}
	copied := float64(*value)
	return &copied
}
