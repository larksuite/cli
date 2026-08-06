// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package skillpolicy composes the CLI's effective embedded skill tree
// from a base skill FS and at most one plugin-supplied SkillsOverlay. It is
// the skill-side analogue of internal/cmdpolicy: plugins contribute a
// delta over a base, and one resolver produces the single tree consumed by
// `skills list`/`read` and framework-generated --help skill pointers.
package skillpolicy

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/skillref"
)

// PluginSkill pairs a plugin name with the SkillsOverlay it contributed, so a
// conflict can be attributed to specific owners. Mirrors
// cmdpolicy.PluginRule.
type PluginSkill struct {
	PluginName    string
	SkillsOverlay *platform.SkillsOverlay
}

// ErrMultipleSkillsOverlays reports that more than one plugin tried to
// customize skill content. Mirrors cmdpolicy.ErrMultipleRestricts: only
// one owner is allowed so independent plugins cannot silently overwrite
// each other's skill tree.
var ErrMultipleSkillsOverlays = errors.New("multiple plugins customized skills; only one plugin may own skill content")

// ErrNoBaseSkillContent reports that Allow or Remove was requested against an
// empty base tree. This most often means an external wrapper main omitted
// cmd.SetEmbeddedSkillContent; exposing a sentinel lets the command layer give
// the integrator that specific recovery action instead of blaming a skill-name
// typo.
var ErrNoBaseSkillContent = errors.New("build embeds no base skill content")

// ErrInvalidHostBase reports that the wrapper-provided base skill tree is
// malformed. It is distinct from a plugin's replacement Base so diagnostics
// can direct the integrator to the correct owner.
var ErrInvalidHostBase = errors.New("host embedded skill content is invalid")

// ErrUnsatisfiedSkillDependency reports that a skill retained by the final
// composed manifest declares another skill that the manifest does not retain.
// The resolver never widens Allow or overrides Remove to repair this: an
// incomplete distribution is a build-integrity error.
var ErrUnsatisfiedSkillDependency = errors.New("composed skill tree has an unsatisfied required skill")

// Resolution is the build-local result of composing embedded skill assets.
// Content serves `skills list`/`read`; References projects canonical
// CLI-authored pointers onto that same tree.
type Resolution struct {
	Content    fs.FS
	References *skillref.Resolver
}

// resolveContent is a test convenience over the production resolution path.
func resolveContent(base fs.FS, specs []PluginSkill) (fs.FS, error) {
	resolved, err := ResolveWithReferences(base, specs)
	if err != nil {
		return nil, err
	}
	return resolved.Content, nil
}

// ResolveWithReferences composes the effective skill tree and its canonical
// reference projection. base is the CLI's embedded skill FS (nil when the build
// embeds none). With no spec, content is unchanged and references resolve by
// identity when the target exists. With exactly one spec it applies, in fixed
// order, Base override -> Allow -> Remove -> Overlay, then validates and
// snapshots ReferenceRemaps against the composed tree. Two or more distinct
// owners is a configuration error.
func ResolveWithReferences(base fs.FS, specs []PluginSkill) (Resolution, error) {
	owners := distinctOwners(specs)
	if len(owners) > 1 {
		return Resolution{}, fmt.Errorf("%w: %v", ErrMultipleSkillsOverlays, owners)
	}
	if len(specs) == 0 || specs[0].SkillsOverlay == nil {
		refs, err := skillref.New(base, nil)
		if err != nil {
			return Resolution{}, err
		}
		return Resolution{Content: base, References: refs}, nil
	}

	owner, spec := specs[0].PluginName, specs[0].SkillsOverlay
	lower := base
	lowerLabel := "host Base"
	if spec.Base != nil {
		lower = protectPluginFS(owner, "Base", spec.Base)
		lowerLabel = "plugin Base"
	}
	upper := protectPluginFS(owner, "Overlay", spec.Overlay)
	lowerSnapshot, err := scanSkillTree(lowerLabel, lower)
	if err != nil {
		if spec.Base == nil {
			return Resolution{}, fmt.Errorf("%w: %w", ErrInvalidHostBase, err)
		}
		return Resolution{}, fmt.Errorf("plugin %q skill spec: %w", owner, err)
	}
	upperSnapshot, err := scanSkillTree("plugin Overlay", upper)
	if err != nil {
		return Resolution{}, fmt.Errorf("plugin %q skill spec: %w", owner, err)
	}
	if err := validateSelection(lowerSnapshot, spec); err != nil {
		return Resolution{}, fmt.Errorf("plugin %q skill spec: %w", owner, err)
	}
	var content fs.FS
	if lower == nil && upper == nil {
		content = nil
	} else {
		composed := newOverlayFS(lowerSnapshot, upperSnapshot, spec.Remove, spec.Allow)
		if err := validateRequiredSkills(composed); err != nil {
			return Resolution{}, fmt.Errorf("plugin %q skill spec: %w", owner, err)
		}
		content = composed
	}
	refs, err := resolveReferences(content, spec.ReferenceRemaps)
	if err != nil {
		return Resolution{}, fmt.Errorf("plugin %q skill spec: %w", owner, err)
	}
	return Resolution{Content: content, References: refs}, nil
}

