// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Architecture gates for the events declaration layer.
//
// events/<domain> packages are declarations: EventKeys, payload shapes, and
// processing hooks. Two kinds of rot would quietly destroy that role:
//
//  1. Importing command wiring, a transport host, or a concrete adapter turns
//     declarations into another place where process and transport concerns
//     accumulate, and drags the whole adapter tree into every binary that
//     only wanted the catalog.
//  2. Re-parsing the envelope header inside a domain duplicates the kernel's
//     single header decode; the copies then drift apart the day the envelope
//     evolves.
//
// These tests turn both into build breaks.
package events_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	archModulePath          = "github.com/larksuite/cli"
	archAdapterImportPrefix = archModulePath + "/internal/event/adapter"
)

// archProductionGoFiles returns every non-test .go file under root,
// skipping testdata directories. Paths are relative to root.
func archProductionGoFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	sort.Strings(files)
	return files
}

// archForbiddenDomainImport reports why importPath is banned in events/, if
// it is. Domains may use the kernel (internal/event, model, catalog,
// processing, ...); they must never see the layers that host or transport
// them.
func archForbiddenDomainImport(importPath string) (reason string, banned bool) {
	switch importPath {
	case "github.com/spf13/cobra":
		return "CLI framework; command wiring lives in cmd, a declaration that needs cobra has stopped being a declaration", true
	case archModulePath + "/internal/event/bus":
		return "bus is a host process; a domain importing its host inverts the dependency direction", true
	case archModulePath + "/internal/event/consume":
		return "consume is a host process; a domain importing its host inverts the dependency direction", true
	}
	if importPath == archAdapterImportPrefix || strings.HasPrefix(importPath, archAdapterImportPrefix+"/") {
		return "concrete adapter; domains must stay transport-agnostic so any host can serve them", true
	}
	return "", false
}

// TestArchEventsImportRedline fails when any production file under events/
// imports command wiring, an event host, or a concrete adapter. It keeps the
// declaration layer linkable everywhere without pulling in transports.
func TestArchEventsImportRedline(t *testing.T) {
	files := archProductionGoFiles(t, ".")
	if len(files) == 0 {
		t.Fatal("scanned zero production files under events/ — the gate is idling; fix the walker before trusting any green run")
	}
	fset := token.NewFileSet()
	for _, file := range files {
		f, err := parser.ParseFile(fset, file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", file, err)
		}
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", file, err)
			}
			if reason, banned := archForbiddenDomainImport(path); banned {
				t.Errorf("%s imports %q: %s", filepath.ToSlash(file), path, reason)
			}
		}
	}
}

// envelopeHeaderTags are the metadata fields the kernel decodes exactly once
// from the envelope header. A domain that re-declares any of them inside a
// json:"header" block is re-parsing the envelope instead of consuming the
// kernel's decode — the duplicate drifts silently when the envelope changes.
var envelopeHeaderTags = map[string]bool{
	"event_id":    true,
	"event_type":  true,
	"create_time": true,
	"app_id":      true,
	"tenant_key":  true,
}

// headerReparseBaseline pins the residue that existed when this gate was
// introduced, keyed by file (relative to events/) with the header metadata
// tags each file re-parses. It is a ratchet, not an approval:
//   - never add an entry — new code must read the kernel-decoded header;
//   - deleting entries is always welcome, and once a file is cleaned the
//     stale check below forces its entry out so the cleanup cannot regress
//     unnoticed.
var headerReparseBaseline = map[string][]string{
	"application/menu.go":                  {"app_id", "create_time", "event_id", "event_type", "tenant_key"},
	"approval/register.go":                 {"create_time", "event_id", "event_type"},
	"im/card_action.go":                    {"create_time", "event_id", "event_type"},
	"im/message_receive.go":                {"create_time", "event_id", "event_type"},
	"minutes/minute_generated.go":          {"create_time", "event_id", "event_type"},
	"vc/note_generated.go":                 {"create_time", "event_id", "event_type"},
	"vc/participant_meeting_ended.go":      {"create_time", "event_id", "event_type"},
	"vc/participant_meeting_joined.go":     {"create_time", "event_id", "event_type"},
	"vc/participant_meeting_started.go":    {"create_time", "event_id", "event_type"},
	"vc/recording_ended.go":                {"create_time", "event_id", "event_type"},
	"vc/recording_started.go":              {"create_time", "event_id", "event_type"},
	"vc/recording_transcript_generated.go": {"create_time", "event_id", "event_type"},
}

