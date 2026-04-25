// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package schemas derives JSON Schema fragments from Go types via reflection.
package schemas

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
)

// FromType derives a JSON Schema for t (cached per reflect.Type).
func FromType(t reflect.Type) json.RawMessage {
	if t == nil {
		return nil
	}
	if cached, ok := cacheLoad(t); ok {
		return cached
	}
	// per-call cache so shared subtypes are walked once; not coupled to the marshaled-JSON cache.
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

type schemaNode struct {
	Type                 string                 `json:"type,omitempty"`
	Description          string                 `json:"description,omitempty"`
	Enum                 []string               `json:"enum,omitempty"`
	Format               string                 `json:"format,omitempty"`
	Properties           map[string]*schemaNode `json:"properties,omitempty"`
	Items                *schemaNode            `json:"items,omitempty"`
	AdditionalProperties *schemaNode            `json:"additionalProperties,omitempty"`
}

// reflectSchema walks t; visiting breaks cycles, cache memoises shared subtypes.
func reflectSchema(t reflect.Type, visiting map[reflect.Type]bool, cache map[reflect.Type]*schemaNode) *schemaNode {
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
		if elem.Kind() == reflect.Uint8 {
			node = &schemaNode{Type: "string"} // []byte → string
		} else {
			node = &schemaNode{
				Type:  "array",
				Items: reflectSchema(elem, visiting, cache),
			}
		}
	case reflect.Map:
		node = &schemaNode{
			Type:                 "object",
			AdditionalProperties: reflectSchema(t.Elem(), visiting, cache),
		}
	case reflect.Interface:
		node = &schemaNode{}
	case reflect.Struct:
		node = reflectStruct(t, visiting, cache)
	default:
		node = &schemaNode{}
	}

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

func collectFields(t reflect.Type, props map[string]*schemaNode, visiting map[reflect.Type]bool, cache map[reflect.Type]*schemaNode) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)

		// Anonymous embed must precede the IsExported check — embedded fields of
		// lowercase types still promote through encoding/json.
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

		if !f.IsExported() {
			continue
		}

		name := parseJSONTag(f)
		if name == "-" {
			continue
		}

		child := reflectSchema(f.Type, visiting, cache)

		// Clone before mutating: the cache shares *schemaNode across all fields of the same type,
		// so direct mutation would leak one field's annotation onto another.
		// For arrays, enum/kind apply to items; desc stays on the outer field.
		desc := f.Tag.Get("desc")
		enumTag := f.Tag.Get("enum")
		kindTag := f.Tag.Get("kind")

		hasTagAnnotation := desc != "" || enumTag != "" || kindTag != ""
		if hasTagAnnotation {
			isArray := child != nil && child.Type == "array" && child.Items != nil

			if isArray {
				itemsClone := *child.Items
				if enumTag != "" {
					itemsClone.Enum = splitCSV(enumTag)
				}
				if kindTag != "" {
					itemsClone.Format = kindTag
				}
				newArr := *child
				newArr.Items = &itemsClone
				if desc != "" {
					newArr.Description = desc
				}
				child = &newArr
			} else {
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

// parseJSONTag returns the wire name; "-" propagates so callers can skip.
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
