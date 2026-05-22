// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"bytes"
	"encoding/json"
	"sort"
	"strconv"
	"sync"

	"github.com/larksuite/cli/internal/registry"
)

// MethodKeyOrder records the natural meta_data.json key order for one method's
// parameters / requestBody / responseBody. Nested object key orders are stored
// under NestedKeys, keyed by dotted path from the method root
// (e.g. "responseBody.items.properties").
type MethodKeyOrder struct {
	Parameters   []string
	RequestBody  []string
	ResponseBody []string
	NestedKeys   map[string][]string
}

var (
	keyOrderIndex    map[string]*MethodKeyOrder // dottedPath -> order
	keyOrderInitOnce sync.Once
)

// lookupKeyOrder returns the key-order record for service.resourcePath.method,
// or nil if the method is not in the embedded data (e.g. remote-cached).
func lookupKeyOrder(service string, resourcePath []string, method string) *MethodKeyOrder {
	keyOrderInitOnce.Do(buildKeyOrderIndex)
	if keyOrderIndex == nil {
		return nil
	}
	dotted := dottedPath(service, resourcePath, method)
	return keyOrderIndex[dotted]
}

func dottedPath(service string, resourcePath []string, method string) string {
	var buf bytes.Buffer
	buf.WriteString(service)
	for _, r := range resourcePath {
		buf.WriteByte('.')
		buf.WriteString(r)
	}
	buf.WriteByte('.')
	buf.WriteString(method)
	return buf.String()
}

// buildKeyOrderIndex parses the embedded meta_data.json bytes once at init,
// walking services -> resources -> methods -> {parameters,requestBody,responseBody}
// and recording each map's key insertion order via json.Decoder.Token().
func buildKeyOrderIndex() {
	raw := registry.EmbeddedMetaJSON()
	if len(raw) == 0 {
		return
	}
	keyOrderIndex = make(map[string]*MethodKeyOrder)

	dec := json.NewDecoder(bytes.NewReader(raw))
	// Top-level: { "services": [...], "version": "..." }
	if !expectDelim(dec, '{') {
		return
	}
	for dec.More() {
		key, _ := readKey(dec)
		if key != "services" {
			skipValue(dec)
			continue
		}
		if !expectDelim(dec, '[') {
			return
		}
		for dec.More() {
			parseService(dec)
		}
		// closing ]
		_, _ = dec.Token()
	}
}

// parseService consumes one service object inside services[].
// meta_data.json may emit "resources" before "name", so we first capture both
// raw fields, then walk resources with the resolved service name.
func parseService(dec *json.Decoder) {
	if !expectDelim(dec, '{') {
		return
	}
	var serviceName string
	var resourcesRaw json.RawMessage
	for dec.More() {
		key, _ := readKey(dec)
		switch key {
		case "name":
			tok, _ := dec.Token()
			if s, ok := tok.(string); ok {
				serviceName = s
			}
		case "resources":
			if err := dec.Decode(&resourcesRaw); err != nil {
				skipValue(dec)
			}
		default:
			skipValue(dec)
		}
	}
	_, _ = dec.Token() // closing }
	if serviceName != "" && len(resourcesRaw) > 0 {
		subDec := json.NewDecoder(bytes.NewReader(resourcesRaw))
		parseResources(subDec, serviceName, nil)
	}
}

// parseResources walks a resources map (resName -> resource object).
// resourcePath is the accumulated path of parent resources (for nested resources).
func parseResources(dec *json.Decoder, service string, resourcePath []string) {
	if !expectDelim(dec, '{') {
		return
	}
	for dec.More() {
		resName, _ := readKey(dec)
		parseResourceObj(dec, service, append(resourcePath, resName))
	}
	_, _ = dec.Token()
}

// parseResourceObj consumes one resource value: { methods: {...}, ... } and may
// recurse into nested resources via "resources" key if present.
func parseResourceObj(dec *json.Decoder, service string, resourcePath []string) {
	if !expectDelim(dec, '{') {
		return
	}
	for dec.More() {
		key, _ := readKey(dec)
		switch key {
		case "methods":
			parseMethods(dec, service, resourcePath)
		case "resources":
			parseResources(dec, service, resourcePath)
		default:
			skipValue(dec)
		}
	}
	_, _ = dec.Token()
}