type archHeaderReparse struct {
	file  string // slash path relative to events/
	line  int
	field string // Go field name inside the header block
	tag   string // offending json tag
}

// archJSONTagName extracts the json name (first comma segment) from a struct
// field tag, or "" when absent.
func archJSONTagName(field *ast.Field) string {
	if field.Tag == nil {
		return ""
	}
	raw, err := strconv.Unquote(field.Tag.Value)
	if err != nil {
		return ""
	}
	name, _, _ := strings.Cut(reflect.StructTag(raw).Get("json"), ",")
	return name
}

// archNamedStructIndex maps type names declared in the given files (one
// package) to their struct bodies, so a json:"header" field with a named
// type still resolves.
func archNamedStructIndex(files []*ast.File) map[string]*ast.StructType {
	index := make(map[string]*ast.StructType)
	for _, f := range files {
		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if st, ok := ts.Type.(*ast.StructType); ok {
					index[ts.Name.Name] = st
				}
			}
		}
	}
	return index
}

// archStructBody resolves expr to a struct body: inline struct types,
// pointers to them, and named types declared in the same package.
func archStructBody(expr ast.Expr, named map[string]*ast.StructType) *ast.StructType {
	switch v := expr.(type) {
	case *ast.StructType:
		return v
	case *ast.StarExpr:
		return archStructBody(v.X, named)
	case *ast.Ident:
		return named[v.Name]
	}
	return nil
}

// archFindHeaderReparses flags every field inside a json:"header" struct
// block whose json tag re-declares envelope header metadata. Fields outside
// header blocks are never flagged: a domain body owning its own create_time
// (e.g. a message's own timestamps) is legitimate.
func archFindHeaderReparses(fset *token.FileSet, file *ast.File, relPath string, named map[string]*ast.StructType) []archHeaderReparse {
	var found []archHeaderReparse
	ast.Inspect(file, func(n ast.Node) bool {
		st, ok := n.(*ast.StructType)
		if !ok {
			return true
		}
		for _, field := range st.Fields.List {
			if archJSONTagName(field) != "header" {
				continue
			}
			body := archStructBody(field.Type, named)
			if body == nil {
				continue
			}
			for _, hf := range body.Fields.List {
				tag := archJSONTagName(hf)
				if !envelopeHeaderTags[tag] {
					continue
				}
				name := "(embedded)"
				if len(hf.Names) > 0 {
					parts := make([]string, len(hf.Names))
					for i, ident := range hf.Names {
						parts[i] = ident.Name
					}
					name = strings.Join(parts, ",")
				}
				found = append(found, archHeaderReparse{
					file:  relPath,
					line:  fset.Position(hf.Pos()).Line,
					field: name,
					tag:   tag,
				})
			}
		}
		return true
	})
	return found
}

