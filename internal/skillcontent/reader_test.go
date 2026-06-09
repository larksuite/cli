// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillcontent

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/larksuite/cli/errs"
)

func testFS() fstest.MapFS {
	return fstest.MapFS{
		"lark-calendar/SKILL.md":             {Data: []byte("---\nname: lark-calendar\nversion: 1.0.0\ndescription: \"Calendar skill\"\nmetadata:\n  requires:\n    bins: [\"lark-cli\"]\n  cliHelp: \"lark-cli calendar --help\"\n---\nbody\n")},
		"lark-calendar/references/agenda.md": {Data: []byte("# Agenda")},
		"lark-calendar/references/create.md": {Data: []byte("# Create")},
		"lark-calendar/assets/tpl.html":      {Data: []byte("<html></html>")},
		"lark-im/SKILL.md":                   {Data: []byte("no frontmatter here\n")},
		"lark-im/references/send.md":         {Data: []byte("# Send")},
	}
}

func TestList(t *testing.T) {
	r := New(testFS())
	skills, err := r.List()
	if err != nil {
		t.Fatalf("List() error: %v", err)
	}
	if len(skills) != 2 {
		t.Fatalf("got %d skills, want 2", len(skills))
	}
	if skills[0].Name != "lark-calendar" || skills[1].Name != "lark-im" {
		t.Fatalf("skills not sorted by name: %v", skills)
	}
	if skills[0].Description != "Calendar skill" {
		t.Errorf("description: got %q, want %q", skills[0].Description, "Calendar skill")
	}
	// version is the frontmatter `version:` field, passed through for drift checks.
	if skills[0].Version != "1.0.0" {
		t.Errorf("version: got %q, want %q", skills[0].Version, "1.0.0")
	}
	// metadata is the frontmatter `metadata:` block, passed through verbatim.
	if skills[0].Metadata == nil {
		t.Fatal("expected metadata for lark-calendar")
	}
	if skills[0].Metadata["cliHelp"] != "lark-cli calendar --help" {
		t.Errorf("metadata.cliHelp: got %v", skills[0].Metadata["cliHelp"])
	}
	// No frontmatter → empty description and nil metadata (omitted from JSON).
	if skills[1].Description != "" {
		t.Errorf("lark-im description: got %q, want empty", skills[1].Description)
	}
	if skills[1].Metadata != nil {
		t.Errorf("lark-im metadata: got %v, want nil", skills[1].Metadata)
	}
	if skills[1].Version != "" {
		t.Errorf("lark-im version: got %q, want empty", skills[1].Version)
	}
}

func TestListPath(t *testing.T) {
	r := New(testFS())

	// Skill root: direct children only (one layer), each path skill-prefixed.
	entries, listed, err := r.ListPath("lark-calendar")
	if err != nil {
		t.Fatalf("ListPath root error: %v", err)
	}
	if listed != "lark-calendar" {
		t.Errorf("listed path: got %q", listed)
	}
	want := map[string]bool{ // path → isDir
		"lark-calendar/SKILL.md":   false,
		"lark-calendar/references": true,
		"lark-calendar/assets":     true,
	}
	if len(entries) != len(want) {
		t.Fatalf("root entries: got %v, want %d entries", entries, len(want))
	}
	for _, e := range entries {
		isDir, ok := want[e.Path]
		if !ok {
			t.Errorf("unexpected entry %q", e.Path)
			continue
		}
		if e.IsDir != isDir {
			t.Errorf("%q is_dir: got %v, want %v", e.Path, e.IsDir, isDir)
		}
	}
	// Entries are sorted by path.
	if entries[0].Path != "lark-calendar/SKILL.md" {
		t.Errorf("entries not sorted: %v", entries)
	}

	// Subdirectory: one layer under <name>/<subpath>.
	subEntries, subListed, err := r.ListPath("lark-calendar/references")
	if err != nil {
		t.Fatalf("ListPath subdir error: %v", err)
	}
	if subListed != "lark-calendar/references" {
		t.Errorf("listed subpath: got %q", subListed)
	}
	if len(subEntries) != 2 ||
		subEntries[0].Path != "lark-calendar/references/agenda.md" ||
		subEntries[1].Path != "lark-calendar/references/create.md" {
		t.Errorf("subdir entries: got %v", subEntries)
	}

	// Unknown skill → typed validation error.
	if _, _, err := r.ListPath("no-such-skill"); err == nil {
		t.Error("expected error for unknown skill")
	} else {
		var verr *errs.ValidationError
		if !errors.As(err, &verr) {
			t.Errorf("expected *errs.ValidationError, got %T", err)
		}
	}

	// Path that points at a file (not a dir) → validation error.
	if _, _, err := r.ListPath("lark-calendar/SKILL.md"); err == nil {
		t.Error("expected error listing a file")
	} else if !strings.Contains(err.Error(), "is a file") {
		t.Errorf("message: got %q", err.Error())
	}

	// Nonexistent subpath → validation error.
	if _, _, err := r.ListPath("lark-calendar/nope"); err == nil {
		t.Error("expected not-found error")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Errorf("message: got %q", err.Error())
	}

	// Traversal in the subpath is rejected, no listing leaked.
	for _, bad := range []string{"lark-calendar/../lark-im", "lark-calendar/../../etc", `lark-calendar/..\x`} {
		entries, _, err := r.ListPath(bad)
		if err == nil {
			t.Errorf("expected rejection for %q", bad)
		}
		if entries != nil {
			t.Errorf("entries leaked for %q: %v", bad, entries)
		}
	}
}

