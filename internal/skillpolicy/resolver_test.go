// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package skillpolicy

import (
	"errors"
	"io/fs"
	"slices"
	"sort"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/skillref"
)

// skillFS builds an in-memory skill tree; each map entry is a path -> content.
func skillFS(files map[string]string) fstest.MapFS {
	m := fstest.MapFS{}
	for p, content := range files {
		m[p] = &fstest.MapFile{Data: []byte(content)}
	}
	return m
}

// baseTree is a representative base: three skills, one with a reference file.
func baseTree() fstest.MapFS {
	return skillFS(map[string]string{
		"lark-a/SKILL.md":        "base a",
		"lark-a/references/x.md": "base a ref",
		"lark-b/SKILL.md":        "base b",
		"lark-shared/SKILL.md":   "base shared",
	})
}

func baseTreeWithRequiredShared() fstest.MapFS {
	return skillFS(map[string]string{
		"lark-a/SKILL.md":      "---\nmetadata:\n  requires:\n    skills: [\"lark-shared\"]\n---\nbase a",
		"lark-b/SKILL.md":      "base b",
		"lark-shared/SKILL.md": "base shared",
	})
}

// topLevel returns the sorted top-level skill names of fsys.
func topLevel(t *testing.T, fsys fs.FS) []string {
	t.Helper()
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		t.Fatalf("ReadDir(.): %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

func readFile(t *testing.T, fsys fs.FS, name string) string {
	t.Helper()
	data, err := fs.ReadFile(fsys, name)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", name, err)
	}
	return string(data)
}

func mustResolve(t *testing.T, base fs.FS, spec *platform.SkillsOverlay) fs.FS {
	t.Helper()
	got, err := resolveContent(base, []PluginSkill{{PluginName: "acme", SkillsOverlay: spec}})
	if err != nil {
		t.Fatalf("Resolve: unexpected error: %v", err)
	}
	return got
}

func TestResolve_NoSpecs_ReturnsBaseUnchanged(t *testing.T) {
	base := baseTree()
	got, err := resolveContent(base, nil)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	gotNames := topLevel(t, got)
	if want := []string{"lark-a", "lark-b", "lark-shared"}; !slices.Equal(gotNames, want) {
		t.Errorf("top level = %v, want %v", gotNames, want)
	}
}

func TestResolve_Remove_HidesSkill(t *testing.T) {
	got := mustResolve(t, baseTree(), &platform.SkillsOverlay{Remove: []string{"lark-shared"}})

	gotNames := topLevel(t, got)
	if want := []string{"lark-a", "lark-b"}; !slices.Equal(gotNames, want) {
		t.Errorf("top level = %v, want %v", gotNames, want)
	}
	// The removed skill is gone for the affordance reader too (Stat gates it).
	if _, err := fs.Stat(got, "lark-shared/SKILL.md"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("Stat removed skill: err = %v, want ErrNotExist", err)
	}
	// Surviving skills still read from base.
	if c := readFile(t, got, "lark-a/SKILL.md"); c != "base a" {
		t.Errorf("lark-a content = %q, want %q", c, "base a")
	}
}

func TestResolve_Overlay_AddsNewSkill(t *testing.T) {
	overlay := skillFS(map[string]string{"lark-new/SKILL.md": "new skill"})
	got := mustResolve(t, baseTree(), &platform.SkillsOverlay{Overlay: overlay})

	gotNames := topLevel(t, got)
	if want := []string{"lark-a", "lark-b", "lark-new", "lark-shared"}; !slices.Equal(gotNames, want) {
		t.Errorf("top level = %v, want %v", gotNames, want)
	}
	if c := readFile(t, got, "lark-new/SKILL.md"); c != "new skill" {
		t.Errorf("lark-new content = %q, want %q", c, "new skill")
	}
}

func TestResolve_Overlay_ReplacesSameNameWholeSkill(t *testing.T) {
	overlay := skillFS(map[string]string{"lark-a/SKILL.md": "overlaid a"})
	got := mustResolve(t, baseTree(), &platform.SkillsOverlay{Overlay: overlay})

	// Same-named skill resolves to the overlay (upper wins).
	if c := readFile(t, got, "lark-a/SKILL.md"); c != "overlaid a" {
		t.Errorf("lark-a content = %q, want overlaid", c)
	}
	// Replacement is whole-skill: the base's reference file is shadowed, not merged.
	if _, err := fs.Stat(got, "lark-a/references/x.md"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("base reference should be shadowed by overlay skill; err = %v, want ErrNotExist", err)
	}
}

func TestResolve_Base_ReplacesEntireTree(t *testing.T) {
	replacement := skillFS(map[string]string{"lark-only/SKILL.md": "only"})
	got := mustResolve(t, baseTree(), &platform.SkillsOverlay{Base: replacement})

	gotNames := topLevel(t, got)
	if want := []string{"lark-only"}; !slices.Equal(gotNames, want) {
		t.Errorf("top level = %v, want %v", gotNames, want)
	}
	if _, err := fs.Stat(got, "lark-a/SKILL.md"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("base skill should be gone after Base replace; err = %v", err)
	}
}

func TestResolve_Base_WithRemoveAndOverlay(t *testing.T) {
	replacement := skillFS(map[string]string{
		"lark-p/SKILL.md": "p",
		"lark-q/SKILL.md": "q",
	})
	overlay := skillFS(map[string]string{"lark-r/SKILL.md": "r"})
	got := mustResolve(t, baseTree(), &platform.SkillsOverlay{
		Base:    replacement,
		Remove:  []string{"lark-q"},
		Overlay: overlay,
	})
	gotNames := topLevel(t, got)
	if want := []string{"lark-p", "lark-r"}; !slices.Equal(gotNames, want) {
		t.Errorf("top level = %v, want %v", gotNames, want)
	}
}

func TestResolveWithReferences_DefaultIdentity(t *testing.T) {
	resolved, err := ResolveWithReferences(baseTree(), nil)
	if err != nil {
		t.Fatalf("ResolveWithReferences: %v", err)
	}
	for _, canonical := range []string{
		"lark-a",
		"lark-a/references/x.md",
	} {
		got, ok := resolved.References.ResolveString(canonical)
		if !ok || got != canonical {
			t.Errorf("reference %q = %q, %v; want byte-identical identity", canonical, got, ok)
		}
	}
}

func TestResolveWithReferences_WholeSkillAndExactPathRemap(t *testing.T) {
	replacement := skillFS(map[string]string{
		"acme-a/SKILL.md":        "custom a",
		"acme-a/references/x.md": "custom ref",
		"acme-a/guides/x.md":     "custom exact ref",
	})
	resolved, err := ResolveWithReferences(baseTree(), []PluginSkill{{
		PluginName: "acme",
		SkillsOverlay: &platform.SkillsOverlay{
			Base: replacement,
			ReferenceRemaps: []platform.SkillRefRemap{
				platform.RemapSkillRef("lark-a", "acme-a"),
				platform.RemapSkillRef("lark-a/references/x.md", "acme-a/guides/x.md"),
			},
		},
	}})
	if err != nil {
		t.Fatalf("ResolveWithReferences: %v", err)
	}
	for canonical, want := range map[string]string{
		"lark-a":                 "acme-a",
		"lark-a/references/x.md": "acme-a/guides/x.md",
	} {
		got, ok := resolved.References.ResolveString(canonical)
		if !ok || got != want {
			t.Errorf("reference %q = %q, %v; want %q, true", canonical, got, ok, want)
		}
	}
}

func TestResolveWithReferences_UnmappedRemovedRefIsAbsent(t *testing.T) {
	resolved, err := ResolveWithReferences(baseTree(), []PluginSkill{{
		PluginName:    "acme",
		SkillsOverlay: &platform.SkillsOverlay{Remove: []string{"lark-a"}},
	}})
	if err != nil {
		t.Fatalf("ResolveWithReferences: %v", err)
	}
	if got, ok := resolved.References.ResolveString("lark-a/references/x.md"); ok || got != "" {
		t.Fatalf("removed reference = %q, %v; want absent", got, ok)
	}
}

func TestResolveWithReferences_DanglingExplicitRemapFails(t *testing.T) {
	_, err := ResolveWithReferences(baseTree(), []PluginSkill{{
		PluginName: "acme",
		SkillsOverlay: &platform.SkillsOverlay{
			ReferenceRemaps: []platform.SkillRefRemap{
				platform.RemapSkillRef("lark-a", "acme-missing"),
			},
		},
	}})
	if !errors.Is(err, skillref.ErrInvalidRemap) {
		t.Fatalf("err = %v, want ErrInvalidRemap", err)
	}
}

func TestResolveWithReferences_InvalidAndDuplicateRemapsFail(t *testing.T) {
	for _, tc := range []struct {
		name   string
		remaps []platform.SkillRefRemap
	}{
		{
			name: "invalid source",
			remaps: []platform.SkillRefRemap{
				platform.RemapSkillRef("../lark-a", "lark-a"),
			},
		},
		{
			name: "invalid target",
			remaps: []platform.SkillRefRemap{
				platform.RemapSkillRef("lark-a", `bad\target`),
			},
		},
		{
			name: "duplicate source",
			remaps: []platform.SkillRefRemap{
				platform.RemapSkillRef("lark-a", "lark-a"),
				platform.RemapSkillRef("lark-a", "lark-b"),
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ResolveWithReferences(baseTree(), []PluginSkill{{
				PluginName:    "acme",
				SkillsOverlay: &platform.SkillsOverlay{ReferenceRemaps: tc.remaps},
			}})
			if !errors.Is(err, skillref.ErrInvalidRemap) {
				t.Fatalf("err = %v, want ErrInvalidRemap", err)
			}
		})
	}
}

// Allow keeps only the listed base skills — the allow-list counterpart
// of Rule.Allow, so a CLI upgrade adding new embedded skills cannot
// widen an allow-listed build.
func TestResolve_Allow_KeepsOnlyListed(t *testing.T) {
	got := mustResolve(t, baseTree(), &platform.SkillsOverlay{Allow: []string{"lark-a"}})

	gotNames := topLevel(t, got)
	if want := []string{"lark-a"}; !slices.Equal(gotNames, want) {
		t.Errorf("top level = %v, want %v", gotNames, want)
	}
	if _, err := fs.Stat(got, "lark-b/SKILL.md"); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("non-allow-listed skill must be absent; err = %v", err)
	}
	// Kept skills still read from base, references included.
	if c := readFile(t, got, "lark-a/references/x.md"); c != "base a ref" {
		t.Errorf("kept skill content = %q, want base content", c)
	}
}

