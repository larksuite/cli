// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import "fmt"

// typedSchemaContract is the private machine contract derived from the single
// public extension/command declaration.
type typedSchemaContract struct {
	Name         string          `json:"name"`
	Description  string          `json:"description"`
	InputSchema  typedSchemaNode `json:"inputSchema"`
	OutputSchema typedSchemaNode `json:"outputSchema"`
	Meta         typedSchemaMeta `json:"_meta"`
}

type typedSchemaNode struct {
	Type                 string                     `json:"type,omitempty"`
	Description          string                     `json:"description,omitempty"`
	Flag                 string                     `json:"flag,omitempty"`
	Hidden               bool                       `json:"hidden,omitempty"`
	Deprecated           string                     `json:"deprecated,omitempty"`
	Aliases              *[]typedSchemaAlias        `json:"aliases,omitempty"`
	ValueSources         []typedValueSource         `json:"value_sources,omitempty"`
	Enum                 []typedJSONValue           `json:"enum,omitempty"`
	Default              *typedJSONValue            `json:"default,omitempty"`
	Format               string                     `json:"format,omitempty"`
	Minimum              *float64                   `json:"minimum,omitempty"`
	Maximum              *float64                   `json:"maximum,omitempty"`
	MinLength            *int                       `json:"minLength,omitempty"`
	MaxLength            *int                       `json:"maxLength,omitempty"`
	MinItems             *int                       `json:"minItems,omitempty"`
	MaxItems             *int                       `json:"maxItems,omitempty"`
	Required             *[]string                  `json:"required,omitempty"`
	Properties           map[string]typedSchemaNode `json:"properties,omitempty"`
	Items                *typedSchemaNode           `json:"items,omitempty"`
	OneOf                []typedSchemaNode          `json:"oneOf,omitempty"`
	Const                *typedJSONValue            `json:"const,omitempty"`
	AdditionalProperties *typedJSONValue            `json:"additionalProperties,omitempty"`
}

type typedSchemaAlias struct {
	Name       string                   `json:"name"`
	Flag       string                   `json:"flag"`
	Mode       typedFlagAliasMode       `json:"mode"`
	Conflict   typedAliasConflictPolicy `json:"conflict,omitempty"`
	Hidden     bool                     `json:"hidden,omitempty"`
	Deprecated bool                     `json:"deprecated,omitempty"`
}

type typedSchemaMeta struct {
	EnvelopeVersion string                   `json:"envelope_version"`
	AccessTokens    []string                 `json:"access_tokens"`
	Danger          bool                     `json:"danger"`
	Risk            typedRisk                `json:"risk"`
	Authorization   typedSchemaAuthorization `json:"authorization"`
	CLI             typedSchemaCLI           `json:"cli"`
	Relations       []typedRelation          `json:"relations"`
	Formats         []typedSchemaFormat      `json:"formats"`
	Outcomes        typedSchemaOutcomes      `json:"outcomes"`
	ResultMeta      *typedSchemaNode         `json:"result_meta,omitempty"`
	Artifacts       []any                    `json:"artifacts"`
}

type typedSchemaAuthorization struct {
	Identities map[typedIdentity]typedIdentityAuthorization `json:"identities"`
}
type typedSchemaCLI struct {
	Flags       map[string]typedSchemaSystemFlag `json:"flags"`
	Constraints []typedSchemaCLIConstraint       `json:"constraints"`
}
type typedSchemaSystemFlag struct {
	Flag     string          `json:"flag"`
	Short    string          `json:"short,omitempty"`
	Role     string          `json:"role"`
	Type     string          `json:"type"`
	Default  *typedJSONValue `json:"default,omitempty"`
	Enum     []string        `json:"enum,omitempty"`
	AliasFor string          `json:"alias_for,omitempty"`
	Omitted  string          `json:"omitted,omitempty"`
}
type typedSchemaCLIConstraint struct {
	Kind          string   `json:"kind"`
	Param         string   `json:"param"`
	Overrides     string   `json:"overrides,omitempty"`
	When          string   `json:"when,omitempty"`
	AllowedValues []string `json:"allowed_values,omitempty"`
}
type typedSchemaFormat struct {
	Name       string   `json:"name"`
	Default    bool     `json:"default"`
	MediaType  string   `json:"media_type"`
	SelectedBy []string `json:"selected_by"`
	EscapeHTML *bool    `json:"escape_html,omitempty"`
}

type typedSchemaOutcomes struct {
	Success        typedSchemaOutcome `json:"success"`
	PartialFailure typedSchemaOutcome `json:"partial_failure"`
}
type typedSchemaOutcome struct {
	Supported   bool   `json:"supported"`
	EnvelopeOK  bool   `json:"envelope_ok"`
	ExitCode    int    `json:"exit_code"`
	Stdout      string `json:"stdout,omitempty"`
	FailedItems any    `json:"failed_items,omitempty"`
}

