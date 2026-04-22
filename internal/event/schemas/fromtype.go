// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package schemas derives JSON Schema fragments from Go types via reflection.
//
// The intended use case is translating typed event structs from the open
// Feishu SDK (e.g. larkim.P2MessageReadV1Data) into a JSON Schema that
// describes the shape of the `event` body an EventKey delivers. Domain
// packages register event keys with an SDK type reference; framework code
// calls FromType to produce the schema at lookup time.
//
// Only publicly exported fields with a `json` tag (or the field name) are
// included. Unexported fields, embedded anonymous structs, and fields
// tagged `json:"-"` are skipped.
package schemas

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
)

// FromType returns a JSON Schema describing the shape of the given Go type.
// The result is cached per reflect.Type.
func FromType(t reflect.Type) json.RawMessage {
	if t == nil {
		return nil
	}
	if cached, ok := cacheLoad(t); ok {
		return cached
	}
	// localCache is shared across reflection recursion so a shared subtype
	// (e.g. UserID referenced from multiple event structs in the same root)
	// is only walked once. Scoped per FromType call to avoid coupling the
	// package-level cache (which stores marshaled JSON) to the intermediate
	// *schemaNode tree.
	localCache := map[reflect.Type]*schemaNode{}
	node := reflectSchema(t, map[reflect.Type]bool{}, localCache)
	out, err := json.Marshal(node)
	if err != nil {
		return nil
	}
	raw := json.RawMessage(out)
	cacheStore(t, raw)
	return raw
}

var (
	cacheMu sync.RWMutex
	cache   = map[reflect.Type]json.RawMessage{}
)

func cacheLoad(t reflect.Type) (json.RawMessage, bool) {
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	v, ok := cache[t]
	return v, ok
}

func cacheStore(t reflect.Type, v json.RawMessage) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	cache[t] = v
}

// schemaNode is the internal building block we marshal to JSON. Keys use
// JSON Schema naming so the marshaled output is directly usable.
type schemaNode struct {
	Type                 string                 `json:"type,omitempty"`
	Description          string                 `json:"description,omitempty"`
	Enum                 []string               `json:"enum,omitempty"`
	Format               string                 `json:"format,omitempty"`
	Properties           map[string]*schemaNode `json:"properties,omitempty"`
	Items                *schemaNode            `json:"items,omitempty"`
	AdditionalProperties *schemaNode            `json:"additionalProperties,omitempty"`
}