func TestResolve_AllowMissingRequiredSkillFailsClosed(t *testing.T) {
	_, err := resolveContent(baseTreeWithRequiredShared(), []PluginSkill{{
		PluginName: "acme",
		SkillsOverlay: &platform.SkillsOverlay{
			Allow: []string{"lark-a"},
		},
	}})
	if !errors.Is(err, ErrUnsatisfiedSkillDependency) {
		t.Fatalf("err = %v, want ErrUnsatisfiedSkillDependency", err)
	}
	for _, want := range []string{"lark-a", "lark-shared"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not identify %q", err, want)
		}
	}
}

func TestResolve_AllowIncludingRequiredSkillSucceeds(t *testing.T) {
	got := mustResolve(t, baseTreeWithRequiredShared(), &platform.SkillsOverlay{
		Allow: []string{"lark-a", "lark-shared"},
	})
	if want := []string{"lark-a", "lark-shared"}; !slices.Equal(topLevel(t, got), want) {
		t.Fatalf("top level = %v, want %v", topLevel(t, got), want)
	}
}

func TestResolve_UTF8BOMFrontmatterRequiredSkillPresentSucceeds(t *testing.T) {
	base := skillFS(map[string]string{
		"lark-a/SKILL.md":      "\uFEFF---\nmetadata:\n  requires:\n    skills: [\"lark-shared\"]\n---\nbase a",
		"lark-shared/SKILL.md": "base shared",
	})

	got := mustResolve(t, base, &platform.SkillsOverlay{
		Allow: []string{"lark-a", "lark-shared"},
	})
	if want := []string{"lark-a", "lark-shared"}; !slices.Equal(topLevel(t, got), want) {
		t.Fatalf("top level = %v, want %v", topLevel(t, got), want)
	}
}

