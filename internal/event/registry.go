// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"fmt"
	"sort"
	"sync"
)

var (
	keys = map[string]*KeyDefinition{}
	mu   sync.RWMutex
)

// RegisterKey panics on duplicate Key, empty EventType, or schema/process contract violations.
func RegisterKey(def KeyDefinition) {
	mu.Lock()
	defer mu.Unlock()

	if _, exists := keys[def.Key]; exists {
		panic(fmt.Sprintf("duplicate EventKey: %s", def.Key))
	}
	if def.EventType == "" {
		panic(fmt.Sprintf("EventKey %s: EventType must not be empty", def.Key))
	}

	validateSchema(def)
	validateParams(def)
	validateAuth(def)

	if def.BufferSize > MaxBufferSize {
		def.BufferSize = MaxBufferSize
	}
	if def.BufferSize <= 0 {
		def.BufferSize = DefaultBufferSize
	}
	if def.Workers <= 0 {
		def.Workers = 1
	}
	keys[def.Key] = &def
}

// validateSchema: exactly one of Native/Custom; Native incompatible with Process.
func validateSchema(def KeyDefinition) {
	nativeSet := def.Schema.Native != nil
	customSet := def.Schema.Custom != nil
	if nativeSet && customSet {
		panic(fmt.Sprintf("EventKey %s: Schema.Native and Schema.Custom are mutually exclusive", def.Key))
	}
	if !nativeSet && !customSet {
		panic(fmt.Sprintf("EventKey %s: Schema requires either Native or Custom", def.Key))
	}
	if nativeSet && def.Process != nil {
		panic(fmt.Sprintf("EventKey %s: Schema.Native forbids Process (Process produces a complete shape — use Schema.Custom)", def.Key))
	}
	if spec := def.Schema.Native; spec != nil {
		validateSpec(def.Key, "Schema.Native", spec)
	}
	if spec := def.Schema.Custom; spec != nil {
		validateSpec(def.Key, "Schema.Custom", spec)
	}
}

func validateSpec(key, field string, s *SchemaSpec) {
	typeSet := s.Type != nil
	rawSet := len(s.Raw) > 0
	if typeSet == rawSet {
		panic(fmt.Sprintf("EventKey %s: %s requires exactly one of Type or Raw", key, field))
	}
}

func validateParams(def KeyDefinition) {
	for _, p := range def.Params {
		switch p.Type {
		case "", ParamString, ParamBool, ParamInt:
		case ParamEnum, ParamMulti:
			if len(p.Values) == 0 {
				panic(fmt.Sprintf("EventKey %s: param %q type %q requires Values", def.Key, p.Name, p.Type))
			}
			for _, v := range p.Values {
				if v.Desc == "" {
					panic(fmt.Sprintf("EventKey %s: param %q value %q requires non-empty Desc", def.Key, p.Name, v.Value))
				}
			}
		default:
			panic(fmt.Sprintf("EventKey %s: param %q has unknown type %q", def.Key, p.Name, p.Type))
		}
	}
}

func validateAuth(def KeyDefinition) {
	for _, t := range def.AuthTypes {
		if t != "user" && t != "bot" {
			panic(fmt.Sprintf("EventKey %s: AuthTypes elements must be \"user\" or \"bot\"; got %q", def.Key, t))
		}
	}
}

func Lookup(key string) (*KeyDefinition, bool) {
	mu.RLock()
	defer mu.RUnlock()
	def, ok := keys[key]
	return def, ok
}

// ListAll returns all KeyDefinitions sorted by Key.
func ListAll() []*KeyDefinition {
	mu.RLock()
	defer mu.RUnlock()
	result := make([]*KeyDefinition, 0, len(keys))
	for _, def := range keys {
		result = append(result, def)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Key < result[j].Key
	})
	return result
}

func resetRegistry() {
	mu.Lock()
	defer mu.Unlock()
	keys = map[string]*KeyDefinition{}
}

func ResetRegistryForTest() { resetRegistry() }

// UnregisterKeyForTest removes one key — use this (not Reset) in tests with synthetic keys
// alongside production keys to keep -count=N reruns idempotent.
func UnregisterKeyForTest(key string) {
	mu.Lock()
	defer mu.Unlock()
	delete(keys, key)
}
