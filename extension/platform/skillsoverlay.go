// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

import "io/fs"

// SkillsOverlay declares how a plugin customizes the CLI's embedded
// skill content, contributed via Builder.EmbeddedSkills. At most one
// source may own skill content; two customizing plugins abort startup.
//
// Allow / Remove mirror Rule's Allow / Deny: an allow-list keeps only
// what it names, a remove-list drops what it names, and Remove wins
// over Allow. Composition order is fixed: Base (or the host-provided base) ->
// Allow -> Remove -> Overlay, a same-named skill resolving to Overlay.
// The repository's root binary provides its base from content_embed.go;
// an external wrapper main has no implicit CLI default and must call
// cmd.SetEmbeddedSkillContent before Execute if it relies on that base.
//
// Skills are addressed by exact name (a directory carrying SKILL.md,
// e.g. "lark-doc"), not by command path and not by glob — the skill
// list is flat, so misspellings abort startup instead of silently
// matching nothing. Removing a skill drops its content and
// framework-generated guidance; it does not disable any command (use
// Restrict for that). ReferenceRemaps lets a distribution map the CLI's
// canonical skill references to the runtime names and files it ships.
//
// The top-level skill set and each skill's owning FS are snapshotted when
// the CLI builds. Later additions or removals of top-level directories do
// not change the manifest; files within an owned skill directory are read
// live. Base and Overlay must contain only valid skill directories.
// A skill may declare hard dependencies under
// metadata.requires.skills in SKILL.md. Every declared dependency must be
// present in the final composed manifest; Allow is never widened and Remove
// is never overridden to satisfy one. A same-named Overlay replacement uses
// the replacement SKILL.md's dependency metadata, not the base copy's.
// Declaring this asset composition is a build-integrity commitment: invalid
// selection, content, ownership, or reference remaps abort the build rather
// than silently falling back to host defaults.
type SkillsOverlay struct {
	// Allow, when non-empty, keeps only these skills (by name) from the
	// base tree — the allow-list counterpart of Rule.Allow. Skills the
	// CLI adds in future versions stay out of the build until listed
	// here, which a Remove-only spec cannot guarantee. A name not
	// present in the base aborts startup. Overlay entries are exempt:
	// content the integrator explicitly ships needs no allow-listing.
	Allow []string

	// Remove hides these skills, by name (e.g. "lark-shared"), from the
	// base tree; it wins over Allow, mirroring Rule's Deny-over-Allow. A
	// name not present in the base aborts startup rather than being
	// silently ignored.
	Remove []string

	// Overlay contributes skills laid over the base: a same-named skill
	// replaces the base's entirely, a new name adds one. It is rooted at
	// the skill list (entries like "my-skill/SKILL.md"); each top-level
	// entry must be a "<name>/" directory containing SKILL.md. Any fs.FS
	// works (embed.FS, os.DirFS, fstest.MapFS); embed.FS is not required.
	Overlay fs.FS

	// Base replaces the host-provided base skill tree instead of layering
	// over it. nil keeps whatever base the host wired with
	// cmd.SetEmbeddedSkillContent; it does not import the repository
	// binary's default into an external wrapper main. Every top-level
	// entry must be a valid skill directory containing SKILL.md. Most
	// integrators leave Base nil and use Remove/Overlay so unchanged
	// host-provided skills need no copy inside the plugin.
	Base fs.FS

	// ReferenceRemaps maps CLI-authored canonical skill references to the
	// runtime names and files this distribution ships. References use the
	// same "name[/relative/path]" form accepted by `lark-cli skills read`.
	//
	// A bare source remaps the whole skill name while preserving the
	// referenced relative path:
	//
	//	RemapSkillRef("lark-doc", "acme-docx")
	//
	// maps both "lark-doc" and
	// "lark-doc/references/lark-doc-fetch.md" to the corresponding paths
	// under "acme-docx". A source carrying a relative path is an exact
	// reference override and wins over the whole-skill mapping:
	//
	//	RemapSkillRef(
	//	    "lark-doc/references/lark-doc-fetch.md",
	//	    "acme-docx/guides/fetch.md",
	//	)
	//
	// Remaps affect structured CLI help/affordance references only. They
	// do not rewrite prose or links inside embedded Markdown. Every
	// explicitly mapped target must exist in the composed tree; otherwise
	// startup fails as an invalid SkillsOverlay. An unmapped canonical
	// reference whose file is absent is simply unavailable to presenters.
	ReferenceRemaps []SkillRefRemap

	// Prevent cross-package unkeyed literals. SkillsOverlay is introduced as
	// part of the plugin SDK and is expected to grow through keyed fields;
	// rejecting positional construction now keeps future additions
	// source-compatible for consumers.
	_ struct{}
}

// SkillRefRemap is an immutable mapping between two structured embedded-skill
// references. Its fields are private so malformed values can only enter via a
// zero value (which the resolver rejects) or RemapSkillRef.
//
// Use From and To for diagnostics; callers should not parse their values to
// perform resolution themselves.
type SkillRefRemap struct {
	from string
	to   string
}

// RemapSkillRef maps a canonical "name[/relative/path]" reference to the
// runtime reference shipped by a distribution. Syntax and target existence are
// validated when the SkillsOverlay is composed, so it participates in the same
// build-integrity failure path as Base, Allow, Remove, and Overlay.
func RemapSkillRef(from, to string) SkillRefRemap {
	return SkillRefRemap{from: from, to: to}
}

// From returns the canonical source reference.
func (m SkillRefRemap) From() string { return m.from }

// To returns the distribution runtime target reference.
func (m SkillRefRemap) To() string { return m.to }
