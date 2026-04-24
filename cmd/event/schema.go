// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"encoding/json"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	eventlib "github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/schemas"
	"github.com/larksuite/cli/internal/output"
)

// resolveSchemaJSON returns the final JSON Schema a consumer would see
// for this EventKey: reflected / parsed base, envelope-wrapped for Native,
// plus FieldOverrides overlay applied. Returns (schema, orphans, err):
// `orphans` is the list of FieldOverrides pointers that did not resolve
// in the rendered schema (used by CI lint; callers that don't care can
// ignore).
func resolveSchemaJSON(def *eventlib.KeyDefinition) (json.RawMessage, []string, error) {
	spec, isNative := pickSpec(def.Schema)
	if spec == nil {
		return nil, nil, nil
	}

	base, err := renderSpec(spec)
	if err != nil {
		return nil, nil, err
	}
	if base == nil {
		return nil, nil, nil
	}

	if isNative {
		base = schemas.WrapV2Envelope(base)
	}

	if len(def.Schema.FieldOverrides) > 0 {
		var parsed map[string]interface{}
		if err := json.Unmarshal(base, &parsed); err != nil {
			return nil, nil, err
		}
		orphans := schemas.ApplyFieldOverrides(parsed, def.Schema.FieldOverrides)
		out, err := json.Marshal(parsed)
		if err != nil {
			return nil, nil, err
		}
		return out, orphans, nil
	}

	return base, nil, nil
}

// pickSpec returns the non-nil spec and whether it is the Native slot
// (framework must wrap V2 envelope around it).
func pickSpec(s eventlib.SchemaDef) (*eventlib.SchemaSpec, bool) {
	if s.Native != nil {
		return s.Native, true
	}
	if s.Custom != nil {
		return s.Custom, false
	}
	return nil, false
}

// renderSpec produces a JSON Schema for a single spec. Type is reflected;
// Raw is returned as a copy so callers can mutate safely.
func renderSpec(s *eventlib.SchemaSpec) (json.RawMessage, error) {
	if s.Type != nil {
		return schemas.FromType(s.Type), nil
	}
	if len(s.Raw) > 0 {
		buf := make(json.RawMessage, len(s.Raw))
		copy(buf, s.Raw)
		return buf, nil
	}
	return nil, fmt.Errorf("schemaSpec has neither Type nor Raw")
}

// NewCmdSchema creates the "event schema" subcommand that shows details for an EventKey.
func NewCmdSchema(f *cmdutil.Factory) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "schema <EventKey>",
		Short: "Show details for an EventKey",
		Long:  "Display detailed information about an EventKey including type, events, parameters, and response schema. Use --json for machine-readable output.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSchema(f, args[0], asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the EventKey definition + resolved schema as JSON (for AI / scripts)")
	return cmd
}

