// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"reflect"
	"strings"
	"testing"
)

// renderedDeclarationFields lists every JSON field the list/schema commands
// are allowed to expose, each with the reason it belongs to the public
// contract. Golden files pin today's bytes; this gate protects tomorrow: a
// field added to the rendered structs (or promoted through the embedded
// definition) must either appear here deliberately or be tagged `json:"-"`.
var renderedDeclarationFields = map[string]string{
	"key":                     "stable identifier agents subscribe by",
	"domain":                  "declared domain override; empty for every shipped key (filtering reads the derived descriptor value), so legacy output is byte-identical",
	"display_name":            "human-readable name for pickers",
	"description":             "what the event means",
	"event_type":              "upstream event type behind this key",
	"subscription_type":       "which console ledger the precheck reads",
	"params":                  "declared consume parameters",
	"schema":                  "declared schema source (native/custom markers)",
	"scopes":                  "OAuth scopes required to consume",
	"auth_types":              "identities the key accepts",
	"required_console_events": "console switches that must be enabled",
	"buffer_size":             "delivery buffer size after normalization",
	"workers":                 "worker count after normalization",
	"single_consumer":         "whether a second consumer is rejected",
	"resolved_output_schema":  "fully resolved JSON schema of stdout events",
	"jq_root_path":            "schema command only: jq root for consuming stdout",
}

// TestRenderContract_NoRuntimeFieldLeaksIntoJSON walks both rendered shapes,
// following embedded struct promotion, and fails on any exported member that
// is neither allowlisted nor explicitly excluded from JSON.
func TestRenderContract_NoRuntimeFieldLeaksIntoJSON(t *testing.T) {
	emitted := map[string]bool{}
	for _, typ := range []reflect.Type{
		reflect.TypeFor[listRow](),
		reflect.TypeFor[schemaPayload](),
	} {
		walkRenderedFields(t, typ, emitted)
	}

	if len(emitted) == 0 {
		t.Fatal("no rendered fields were visited; the gate scanned nothing")
	}
	// The embedded definition is where leaks would hide: prove promotion was
	// actually followed by requiring fields that only exist on it.
	for _, sentinel := range []string{"key", "event_type", "resolved_output_schema"} {
		if !emitted[sentinel] {
			t.Fatalf("field %q was not visited; embedded promotion is no longer being walked", sentinel)
		}
	}
	for name := range renderedDeclarationFields {
		if !emitted[name] {
			t.Errorf("allowlist entry %q is stale: no rendered struct emits it", name)
		}
	}
}

func walkRenderedFields(t *testing.T, typ reflect.Type, emitted map[string]bool) {
	t.Helper()
	if typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		if field.Anonymous {
			ft := field.Type
			if ft.Kind() == reflect.Pointer {
				ft = ft.Elem()
			}
			if ft.Kind() == reflect.Struct {
				walkRenderedFields(t, ft, emitted)
				continue
			}
		}
		tag := field.Tag.Get("json")
		if tag == "-" {
			continue
		}
		if tag == "" {
			t.Errorf("%s.%s has no json tag and would render under its Go name; tag it or exclude it with json:\"-\"", typ.Name(), field.Name)
			continue
		}
		name, _, _ := strings.Cut(tag, ",")
		if _, ok := renderedDeclarationFields[name]; !ok {
			t.Errorf("%s.%s renders JSON field %q that is not in the declared output contract; add it deliberately or exclude it", typ.Name(), field.Name, name)
		}
		emitted[name] = true
	}
}