func resolveReferences(content fs.FS, remaps []platform.SkillRefRemap) (*skillref.Resolver, error) {
	mappings := make([]skillref.Mapping, 0, len(remaps))
	for _, remap := range remaps {
		from, err := skillref.Parse(remap.From())
		if err != nil {
			return nil, fmt.Errorf("%w: source %q: %w", skillref.ErrInvalidRemap, remap.From(), err)
		}
		to, err := skillref.Parse(remap.To())
		if err != nil {
			return nil, fmt.Errorf("%w: target %q: %w", skillref.ErrInvalidRemap, remap.To(), err)
		}
		mappings = append(mappings, skillref.Mapping{From: from, To: to})
	}
	return skillref.New(content, mappings)
}

// distinctOwners returns the unique contributing plugin names in
// first-seen order. Mirrors cmdpolicy.distinctOwners.
func distinctOwners(specs []PluginSkill) []string {
	seen := map[string]bool{}
	owners := make([]string, 0, len(specs))
	for _, s := range specs {
		if !seen[s.PluginName] {
			seen[s.PluginName] = true
			owners = append(owners, s.PluginName)
		}
	}
	return owners
}

type skillTreeSnapshot struct {
	source fs.FS
	skills map[string]skillManifest
}

// scanSkillTree validates and snapshots a skill tree's top level in one
// pass. The returned set is the only source used by validation and overlay
// composition, so a mutable FS cannot swap unvalidated names between phases.
func scanSkillTree(label string, source fs.FS) (skillTreeSnapshot, error) {
	snapshot := skillTreeSnapshot{source: source, skills: map[string]skillManifest{}}
	if source == nil {
		return snapshot, nil
	}
	entries, err := fs.ReadDir(source, ".")
	if err != nil {
		return snapshot, fmt.Errorf("%s: cannot read root: %w", label, err)
	}
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() {
			return snapshot, fmt.Errorf("%s: %q is not a directory; every %s entry must be a <skill>/ dir", label, name, label)
		}
		if !isSkillName(name) {
			return snapshot, fmt.Errorf("%s: %q is not a valid skill name", label, name)
		}
		ok, err := skillExists(source, name)
		if err != nil {
			return snapshot, fmt.Errorf("%s: probing skill %q: %w", label, name, err)
		}
		if !ok {
			return snapshot, fmt.Errorf("%s: skill %q is missing SKILL.md", label, name)
		}
		manifest, err := readSkillManifest(source, name)
		if err != nil {
			return snapshot, fmt.Errorf("%s: skill %q has invalid metadata: %w", label, name, err)
		}
		snapshot.skills[name] = manifest
	}
	return snapshot, nil
}

// validateSelection rejects allow/remove entries that cannot compose against
// the already-validated base snapshot.
func validateSelection(lower skillTreeSnapshot, spec *platform.SkillsOverlay) error {
	if err := validateSkillNames("Allow", spec.Allow); err != nil {
		return err
	}
	if err := validateSkillNames("Remove", spec.Remove); err != nil {
		return err
	}
	if len(lower.skills) == 0 && (len(spec.Allow) > 0 || len(spec.Remove) > 0) {
		return fmt.Errorf("%w; Allow/Remove require skills in the base tree", ErrNoBaseSkillContent)
	}
	if err := validateSkillsInBase("Allow", spec.Allow, lower); err != nil {
		return err
	}
	return validateSkillsInBase("Remove", spec.Remove, lower)
}

func validateSkillNames(field string, names []string) error {
	for _, name := range names {
		if !isSkillName(name) {
			return fmt.Errorf("%s: %q is not a valid skill name", field, name)
		}
	}
	return nil
}

func validateSkillsInBase(field string, names []string, base skillTreeSnapshot) error {
	for _, name := range names {
		if _, exists := base.skills[name]; !exists {
			return fmt.Errorf("%s: skill %q is not in the base tree", field, name)
		}
	}
	return nil
}

// skillExists reports whether fsys holds a skill named name -- a
// directory carrying SKILL.md, the shape internal/skillcontent treats as
// a skill.
func skillExists(fsys fs.FS, name string) (bool, error) {
	if fsys == nil {
		return false, nil
	}
	info, err := fs.Stat(fsys, name+"/SKILL.md")
	switch {
	case err == nil:
		return !info.IsDir(), nil
	case errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrInvalid):
		return false, nil
	default:
		// A permission or I/O fault is a real cause, not absence.
		return false, err
	}
}

// isSkillName rejects empty, dotted, or path-bearing names so Remove
// cannot smuggle a traversal or match outside the top level.
func isSkillName(name string) bool {
	return skillref.ValidSkillName(name)
}
