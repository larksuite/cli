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
	"github.com/larksuite/cli/internal/event/catalog"
	"github.com/larksuite/cli/internal/output"
)

func NewCmdSchema(f *cmdutil.Factory, snap *catalog.Snapshot) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "schema <EventKey>",
		Short: "Show details for an EventKey",
		Long:  "Display detailed information about an EventKey including type, events, parameters, and response schema. Use --json for machine-readable output.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSchema(f, snap, args[0], asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the EventKey definition + resolved schema as JSON (for AI / scripts)")
	cmdutil.SetRisk(cmd, cmdutil.RiskRead)
	return cmd
}

func runSchema(f *cmdutil.Factory, snap *catalog.Snapshot, key string, asJSON bool) error {
	entry, ok := snap.Resolve(key)
	if !ok {
		return unknownEventKeyErr(snap, key)
	}
	def := entry.Definition()

	if asJSON {
		return writeSchemaJSON(f, entry)
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
		fmt.Fprintf(w, "  NAME\tTYPE\tREQUIRED\tSUB-KEY\tDEFAULT\tDESCRIPTION\n")
		for _, p := range def.Params {
			required := "no"
			if p.Required {
				required = "yes"
			}
			subKey := "no"
			if p.SubscriptionKey {
				subKey = "yes"
			}
			defaultVal := p.Default
			if defaultVal == "" {
				defaultVal = "-"
			}
			desc := p.Description
			if desc == "" {
				desc = "-"
			}
			fmt.Fprintf(w, "  %s\t%s\t%s\t%s\t%s\t%s\n", p.Name, p.Type, required, subKey, defaultVal, desc)
		}
		w.Flush()

		// Inline Values below the table so AI consumers see allowed enum/multi values without --json.
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

	resolved := entry.Output().SchemaJSON
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

// printIndentedJSON pretty-prints raw JSON with a 2-space leading indent.
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

// schemaPayload is the JSON shape of `event schema --json`. It is a named
// type (not a function-local literal) so the render contract test can walk
// its fields and reject accidental additions to the public output.
type schemaPayload struct {
	*eventlib.KeyDefinition
	ResolvedSchema json.RawMessage `json:"resolved_output_schema,omitempty"`
	JQRootPath     string          `json:"jq_root_path,omitempty"`
}

// writeSchemaJSON emits the EventKey definition plus resolved schema; jq_root_path tells callers whether fields live at `.` or `.event`.
func writeSchemaJSON(f *cmdutil.Factory, entry *catalog.Entry) error {
	contract := entry.Output()
	output.PrintJson(f.IOStreams.Out, schemaPayload{
		KeyDefinition:  entry.Definition(),
		ResolvedSchema: contract.SchemaJSON,
		JQRootPath:     contract.JQRootPath,
	})
	return nil
}
