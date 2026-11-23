// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package events_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"testing"

	"github.com/larksuite/cli/internal/event/catalog"
)

// The output baseline freezes what every Processed key writes to stdout; the
// compiled catalog promises a schema for the same bytes. This test closes the
// loop between the two: every frozen output must be an instance of its key's
// resolved schema, so a schema and its real output can never drift apart with
// both sides individually green.
//
// The repository deliberately carries no JSON Schema validation dependency,
// so validation is done by a minimal in-repo checker that covers exactly the
// subset the catalog compiler emits (see validateValue). Any schema construct
// outside that subset is a loud failure, never a silent pass.
func TestProcessedBaselineOutputs_ConformToDeclaredSchemas(t *testing.T) {
	snap := compileRealCatalog(t)
	baseline := readBaselineSnapshot(t)

	validated := 0
	for _, entry := range snap.Entries() {
		out := entry.Output()
		if out.Mode != catalog.OutputProcessed {
			continue
		}
		key := entry.Descriptor().Key
		frozen, ok := baseline[key]
		if !ok {
			t.Errorf("%s: Processed key has no entry in %s; regenerate the baseline first", key, baselineSnapshotPath)
			continue
		}
		schema := decodeSchemaNode(t, key, out.SchemaJSON)
		instance := decodeInstance(t, key, frozen)
		for _, problem := range validateValue("$", schema, instance) {
			t.Errorf("%s: frozen output violates the declared schema: %s", key, problem)
		}
		validated++
	}

	// Idle detection, both directions: every Processed key was checked
	// against a baseline entry, and no baseline entry escaped the check.
	if validated == 0 {
		t.Fatal("no Processed key was validated; the gate scanned nothing")
	}
	if validated != len(baseline) {
		t.Fatalf("validated %d Processed keys but the baseline holds %d entries — a baseline entry matches no compiled Processed key (keys: %v)",
			validated, len(baseline), sortedKeys(baseline))
	}
}

// The validator itself must bite: an output tampered with in memory — an
// undeclared field, a primitive type flip — has to produce findings,
// otherwise a green conformance run proves nothing. The baseline file is
// never modified.
func TestSchemaInstanceValidator_BitesOnTamperedOutput(t *testing.T) {
	const key = "im.message.receive_v1"
	snap := compileRealCatalog(t)
	entry, ok := snap.Resolve(key)
	if !ok {
		t.Fatalf("key %s is gone from the catalog; pick another Processed key for this self-check", key)
	}
	baseline := readBaselineSnapshot(t)
	frozen, ok := baseline[key]
	if !ok {
		t.Fatalf("key %s has no baseline entry; the self-check needs a real frozen output", key)
	}
	schema := decodeSchemaNode(t, key, entry.Output().SchemaJSON)

	// Control: the untampered output is conformant, so any finding below is
	// caused by the tampering alone.
	if problems := validateValue("$", schema, decodeInstance(t, key, frozen)); len(problems) != 0 {
		t.Fatalf("control failed: the untampered output already has findings: %v", problems)
	}

	tampered, ok := decodeInstance(t, key, frozen).(map[string]any)
	if !ok {
		t.Fatalf("baseline output for %s is not a JSON object", key)
	}
	tampered["field_the_schema_never_declared"] = "smuggled"
	if problems := validateValue("$", schema, tampered); len(problems) != 1 {
		t.Errorf("an undeclared field must produce exactly one finding, got: %v", problems)
	}

	flipped, ok := decodeInstance(t, key, frozen).(map[string]any)
	if !ok {
		t.Fatalf("baseline output for %s is not a JSON object", key)
	}
	flipped["message_id"] = true // declared as a string
	if problems := validateValue("$", schema, flipped); len(problems) != 1 {
		t.Errorf("a primitive type flip must produce exactly one finding, got: %v", problems)
	}
}

func readBaselineSnapshot(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(baselineSnapshotPath)
	if err != nil {
		t.Fatalf("read %s: %v", baselineSnapshotPath, err)
	}
	var out map[string]json.RawMessage
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("%s is not valid JSON: %v", baselineSnapshotPath, err)
	}
	return out
}

func decodeSchemaNode(t *testing.T, key string, raw json.RawMessage) map[string]any {
	t.Helper()
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("%s: resolved schema is not a JSON object: %v", key, err)
	}
	return schema
}

