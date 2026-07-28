// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/skillcontent"
)

// withBaseSkills swaps the process-global embedded skill tree for the
// duration of a test, restoring it afterward.
func withBaseSkills(t *testing.T, files map[string]string) {
	t.Helper()
	base := fstest.MapFS{}
	for p, content := range files {
		base[p] = &fstest.MapFile{Data: []byte(content)}
	}
	saved := embeddedSkillContent
	t.Cleanup(func() { embeddedSkillContent = saved })
	embeddedSkillContent = base
}

// A plugin's SkillsOverlay must reshape the tree the factory serves: skills
// list/read read f.SkillContent, so a resolved removal/overlay shows up here.
// (Framework-generated --help pointers are gated on the same f.SkillContent;
// that gating is covered by the PrepareDomainHelp/PrepareMethodHelp tests in
// cmd/service.)
func TestBuildInternal_appliesPluginSkillsOverlay(t *testing.T) {
	tmpHome(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	withBaseSkills(t, map[string]string{
		"lark-a/SKILL.md":      "---\ndescription: a\n---\n",
		"lark-b/SKILL.md":      "---\ndescription: b\n---\n",
		"lark-shared/SKILL.md": "---\ndescription: shared\n---\n",
	})

	overlay := fstest.MapFS{
		"lark-new/SKILL.md": &fstest.MapFile{Data: []byte("---\ndescription: new\n---\n")},
	}
	platform.Register(platform.NewPlugin("acme", "1.0").
		EmbeddedSkills(&platform.SkillsOverlay{
			Remove:  []string{"lark-shared"},
			Overlay: overlay,
		}).MustBuild())

	f, _, _ := buildInternal(context.Background(), buildInvocationForTest(t))
	if f.SkillContent == nil {
		t.Fatal("f.SkillContent is nil after skill resolution")
	}

	skills, err := skillcontent.New(f.SkillContent).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var names []string
	for _, s := range skills {
		names = append(names, s.Name)
	}
	if got := strings.Join(names, ","); got != "lark-a,lark-b,lark-new" {
		t.Errorf("skills = %q, want lark-a,lark-b,lark-new (shared removed, new added)", got)
	}
}

// Two plugins each customizing skills must abort at dispatch with a
// structured envelope carrying reason_code multiple_skills_overlay_plugins, not
// silently fall back to the default tree.
func TestBuildInternal_multipleSkillPluginsGuard(t *testing.T) {
	tmpHome(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	withBaseSkills(t, map[string]string{"lark-a/SKILL.md": "---\ndescription: a\n---\n"})

	platform.Register(platform.NewPlugin("acme", "1.0").
		EmbeddedSkills(&platform.SkillsOverlay{Remove: []string{"lark-a"}}).MustBuild())
	platform.Register(platform.NewPlugin("globex", "1.0").
		EmbeddedSkills(&platform.SkillsOverlay{Remove: []string{"lark-a"}}).MustBuild())

	_, root, reg := buildInternal(context.Background(), buildInvocationForTest(t))
	if reg != nil {
		t.Errorf("skill conflict guard path should yield nil registry")
	}

	leaf := findRunnableLeaf(root)
	if leaf == nil {
		t.Fatal("no runnable leaf in command tree")
	}
	err := leaf.RunE(leaf, nil)
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *errs.ValidationError, got %T %+v", err, err)
	}
	if verr.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %q, want failed_precondition", verr.Subtype)
	}
	if !strings.Contains(verr.Hint, "multiple_skills_overlay_plugins") {
		t.Errorf("hint should surface reason_code multiple_skills_overlay_plugins, got %q", verr.Hint)
	}
}

// Allow keeps only the listed skills from the base — a CLI upgrade adding
// new embedded skills cannot widen an allow-listed build.
func TestBuildInternal_appliesAllowList(t *testing.T) {
	tmpHome(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	withBaseSkills(t, map[string]string{
		"lark-a/SKILL.md": "---\ndescription: a\n---\n",
		"lark-b/SKILL.md": "---\ndescription: b\n---\n",
		"lark-c/SKILL.md": "---\ndescription: c\n---\n",
	})
	platform.Register(platform.NewPlugin("acme", "1.0").
		EmbeddedSkills(&platform.SkillsOverlay{Allow: []string{"lark-a", "lark-c"}}).
		MustBuild())

	f, _, _ := buildInternal(context.Background(), buildInvocationForTest(t))
	skills, err := skillcontent.New(f.SkillContent).List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var names []string
	for _, s := range skills {
		names = append(names, s.Name)
	}
	if got := strings.Join(names, ","); got != "lark-a,lark-c" {
		t.Errorf("skills = %q, want lark-a,lark-c (allow-list)", got)
	}
}

// A plugin whose SkillsOverlay cannot compose (Remove naming a skill absent
// from the base) must abort with reason_code invalid_skills_overlay.
func TestBuildInternal_invalidSkillsOverlayGuard(t *testing.T) {
	tmpHome(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	withBaseSkills(t, map[string]string{"lark-a/SKILL.md": "---\ndescription: a\n---\n"})

	platform.Register(platform.NewPlugin("acme", "1.0").
		EmbeddedSkills(&platform.SkillsOverlay{Remove: []string{"lark-does-not-exist"}}).MustBuild())

	_, root, _ := buildInternal(context.Background(), buildInvocationForTest(t))
	leaf := findRunnableLeaf(root)
	if leaf == nil {
		t.Fatal("no runnable leaf in command tree")
	}
	err := leaf.RunE(leaf, nil)
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *errs.ValidationError, got %T %+v", err, err)
	}
	if verr.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %q, want failed_precondition", verr.Subtype)
	}
	if !strings.Contains(verr.Hint, "invalid_skills_overlay") {
		t.Errorf("hint should surface reason_code invalid_skills_overlay, got %q", verr.Hint)
	}
}

// A wrapper main that forgets to wire its embedded skill base should get the
// missing host assembly step, not the same recovery hint as a misspelled
// Allow/Remove name.
func TestBuildInternal_missingBaseSkillsGuardHint(t *testing.T) {
	tmpHome(t)
	platform.ResetForTesting()
	t.Cleanup(platform.ResetForTesting)

	saved := embeddedSkillContent
	t.Cleanup(func() { embeddedSkillContent = saved })
	embeddedSkillContent = nil

	platform.Register(platform.NewPlugin("acme", "1.0").
		EmbeddedSkills(&platform.SkillsOverlay{Remove: []string{"lark-a"}}).MustBuild())

	_, root, _ := buildInternal(context.Background(), buildInvocationForTest(t))
	leaf := findRunnableLeaf(root)
	if leaf == nil {
		t.Fatal("no runnable leaf in command tree")
	}
	err := leaf.RunE(leaf, nil)
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *errs.ValidationError, got %T %+v", err, err)
	}
	if verr.Subtype != errs.SubtypeFailedPrecondition {
		t.Errorf("subtype = %q, want failed_precondition", verr.Subtype)
	}
	if !strings.Contains(verr.Hint, "this build embeds no base skill content") {
		t.Errorf("hint should name the missing embedded content, got %q", verr.Hint)
	}
	if !strings.Contains(verr.Hint, "cmd.SetEmbeddedSkillContent") {
		t.Errorf("hint should name the wrapper-main wiring API, got %q", verr.Hint)
	}
	if !strings.Contains(verr.Hint, "invalid_skills_overlay") {
		t.Errorf("hint should preserve reason_code invalid_skills_overlay, got %q", verr.Hint)
	}
}
