// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	eventlib "github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/output"
)

func NewCmdList(f *cmdutil.Factory) *cobra.Command {
	var asJSON bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all available EventKeys",
		Long:  "Show all registered EventKeys grouped by domain (first segment of the key). Use --json for machine-readable output.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(f, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the full EventKey list as JSON (for AI / scripts)")
	return cmd
}

func runList(f *cmdutil.Factory, asJSON bool) error {
	all := eventlib.ListAll()

	if asJSON {
		return writeListJSON(f, all)
	}

	if len(all) == 0 {
		// stderr so `event list | jq` doesn't ingest it as a row.
		fmt.Fprintln(f.IOStreams.ErrOut, "No EventKeys registered.")
		return nil
	}

	type group struct {
		domain string
		keys   []*eventlib.KeyDefinition
	}
	order := []string{}
	groups := map[string]*group{}

	for _, def := range all {
		domain := def.Key
		if idx := strings.Index(def.Key, "."); idx > 0 {
			domain = def.Key[:idx]
		}
		g, ok := groups[domain]
		if !ok {
			g = &group{domain: domain}
			groups[domain] = g
			order = append(order, domain)
		}
		g.keys = append(g.keys, def)
	}

	// Global widths (not per-section) keep "── domain ──" dividers aligned across groups.
	headers := []string{"KEY", "AUTH", "PARAMS", "DESCRIPTION"}
	rowsByDomain := make(map[string][][]string, len(order))
	var allRows [][]string
	for _, domain := range order {
		for _, def := range groups[domain].keys {
			auth := "-"
			if len(def.AuthTypes) > 0 {
				auth = strings.Join(def.AuthTypes, "|")
			}
			desc := def.Description
			if desc == "" {
				desc = "-"
			}
			row := []string{
				def.Key,
				auth,
				fmt.Sprintf("%d", len(def.Params)),
				desc,
			}
			rowsByDomain[domain] = append(rowsByDomain[domain], row)
			allRows = append(allRows, row)
		}
	}

	out := f.IOStreams.Out
	const colGap = "  "
	widths := tableWidths(headers, allRows)
	printTableRow(out, widths, headers, colGap)
	for _, domain := range order {
		fmt.Fprintf(out, "\n── %s ──\n", domain)
		for _, row := range rowsByDomain[domain] {
			printTableRow(out, widths, row, colGap)
		}
	}
	// stderr keeps stdout pipe-clean for `event list | jq`.
	fmt.Fprintln(f.IOStreams.ErrOut, "\nUse 'event schema <key>' for details.")
	return nil
}

func writeListJSON(f *cmdutil.Factory, all []*eventlib.KeyDefinition) error {
	type row struct {
		*eventlib.KeyDefinition
		ResolvedSchema json.RawMessage `json:"resolved_output_schema,omitempty"`
	}
	rows := make([]row, len(all))
	for i, def := range all {
		resolved, _, err := resolveSchemaJSON(def)
		if err != nil {
			return err
		}
		rows[i] = row{KeyDefinition: def, ResolvedSchema: resolved}
	}
	output.PrintJson(f.IOStreams.Out, rows)
	return nil
}
