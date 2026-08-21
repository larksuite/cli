// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"strings"

	"github.com/larksuite/cli/internal/affordance"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/imcontract"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/skillref"
	"github.com/spf13/cobra"
)

// PrepareDomainHelp appends navigational guidance (routing line, risk legend,
// skill pointers) to a top-level Lark domain's description, returning false
// for anything that is not such a domain. Built lazily at help time because
// shortcuts attach after service registration. skillFS (nil-safe) gates the
// skill pointers.
//
// A hand-authored Long is preserved as the base (e.g. event's "Use 'event
// consume <EventKey>'…"); service domains carry only a Short at this point, so
// we fall back to it. The pristine base is captured once into an annotation so
// re-rendering does not append the guidance twice.
func PrepareDomainHelp(cmd *cobra.Command, skillFS fs.FS) bool {
	return PrepareDomainHelpWithReferences(cmd, skillFS, nil)
}

// PrepareDomainHelpWithReferences is PrepareDomainHelp with a build-local
// canonical-to-runtime skill projection.
func PrepareDomainHelpWithReferences(cmd *cobra.Command, skillFS fs.FS, references *skillref.Resolver) bool {
	if cmd.Annotations[schemaPathAnnotation] != "" {
		return false // a method command
	}
	// Direct child of root only — so Domain() reads this command's own tag, and
	// nested resource groups are excluded.
	if cmd.Parent() == nil || cmd.Parent().Parent() != nil {
		return false
	}
	// A domain is service-sourced or shortcut-tagged; CLI tooling has neither.
	if src, _ := cmdmeta.SourceOf(cmd); src != cmdmeta.SourceService && cmdmeta.Domain(cmd) == "" {
		return false
	}
	if !cmd.HasAvailableSubCommands() {
		return false
	}

	hasShortcuts, hasResources := false, false
	for _, c := range cmd.Commands() {
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		if strings.HasPrefix(c.Name(), "+") {
			hasShortcuts = true
		} else {
			hasResources = true
		}
	}

	var b strings.Builder
	b.WriteString(domainHelpBase(cmd))
	if hasShortcuts && hasResources { // routing only matters when both styles exist
		b.WriteString("\n\nPrefer a +-prefixed shortcut when one matches your task; otherwise use the raw API resource below.")
	}
	b.WriteString("\n\nRisk levels (read | write | high-risk-write) appear in each command's --help; high-risk-write requires --yes, only after the user confirms.")
	canonicalSkills := []string{"lark-" + cmd.Name()}
	if declared, ok := affordance.DomainSkills(cmdmeta.Domain(cmd)); ok {
		canonicalSkills = declared
	}
	writeDomainSkills(&b, canonicalSkills, skillFS, references)
	cmd.Long = b.String()
	return true
}

// domainHelpBase returns the description to seed domain help with — the
// hand-authored Long when present, else the Short.
func domainHelpBase(cmd *cobra.Command) string {
	return captureHelpBase(cmd, domainBaseAnnotation)
}

// captureHelpBase records a command's pristine lead text once — its
// hand-authored Long, or Short when Long is empty — into the given annotation,
// so lazy re-renders compose onto the original text instead of onto an
// already-augmented Long. This is what lets a shortcut's PostMount-authored
// Long survive: it becomes the base the affordance block is appended below.
func captureHelpBase(cmd *cobra.Command, key string) string {
	if base, ok := cmd.Annotations[key]; ok {
		return base
	}
	base := cmd.Long
	if base == "" {
		base = cmd.Short
	}
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmd.Annotations[key] = base
	return base
}

// methodLong is the build-time Long (description + schema pointer +
// params-only addendum). Agent guidance is added lazily by PrepareMethodHelp,
// so command construction never parses the overlay.
func methodLong(description, schemaPath, paramsOnly string) string {
	var b strings.Builder
	b.WriteString(description)
	fmt.Fprintf(&b, "\n\nFull parameter schema:\n  lark-cli schema %s", schemaPath)
	b.WriteString(paramsOnly)
	return b.String()
}