// reflectSchema walks t and produces a schemaNode. The visiting map breaks
// cycles: if a type references itself (directly or transitively) we stop
// at {type:object} without recursing further. cache memoises already-seen
// types within a single FromType call so shared subtypes (same *schemaNode
// referenced from multiple parents) are walked once.
func reflectSchema(t reflect.Type, visiting map[reflect.Type]bool, cache map[reflect.Type]*schemaNode) *schemaNode {
	// Unwrap pointers.
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if visiting[t] {
		return &schemaNode{Type: "object"}
	}
	if cached, ok := cache[t]; ok {
		return cached
	}

	var node *schemaNode
	switch t.Kind() {
	case reflect.String:
		node = &schemaNode{Type: "string"}
	case reflect.Bool:
		node = &schemaNode{Type: "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		node = &schemaNode{Type: "integer"}
	case reflect.Float32, reflect.Float64:
		node = &schemaNode{Type: "number"}
	case reflect.Slice, reflect.Array:
		elem := t.Elem()
		// []byte → string (common JSON convention)
		if elem.Kind() == reflect.Uint8 {
			node = &schemaNode{Type: "string"}
		} else {
			node = &schemaNode{
				Type:  "array",
				Items: reflectSchema(elem, visiting, cache),
			}
		}
	case reflect.Map:
		// JSON objects with dynamic keys: enumerate the value type via
		// additionalProperties so consumers know what map[string]V becomes.
		node = &schemaNode{
			Type:                 "object",
			AdditionalProperties: reflectSchema(t.Elem(), visiting, cache),
		}
	case reflect.Interface:
		// Unconstrained — anything could be here. Leave type unset so the
		// schema is permissive.
		node = &schemaNode{}
	case reflect.Struct:
		node = reflectStruct(t, visiting, cache)
	default:
		node = &schemaNode{}
	}

	// Only cache structs and named types where reuse actually pays off; for
	// anonymous leaf nodes (reflect returns the same reflect.Type for any
	// string field, so caching helps) we still populate — it's cheap.
	cache[t] = node
	return node
}

func reflectStruct(t reflect.Type, visiting map[reflect.Type]bool, cache map[reflect.Type]*schemaNode) *schemaNode {
	visiting[t] = true
	defer delete(visiting, t)

	node := &schemaNode{
		Type:       "object",
		Properties: map[string]*schemaNode{},
	}

	collectFields(t, node.Properties, visiting, cache)

	if len(node.Properties) == 0 {
		node.Properties = nil
	}
	return node
}

// collectFields walks struct fields, handling anonymous embedded fields by
// recursing into their fields (so the embedded type's JSON fields appear
// alongside the parent's).
func collectFields(t reflect.Type, props map[string]*schemaNode, visiting map[reflect.Type]bool, cache map[reflect.Type]*schemaNode) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		// Anonymous embed: recurse into its fields (after unwrapping pointer).
		// This must come before the exported check because reflect reports
		// an anonymous field of a lowercase type as unexported, yet its
		// exported fields still promote through encoding/json.
		if f.Anonymous {
			embedded := f.Type
			for embedded.Kind() == reflect.Ptr {
				embedded = embedded.Elem()
			}
			if embedded.Kind() == reflect.Struct {
				collectFields(embedded, props, visiting, cache)
			}
			continue
		}

		// Skip unexported fields.
		if !f.IsExported() {
			continue
		}

		name := parseJSONTag(f)
		if name == "-" {
			continue
		}

		child := reflectSchema(f.Type, visiting, cache)

		// Field-level tags (desc, enum, kind) are annotations on a specific
		// field, not on the underlying type. The cache shares *schemaNode
		// across all fields of the same type, so we must clone before
		// mutating to avoid leaking one field's annotation onto another.
		//
		// For array fields (child.Type == "array" && child.Items != nil),
		// enum/kind describe the element type, not the array itself. We dive
		// into items: clone the items node, annotate it, then rebuild the
		// array node with the new items pointer. desc always stays on the
		// outer field.
		desc := f.Tag.Get("desc")
		enumTag := f.Tag.Get("enum")
		kindTag := f.Tag.Get("kind")

		hasTagAnnotation := desc != "" || enumTag != "" || kindTag != ""
		if hasTagAnnotation {
			isArray := child != nil && child.Type == "array" && child.Items != nil

			if isArray {
				// Clone the items node and apply enum/kind to it.
				itemsClone := *child.Items
				if enumTag != "" {
					itemsClone.Enum = splitCSV(enumTag)
				}
				if kindTag != "" {
					itemsClone.Format = kindTag
				}
				// Rebuild the array node with cloned+annotated items.
				// desc stays on the array node itself.
				newArr := *child
				newArr.Items = &itemsClone
				if desc != "" {
					newArr.Description = desc
				}
				child = &newArr
			} else {
				// Scalar (or map, etc.) — clone the field node and annotate.
				cloned := *child
				if desc != "" {
					cloned.Description = desc
				}
				if enumTag != "" {
					cloned.Enum = splitCSV(enumTag)
				}
				if kindTag != "" {
					cloned.Format = kindTag
				}
				child = &cloned
			}
		}

		props[name] = child
	}
}

// splitCSV splits a comma-separated string into trimmed, non-empty parts.
// Used for parsing `enum:"a,b,c"` struct tags.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// parseJSONTag returns the wire name. `json:"-"` stays as "-" so callers
// can detect and skip. Empty / missing tags fall back to Go field name.
func parseJSONTag(f reflect.StructField) string {
	tag := f.Tag.Get("json")
	if tag == "" {
		return f.Name
	}
	name := strings.SplitN(tag, ",", 2)[0]
	if name == "" {
		return f.Name
	}
	return name
}