func TestResolve_UTF8BOMFrontmatterMissingRequiredSkillFailsClosed(t *testing.T) {
	base := skillFS(map[string]string{
		"lark-a/SKILL.md":      "\uFEFF---\nmetadata:\n  requires:\n    skills: [\"lark-shared\"]\n---\nbase a",
		"lark-shared/SKILL.md": "base shared",
	})

	_, err := resolveContent(base, []PluginSkill{{
		PluginName: "acme",
		SkillsOverlay: &platform.SkillsOverlay{
			Allow: []string{"lark-a"},
		},
	}})
	if !errors.Is(err, ErrUnsatisfiedSkillDependency) {
		t.Fatalf("err = %v, want ErrUnsatisfiedSkillDependency", err)
	}
	for _, want := range []string{"lark-a", "lark-shared"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not identify %q", err, want)
		}
	}
}

func TestResolve_RemoveRequiredSkillFailsClosed(t *testing.T) {
	_, err := resolveContent(baseTreeWithRequiredShared(), []PluginSkill{{
		PluginName: "acme",
		SkillsOverlay: &platform.SkillsOverlay{
			Remove: []string{"lark-shared"},
		},
	}})
	if !errors.Is(err, ErrUnsatisfiedSkillDependency) {
		t.Fatalf("err = %v, want ErrUnsatisfiedSkillDependency", err)
	}
}

