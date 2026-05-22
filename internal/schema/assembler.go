// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"bytes"
	"encoding/json"
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