// Annotation keys PrepareMethodHelp reads to rebuild a method command's Long.
// The affordance overlay coordinates live in cmdmeta (shared with shortcuts).
const (
	schemaPathAnnotation   = "method-schema-path"
	paramsOnlyAnnotation   = "method-params-only"
	domainBaseAnnotation   = "affordance-domain-base"
	shortcutBaseAnnotation = "affordance-shortcut-base"
)

// setMethodHelpData records the coordinates PrepareMethodHelp needs (storing a
// few strings is the only build-time cost; the overlay stays untouched).
func setMethodHelpData(cmd *cobra.Command, service, methodID, schemaPath, paramsOnly string) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmdmeta.SetAffordanceRef(cmd, service, methodID)
	cmd.Annotations[schemaPathAnnotation] = schemaPath
	if paramsOnly != "" {
		cmd.Annotations[paramsOnlyAnnotation] = paramsOnly
	}
}

// PrepareMethodHelp rebuilds a generated method command's Long with the agent
// guidance at the TOP (Risk, then the affordance block, then the schema
// pointer), returning false for non-method commands. The overlay is parsed
// here — only when help is rendered. skillFS (nil-safe) gates the related-skill
// pointers: each is emitted only when it resolves in the skill tree (see
// affordance.SkillStatPath), so a typo or a build without embedded skills never
// prints a `skills read` that cannot be opened.
func PrepareMethodHelp(cmd *cobra.Command, skillFS fs.FS) bool {
	return PrepareMethodHelpWithReferences(cmd, skillFS, nil)
}

// PrepareMethodHelpWithReferences is PrepareMethodHelp with a build-local
// canonical-to-runtime skill projection.
func PrepareMethodHelpWithReferences(cmd *cobra.Command, skillFS fs.FS, references *skillref.Resolver) bool {
	return prepareMethodHelp(cmd, skillFS, references, nil)
}

// PrepareMethodHelpWithProjection is PrepareMethodHelpWithReferences with the
// command tree's lazy, build-local schema-reference decision. The established
// helpers remain fully-visible by default; cmd.Build uses this form so the
// framework-owned schema pointer follows the same surface as execution.
func PrepareMethodHelpWithProjection(
	cmd *cobra.Command,
	skillFS fs.FS,
	references *skillref.Resolver,
	canReferenceSchema func() bool,
) bool {
	return prepareMethodHelp(cmd, skillFS, references, canReferenceSchema)
}

func prepareMethodHelp(
	cmd *cobra.Command,
	skillFS fs.FS,
	references *skillref.Resolver,
	canReferenceSchema func() bool,
) bool {
	ann := cmd.Annotations
	if ann == nil {
		return false
	}
	schemaPath, ok := ann[schemaPathAnnotation]
	if !ok {
		return false
	}

	var b strings.Builder
	b.WriteString(cmd.Short)
	writeRisk(&b, cmd)

	var skills []string
	if raw, ok := affordanceRaw(cmd); ok {
		if a, ok := (meta.Method{Affordance: raw}).ParsedAffordance(); ok {
			if block := renderAffordanceValue(a); block != "" {
				b.WriteString("\n\n")
				b.WriteString(block)
			}
			skills = a.Skills
		}
	}

	writeContractHelp(&b, cmd)
	if canReferenceSchema == nil || canReferenceSchema() {
		fmt.Fprintf(&b, "\n\nFull parameter schema:\n  lark-cli schema %s", schemaPath)
	}
	b.WriteString(ann[paramsOnlyAnnotation])

	writeRelatedSkills(&b, skills, skillFS, references)

	cmd.Long = b.String()
	return true
}

