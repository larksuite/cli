// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	eventlib "github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/catalog"
	"github.com/larksuite/cli/internal/output"
)

func NewCmdList(f *cmdutil.Factory, snap *catalog.Snapshot) *cobra.Command {
	var asJSON bool
	var domain string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all available EventKeys",
		Long:  "Show all registered EventKeys grouped by domain (first segment of the key). Use --domain to keep one domain only, --json for machine-readable output.",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runList(f, snap, domain, asJSON)
		},
	}
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit the full EventKey list as JSON (for AI / scripts)")
	cmd.Flags().StringVar(&domain, "domain", "", "Only list EventKeys of this domain (the key's first segment, e.g. im, vc)")
	cmdutil.SetRisk(cmd, "read")
	return cmd
}

func runList(f *cmdutil.Factory, snap *catalog.Snapshot, domain string, asJSON bool) error {
	entries, err := entriesForDomain(snap, domain)
	if err != nil {
		return err
	}
	if asJSON {
		return writeListJSON(f, entries)
	}
	all := make([]*eventlib.KeyDefinition, 0, len(entries))
	for _, entry := range entries {
		all = append(all, entry.Definition())
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

// listRow is the JSON shape of one `event list --json` row. It is a named
// type (not a function-local literal) so the render contract test can walk
// its fields and reject accidental additions to the public output.
type listRow struct {
	*eventlib.KeyDefinition
	ResolvedSchema json.RawMessage `json:"resolved_output_schema,omitempty"`
}

// entriesForDomain filters at the snapshot query layer: without a domain the
// full catalog comes back untouched; with one, rows are only removed, never
// reshaped. An unknown domain is rejected with the valid set spelled out.
func entriesForDomain(snap *catalog.Snapshot, domain string) ([]*catalog.Entry, error) {
	if domain == "" {
		return snap.Entries(), nil
	}
	var filtered []*catalog.Entry
	for _, entry := range snap.Entries() {
		if entry.Descriptor().Domain == domain {
			filtered = append(filtered, entry)
		}
	}
	if len(filtered) == 0 {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"unknown domain: %s", domain).
			WithParam("--domain").
			WithHint("valid domains: %s", strings.Join(snap.Domains(), ", "))
	}
	return filtered, nil
}

func writeListJSON(f *cmdutil.Factory, entries []*catalog.Entry) error {
	rows := make([]listRow, len(entries))
	for i, entry := range entries {
		rows[i] = listRow{
			KeyDefinition:  entry.Definition(),
			ResolvedSchema: entry.Output().SchemaJSON,
		}
	}
	output.PrintJson(f.IOStreams.Out, rows)
	return nil
}