func TestResolve_OverlayReplacementUsesReplacementDependencies(t *testing.T) {
	overlay := skillFS(map[string]string{
		"lark-a/SKILL.md": "replacement a without dependencies",
	})
	got := mustResolve(t, baseTreeWithRequiredShared(), &platform.SkillsOverlay{
		Allow:   []string{"lark-a"},
		Overlay: overlay,
	})
	if want := []string{"lark-a"}; !slices.Equal(topLevel(t, got), want) {
		t.Fatalf("top level = %v, want %v", topLevel(t, got), want)
	}
}

func TestResolve_OverlayReplacementMissingOwnDependencyFailsClosed(t *testing.T) {
	overlay := skillFS(map[string]string{
		"lark-a/SKILL.md": "---\nmetadata:\n  requires:\n    skills: [\"acme-runtime\"]\n---\nreplacement a",
	})
	_, err := resolveContent(baseTreeWithRequiredShared(), []PluginSkill{{
		PluginName: "acme",
		SkillsOverlay: &platform.SkillsOverlay{
			Allow:   []string{"lark-a"},
			Overlay: overlay,
		},
	}})
	if !errors.Is(err, ErrUnsatisfiedSkillDependency) {
		t.Fatalf("err = %v, want ErrUnsatisfiedSkillDependency", err)
	}
	if !strings.Contains(err.Error(), "acme-runtime") {
		t.Fatalf("error does not identify replacement dependency: %v", err)
	}
}

func TestResolve_DoesNotInferDependenciesFromMarkdownLinks(t *testing.T) {
	base := skillFS(map[string]string{
		"lark-a/SKILL.md": "Read [missing](../lark-missing/SKILL.md) when useful.",
	})
	got := mustResolve(t, base, &platform.SkillsOverlay{Allow: []string{"lark-a"}})
	if want := []string{"lark-a"}; !slices.Equal(topLevel(t, got), want) {
		t.Fatalf("top level = %v, want %v", topLevel(t, got), want)
	}
}

func TestResolve_UnclosedFrontmatterFailsClosed(t *testing.T) {
	base := skillFS(map[string]string{
		"lark-a/SKILL.md": "---\nmetadata:\n  requires:\n    skills: [\"lark-shared\"]\nbase a",
	})
	_, err := resolveContent(base, []PluginSkill{{
		PluginName:    "acme",
		SkillsOverlay: &platform.SkillsOverlay{Allow: []string{"lark-a"}},
	}})
	if !errors.Is(err, ErrInvalidHostBase) {
		t.Fatalf("err = %v, want ErrInvalidHostBase", err)
	}
	for _, want := range []string{"lark-a", "invalid metadata", "frontmatter is not closed"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not identify %q", err, want)
		}
	}
}

func TestResolve_InvalidRequiredSkillNameFailsClosed(t *testing.T) {
	base := skillFS(map[string]string{
		"lark-a/SKILL.md": "---\nmetadata:\n  requires:\n    skills: [\"../escape\"]\n---\nbase a",
	})
	_, err := resolveContent(base, []PluginSkill{{
		PluginName:    "acme",
		SkillsOverlay: &platform.SkillsOverlay{Allow: []string{"lark-a"}},
	}})
	if !errors.Is(err, ErrInvalidHostBase) {
		t.Fatalf("err = %v, want ErrInvalidHostBase", err)
	}
	for _, want := range []string{"lark-a", "invalid metadata", "../escape"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not identify %q", err, want)
		}
	}
}

// Remove wins over Allow, mirroring Rule's Deny-over-Allow.
func TestResolve_RemoveWinsOverAllow(t *testing.T) {
	got := mustResolve(t, baseTree(), &platform.SkillsOverlay{
		Allow:  []string{"lark-a", "lark-b"},
		Remove: []string{"lark-b"},
	})
	gotNames := topLevel(t, got)
	if want := []string{"lark-a"}; !slices.Equal(gotNames, want) {
		t.Errorf("top level = %v, want %v", gotNames, want)
	}
}

// Overlay entries are exempt from Allow: content the integrator
// explicitly ships needs no allow-listing.
func TestResolve_OverlayExemptFromAllow(t *testing.T) {
	overlay := skillFS(map[string]string{"acme-guide/SKILL.md": "mine"})
	got := mustResolve(t, baseTree(), &platform.SkillsOverlay{
		Allow:   []string{"lark-a"},
		Overlay: overlay,
	})
	gotNames := topLevel(t, got)
	if want := []string{"acme-guide", "lark-a"}; !slices.Equal(gotNames, want) {
		t.Errorf("top level = %v, want %v", gotNames, want)
	}
}

