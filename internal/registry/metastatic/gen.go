// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build ignore

// Command gen reads internal/registry/meta_data.json and emits
// meta_data_gen.go: the embedded command spec as a single static
// metaschema.Registry literal (zero runtime JSON parse, zero startup heap
// allocation). Run via: go run internal/registry/metastatic/gen.go
//
// Maps in the JSON (resources/methods/fields) are emitted as slices sorted by
// key so generation is deterministic.
package main

import (
	"encoding/json"
	"fmt"
	"go/format"
	"os"
	"sort"
	"strings"
)

const (
	inPath  = "internal/registry/meta_data.json"
	outPath = "internal/registry/metastatic/meta_data_gen.go"
)

func gs(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}
func gb(m map[string]any, k string) bool {
	if v, ok := m[k].(bool); ok {
		return v
	}
	return false
}
func gss(m map[string]any, k string) []string {
	raw, _ := m[k].([]any)
	out := make([]string, 0, len(raw))
	for _, e := range raw {
		if s, ok := e.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
func gm(m map[string]any, k string) map[string]any {
	if v, ok := m[k].(map[string]any); ok {
		return v
	}
	return nil
}
func sortedKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

func emitStrSlice(b *strings.Builder, name string, vs []string) {
	if len(vs) == 0 {
		return
	}
	fmt.Fprintf(b, "%s: []string{", name)
	for _, v := range vs {
		fmt.Fprintf(b, "%q, ", v)
	}
	b.WriteString("},\n")
}

func emitOptions(b *strings.Builder, raw []any) {
	if len(raw) == 0 {
		return
	}
	b.WriteString("Options: []metaschema.Option{")
	for _, e := range raw {
		o, _ := e.(map[string]any)
		fmt.Fprintf(b, "{Value: %q, Description: %q}, ", gs(o, "value"), gs(o, "description"))
	}
	b.WriteString("},\n")
}

// emitFields emits a metaschema.Field slice from a JSON map[fieldName]fieldSpec.
func emitFields(b *strings.Builder, label string, fm map[string]any) {
	if len(fm) == 0 {
		return
	}
	fmt.Fprintf(b, "%s: []metaschema.Field{\n", label)
	for _, name := range sortedKeys(fm) {
		f, _ := fm[name].(map[string]any)
		if f == nil {
			continue
		}
		b.WriteString("{")
		fmt.Fprintf(b, "Name: %q, ", name)
		for _, kv := range []struct{ k, field string }{
			{"type", "Type"}, {"location", "Location"}, {"description", "Description"},
			{"default", "Default"}, {"example", "Example"}, {"enumName", "EnumName"},
			{"min", "Min"}, {"max", "Max"}, {"ref", "Ref"},
		} {
			if v := gs(f, kv.k); v != "" {
				fmt.Fprintf(b, "%s: %q, ", kv.field, v)
			}
		}
		if gb(f, "required") {
			b.WriteString("Required: true, ")
		}
		emitStrSlice(b, "Enum", gss(f, "enum"))
		emitStrSlice(b, "Annotations", gss(f, "annotations"))
		if opts, ok := f["options"].([]any); ok {
			emitOptions(b, opts)
		}
		if props := gm(f, "properties"); props != nil {
			emitFields(b, "Properties", props)
		}
		b.WriteString("},\n")
	}
	b.WriteString("},\n")
}

// emitAffordance emits a metaschema.Affordance literal from a method's
// "affordance" JSON object, or nothing when absent/empty.
func emitAffordance(b *strings.Builder, raw map[string]any) {
	if raw == nil {
		return
	}
	useWhen := gss(raw, "use_when")
	doNot := gss(raw, "do_not_use_when")
	prereq := gss(raw, "prerequisites")
	related := gss(raw, "related")
	examples, _ := raw["examples"].([]any)
	if len(useWhen) == 0 && len(doNot) == 0 && len(prereq) == 0 && len(related) == 0 && len(examples) == 0 {
		return
	}
	b.WriteString("Affordance: &metaschema.Affordance{")
	emitStrSlice(b, "UseWhen", useWhen)
	emitStrSlice(b, "DoNotUseWhen", doNot)
	emitStrSlice(b, "Prerequisites", prereq)
	if len(examples) > 0 {
		b.WriteString("Examples: []metaschema.AffordanceExample{")
		for _, e := range examples {
			ex, _ := e.(map[string]any)
			fmt.Fprintf(b, "{Description: %q, Command: %q}, ", gs(ex, "description"), gs(ex, "command"))
		}
		b.WriteString("},\n")
	}
	emitStrSlice(b, "Related", related)
	b.WriteString("},\n")
}

func emitMethods(b *strings.Builder, mm map[string]any) {
	b.WriteString("Methods: []metaschema.Method{\n")
	for _, name := range sortedKeys(mm) {
		m, _ := mm[name].(map[string]any)
		if m == nil {
			continue
		}
		b.WriteString("{")
		fmt.Fprintf(b, "Name: %q, ID: %q, Path: %q, HTTPMethod: %q, Description: %q, ",
			name, gs(m, "id"), gs(m, "path"), gs(m, "httpMethod"), gs(m, "description"))
		if v := gs(m, "risk"); v != "" {
			fmt.Fprintf(b, "Risk: %q, ", v)
		}
		if v := gs(m, "docUrl"); v != "" {
			fmt.Fprintf(b, "DocURL: %q, ", v)
		}
		if gb(m, "danger") {
			b.WriteString("Danger: true, ")
		}
		b.WriteString("\n")
		emitStrSlice(b, "Scopes", gss(m, "scopes"))
		emitStrSlice(b, "AccessTokens", gss(m, "accessTokens"))
		emitStrSlice(b, "ParameterOrder", gss(m, "parameterOrder"))
		emitStrSlice(b, "RequiredScopes", gss(m, "requiredScopes"))
		emitFields(b, "Parameters", gm(m, "parameters"))
		emitFields(b, "RequestBody", gm(m, "requestBody"))
		emitFields(b, "ResponseBody", gm(m, "responseBody"))
		emitAffordance(b, gm(m, "affordance"))
		b.WriteString("},\n")
	}
	b.WriteString("},\n")
}

func main() {
	data, err := os.ReadFile(inPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "read:", err)
		os.Exit(1)
	}
	var reg map[string]any
	if err := json.Unmarshal(data, &reg); err != nil {
		fmt.Fprintln(os.Stderr, "unmarshal:", err)
		os.Exit(1)
	}

	var b strings.Builder
	b.WriteString("// Code generated from meta_data.json by gen.go. DO NOT EDIT.\n")
	b.WriteString("// Gitignored; produced at build time by `make fetch_meta`.\n\n")
	b.WriteString("package metastatic\n\n")
	b.WriteString("import \"github.com/larksuite/cli/internal/registry/metaschema\"\n\n")
	b.WriteString("// registryData holds the command spec as static Go data. It is a\n")
	b.WriteString("// package-level var, so its backing arrays live in the binary's static\n")
	b.WriteString("// section (zero heap alloc on read). init() wires it into the Registry\n")
	b.WriteString("// declared by stub.go with a single struct-header copy. No build tag is\n")
	b.WriteString("// needed: when this generated file is absent (fresh checkout) stub.go's\n")
	b.WriteString("// empty Registry stands alone; when present, init() augments it.\n")
	b.WriteString("var registryData = metaschema.Registry{\n")
	fmt.Fprintf(&b, "Version: %q,\n", gs(reg, "version"))
	b.WriteString("Services: []metaschema.Service{\n")
	svcs, _ := reg["services"].([]any)
	for _, sv := range svcs {
		s, _ := sv.(map[string]any)
		if s == nil {
			continue
		}
		b.WriteString("{")
		fmt.Fprintf(&b, "Name: %q, Version: %q, Title: %q, Description: %q, ServicePath: %q,\n",
			gs(s, "name"), gs(s, "version"), gs(s, "title"), gs(s, "description"), gs(s, "servicePath"))
		b.WriteString("Resources: []metaschema.Resource{\n")
		res := gm(s, "resources")
		for _, rname := range sortedKeys(res) {
			r, _ := res[rname].(map[string]any)
			if r == nil {
				continue
			}
			fmt.Fprintf(&b, "{Name: %q,\n", rname)
			emitMethods(&b, gm(r, "methods"))
			b.WriteString("},\n")
		}
		b.WriteString("},\n") // Resources
		b.WriteString("},\n") // Service
	}
	b.WriteString("},\n")  // Services
	b.WriteString("}\n\n") // registryData literal
	b.WriteString("func init() { Registry = registryData }\n")

	src, err := format.Source([]byte(b.String()))
	if err != nil {
		// Write unformatted for debugging, then fail.
		_ = os.WriteFile(outPath+".broken", []byte(b.String()), 0644)
		fmt.Fprintln(os.Stderr, "gofmt:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outPath, src, 0644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s (%d services, %d bytes)\n", outPath, len(svcs), len(src))
}
