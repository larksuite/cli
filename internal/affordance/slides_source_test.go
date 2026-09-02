// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/meta"
)

// slidesShortcutReferences is the audit table of every Slides shortcut and the
// skill reference paths its `### Skills` block must route to. Migrated from the
// former hardcoded slidesShortcutReferencePaths map in cmd/service, with the
// paths corrected to the current cli/, xml/, and workflow/ subdirectories.
var slidesDomainReferences = []string{
	"lark-slides/references/xml/xml-schema-quick-ref.md",
}

var slidesShortcutReferences = map[string][]string{
	"+create":                {"lark-slides/references/cli/lark-slides-create.md"},
	"+add-slide":             {"lark-slides/references/cli/lark-slides-add-slide.md"},
	"+delete-slide":          {"lark-slides/references/cli/lark-slides-delete-slide.md"},
	"+xml-get":               {"lark-slides/references/cli/lark-slides-xml-presentations-get.md"},
	"+screenshot":            {"lark-slides/references/cli/lark-slides-screenshot.md"},
	"+media-upload":          {"lark-slides/references/cli/lark-slides-media-upload.md"},
	"+history-list":          {"lark-slides/references/cli/lark-slides-history.md"},
	"+history-revert":        {"lark-slides/references/cli/lark-slides-history.md"},
	"+history-revert-status": {"lark-slides/references/cli/lark-slides-history.md"},
	"+replace-slide": {
		"lark-slides/references/cli/lark-slides-replace-slide.md",
		"lark-slides/references/workflow/slides-editing.md",
		"lark-slides/references/xml/xml-schema-quick-ref.md",
	},
	"+update-slide": {
		"lark-slides/references/cli/lark-slides-update-slide.md",
		"lark-slides/references/workflow/slides-editing.md",
		"lark-slides/references/xml/xml-schema-quick-ref.md",
	},
	// +update is the hidden alias of +update-slide; keep its routes in sync.
	"+update": {
		"lark-slides/references/cli/lark-slides-update-slide.md",
		"lark-slides/references/workflow/slides-editing.md",
		"lark-slides/references/xml/xml-schema-quick-ref.md",
	},
	// +replace-pages is deprecated in favor of +update-slide, so it routes to
	// the replacement's docs.
	"+replace-pages": {
		"lark-slides/references/cli/lark-slides-update-slide.md",
		"lark-slides/references/workflow/slides-editing.md",
		"lark-slides/references/xml/xml-schema-quick-ref.md",
	},
}

func TestSlidesAffordanceReferenceRoutes(t *testing.T) {
	prev := mdSource
	t.Cleanup(func() { SetSource(prev) })
	SetSource(os.DirFS("../../affordance"))

	if got, ok := DomainSkill("slides"); !ok || got != "lark-slides" {
		t.Fatalf("DomainSkill(slides) = (%q, %v), want (lark-slides, true)", got, ok)
	}
	if got, ok := DomainSkills("slides"); !ok {
		t.Fatal("DomainSkills(slides) ok=false")
	} else {
		for _, ref := range append([]string{"lark-slides"}, slidesDomainReferences...) {
			if !containsExact(got, ref) {
				t.Fatalf("DomainSkills(slides) = %v, want %q", got, ref)
			}
		}
	}

	affordanceSource, err := os.ReadFile("../../affordance/slides.md")
	if err != nil {
		t.Fatal(err)
	}
	parsedDomain := parseDomainMD(affordanceSource, commandFormResolver("slides"))
	if got, want := len(parsedDomain.methods), len(slidesShortcutReferences); got != want {
		t.Fatalf("parsed Slides affordance entries = %d, audited refs = %d", got, want)
	}
	for command := range parsedDomain.methods {
		if _, ok := slidesShortcutReferences[command]; !ok {
			t.Errorf("Slides affordance entry %s bypasses the skill-source audit table", command)
		}
	}

	for command, refs := range slidesShortcutReferences {
		t.Run(command, func(t *testing.T) {
			raw, ok := For("slides", command)
			if !ok {
				t.Fatalf("For(slides, %s) ok=false", command)
			}
			a, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
			if !ok {
				t.Fatalf("slides %s affordance did not parse", command)
			}
			// The domain skill is prepended by mergeSkills; each mapped
			// reference path must follow it.
			if !containsExact(a.Skills, "lark-slides") {
				t.Fatalf("slides %s skills = %v, missing domain skill", command, a.Skills)
			}
			for _, ref := range refs {
				if !containsExact(a.Skills, ref) {
					t.Fatalf("slides %s skills = %v, want %q", command, a.Skills, ref)
				}
			}
		})
	}
}

// TestSlidesAffordanceReferencePathsResolveToRealDocs pins the exact invariant
// this migration fixed: every reference path must resolve to a live document,
// not a moved-away compatibility stub. The former hardcoded map pointed at the
// flat references/*.md paths that are now redirect stubs (references/cli/,
// references/xml/, references/workflow/ hold the real docs), so a stub target
// would silently route agents to a "本文档已迁移" placeholder.
func TestSlidesAffordanceReferencePathsResolveToRealDocs(t *testing.T) {
	skillsRoot := filepath.Join("..", "..", "skills")
	seen := map[string]bool{}
	for _, ref := range append([]string{"lark-slides"}, slidesDomainReferences...) {
		seen[ref] = true
		data, err := os.ReadFile(filepath.Join(skillsRoot, SkillStatPath(ref)))
		if err != nil {
			t.Errorf("domain help references missing file %q: %v", ref, err)
			continue
		}
		if strings.HasSuffix(ref, ".md") &&
			(strings.Contains(string(data), "兼容入口") || strings.Contains(string(data), "本文档已迁移")) {
			t.Errorf("domain help references compatibility stub %q; point at the migrated doc instead", ref)
		}
	}
	for command, refs := range slidesShortcutReferences {
		for _, ref := range refs {
			if seen[ref] {
				continue
			}
			seen[ref] = true
			data, err := os.ReadFile(filepath.Join(skillsRoot, SkillStatPath(ref)))
			if err != nil {
				t.Errorf("shortcut %q references missing file %q: %v", command, ref, err)
				continue
			}
			if strings.Contains(string(data), "兼容入口") || strings.Contains(string(data), "本文档已迁移") {
				t.Errorf("shortcut %q references compatibility stub %q; point at the migrated doc instead", command, ref)
			}
		}
	}
}