func runSchema(f *cmdutil.Factory, key string, asJSON bool) error {
	def, ok := eventlib.Lookup(key)
	if !ok {
		return unknownEventKeyErr(key)
	}

	if asJSON {
		return writeSchemaJSON(f, def)
	}

	out := f.IOStreams.Out

	fmt.Fprintf(out, "Key:         %s\n", def.Key)
	if def.Description != "" {
		fmt.Fprintf(out, "Description: %s\n", def.Description)
	}
	fmt.Fprintf(out, "Event:       %s\n", def.EventType)

	if def.PreConsume != nil {
		fmt.Fprintf(out, "Pre-consume: yes\n")
	}

	if len(def.Scopes) > 0 {
		fmt.Fprintf(out, "\nRequired Scopes:\n")
		for _, s := range def.Scopes {
			fmt.Fprintf(out, "  - %s\n", s)
		}
	}

	if len(def.RequiredConsoleEvents) > 0 {
		fmt.Fprintf(out, "\nRequired Console Events (must be enabled in developer console):\n")
		for _, e := range def.RequiredConsoleEvents {
			fmt.Fprintf(out, "  - %s\n", e)
		}
	}

	if len(def.Params) > 0 {
		fmt.Fprintf(out, "\nParameters:\n")
		w := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
		fmt.Fprintf(w, "  NAME\tTYPE\tREQUIRED\tDEFAULT\tDESCRIPTION\n")
		for _, p := range def.Params {
			required := "no"
			if p.Required {
				required = "yes"
			}
			defaultVal := p.Default
			if defaultVal == "" {
				defaultVal = "-"
			}
			desc := p.Description
			if desc == "" {
				desc = "-"
			}
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\n", p.Name, p.Type, required, defaultVal, desc)
		}
		w.Flush()

		// Render Values inline below the param table for Enum / Multi params
		// so AI consumers can see which values are allowed and what each
		// means without having to parse --json.
		for _, p := range def.Params {
			if len(p.Values) == 0 {
				continue
			}
			fmt.Fprintf(out, "\n  %s values:\n", p.Name)
			vw := tabwriter.NewWriter(out, 0, 4, 2, ' ', 0)
			for _, v := range p.Values {
				fmt.Fprintf(vw, "    %s\t%s\n", v.Value, v.Desc)
			}
			vw.Flush()
		}
	}

	resolved, _, err := resolveSchemaJSON(def)
	if err != nil {
		return output.Errorf(output.ExitInternal, "internal", "resolve schema: %v", err)
	}
	if resolved != nil {
		fmt.Fprintf(out, "\nOutput Schema:\n")
		printIndentedJSON(out, resolved)
	} else {
		fmt.Fprintf(out, "\nOutput Schema: (schema not declared)\n")
		if def.Schema.Native != nil {
			fmt.Fprintf(out, "  Consumers receive the V2 envelope: {schema, header, event}.\n")
			fmt.Fprintf(out, "  Inspect real payloads via `lark-cli event consume %s`.\n", def.Key)
		}
	}

	return nil
}

// printIndentedJSON pretty-prints raw JSON to out with a 2-space leading
// indent so it lines up under the "Output Schema:" label.
func printIndentedJSON(out io.Writer, raw json.RawMessage) {
	var parsed json.RawMessage
	if err := json.Unmarshal(raw, &parsed); err != nil {
		fmt.Fprintln(out, "  <invalid JSON>")
		return
	}
	formatted, err := json.MarshalIndent(parsed, "  ", "  ")
	if err != nil {
		return
	}
	fmt.Fprintf(out, "  %s\n", string(formatted))
}

// writeSchemaJSON emits the EventKey definition plus the resolved schema
// (whether native-wrapped-in-V2-envelope or Process-declared) as a single
// JSON object so callers can ingest schema + metadata in one pass.
//
// root_path_hint tells AI callers whether to write jq paths as `.field`
// (flat / Custom schema — e.g. im.message.receive_v1) or `.event.field`
// (V2 envelope — every Native-schema key). Without this, the caller has
// to either inspect resolved_output_schema shape or eyeball a sample event.
func writeSchemaJSON(f *cmdutil.Factory, def *eventlib.KeyDefinition) error {
	type payload struct {
		*eventlib.KeyDefinition
		// JSON wire name kept as `resolved_output_schema` for backward
		// compatibility with existing AI / script consumers; Go field was
		// shortened to `ResolvedSchema` after the OutputSchema/OutputType
		// fields were removed from KeyDefinition.
		ResolvedSchema json.RawMessage `json:"resolved_output_schema,omitempty"`
		RootPathHint   string          `json:"root_path_hint,omitempty"`
	}
	resolved, _, err := resolveSchemaJSON(def)
	if err != nil {
		return err
	}
	var rootPathHint string
	if resolved != nil {
		// isNative is the truth source: Native schemas get WrapV2Envelope
		// applied in resolveSchemaJSON, so consumers see {schema, header,
		// event, ...} and must reach fields via `.event.xxx`. Custom
		// schemas (like im.message.receive_v1, which Process flattens)
		// deliver fields at the top level.
		_, isNative := pickSpec(def.Schema)
		rootPathHint = "."
		if isNative {
			rootPathHint = ".event"
		}
	}
	output.PrintJson(f.IOStreams.Out, payload{
		KeyDefinition:  def,
		ResolvedSchema: resolved,
		RootPathHint:   rootPathHint,
	})
	return nil
}