func buildTypedSchemaContract(command *compiledCommand) typedSchemaContract {
	required := []string{}
	input := typedSchemaNode{Type: "object", Required: &required, Properties: make(map[string]typedSchemaNode)}
	closed := typedJSONValue(false)
	input.AdditionalProperties = &closed
	for _, field := range command.fields {
		node := schemaNodeFromShape(field.shape)
		node.Description = field.description
		node.Flag = "--" + field.name
		node.Hidden = field.cli.Hidden
		node.Deprecated = field.cli.Deprecated
		aliases := []typedSchemaAlias{}
		node.Aliases = &aliases
		for _, alias := range field.cli.Aliases {
			*node.Aliases = append(*node.Aliases, typedSchemaAlias{Name: alias.Name, Flag: "--" + alias.Name, Mode: alias.Mode, Conflict: alias.Conflict, Hidden: alias.Hidden, Deprecated: alias.Deprecated})
		}
		node.ValueSources = append([]typedValueSource(nil), field.cli.ValueSources...)
		if len(node.ValueSources) == 0 {
			node.ValueSources = []typedValueSource{typedSourceFlag}
		}
		if field.defaultValue.Set {
			value := field.defaultValue.Value
			node.Default = &value
		}
		input.Properties[field.name] = node
		if field.required {
			*input.Required = append(*input.Required, field.name)
		}
	}
	schemaFormats := typedOutputFormats(command)
	authorizationIdentities := make(map[typedIdentity]typedIdentityAuthorization, len(command.metadata.Authorization.Identities))
	for identity, authorization := range command.metadata.Authorization.Identities {
		authorization.RequiredScopes = append([]string{}, authorization.RequiredScopes...)
		authorization.ConditionalScopes = append([]typedConditionalScope{}, authorization.ConditionalScopes...)
		authorizationIdentities[identity] = authorization
	}
	accessTokens := make([]string, 0, len(command.metadata.Authorization.Identities))
	for _, identity := range []typedIdentity{typedIdentityBot, typedIdentityUser} {
		if _, ok := command.metadata.Authorization.Identities[identity]; ok {
			accessTokens = append(accessTokens, string(identity))
		}
	}
	businessFlags := legacyFlagsFromCompiled(command.fields)
	return typedSchemaContract{
		Name:         string(command.metadata.Service) + " " + command.metadata.Command,
		Description:  command.metadata.Description,
		InputSchema:  input,
		OutputSchema: schemaNodeFromShape(command.dataShape),
		Meta: typedSchemaMeta{
			EnvelopeVersion: "1.0", AccessTokens: accessTokens, Danger: command.metadata.Risk == typedRiskHighRiskWrite, Risk: command.metadata.Risk,
			Authorization: typedSchemaAuthorization{Identities: authorizationIdentities},
			CLI:           defaultTypedCLI(accessTokens, businessFlags), Relations: append([]typedRelation{}, commandInputRelations(command)...), Formats: schemaFormats,
			Outcomes:   typedSchemaOutcomes{Success: typedSchemaOutcome{Supported: true, EnvelopeOK: true, ExitCode: 0, Stdout: "result_envelope"}, PartialFailure: typedSchemaOutcome{Supported: false}},
			ResultMeta: typedResultMetaSchema(command.output.Meta),
			Artifacts:  []any{},
		},
	}
}

func typedResultMetaSchema(definition typedResultMetaDefinition) *typedSchemaNode {
	if !definition.Pagination {
		return nil
	}
	additional := typedJSONValue(false)
	result := &typedSchemaNode{Type: "object", Properties: make(map[string]typedSchemaNode), AdditionalProperties: &additional}
	if definition.Pagination {
		zero, one := float64(0), float64(1)
		required := []string{"complete", "pages", "items"}
		pagination := typedSchemaNode{Type: "object", Required: &required, Properties: map[string]typedSchemaNode{
			"complete":   {Type: "boolean", Description: "whether the server exhausted the result set"},
			"pages":      {Type: "integer", Minimum: &one, Description: "successful API pages included in the result"},
			"items":      {Type: "integer", Minimum: &zero, Description: "records returned after command-level processing"},
			"next_token": {Type: "string", Description: "resume token for an incomplete result"},
		}, AdditionalProperties: &additional}
		result.Properties["pagination"] = pagination
	}
	return result
}

func commandInputRelations(command *compiledCommand) []typedRelation {
	result := make([]typedRelation, 0, len(command.relations))
	for _, relation := range command.relations {
		params := make([]string, 0, len(relation.fields))
		for _, index := range relation.fields {
			params = append(params, command.fields[index].name)
		}
		result = append(result, typedRelation{Kind: relation.kind, Params: params, Presence: relation.presence, Stage: relation.stage})
	}
	return result
}

