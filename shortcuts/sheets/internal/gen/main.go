// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Command gen regenerates flag_defs_gen.go and flag_schemas_gen.go from the
// data/*.json spec artifacts, so command startup pays no JSON unmarshal.
//
// Invoked via `go generate ./shortcuts/sheets/...` (see ../../generate.go).
// data/*.json stays the canonical source (synced from sheet-skill-spec); the
// *_gen.go files are committed, derived artifacts. CI should run go generate
// and fail on a dirty tree to keep them in lockstep.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/format"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

type flagDef struct {
	Name     string   `json:"name"`
	Kind     string   `json:"kind"`
	Type     string   `json:"type"`
	Required string   `json:"required"`
	Desc     string   `json:"desc"`
	Default  string   `json:"default"`
	Hidden   bool     `json:"hidden"`
	Enum     []string `json:"enum"`
	Input    []string `json:"input"`
}

type commandDef struct {
	Risk  string    `json:"risk"`
	Flags []flagDef `json:"flags"`
}

// sheetsDir resolves shortcuts/sheets from this generator's own location, so
// the tool works regardless of the caller's working directory.
func sheetsDir() string {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		log.Fatal("gen: cannot resolve caller path")
	}
	// thisFile = <repo>/shortcuts/sheets/internal/gen/main.go
	return filepath.Join(filepath.Dir(thisFile), "..", "..")
}

func writeFormatted(path string, b *bytes.Buffer) {
	out, err := format.Source(b.Bytes())
	if err != nil {
		log.Fatalf("gen: format %s: %v", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %s (%d bytes)\n", filepath.Base(path), len(out))
}

func main() {
	dir := sheetsDir()
	genFlagDefs(dir)
	genFlagSchemas(dir)
}

const flagDefsHeader = `// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Code generated from data/flag-defs.json; DO NOT EDIT.

package sheets

// flagDefs is the compiled form of data/flag-defs.json — every CLI flag's
// metadata for every shortcut, emitted as a Go literal so command startup
// pays no JSON unmarshal (see flag_defs.go). Do not hand-edit; regenerate
// with ` + "`go generate ./shortcuts/sheets/...`" + ` after data/flag-defs.json
// changes.
var flagDefs = map[string]commandDef{
`

func sliceLit(s []string) string {
	parts := make([]string, len(s))
	for i, v := range s {
		parts[i] = fmt.Sprintf("%q", v)
	}
	return "[]string{" + strings.Join(parts, ", ") + "}"
}

func flagLit(f flagDef) string {
	var p []string
	if f.Name != "" {
		p = append(p, fmt.Sprintf("Name: %q", f.Name))
	}
	if f.Kind != "" {
		p = append(p, fmt.Sprintf("Kind: %q", f.Kind))
	}
	if f.Type != "" {
		p = append(p, fmt.Sprintf("Type: %q", f.Type))
	}
	if f.Required != "" {
		p = append(p, fmt.Sprintf("Required: %q", f.Required))
	}
	if f.Desc != "" {
		p = append(p, fmt.Sprintf("Desc: %q", f.Desc))
	}
	if f.Default != "" {
		p = append(p, fmt.Sprintf("Default: %q", f.Default))
	}
	if f.Hidden {
		p = append(p, "Hidden: true")
	}
	if f.Enum != nil {
		p = append(p, "Enum: "+sliceLit(f.Enum))
	}
	if f.Input != nil {
		p = append(p, "Input: "+sliceLit(f.Input))
	}
	return "{" + strings.Join(p, ", ") + "}"
}

func genFlagDefs(dir string) {
	raw, err := os.ReadFile(filepath.Join(dir, "data", "flag-defs.json"))
	if err != nil {
		log.Fatal(err)
	}
	var defs map[string]commandDef
	if err := json.Unmarshal(raw, &defs); err != nil {
		log.Fatal(err)
	}

	keys := make([]string, 0, len(defs))
	for k := range defs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b bytes.Buffer
	b.WriteString(flagDefsHeader)
	for _, k := range keys {
		cd := defs[k]
		fmt.Fprintf(&b, "%q: {\n", k)
		if cd.Risk != "" {
			fmt.Fprintf(&b, "Risk: %q,\n", cd.Risk)
		}
		if cd.Flags != nil {
			b.WriteString("Flags: []flagDef{\n")
			for _, f := range cd.Flags {
				b.WriteString(flagLit(f))
				b.WriteString(",\n")
			}
			b.WriteString("},\n")
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n")

	writeFormatted(filepath.Join(dir, "flag_defs_gen.go"), &b)
}

const flagSchemasHeader = `// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Code generated from data/flag-schemas.json; DO NOT EDIT.

package sheets

// commandsWithSchema is the set of shortcut commands that have at least one
// introspectable composite flag in data/flag-schemas.json. Codegen'd so the
// registration loop (shortcuts.go) and the validate fast-path can gate on it
// without parsing the 256KB schema blob at startup (that parse used to run on
// every CLI invocation, sheets or not). The 256KB is now only unmarshaled
// on --print-schema or when validating a command that is in this set. Do not
// hand-edit; regenerate with ` + "`go generate ./shortcuts/sheets/...`" + `.
var commandsWithSchema = map[string]struct{}{
`

func genFlagSchemas(dir string) {
	raw, err := os.ReadFile(filepath.Join(dir, "data", "flag-schemas.json"))
	if err != nil {
		log.Fatal(err)
	}
	var doc struct {
		Flags map[string]json.RawMessage `json:"flags"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		log.Fatal(err)
	}

	keys := make([]string, 0, len(doc.Flags))
	for k := range doc.Flags {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b bytes.Buffer
	b.WriteString(flagSchemasHeader)
	for _, k := range keys {
		fmt.Fprintf(&b, "%q: {},\n", k)
	}
	b.WriteString("}\n")

	writeFormatted(filepath.Join(dir, "flag_schemas_gen.go"), &b)
}