// An Allow name absent from the base aborts startup, same as Remove.
func TestResolve_AllowUnknown_Errors(t *testing.T) {
	_, err := resolveContent(baseTree(), []PluginSkill{{PluginName: "acme", SkillsOverlay: &platform.SkillsOverlay{Allow: []string{"lark-nope"}}}})
	if err == nil {
		t.Fatal("expected error allow-listing a skill absent from base")
	}
}

func TestResolve_RemoveUnknown_Errors(t *testing.T) {
	_, err := resolveContent(baseTree(), []PluginSkill{{PluginName: "acme", SkillsOverlay: &platform.SkillsOverlay{Remove: []string{"lark-nope"}}}})
	if err == nil {
		t.Fatal("expected error removing a skill absent from base")
	}
}

func TestResolve_SelectionWithoutBaseReportsMissingContent(t *testing.T) {
	for _, tc := range []struct {
		name string
		base fs.FS
		spec *platform.SkillsOverlay
	}{
		{name: "nil base allow", spec: &platform.SkillsOverlay{Allow: []string{"lark-a"}}},
		{name: "nil base remove", spec: &platform.SkillsOverlay{Remove: []string{"lark-a"}}},
		{name: "empty base allow", base: fstest.MapFS{}, spec: &platform.SkillsOverlay{Allow: []string{"lark-a"}}},
		{name: "empty replacement base remove", base: baseTree(), spec: &platform.SkillsOverlay{
			Base:   fstest.MapFS{},
			Remove: []string{"lark-a"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveContent(tc.base, []PluginSkill{{PluginName: "acme", SkillsOverlay: tc.spec}})
			if !errors.Is(err, ErrNoBaseSkillContent) {
				t.Fatalf("err = %v, want ErrNoBaseSkillContent", err)
			}
		})
	}
}

func TestResolve_RemoveInvalidName_Errors(t *testing.T) {
	for _, bad := range []string{"", ".", "..", "a/b", `a\b`} {
		if _, err := resolveContent(baseTree(), []PluginSkill{{PluginName: "acme", SkillsOverlay: &platform.SkillsOverlay{Remove: []string{bad}}}}); err == nil {
			t.Errorf("Remove %q: expected error, got nil", bad)
		}
	}
}

func TestResolve_OverlayMissingSKILLmd_Errors(t *testing.T) {
	overlay := skillFS(map[string]string{"lark-bad/other.md": "no skill.md here"})
	_, err := resolveContent(baseTree(), []PluginSkill{{PluginName: "acme", SkillsOverlay: &platform.SkillsOverlay{Overlay: overlay}}})
	if err == nil {
		t.Fatal("expected error for overlay entry missing SKILL.md")
	}
}

func TestResolve_OverlayNonDirEntry_Errors(t *testing.T) {
	overlay := skillFS(map[string]string{"loose.md": "top-level file, not a skill dir"})
	_, err := resolveContent(baseTree(), []PluginSkill{{PluginName: "acme", SkillsOverlay: &platform.SkillsOverlay{Overlay: overlay}}})
	if err == nil {
		t.Fatal("expected error for non-directory overlay entry")
	}
}

func TestResolve_TwoOwners_Errors(t *testing.T) {
	_, err := resolveContent(baseTree(), []PluginSkill{
		{PluginName: "acme", SkillsOverlay: &platform.SkillsOverlay{Remove: []string{"lark-a"}}},
		{PluginName: "globex", SkillsOverlay: &platform.SkillsOverlay{Remove: []string{"lark-b"}}},
	})
	if !errors.Is(err, ErrMultipleSkillsOverlays) {
		t.Fatalf("err = %v, want ErrMultipleSkillsOverlays", err)
	}
}

// TestResolve_ComposedConformsToFS runs the composed tree through the
// standard io/fs conformance checker to catch Open/ReadDir/Stat drift.
func TestResolve_ComposedConformsToFS(t *testing.T) {
	overlay := skillFS(map[string]string{
		"lark-new/SKILL.md":        "new",
		"lark-new/references/y.md": "new ref",
	})
	got := mustResolve(t, baseTree(), &platform.SkillsOverlay{
		Remove:  []string{"lark-shared"},
		Overlay: overlay,
	})
	if err := fstest.TestFS(got,
		"lark-a/SKILL.md",
		"lark-a/references/x.md",
		"lark-b/SKILL.md",
		"lark-new/SKILL.md",
		"lark-new/references/y.md",
	); err != nil {
		t.Fatalf("composed FS failed fs conformance: %v", err)
	}
}

// brokenFS fails every Open, standing in for an integrator Base that points at
// a missing or unreadable tree.
type brokenFS struct{}

func (brokenFS) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }

// A custom Base whose root cannot be read must fail at Resolve (startup), not
// surface later as an empty `skills list`.
func TestResolve_unreadableBaseFailsFast(t *testing.T) {
	_, err := resolveContent(baseTree(), []PluginSkill{{
		PluginName:    "acme",
		SkillsOverlay: &platform.SkillsOverlay{Base: brokenFS{}},
	}})
	if err == nil || !strings.Contains(err.Error(), "Base") {
		t.Fatalf("unreadable Base must fail fast with a named error, got %v", err)
	}
}

// Overlay top-level directories are skill names and must pass the same name
// rule as Allow/Remove; a name the reader can never serve (backslash and
// friends) fails composition instead of being listed but unreadable.
func TestResolve_overlayInvalidSkillNameFailsFast(t *testing.T) {
	overlay := skillFS(map[string]string{`bad\name/SKILL.md`: "x"})
	_, err := resolveContent(baseTree(), []PluginSkill{{
		PluginName:    "acme",
		SkillsOverlay: &platform.SkillsOverlay{Overlay: overlay},
	}})
	if err == nil || !strings.Contains(err.Error(), "not a valid skill name") {
		t.Fatalf("invalid overlay skill name must fail fast, got %v", err)
	}
}

// The allow-list is applied to the resolve-time manifest: a skill added to
// the base FS after composition stays invisible to both list and read.
func TestResolve_allowGatesSkillsAddedAfterResolve(t *testing.T) {
	base := baseTree()
	got, err := resolveContent(base, []PluginSkill{{
		PluginName:    "acme",
		SkillsOverlay: &platform.SkillsOverlay{Allow: []string{"lark-a"}},
	}})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if want := []string{"lark-a"}; !slices.Equal(topLevel(t, got), want) {
		t.Fatalf("top level = %v, want %v", topLevel(t, got), want)
	}

	// Mutate the base afterwards, as a CLI upgrade (or a misbehaving
	// integrator FS) would.
	base["lark-late/SKILL.md"] = &fstest.MapFile{Data: []byte("late")}

	if names := topLevel(t, got); !slices.Equal(names, []string{"lark-a"}) {
		t.Errorf("late-added skill leaked into the listing: %v", names)
	}
	if _, err := fs.ReadFile(got, "lark-late/SKILL.md"); err == nil {
		t.Error("late-added skill must not be readable through the allow gate")
	}
}

// The manifest makes list and read agree under a mutated FS: names route by
// the composition-time snapshot, so a top-level skill added to EITHER tree
// afterwards neither lists nor reads, and ownership never flips.
func TestResolve_manifestKeepsListAndReadConsistent(t *testing.T) {
	base := baseTree()
	overlay := skillFS(map[string]string{"lark-a/SKILL.md": "upper a"})
	got, err := resolveContent(base, []PluginSkill{{
		PluginName:    "acme",
		SkillsOverlay: &platform.SkillsOverlay{Overlay: overlay},
	}})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	// Late additions to both trees stay invisible to list AND read.
	base["lark-late/SKILL.md"] = &fstest.MapFile{Data: []byte("late lower")}
	overlay["lark-upper-late/SKILL.md"] = &fstest.MapFile{Data: []byte("late upper")}
	names := topLevel(t, got)
	for _, n := range names {
		if n == "lark-late" || n == "lark-upper-late" {
			t.Errorf("late-added skill %q leaked into listing", n)
		}
	}
	if _, err := fs.ReadFile(got, "lark-late/SKILL.md"); err == nil {
		t.Error("late-added lower skill must not be readable")
	}
	if _, err := fs.ReadFile(got, "lark-upper-late/SKILL.md"); err == nil {
		t.Error("late-added upper skill must not be readable")
	}

	// Ownership stays with the snapshot: lark-a keeps reading from upper.
	if body := readFile(t, got, "lark-a/SKILL.md"); body != "upper a" {
		t.Errorf("lark-a = %q, want the overlay copy", body)
	}

	// A late upper entry cannot steal a name the lower snapshot already owns.
	overlay["lark-b/SKILL.md"] = &fstest.MapFile{Data: []byte("late upper b")}
	if body := readFile(t, got, "lark-b/SKILL.md"); body != "base b" {
		t.Errorf("lark-b = %q, want the original lower owner", body)
	}

	// If the snapshotted upper owner disappears, reads fail from that owner;
	// they must not silently fall back to a same-named lower skill.
	delete(overlay, "lark-a/SKILL.md")
	if body, err := fs.ReadFile(got, "lark-a/SKILL.md"); err == nil {
		t.Errorf("deleted upper owner fell back to lower content %q", body)
	}
}