// decodeInstance parses a frozen output with UseNumber so integer/number
// checks see the literal digits instead of a lossy float64.
func decodeInstance(t *testing.T, key string, raw json.RawMessage) any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		t.Fatalf("%s: baseline output is not valid JSON: %v", key, err)
	}
	return v
}

// validateValue checks one instance value against one schema node and returns
// the problems found. It implements only the subset the catalog compiler can
// emit (schemas.FromType plus raw declarations shaped the same way):
//
//   - type object with properties: every instance field must be declared in
//     properties and conform to its node; undeclared fields are errors.
//     Absent declared fields are legal (handlers omit empty members).
//   - type string / integer / number / boolean: the JSON value kind must
//     match.
//   - type array with items: every element must conform to items.
//
// description/format/enum annotations are metadata, not instance constraints
// here. Any construct outside the subset — a missing or unknown type, an
// object without properties, additionalProperties, an array without items —
// is reported as a problem so the validator can only be extended
// deliberately, never bypassed by a schema it does not understand.
func validateValue(path string, schema map[string]any, value any) []string {
	typ, ok := schema["type"].(string)
	if !ok {
		return []string{fmt.Sprintf("%s: schema node has no \"type\"; outside the minimal validator subset, extend the validator deliberately", path)}
	}

	switch typ {
	case "object":
		obj, ok := value.(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: schema declares object, output has %s", path, jsonKind(value))}
		}
		if _, has := schema["additionalProperties"]; has {
			return []string{fmt.Sprintf("%s: schema uses additionalProperties; outside the minimal validator subset, extend the validator deliberately", path)}
		}
		props, ok := schema["properties"].(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: object schema without properties; outside the minimal validator subset, extend the validator deliberately", path)}
		}
		var problems []string
		for _, field := range sortedFieldNames(obj) {
			fieldPath := path + "." + field
			node, declared := props[field]
			if !declared {
				problems = append(problems, fmt.Sprintf("%s: field is not declared in the schema properties", fieldPath))
				continue
			}
			nodeObj, ok := node.(map[string]any)
			if !ok {
				problems = append(problems, fmt.Sprintf("%s: schema property is not an object", fieldPath))
				continue
			}
			problems = append(problems, validateValue(fieldPath, nodeObj, obj[field])...)
		}
		return problems

	case "string":
		if _, ok := value.(string); !ok {
			return []string{fmt.Sprintf("%s: schema declares string, output has %s", path, jsonKind(value))}
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return []string{fmt.Sprintf("%s: schema declares boolean, output has %s", path, jsonKind(value))}
		}
	case "integer":
		num, ok := value.(json.Number)
		if !ok {
			return []string{fmt.Sprintf("%s: schema declares integer, output has %s", path, jsonKind(value))}
		}
		if _, err := strconv.ParseInt(num.String(), 10, 64); err != nil {
			return []string{fmt.Sprintf("%s: schema declares integer, output has non-integer number %s", path, num)}
		}
	case "number":
		if _, ok := value.(json.Number); !ok {
			return []string{fmt.Sprintf("%s: schema declares number, output has %s", path, jsonKind(value))}
		}

	case "array":
		arr, ok := value.([]any)
		if !ok {
			return []string{fmt.Sprintf("%s: schema declares array, output has %s", path, jsonKind(value))}
		}
		items, ok := schema["items"].(map[string]any)
		if !ok {
			return []string{fmt.Sprintf("%s: array schema without items; outside the minimal validator subset, extend the validator deliberately", path)}
		}
		var problems []string
		for i, elem := range arr {
			problems = append(problems, validateValue(fmt.Sprintf("%s[%d]", path, i), items, elem)...)
		}
		return problems

	default:
		return []string{fmt.Sprintf("%s: schema type %q; outside the minimal validator subset, extend the validator deliberately", path, typ)}
	}
	return nil
}

// jsonKind names a decoded JSON value's kind for problem messages.
func jsonKind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case bool:
		return "boolean"
	case string:
		return "string"
	case json.Number:
		return "number"
	case []any:
		return "array"
	case map[string]any:
		return "object"
	default:
		return fmt.Sprintf("%T", v)
	}
}

func sortedFieldNames(obj map[string]any) []string {
	names := make([]string, 0, len(obj))
	for name := range obj {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
