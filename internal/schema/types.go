// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// Envelope is the MCP Tool spec contract for a single API method command.
type Envelope struct {
	Name         string        `json:"name"`
	Description  string        `json:"description"`
	InputSchema  *InputSchema  `json:"inputSchema"`
	OutputSchema *OutputSchema `json:"outputSchema"`
	Meta         *Meta         `json:"_meta"`
}

// InputSchema is JSON Schema Draft 2020-12 flattened.
//
// Required is intentionally rendered (no omitempty) so the envelope shape
// stays stable for AI consumers — an empty []string means "no required
// fields" rather than "schema is missing the field".
type InputSchema struct {
	Type       string        `json:"type"`
	Required   []string      `json:"required"`
	Properties *OrderedProps `json:"properties"`
}

// OutputSchema wraps responseBody into a JSON Schema object.
type OutputSchema struct {
	Type       string        `json:"type"`
	Properties *OrderedProps `json:"properties"`
}

// Property is one field's JSON Schema shape, recursive.
//
// Required is used when Property describes a nested object (e.g. the
// "params" / "data" sub-objects inside inputSchema): it lists which keys
// inside that object's Properties are mandatory. Leaf fields ignore it.
type Property struct {
	Type        string        `json:"type,omitempty"`
	Description string        `json:"description,omitempty"`
	Enum        []interface{} `json:"enum,omitempty"`
	Default     interface{}   `json:"default,omitempty"`
	Example     interface{}   `json:"example,omitempty"`
	Minimum     *float64      `json:"minimum,omitempty"`
	Maximum     *float64      `json:"maximum,omitempty"`
	Format      string        `json:"format,omitempty"`
	Required    []string      `json:"required,omitempty"`
	Properties  *OrderedProps `json:"properties,omitempty"`
	Items       *Property     `json:"items,omitempty"`
}

// Meta is the Lark-specific extension namespace.
type Meta struct {
	EnvelopeVersion string      `json:"envelope_version"`
	Scopes          []string    `json:"scopes"`
	RequiredScopes  []string    `json:"required_scopes"`
	AccessTokens    []string    `json:"access_tokens"`
	Danger          bool        `json:"danger"`
	Risk            string      `json:"risk"`
	DocURL          string      `json:"doc_url,omitempty"`
	Affordance      *Affordance `json:"affordance,omitempty"`
}

// Affordance is the hand-written overlay (PR-1 only defines the type, no YAML loaded).
type Affordance struct {
	UseWhen       []string         `json:"use_when,omitempty"`
	DoNotUseWhen  []string         `json:"do_not_use_when,omitempty"`
	Prerequisites []string         `json:"prerequisites,omitempty"`
	Examples      []AffordanceCase `json:"examples,omitempty"`
	Related       []string         `json:"related,omitempty"`
}

// AffordanceCase is one example entry: a one-line description plus a
// ready-to-run lark-cli command string.
type AffordanceCase struct {
	Description string `json:"description"`
	Command     string `json:"command"`
}

// OrderedProps is map[string]Property with preserved key order on MarshalJSON.
// It is used wherever JSON output must reflect meta_data.json's natural field
// order rather than Go's default alphabetical map encoding.
type OrderedProps struct {
	Order []string
	Map   map[string]Property
}

// MarshalJSON emits keys in Order, not alphabetical. If Order is empty but
// Map has entries, fall back to alphabetical key order over Map so callers
// that only populated Map (no explicit ordering) still see their fields.
func (o *OrderedProps) MarshalJSON() ([]byte, error) {
	if o == nil || (len(o.Order) == 0 && len(o.Map) == 0) {
		return []byte("{}"), nil
	}
	keys := o.Order
	if len(keys) == 0 {
		keys = make([]string, 0, len(o.Map))
		for k := range o.Map {
			keys = append(keys, k)
		}
		sort.Strings(keys)
	}
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		keyJSON, err := json.Marshal(k)
		if err != nil {
			return nil, fmt.Errorf("marshal key %q: %w", k, err)
		}
		buf.Write(keyJSON)
		buf.WriteByte(':')
		valJSON, err := json.Marshal(o.Map[k])
		if err != nil {
			return nil, fmt.Errorf("marshal value for %q: %w", k, err)
		}
		buf.Write(valJSON)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// UnmarshalJSON parses an object preserving key order via json.Decoder.Token().
// Used for round-tripping in tests (and future golden update flows).
func (o *OrderedProps) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return fmt.Errorf("expected object, got %v", tok)
	}
	o.Order = nil
	o.Map = make(map[string]Property)
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return err
		}
		key, ok := keyTok.(string)
		if !ok {
			return fmt.Errorf("expected string key, got %v", keyTok)
		}
		var prop Property
		if err := dec.Decode(&prop); err != nil {
			return err
		}
		o.Order = append(o.Order, key)
		o.Map[key] = prop
	}
	if _, err := dec.Token(); err != nil {
		return err
	}
	return nil
}