func schemaNodeFromShape(shape typedValueShape) typedSchemaNode {
	switch value := shape.(type) {
	case anyJSONShape:
		return typedSchemaNode{}
	case typedStringShape:
		node := typedSchemaNode{Type: "string", Format: value.Format, MinLength: value.MinLength, MaxLength: value.MaxLength}
		for _, item := range value.Enum {
			node.Enum = append(node.Enum, item)
		}
		return node
	case typedBooleanShape:
		node := typedSchemaNode{Type: "boolean"}
		for _, item := range value.Enum {
			node.Enum = append(node.Enum, item)
		}
		return node
	case typedIntegerShape:
		node := typedSchemaNode{Type: "integer"}
		if value.Minimum != nil {
			v := float64(*value.Minimum)
			node.Minimum = &v
		}
		if value.Maximum != nil {
			v := float64(*value.Maximum)
			node.Maximum = &v
		}
		for _, item := range value.Enum {
			node.Enum = append(node.Enum, item)
		}
		return node
	case typedNumberShape:
		node := typedSchemaNode{Type: "number", Minimum: value.Minimum, Maximum: value.Maximum}
		for _, item := range value.Enum {
			node.Enum = append(node.Enum, item)
		}
		return node
	case typedNullShape:
		return typedSchemaNode{Type: "null"}
	case typedConstShape:
		item := value.Value
		return typedSchemaNode{Const: &item}
	case typedArrayShape:
		item := schemaNodeFromShape(value.Items)
		return typedSchemaNode{Type: "array", Items: &item, MinItems: value.MinItems, MaxItems: value.MaxItems}
	case typedObjectShape:
		additional := typedJSONValue(value.AdditionalProperties)
		if value.AdditionalPropertiesShape != nil {
			additional = schemaNodeFromShape(value.AdditionalPropertiesShape)
		}
		required := []string{}
		node := typedSchemaNode{Type: "object", Required: &required, Properties: make(map[string]typedSchemaNode), AdditionalProperties: &additional}
		for _, field := range value.Fields {
			child := schemaNodeFromShape(field.Shape)
			child.Description = field.Description
			node.Properties[field.Name] = child
			if field.Required {
				*node.Required = append(*node.Required, field.Name)
			}
		}
		return node
	case typedOneOfShape:
		node := typedSchemaNode{}
		for _, variant := range value.Variants {
			node.OneOf = append(node.OneOf, schemaNodeFromShape(variant))
		}
		return node
	default:
		panic(fmt.Sprintf("uncompiled ValueShape %T", shape))
	}
}

func typedOutputFormats(command *compiledCommand) []typedSchemaFormat {
	jsonSelectors := []string{"json"}
	if command.output.Mode == typedOutputFixedJSON {
		jsonSelectors = []string{"json", "pretty", "table", "ndjson", "csv"}
	} else if command.hooks.renderers["pretty"] == nil {
		// Legacy OutFormat accepts --format pretty without a renderer and falls
		// back to the JSON envelope. Keep that argv compatibility explicit while
		// advertising only output formats the command can actually produce.
		jsonSelectors = append(jsonSelectors, "pretty")
	}
	escapeHTML := !command.output.DisableHTMLEscaping
	formats := []typedSchemaFormat{{Name: "json", Default: true, MediaType: "application/json", SelectedBy: jsonSelectors, EscapeHTML: &escapeHTML}}
	if command.output.Mode == typedOutputFixedJSON {
		return formats
	}
	if command.hooks.renderers["pretty"] != nil {
		formats = append(formats, typedSchemaFormat{Name: "pretty", MediaType: "text/plain", SelectedBy: []string{"pretty"}})
	}
	return append(formats,
		typedSchemaFormat{Name: "table", MediaType: "text/plain", SelectedBy: []string{"table"}},
		typedSchemaFormat{Name: "ndjson", MediaType: "application/x-ndjson", SelectedBy: []string{"ndjson"}},
		typedSchemaFormat{Name: "csv", MediaType: "text/csv", SelectedBy: []string{"csv"}},
	)
}

func defaultTypedCLI(identities []string, businessFlags []Flag) typedSchemaCLI {
	falseValue, jsonValue := typedJSONValue(false), typedJSONValue("json")
	format := typedSchemaSystemFlag{Flag: "--format", Role: "output", Type: "string", Default: &jsonValue, Enum: []string{"json", "pretty", "table", "ndjson", "csv"}}
	for _, flag := range businessFlags {
		if flag.Name != "format" {
			continue
		}
		value := typedJSONValue(flag.Default)
		format.Default = &value
		format.Enum = append([]string(nil), flag.Enum...)
		break
	}
	view := Shortcut{Flags: businessFlags}
	cli := typedSchemaCLI{Flags: map[string]typedSchemaSystemFlag{
		"as":      {Flag: "--as", Role: "identity", Type: "string", Enum: identities, Omitted: "resolve profile default, then auto-detect"},
		"dry-run": {Flag: "--dry-run", Role: "execution", Type: "boolean", Default: &falseValue},
		"format":  format,
		"jq":      {Flag: "--jq", Short: "-q", Role: "output", Type: "string"},
	}, Constraints: []typedSchemaCLIConstraint{{Kind: "requires_format", Param: "jq", AllowedValues: []string{"json"}}}}
	if !shortcutDeclaresJSONFlag(&view) && shortcutFormatSupportsJSON(&view) {
		cli.Flags["json"] = typedSchemaSystemFlag{Flag: "--json", Role: "output", Type: "boolean", AliasFor: "--format=json"}
		cli.Constraints = append([]typedSchemaCLIConstraint{{Kind: "overrides", Param: "format", Overrides: "json", When: "both_explicit"}}, cli.Constraints...)
	}
	return cli
}
