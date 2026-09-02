// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillref

import (
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"
)

func referenceTree() fstest.MapFS {
	return fstest.MapFS{
		"lark-doc/SKILL.md":                     &fstest.MapFile{Data: []byte("canonical")},
		"lark-doc/references/fetch.md":          &fstest.MapFile{Data: []byte("canonical fetch")},
		"acme-docx/SKILL.md":                    &fstest.MapFile{Data: []byte("custom")},
		"acme-docx/references/fetch.md":         &fstest.MapFile{Data: []byte("custom fetch")},
		"acme-docx/guides/fetch-v2.md":          &fstest.MapFile{Data: []byte("custom fetch v2")},
		"acme-docx/guides/another-reference.md": &fstest.MapFile{Data: []byte("custom other")},
	}
}

func mustRef(t *testing.T, raw string) Ref {
	t.Helper()
	ref, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse(%q): %v", raw, err)
	}
	return ref
}

func TestResolverIdentityIsByteStable(t *testing.T) {
	r, err := New(referenceTree(), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, canonical := range []string{
		"lark-doc",
		"lark-doc/references/fetch.md",
	} {
		got, ok := r.ResolveString(canonical)
		if !ok || got != canonical {
			t.Errorf("ResolveString(%q) = %q, %v; want identity", canonical, got, ok)
		}
	}
}

func TestResolverWholeSkillRemapPreservesPath(t *testing.T) {
	r, err := New(referenceTree(), []Mapping{{
		From: mustRef(t, "lark-doc"),
		To:   mustRef(t, "acme-docx"),
	}})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	for canonical, want := range map[string]string{
		"lark-doc":                     "acme-docx",
		"lark-doc/references/fetch.md": "acme-docx/references/fetch.md",
	} {
		got, ok := r.ResolveString(canonical)
		if !ok || got != want {
			t.Errorf("ResolveString(%q) = %q, %v; want %q, true", canonical, got, ok, want)
		}
	}
}

func TestResolverExactRefOverridesWholeSkillRemap(t *testing.T) {
	r, err := New(referenceTree(), []Mapping{
		{From: mustRef(t, "lark-doc"), To: mustRef(t, "acme-docx")},
		{
			From: mustRef(t, "lark-doc/references/fetch.md"),
			To:   mustRef(t, "acme-docx/guides/fetch-v2.md"),
		},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	got, ok := r.ResolveString("lark-doc/references/fetch.md")
	if !ok || got != "acme-docx/guides/fetch-v2.md" {
		t.Fatalf("ResolveString = %q, %v", got, ok)
	}
}

func TestResolverUnmappedMissingReferenceIsAbsent(t *testing.T) {
	r, err := New(referenceTree(), nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got, ok := r.ResolveString("lark-doc/references/missing.md"); ok || got != "" {
		t.Fatalf("ResolveString missing = %q, %v; want absent", got, ok)
	}
}

func TestResolverExplicitTargetMustExist(t *testing.T) {
	_, err := New(referenceTree(), []Mapping{{
		From: mustRef(t, "lark-doc/references/fetch.md"),
		To:   mustRef(t, "acme-docx/guides/missing.md"),
	}})
	if !errors.Is(err, ErrInvalidRemap) {
		t.Fatalf("err = %v, want ErrInvalidRemap", err)
	}
}

type unreadableFS struct{}

func (unreadableFS) Open(string) (fs.File, error) { return nil, fs.ErrPermission }

func TestResolverExplicitTargetPreservesProbeFailure(t *testing.T) {
	_, err := New(unreadableFS{}, []Mapping{{
		From: mustRef(t, "lark-doc"),
		To:   mustRef(t, "acme-docx"),
	}})
	if !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("err = %v, want wrapped fs.ErrPermission", err)
	}
}

func TestResolverRejectsDuplicateSource(t *testing.T) {
	_, err := New(referenceTree(), []Mapping{
		{From: mustRef(t, "lark-doc"), To: mustRef(t, "acme-docx")},
		{From: mustRef(t, "lark-doc"), To: mustRef(t, "lark-doc")},
	})
	if !errors.Is(err, ErrInvalidRemap) {
		t.Fatalf("err = %v, want ErrInvalidRemap", err)
	}
}

func TestResolverRejectsWholeSkillToFile(t *testing.T) {
	_, err := New(referenceTree(), []Mapping{{
		From: mustRef(t, "lark-doc"),
		To:   mustRef(t, "acme-docx/guides/fetch-v2.md"),
	}})
	if !errors.Is(err, ErrInvalidRemap) {
		t.Fatalf("err = %v, want ErrInvalidRemap", err)
	}
}

func TestResolverDoesNotAliasCallerMappings(t *testing.T) {
	mappings := []Mapping{{
		From: mustRef(t, "lark-doc"),
		To:   mustRef(t, "acme-docx"),
	}}
	r, err := New(referenceTree(), mappings)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	mappings[0].To = mustRef(t, "lark-doc")

	got, ok := r.ResolveString("lark-doc")
	if !ok || got != "acme-docx" {
		t.Fatalf("ResolveString after caller mutation = %q, %v", got, ok)
	}
}