func TestReadSkill(t *testing.T) {
	r := New(testFS())

	data, err := r.ReadSkill("lark-calendar")
	if err != nil {
		t.Fatalf("ReadSkill error: %v", err)
	}
	if !strings.HasPrefix(string(data), "---\nname: lark-calendar") {
		t.Errorf("unexpected content: %q", string(data))
	}

	_, err = r.ReadSkill("no-such-skill")
	if err == nil {
		t.Fatal("expected error for unknown skill")
	}
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *errs.ValidationError, got %T", err)
	}
	if !strings.Contains(verr.Message, `unknown skill "no-such-skill"`) {
		t.Errorf("message: got %q", verr.Message)
	}

	if _, err := r.ReadSkill("../etc"); err == nil {
		t.Error("expected error for name with separator")
	}
}

func TestReadReference(t *testing.T) {
	r := New(testFS())

	data, cleaned, err := r.ReadReference("lark-calendar", "references/agenda.md")
	if err != nil {
		t.Fatalf("ReadReference error: %v", err)
	}
	if string(data) != "# Agenda" {
		t.Errorf("content: got %q", string(data))
	}
	if cleaned != "references/agenda.md" {
		t.Errorf("cleaned path: got %q", cleaned)
	}

	if _, _, err := r.ReadReference("lark-calendar", "references/nope.md"); err == nil {
		t.Error("expected not-found error")
	} else if !strings.Contains(err.Error(), "not found") {
		t.Errorf("message: got %q", err.Error())
	}

	if _, _, err := r.ReadReference("lark-calendar", "references"); err == nil {
		t.Error("expected directory error")
	} else if !strings.Contains(err.Error(), "is a directory") {
		t.Errorf("message: got %q", err.Error())
	}

	for _, bad := range []string{"../../etc/passwd", "/etc/passwd", "..", "", "references/../../im/SKILL.md", `..\..\x`} {
		data, _, err := r.ReadReference("lark-calendar", bad)
		if err == nil {
			t.Errorf("expected rejection for %q", bad)
		}
		if data != nil {
			t.Errorf("content leaked for %q: %q", bad, string(data))
		}
		var verr *errs.ValidationError
		if !errors.As(err, &verr) {
			t.Errorf("expected validation error for %q, got %T", bad, err)
		}
	}
}

func TestParseFrontmatter(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		wantDesc    string
		wantVer     string
		wantHasMeta bool
	}{
		{
			name:        "description, version and metadata",
			input:       "---\ndescription: My skill\nversion: 2.1.0\nmetadata:\n  cliHelp: \"x\"\n---\nbody\n",
			wantDesc:    "My skill",
			wantVer:     "2.1.0",
			wantHasMeta: true,
		},
		{
			name:     "description only, no metadata",
			input:    "---\ndescription: Plain\n---\nbody\n",
			wantDesc: "Plain",
		},
		{
			name:  "no frontmatter",
			input: "no frontmatter here\n",
		},
		{
			name:  "unclosed frontmatter",
			input: "---\ndescription: Never closed\n",
		},
		{
			name:  "malformed YAML inside frontmatter",
			input: "---\n: bad: yaml: [\n---\nbody\n",
		},
		{
			name:        "CRLF line endings",
			input:       "---\r\ndescription: CRLF skill\r\nmetadata:\r\n  cliHelp: \"y\"\r\n---\r\nbody\r\n",
			wantDesc:    "CRLF skill",
			wantHasMeta: true,
		},
		{
			name:  "empty input",
			input: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			desc, ver, meta := parseFrontmatter([]byte(tc.input))
			if desc != tc.wantDesc {
				t.Errorf("description = %q, want %q", desc, tc.wantDesc)
			}
			if ver != tc.wantVer {
				t.Errorf("version = %q, want %q", ver, tc.wantVer)
			}
			if (meta != nil) != tc.wantHasMeta {
				t.Errorf("metadata = %v, wantHasMeta %v", meta, tc.wantHasMeta)
			}
		})
	}
}

func TestReadSkillMissingFile(t *testing.T) {
	// Use a separate MapFS so testFS() (and TestList) are unaffected.
	emptyFS := fstest.MapFS{
		"lark-empty/references/x.md": {Data: []byte("# X")},
	}
	r := New(emptyFS)
	_, err := r.ReadSkill("lark-empty")
	if err == nil {
		t.Fatal("expected error when SKILL.md is absent")
	}
	var ierr *errs.InternalError
	if !errors.As(err, &ierr) {
		t.Fatalf("expected *errs.InternalError, got %T: %v", err, err)
	}
}
