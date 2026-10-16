// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/meta"
)

// TestBaseDashboardLayoutAffordance verifies the base domain overlay resolves
// for +dashboard-layout and routes to the layout standards reference. The
// shortcut's Go metadata owns WHAT and keeps its own Tips; the overlay only
// adds the decision context (when / instead-of / what you need first).
func TestBaseDashboardLayoutAffordance(t *testing.T) {
	prev := mdSource
	t.Cleanup(func() { SetSource(prev) })
	SetSource(os.DirFS("../../affordance"))

	raw, ok := For(apicatalog.Catalog{}, "base", "+dashboard-layout")
	if !ok {
		t.Fatalf("For(base, +dashboard-layout) ok=false")
	}
	a, ok := (meta.Method{Affordance: raw}).ParsedAffordance()
	if !ok {
		t.Fatalf("base +dashboard-layout affordance did not parse: %s", raw)
	}
	if len(a.UseWhen) == 0 {
		t.Fatalf("missing lead paragraph: %#v", a)
	}
	if len(a.Tips) != 0 {
		t.Fatalf("overlay Tips replace the Go tips instead of merging; keep them in the shortcut: %#v", a.Tips)
	}
	if !containsSubstring(a.AvoidWhen, "+dashboard-block-update") {
		t.Fatalf("avoid-when should route single-block moves elsewhere: %#v", a.AvoidWhen)
	}
	if !containsSubstring(a.Prerequisites, "+dashboard-block-list") {
		t.Fatalf("prerequisites should name where block_id comes from: %#v", a.Prerequisites)
	}
	if len(a.Examples) == 0 || !strings.Contains(a.Examples[0].Command, "+dashboard-layout") {
		t.Fatalf("examples must be runnable +dashboard-layout commands: %#v", a.Examples)
	}
	// The layout standards reference is the whole point of the routing entry;
	// a shipped pointer that does not resolve silently disappears from help.
	const layoutRef = "lark-base/references/lark-base-dashboard-visual-standards.md"
	if !containsSubstring(a.Skills, layoutRef) {
		t.Fatalf("skills should route to %s: %#v", layoutRef, a.Skills)
	}
	if _, err := os.Stat("../../skills/" + layoutRef); err != nil {
		t.Fatalf("skill reference %s does not ship: %v", layoutRef, err)
	}
}

// TestBaseAffordanceHasNoRetiredArrangeCommand guards the retired
// +dashboard-arrange from creeping back into shipped guidance: the affordance
// source, the lark-base SKILL.md entry and every lark-base reference. The
// command itself is kept (hidden from --help) only so older skills shipped
// from other repos keep working; new guidance must route to +dashboard-layout.
func TestBaseAffordanceHasNoRetiredArrangeCommand(t *testing.T) {
	files := []string{"../../affordance/base.md", "../../skills/lark-base/SKILL.md"}
	err := filepath.WalkDir("../../skills/lark-base/references", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(path, ".md") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk skills/lark-base/references: %v", err)
	}
	if len(files) < 3 {
		t.Fatalf("found no lark-base references to check: %v", files)
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(data), "+dashboard-arrange") {
			t.Errorf("%s references the retired +dashboard-arrange; use +dashboard-layout", path)
		}
	}
}

func containsSubstring(items []string, want string) bool {
	for _, item := range items {
		if strings.Contains(item, want) {
			return true
		}
	}
	return false
}