type oneShotRootFS struct {
	fs.FS
	rootReads int
}

func (f *oneShotRootFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == "." {
		f.rootReads++
		if f.rootReads > 1 {
			return nil, errors.New("root directory read more than once")
		}
	}
	return fs.ReadDir(f.FS, name)
}

// Validation and manifest construction must consume the same root snapshot.
// A mutable FS may change between reads, so Resolve reads each tree's root
// exactly once and routes all later list/read operations through that snapshot.
func TestResolve_scansEachSkillTreeRootOnce(t *testing.T) {
	base := &oneShotRootFS{FS: baseTree()}
	overlay := &oneShotRootFS{FS: skillFS(map[string]string{
		"lark-new/SKILL.md": "new",
	})}

	got, err := resolveContent(base, []PluginSkill{{
		PluginName: "acme",
		SkillsOverlay: &platform.SkillsOverlay{
			Remove:  []string{"lark-shared"},
			Overlay: overlay,
		},
	}})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if base.rootReads != 1 || overlay.rootReads != 1 {
		t.Fatalf("root reads = base:%d overlay:%d, want one each", base.rootReads, overlay.rootReads)
	}
	names := topLevel(t, got)
	if want := []string{"lark-a", "lark-b", "lark-new"}; !slices.Equal(names, want) {
		t.Fatalf("top level = %v, want %v", names, want)
	}
	if body := readFile(t, got, "lark-new/SKILL.md"); body != "new" {
		t.Fatalf("lark-new = %q, want overlay content", body)
	}
}

type mutableDirEntry struct {
	fs.DirEntry
	name string
}

func (e *mutableDirEntry) Name() string { return e.name }

type aliasedRootFS struct {
	fs.FS
	entries []fs.DirEntry
}

func (f *aliasedRootFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if name == "." {
		return f.entries, nil
	}
	return fs.ReadDir(f.FS, name)
}

// Resolve must freeze the validated entry names, not retain a custom
// ReadDirFS's aliased DirEntry objects. Otherwise a plugin could mutate the
// listing after validation while routing still uses the old owner key.
func TestResolve_freezesAliasedDirEntries(t *testing.T) {
	base := baseTree()
	rootEntries, err := fs.ReadDir(base, ".")
	if err != nil {
		t.Fatalf("ReadDir(base): %v", err)
	}
	aliased := make([]fs.DirEntry, len(rootEntries))
	mutable := make([]*mutableDirEntry, len(rootEntries))
	for i, entry := range rootEntries {
		mutable[i] = &mutableDirEntry{DirEntry: entry, name: entry.Name()}
		aliased[i] = mutable[i]
	}
	source := &aliasedRootFS{FS: base, entries: aliased}
	got := mustResolve(t, source, &platform.SkillsOverlay{})

	mutable[0].name = "changed-after-validation"

	if want := []string{"lark-a", "lark-b", "lark-shared"}; !slices.Equal(topLevel(t, got), want) {
		t.Fatalf("top level changed through aliased DirEntry: got %v want %v", topLevel(t, got), want)
	}
	if body := readFile(t, got, "lark-a/SKILL.md"); body != "base a" {
		t.Fatalf("snapshotted owner stopped routing: got %q", body)
	}
}

// A replacement Base is held to the same shape rules as Overlay: stray files,
// invalid names, or a directory without SKILL.md fail at Resolve.
func TestResolve_baseValidatedLikeOverlay(t *testing.T) {
	cases := []struct {
		name string
		base fstest.MapFS
		want string
	}{
		{"stray file", skillFS(map[string]string{"README.md": "x"}), "not a directory"},
		{"missing SKILL.md", fstest.MapFS{"lark-a/notes.txt": &fstest.MapFile{Data: []byte("x")}}, "missing SKILL.md"},
		{"invalid name", skillFS(map[string]string{`bad\name/SKILL.md`: "x"}), "not a valid skill name"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolveContent(baseTree(), []PluginSkill{{
				PluginName:    "acme",
				SkillsOverlay: &platform.SkillsOverlay{Base: tc.base},
			}})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want error containing %q, got %v", tc.want, err)
			}
		})
	}
}