// parseMethods consumes the methods map (methodName -> method object).
func parseMethods(dec *json.Decoder, service string, resourcePath []string) {
	if !expectDelim(dec, '{') {
		return
	}
	for dec.More() {
		methodName, _ := readKey(dec)
		mko := parseMethod(dec)
		dotted := dottedPath(service, resourcePath, methodName)
		keyOrderIndex[dotted] = mko
	}
	_, _ = dec.Token()
}

// parseMethod consumes one method object and records key orders.
func parseMethod(dec *json.Decoder) *MethodKeyOrder {
	mko := &MethodKeyOrder{NestedKeys: make(map[string][]string)}
	if !expectDelim(dec, '{') {
		return mko
	}
	for dec.More() {
		key, _ := readKey(dec)
		switch key {
		case "parameters":
			mko.Parameters = recordObjectKeysRecursive(dec, "parameters", mko.NestedKeys)
		case "requestBody":
			mko.RequestBody = recordObjectKeysRecursive(dec, "requestBody", mko.NestedKeys)
		case "responseBody":
			mko.ResponseBody = recordObjectKeysRecursive(dec, "responseBody", mko.NestedKeys)
		default:
			skipValue(dec)
		}
	}
	_, _ = dec.Token()
	return mko
}

// recordObjectKeysRecursive consumes an object and records the top-level key
// order. It also recurses into each child's "properties" submap, recording
// nested orders under prefix.subpath in nestedKeys. Returns the top-level keys
// in order.
func recordObjectKeysRecursive(dec *json.Decoder, prefix string, nestedKeys map[string][]string) []string {
	if !expectDelim(dec, '{') {
		return nil
	}
	var order []string
	for dec.More() {
		key, _ := readKey(dec)
		order = append(order, key)
		// Each child value is itself an object; we want its nested "properties" order if present.
		consumeFieldRecursive(dec, prefix+"."+key, nestedKeys)
	}
	_, _ = dec.Token()
	if prefix != "" && len(order) > 0 {
		nestedKeys[prefix] = order
	}
	return order
}

// consumeFieldRecursive consumes a field object (e.g. one parameter spec) and,
// if it contains "properties": {...}, recursively records that submap's order.
func consumeFieldRecursive(dec *json.Decoder, path string, nestedKeys map[string][]string) {
	tok, err := dec.Token()
	if err != nil {
		return
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		// Not an object — skip the rest of the value
		skipValueAfterToken(dec, tok)
		return
	}
	for dec.More() {
		fieldKey, _ := readKey(dec)
		if fieldKey == "properties" {
			recordObjectKeysRecursive(dec, path+".properties", nestedKeys)
		} else {
			skipValue(dec)
		}
	}
	_, _ = dec.Token()
}

// --- json.Decoder helpers ---

func expectDelim(dec *json.Decoder, want json.Delim) bool {
	tok, err := dec.Token()
	if err != nil {
		return false
	}
	delim, ok := tok.(json.Delim)
	return ok && delim == want
}

func readKey(dec *json.Decoder) (string, error) {
	tok, err := dec.Token()
	if err != nil {
		return "", err
	}
	s, _ := tok.(string)
	return s, nil
}

// skipValue consumes the next complete value (scalar, object, or array).
func skipValue(dec *json.Decoder) {
	tok, err := dec.Token()
	if err != nil {
		return
	}
	skipValueAfterToken(dec, tok)
}