// TestArchEventsNoHeaderMetadataReparse fails when a production file under
// events/ declares a json:"header" struct block that re-parses envelope
// header metadata, except for the pinned pre-existing residue in
// headerReparseBaseline (which may only shrink).
func TestArchEventsNoHeaderMetadataReparse(t *testing.T) {
	files := archProductionGoFiles(t, ".")
	if len(files) == 0 {
		t.Fatal("scanned zero production files under events/ — the gate is idling; fix the walker before trusting any green run")
	}

	// Parse per directory so named header types declared in a sibling file
	// of the same package still resolve.
	byDir := make(map[string][]string)
	for _, file := range files {
		dir := filepath.Dir(file)
		byDir[dir] = append(byDir[dir], file)
	}
	dirs := make([]string, 0, len(byDir))
	for dir := range byDir {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	fset := token.NewFileSet()
	var violations []archHeaderReparse
	for _, dir := range dirs {
		astFiles := make([]*ast.File, 0, len(byDir[dir]))
		for _, file := range byDir[dir] {
			f, err := parser.ParseFile(fset, file, nil, parser.SkipObjectResolution)
			if err != nil {
				t.Fatalf("parse %s: %v", file, err)
			}
			astFiles = append(astFiles, f)
		}
		named := archNamedStructIndex(astFiles)
		for i, f := range astFiles {
			rel := filepath.ToSlash(byDir[dir][i])
			violations = append(violations, archFindHeaderReparses(fset, f, rel, named)...)
		}
	}

	seen := make(map[string]bool)
	for _, v := range violations {
		seen[v.file+"\x00"+v.tag] = true
		if slices.Contains(headerReparseBaseline[v.file], v.tag) {
			continue
		}
		t.Errorf("%s:%d field %s re-parses envelope header metadata %q inside a json:\"header\" block — consume the kernel-decoded header instead of unmarshalling the envelope again", v.file, v.line, v.field, v.tag)
	}

	// Stale baseline entries: once a file stops re-parsing a tag, its entry
	// must go, otherwise the ratchet is wider than reality and the cleanup
	// can silently regress.
	for file, tags := range headerReparseBaseline {
		for _, tag := range tags {
			if !seen[file+"\x00"+tag] {
				t.Errorf("stale baseline entry %s / %q: no code matches it anymore — delete the entry so the cleanup is locked in", file, tag)
			}
		}
	}
}

// TestArchEventsHeaderReparseDetectorSelfCheck runs the header-reparse
// detector on synthetic sources with a known violation count. If the
// detector rots (tag parsing, named-type resolution, header matching), the
// main gate would report green on a violating tree; this test makes that
// failure mode loud.
func TestArchEventsHeaderReparseDetectorSelfCheck(t *testing.T) {
	parse := func(src string) []archHeaderReparse {
		t.Helper()
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, "synthetic.go", src, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parse synthetic source: %v", err)
		}
		files := []*ast.File{f}
		return archFindHeaderReparses(fset, f, "synthetic.go", archNamedStructIndex(files))
	}

	const violating = `package synth

type namedHeader struct {
	AppID string ` + "`json:\"app_id\"`" + `
}

type envelope struct {
	Header struct {
		EventID   string ` + "`json:\"event_id\"`" + `
		TenantKey string ` + "`json:\"tenant_key,omitempty\"`" + `
		Custom    string ` + "`json:\"custom\"`" + `
	} ` + "`json:\"header,omitempty\"`" + `
	Named *namedHeader ` + "`json:\"header\"`" + `
	Body  struct {
		CreateTime string ` + "`json:\"create_time\"`" + `
	} ` + "`json:\"body\"`" + `
}
`
	got := parse(violating)
	gotIDs := make([]string, len(got))
	for i, v := range got {
		gotIDs[i] = v.field + ":" + v.tag
	}
	sort.Strings(gotIDs)
	wantIDs := []string{"AppID:app_id", "EventID:event_id", "TenantKey:tenant_key"}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("detector self-check: flagged %v, want exactly %v — the detector has drifted and the main gate cannot be trusted", gotIDs, wantIDs)
	}

	const clean = `package synth

type output struct {
	EventID string ` + "`json:\"event_id\"`" + `
	Header  struct {
		Custom string ` + "`json:\"custom\"`" + `
	} ` + "`json:\"header\"`" + `
}
`
	if got := parse(clean); len(got) != 0 {
		t.Fatalf("detector self-check: clean synthetic source flagged %+v — the detector over-triggers and will produce false reds", got)
	}
}
