// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sheets

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestFlagSchemas_EmbedParses asserts the synced flag-schemas.json
// embedded blob is valid JSON and has at least one shortcut/flag entry.
// If sync_to_consumers.mjs ever ships an empty or broken artifact, this
// catches it at build time of the test binary.
func TestFlagSchemas_EmbedParses(t *testing.T) {
	t.Parallel()
	idx, err := loadFlagSchemas()
	if err != nil {
		t.Fatalf("loadFlagSchemas error: %v", err)
	}
	if idx == nil || len(idx.Flags) == 0 {
		t.Fatalf("flag-schemas.json has no entries")
	}
	if idx.SchemaVersion == "" {
		t.Errorf("schema_version missing")
	}
	// Spot-check a couple of canonical entries we know upstream guarantees.
	for _, want := range []string{"+cells-set", "+chart-create", "+batch-update"} {
		if _, ok := idx.Flags[want]; !ok {
			t.Errorf("missing shortcut entry %q (regenerate via sheet-skill-spec/scripts/sync_to_consumers.mjs)", want)
		}
	}
}

// TestPrintFlagSchema_ListIntrospectable verifies that calling the
// closure with an empty flag name returns the JSON listing of
// introspectable flags for the shortcut.
func TestPrintFlagSchema_ListIntrospectable(t *testing.T) {
	t.Parallel()
	out, err := printFlagSchemaFor("+cells-set")("")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	var got map[string]interface{}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if got["shortcut"] != "+cells-set" {
		t.Errorf("shortcut = %v, want +cells-set", got["shortcut"])
	}
	flags, _ := got["introspectable_flags"].([]interface{})
	if len(flags) == 0 || flags[0] != "cells" {
		t.Errorf("introspectable_flags = %v, want [cells]", flags)
	}
}

// TestPrintFlagSchema_NamedFlagReturnsSchemaSubtree verifies a hit on
// (+chart-create, properties) yields a JSON Schema object with the
// expected top-level fields.
func TestPrintFlagSchema_NamedFlagReturnsSchemaSubtree(t *testing.T) {
	t.Parallel()
	out, err := printFlagSchemaFor("+chart-create")("properties")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	var schema map[string]interface{}
	if err := json.Unmarshal(out, &schema); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if schema["type"] != "object" {
		t.Errorf("schema.type = %v, want object", schema["type"])
	}
	if _, ok := schema["properties"]; !ok {
		t.Errorf("schema missing nested .properties: keys=%v", keysOf(schema))
	}
}

// TestPrintFlagSchema_UnknownFlagListsAvailable confirms the error
// message tells the caller which flags exist for the shortcut.
func TestPrintFlagSchema_UnknownFlagListsAvailable(t *testing.T) {
	t.Parallel()
	_, err := printFlagSchemaFor("+chart-create")("does-not-exist")
	if err == nil {
		t.Fatal("expected error for unknown flag, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "+chart-create") || !strings.Contains(msg, "properties") {
		t.Errorf("error should mention shortcut + available flags; got %q", msg)
	}
}

// TestPrintFlagSchema_UnknownShortcut surfaces a missing shortcut entry.
func TestPrintFlagSchema_UnknownShortcut(t *testing.T) {
	t.Parallel()
	_, err := printFlagSchemaFor("+not-a-real-shortcut")("")
	if err == nil {
		t.Fatal("expected error for unknown shortcut")
	}
}

// TestShortcuts_AttachesPrintFlagSchema confirms the registration loop
// in Shortcuts() wires PrintFlagSchema onto each shortcut whose command
// has a schema entry, and leaves it nil for shortcuts that don't.
func TestShortcuts_AttachesPrintFlagSchema(t *testing.T) {
	t.Parallel()
	all := Shortcuts()
	withSchema := commandsWithFlagSchema()
	for _, s := range all {
		_, expected := withSchema[s.Command]
		got := s.PrintFlagSchema != nil
		if got != expected {
			t.Errorf("%s: PrintFlagSchema attached=%v, expected=%v", s.Command, got, expected)
		}
	}
}

// TestPrintSchema_SystemFlagShortCircuit verifies the framework's
// --print-schema interception: required flags are relaxed, Validate /
// Execute are skipped, and the schema JSON appears on stdout.
func TestPrintSchema_SystemFlagShortCircuit(t *testing.T) {
	t.Parallel()
	// +cells-set has required --range / --cells / --sheet-id; without
	// --print-schema, cobra would reject the call. With --print-schema,
	// it should print the schema and exit cleanly. The PrintFlagSchema
	// closure is normally attached by Shortcuts(), so we attach it here
	// to mirror that registration path.
	sc := CellsSet
	sc.PrintFlagSchema = printFlagSchemaFor(sc.Command)
	stdout, err := runShortcut(t, sc, []string{"--print-schema", "--flag-name", "cells"})
	if err != nil {
		t.Fatalf("err: %v\nstdout=%s", err, stdout)
	}
	if !strings.Contains(stdout, "\"type\"") {
		t.Errorf("expected JSON Schema with \"type\" key; got=%s", stdout)
	}
}

// TestPrintSchema_ListingWhenNoFlagNameGiven exercises the discovery
// path: `--print-schema` without `--flag-name` should list the
// shortcut's introspectable flags as JSON on stdout.
func TestPrintSchema_ListingWhenNoFlagNameGiven(t *testing.T) {
	t.Parallel()
	sc := CellsSet
	sc.PrintFlagSchema = printFlagSchemaFor(sc.Command)
	stdout, err := runShortcut(t, sc, []string{"--print-schema"})
	if err != nil {
		t.Fatalf("err: %v\nstdout=%s", err, stdout)
	}
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	flags, _ := got["introspectable_flags"].([]interface{})
	if len(flags) == 0 {
		t.Errorf("introspectable_flags empty: %#v", got)
	}
}

// TestPrintSchema_SystemFlagAbsentForReadOnlyShortcut ensures we don't
// inject --print-schema onto shortcuts that have no composite flags.
// +workbook-info is read-only and not in the schema map.
func TestPrintSchema_SystemFlagAbsentForReadOnlyShortcut(t *testing.T) {
	t.Parallel()
	_, _, err := runShortcutCapturingErr(t, WorkbookInfo, []string{"--url", testURL, "--print-schema"})
	if err == nil {
		t.Fatal("expected unknown flag error")
	}
	if !strings.Contains(err.Error(), "unknown flag") {
		t.Errorf("expected 'unknown flag'; got %v", err)
	}
}

func keysOf(m map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