func skipValueAfterToken(dec *json.Decoder, tok json.Token) {
	delim, ok := tok.(json.Delim)
	if !ok {
		return
	}
	// We started inside a container of type `delim` ({ or [) and must eat
	// tokens until that container closes, tracking nested containers of any
	// kind. depth counts how many open containers we are currently inside.
	_ = delim
	depth := 1
	for depth > 0 {
		t, err := dec.Token()
		if err != nil {
			return
		}
		if d, ok := t.(json.Delim); ok {
			switch d {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
		}
	}
}

// convertProperty recursively converts one meta_data field map into a Property.
// nestedPath is the dotted lookup key into the current method's NestedKeys map
// (e.g. "responseBody.items.properties"). Empty path = top-level, no nested
// lookup needed.
func convertProperty(field map[string]interface{}, nestedPath string) Property {
	var p Property

	rawType, _ := field["type"].(string)
	switch rawType {
	case "file":
		p.Type = "string"
		p.Format = "binary"
	default:
		p.Type = rawType
	}

	if s, ok := field["description"].(string); ok {
		p.Description = s
	}
	if v, ok := field["default"]; ok {
		p.Default = v
	}
	if v, ok := field["example"]; ok {
		p.Example = v
	}

	// min / max are stored as strings in meta_data; parse on best-effort.
	if minStr, ok := field["min"].(string); ok && minStr != "" {
		if v, err := strconv.ParseFloat(minStr, 64); err == nil {
			p.Minimum = &v
		}
	}
	if maxStr, ok := field["max"].(string); ok && maxStr != "" {
		if v, err := strconv.ParseFloat(maxStr, 64); err == nil {
			p.Maximum = &v
		}
	}

	// enum: prefer existing "enum" array; else extract from options[].value
	if enumRaw, ok := field["enum"].([]interface{}); ok && len(enumRaw) > 0 {
		for _, e := range enumRaw {
			if s, ok := e.(string); ok {
				p.Enum = append(p.Enum, s)
			}
		}
	} else if optsRaw, ok := field["options"].([]interface{}); ok && len(optsRaw) > 0 {
		seen := make(map[string]bool)
		for _, o := range optsRaw {
			om, ok := o.(map[string]interface{})
			if !ok {
				continue
			}
			if v, ok := om["value"].(string); ok && !seen[v] {
				seen[v] = true
				p.Enum = append(p.Enum, v)
			}
		}
		sort.Strings(p.Enum)
	}

	// nested properties: recurse
	if propsRaw, ok := field["properties"].(map[string]interface{}); ok && len(propsRaw) > 0 {
		nested := buildOrderedProps(propsRaw, nestedPath)
		if rawType == "array" {
			// meta_data quirk: array element schema is wrapped in "properties".
			// Unfold into Items: { type: "object", properties: <nested> }
			p.Items = &Property{
				Type:       "object",
				Properties: nested,
			}
			// Property.Properties stays nil for arrays
		} else {
			if p.Type == "" {
				p.Type = "object" // infer
			}
			p.Properties = nested
		}
	}

	return p
}

// buildOrderedProps converts a map[string]interface{} of field specs into an
// OrderedProps, using the key-order index for the given nested path if
// available; otherwise falls back to alphabetical order.
func buildOrderedProps(raw map[string]interface{}, nestedPath string) *OrderedProps {
	op := &OrderedProps{Map: make(map[string]Property, len(raw))}

	keys := orderedKeys(raw, nestedPath)
	for _, k := range keys {
		fieldRaw, _ := raw[k].(map[string]interface{})
		op.Order = append(op.Order, k)
		op.Map[k] = convertProperty(fieldRaw, nestedPath+"."+k+".properties")
	}
	return op
}

// currentMethodOrder is the per-method key-order context used by orderedKeys.
// It is set inside AssembleEnvelope (under assembleMu) and reset on return.
var currentMethodOrder *MethodKeyOrder

// orderedKeys returns the keys of raw in their meta_data natural order if
// the current per-method key-order context has them recorded; otherwise
// alphabetical fallback.
func orderedKeys(raw map[string]interface{}, nestedPath string) []string {
	if currentMethodOrder != nil && nestedPath != "" {
		if order, ok := currentMethodOrder.NestedKeys[nestedPath]; ok {
			// Filter to keys that actually exist in raw (defensive)
			out := make([]string, 0, len(order))
			seen := make(map[string]bool)
			for _, k := range order {
				if _, ok := raw[k]; ok {
					out = append(out, k)
					seen[k] = true
				}
			}
			// Append any keys present in raw but missing from order (defensive),
			// alphabetically for determinism.
			var extra []string
			for k := range raw {
				if !seen[k] {
					extra = append(extra, k)
				}
			}
			sort.Strings(extra)
			out = append(out, extra...)
			return out
		}
	}
	// Fallback: alphabetical
	keys := make([]string, 0, len(raw))
	for k := range raw {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