// PrepareShortcutHelp composes a +-prefixed shortcut's Long from its affordance
// overlay and contract help. Risk and Tips are deliberately not rendered into
// Long: the root help renderer appends them after Usage/Flags for every
// shortcut, so contract-bearing and ordinary shortcuts keep one layout.
// Returns false when the command is not a shortcut or carries neither an
// overlay nor contract help.
//
// The lead is the command's pristine base (captureHelpBase): a shortcut with a
// hand-authored Long keeps it, while structured affordance guidance is
// appended below without clobbering the business description.
//
// Tips precedence (intentional, not a bug): the overlay's ### Tips win. The
// shortcut's declarative Tips (the Go Tips field) are only a fallback used when
// the overlay declares none. The selected list is stored back on the command
// and removed from the affordance block so the root renderer emits it once.
func PrepareShortcutHelp(cmd *cobra.Command, skillFS fs.FS) bool {
	return PrepareShortcutHelpWithReferences(cmd, skillFS, nil)
}

// PrepareShortcutHelpWithReferences is PrepareShortcutHelp with a build-local
// canonical-to-runtime skill projection.
func PrepareShortcutHelpWithReferences(cmd *cobra.Command, skillFS fs.FS, references *skillref.Resolver) bool {
	if src, _ := cmdmeta.SourceOf(cmd); src != cmdmeta.SourceShortcut {
		return false
	}
	var a meta.Affordance
	hasAffordance := false
	if raw, ok := affordanceRaw(cmd); ok {
		if parsed, parsedOK := (meta.Method{Affordance: raw}).ParsedAffordance(); parsedOK {
			a = parsed
			hasAffordance = true
		}
	}
	contractHelp := imcontract.HelpText(cmd)
	if !hasAffordance && contractHelp == "" {
		return false
	}
	tips := a.Tips
	if len(tips) == 0 {
		tips = cmdutil.GetTips(cmd)
	}
	cmdutil.SetTips(cmd, tips)
	a.Tips = nil

	var b strings.Builder
	b.WriteString(captureHelpBase(cmd, shortcutBaseAnnotation))
	if block := renderAffordanceValue(a); block != "" {
		b.WriteString("\n\n")
		b.WriteString(block)
	}
	if contractHelp != "" {
		b.WriteString("\n\n")
		b.WriteString(contractHelp)
	}
	writeRelatedSkills(&b, a.Skills, skillFS, references)

	cmd.Long = b.String()
	return true
}

func writeContractHelp(b *strings.Builder, cmd *cobra.Command) {
	if text := imcontract.HelpText(cmd); text != "" {
		b.WriteString("\n\n")
		b.WriteString(text)
	}
}

// writeRisk appends the "Risk: <level>" line, warning agents not to self-approve
// high-risk-write commands. A no-op when the command has no risk annotation.
func writeRisk(b *strings.Builder, cmd *cobra.Command) {
	level, ok := cmdutil.GetRisk(cmd)
	if !ok {
		return
	}
	fmt.Fprintf(b, "\n\n%s", cmdutil.RiskHelpText(level))
}

// writeRelatedSkills appends the "Related skills" block for the entries that
// exist in skillFS. Nothing is written when skillFS is nil or no entry resolves,
// so help never prints a `skills read` pointer that cannot be opened.
func writeRelatedSkills(b *strings.Builder, skills []string, skillFS fs.FS, references *skillref.Resolver) {
	avail := availableSkillReferences(skills, skillFS, references)
	if len(avail) == 0 {
		return
	}
	b.WriteString("\n\nRelated skills (read for end-to-end usage):")
	for _, s := range avail {
		fmt.Fprintf(b, "\n  lark-cli skills read %s", s)
	}
}

// writeDomainSkills appends the domain-level skill navigation configured by
// affordance. Preserve the established single-guide sentence for existing
// domains; only domains that opt into multiple entries get the list form.
func writeDomainSkills(b *strings.Builder, skills []string, skillFS fs.FS, references *skillref.Resolver) {
	avail := availableSkillReferences(skills, skillFS, references)
	switch len(avail) {
	case 0:
		return
	case 1:
		fmt.Fprintf(b, "\n\nDomain guide (concepts, command choice, conventions): lark-cli skills read %s", avail[0])
	default:
		b.WriteString("\n\nDomain skills (concepts, command choice, conventions):")
		for _, skill := range avail {
			fmt.Fprintf(b, "\n  lark-cli skills read %s", skill)
		}
	}
}