func TestResolve_distinguishesHostBaseFromPluginReplacementBase(t *testing.T) {
	invalid := skillFS(map[string]string{"README.md": "not a skill directory"})

	_, hostErr := resolveContent(invalid, []PluginSkill{{
		PluginName:    "acme",
		SkillsOverlay: &platform.SkillsOverlay{},
	}})
	if !errors.Is(hostErr, ErrInvalidHostBase) ||
		strings.Contains(hostErr.Error(), `plugin "acme" skill spec`) {
		t.Fatalf("host base error attributed to plugin: %v", hostErr)
	}

	_, pluginErr := resolveContent(baseTree(), []PluginSkill{{
		PluginName: "acme",
		SkillsOverlay: &platform.SkillsOverlay{
			Base: invalid,
		},
	}})
	if errors.Is(pluginErr, ErrInvalidHostBase) ||
		!strings.Contains(pluginErr.Error(), `plugin "acme" skill spec`) {
		t.Fatalf("replacement Base error lost plugin ownership: %v", pluginErr)
	}
}

type panicSkillFS struct {
	fs.FS
	panicOp string
}

func (f *panicSkillFS) Open(name string) (fs.File, error) {
	if f.panicOp == "open" {
		panic("open exploded")
	}
	return f.FS.Open(name)
}

func (f *panicSkillFS) Stat(name string) (fs.FileInfo, error) {
	if f.panicOp == "stat" {
		panic("stat exploded")
	}
	return fs.Stat(f.FS, name)
}

func (f *panicSkillFS) ReadFile(name string) ([]byte, error) {
	if f.panicOp == "readfile" {
		panic("readfile exploded")
	}
	return fs.ReadFile(f.FS, name)
}

func (f *panicSkillFS) ReadDir(name string) ([]fs.DirEntry, error) {
	if f.panicOp == "readdir" {
		panic("readdir exploded")
	}
	return fs.ReadDir(f.FS, name)
}

func TestResolve_recoversPluginFSPanicDuringComposition(t *testing.T) {
	for _, field := range []string{"Base", "Overlay"} {
		t.Run(field, func(t *testing.T) {
			source := &panicSkillFS{FS: baseTree(), panicOp: "readdir"}
			spec := &platform.SkillsOverlay{}
			if field == "Base" {
				spec.Base = source
			} else {
				spec.Overlay = source
			}

			_, err := resolveContent(baseTree(), []PluginSkill{{
				PluginName:    "acme",
				SkillsOverlay: spec,
			}})
			if err == nil {
				t.Fatal("Resolve succeeded after plugin filesystem panic")
			}
			for _, fragment := range []string{`plugin "acme"`, field, "readdir", "."} {
				if !strings.Contains(err.Error(), fragment) {
					t.Errorf("error %q does not contain %q", err, fragment)
				}
			}
		})
	}
}

func TestResolve_recoversPluginFSPanicDuringRuntimeReads(t *testing.T) {
	source := &panicSkillFS{FS: skillFS(map[string]string{
		"lark-plugin/SKILL.md": "plugin",
	})}
	resolved := mustResolve(t, baseTree(), &platform.SkillsOverlay{Overlay: source})

	tests := []struct {
		op   string
		path string
		call func() error
	}{
		{
			op:   "open",
			path: "lark-plugin/SKILL.md",
			call: func() error {
				_, err := resolved.Open("lark-plugin/SKILL.md")
				return err
			},
		},
		{
			op:   "stat",
			path: "lark-plugin/SKILL.md",
			call: func() error {
				_, err := fs.Stat(resolved, "lark-plugin/SKILL.md")
				return err
			},
		},
		{
			op:   "readfile",
			path: "lark-plugin/SKILL.md",
			call: func() error {
				_, err := fs.ReadFile(resolved, "lark-plugin/SKILL.md")
				return err
			},
		},
		{
			op:   "readdir",
			path: "lark-plugin",
			call: func() error {
				_, err := fs.ReadDir(resolved, "lark-plugin")
				return err
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.op, func(t *testing.T) {
			source.panicOp = tt.op
			err := tt.call()
			source.panicOp = ""
			if err == nil {
				t.Fatalf("%s succeeded after plugin filesystem panic", tt.op)
			}
			for _, fragment := range []string{`plugin "acme"`, "Overlay", tt.op, tt.path} {
				if !strings.Contains(err.Error(), fragment) {
					t.Errorf("error %q does not contain %q", err, fragment)
				}
			}
		})
	}
}