func availableSkillReferences(skills []string, skillFS fs.FS, references *skillref.Resolver) []string {
	if skillFS == nil || len(skills) == 0 {
		return nil
	}
	var avail []string
	for _, skill := range skills {
		resolved, ok := resolveSkillReference(skill, skillFS, references)
		if !ok {
			continue
		}
		avail = append(avail, resolved)
	}
	return avail
}

func resolveSkillReference(canonical string, skillFS fs.FS, references *skillref.Resolver) (string, bool) {
	// A nil skillFS is also the command-surface gate supplied by cmd.Build:
	// embedded bytes may still exist, but presenters must not point at them
	// when `skills read` is concealed.
	if skillFS == nil {
		return "", false
	}
	if references != nil {
		return references.ResolveString(canonical)
	}
	if _, err := fs.Stat(skillFS, affordance.SkillStatPath(canonical)); err != nil {
		return "", false
	}
	return canonical, true
}

// affordanceLookup is the overlay source; a package var so tests can inject.
var affordanceLookup = affordance.For

// RenderAffordanceForCmd renders a method command's affordance block, or "" when
// it carries none.
func RenderAffordanceForCmd(cmd *cobra.Command) string {
	raw, ok := affordanceRaw(cmd)
	if !ok {
		return ""
	}
	return renderAffordance(meta.Method{Affordance: raw})
}

func affordanceRaw(cmd *cobra.Command) (json.RawMessage, bool) {
	service, methodID, ok := cmdmeta.AffordanceRef(cmd)
	if !ok {
		return nil, false
	}
	return affordanceLookup(service, methodID)
}

// renderAffordance renders a method's affordance as a help block, or "" when it
// has none. Sections are joined with blank lines so they scan as distinct groups.
func renderAffordance(m meta.Method) string {
	a, ok := m.ParsedAffordance()
	if !ok {
		return ""
	}
	return renderAffordanceValue(a)
}

// renderAffordanceValue renders an already-parsed affordance. Split from
// renderAffordance so callers can render a value they have adjusted first (e.g.
// a shortcut folding its declarative tips into an overlay that has none).
func renderAffordanceValue(a meta.Affordance) string {
	var sections []string
	bullets := func(title string, items []string) {
		var nonEmpty []string
		for _, it := range items {
			if strings.TrimSpace(it) != "" {
				nonEmpty = append(nonEmpty, it)
			}
		}
		if len(nonEmpty) == 0 {
			return
		}
		var s strings.Builder
		fmt.Fprintf(&s, "%s:\n", title)
		for _, it := range nonEmpty {
			fmt.Fprintf(&s, "  • %s\n", it)
		}
		sections = append(sections, strings.TrimRight(s.String(), "\n"))
	}

	bullets("When to use", a.UseWhen)
	bullets("Avoid when", a.AvoidWhen)
	bullets("Prerequisites", a.Prerequisites)
	bullets("Tips", a.Tips)
	if len(a.Examples) > 0 {
		var lines []string
		for _, ex := range a.Examples {
			if ex.Command == "" {
				continue
			}
			if ex.Description != "" {
				lines = append(lines, fmt.Sprintf("  • %s\n      %s", ex.Description, ex.Command))
			} else {
				lines = append(lines, fmt.Sprintf("  • %s", ex.Command))
			}
		}
		if len(lines) > 0 {
			sections = append(sections, "Examples:\n"+strings.Join(lines, "\n"))
		}
	}
	for _, ext := range a.Extensions {
		bullets(ext.Label, ext.Items)
	}
	bullets("Related", a.Related)

	return strings.Join(sections, "\n\n")
}
