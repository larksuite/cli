// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package service

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/affordance"
	"github.com/larksuite/cli/internal/cmdmeta"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/meta"
	"github.com/larksuite/cli/internal/schema"
	"github.com/larksuite/cli/internal/skillref"
	"github.com/spf13/cobra"
)

// HelpRenderer composes agent guidance into command help for exactly one
// command tree. It is built once per cmd.Build and holds that build's
// affordance resolver and skill surface, so help never consults process-global
// state or another build's catalog. Function fields are evaluated at render
// time because skill customization is resolved after the help hook is
// installed.
type HelpRenderer struct {
	// Guidance resolves affordance overlays against the catalog that built the
	// tree. nil renders no guidance blocks.
	Guidance *affordance.Resolver
	// SkillContent returns the skill tree readable through this build's command
	// surface, or nil when `skills read` cannot be referenced (which suppresses
	// every skill pointer). nil behaves like a nil tree.
	SkillContent func() fs.FS
	// SkillReferences returns the build's canonical-to-runtime skill projection;
	// nil (or a nil result) keeps canonical names and gates them by existence.
	SkillReferences func() *skillref.Resolver
	// CanReferenceSchema reports whether the `lark-cli schema` pointer may be
	// rendered on method help. nil means it may.
	CanReferenceSchema func() bool
}

func (r *HelpRenderer) skillContent() fs.FS {
	if r == nil || r.SkillContent == nil {
		return nil
	}
	return r.SkillContent()
}

func (r *HelpRenderer) skillReferences() *skillref.Resolver {
	if r == nil || r.SkillReferences == nil {
		return nil
	}
	return r.SkillReferences()
}

func (r *HelpRenderer) guidance() *affordance.Resolver {
	if r == nil {
		return nil
	}
	return r.Guidance
}

// PrepareDomainHelp appends navigational guidance (routing line, risk legend,
// skill pointers) to a top-level Lark domain's description, returning false
// for anything that is not such a domain. Built lazily at help time because
// shortcuts attach after service registration.
//
// A hand-authored Long is preserved as the base (e.g. event's "Use 'event
// consume <EventKey>'…"); service domains carry only a Short at this point, so
// we fall back to it. The pristine base is captured once into an annotation so
// re-rendering does not append the guidance twice.
func (r *HelpRenderer) PrepareDomainHelp(cmd *cobra.Command) bool {
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
	// The API surface is counted from the flattened listing, not from the visible
	// children: the resource commands are hidden, so a child-based check would
	// find no API side at all — it would drop the routing line below, and for a
	// domain whose only children are API resources it would skip this rendering
	// altogether, leaving that domain's methods unlisted anywhere.
	flat := flattenedAPIMethods(cmd)

	if !cmd.HasAvailableSubCommands() && len(flat) == 0 {
		return false
	}

	hasShortcuts := false
	for _, c := range cmd.Commands() {
		if c.Hidden || c.Name() == "help" || c.Name() == "completion" {
			continue
		}
		if strings.HasPrefix(c.Name(), "+") {
			hasShortcuts = true
		}
	}

	var b strings.Builder
	b.WriteString(captureHelpBase(cmd, domainBaseAnnotation))
	if hasShortcuts && len(flat) > 0 { // routing only matters when both styles exist
		b.WriteString("\n\nPrefer a +-prefixed shortcut when one matches your task; otherwise use the raw API method below.")
	}
	b.WriteString("\n\nRisk levels (read | write | high-risk-write) appear in each command's --help; high-risk-write requires --yes, only after the user confirms.")
	canonicalSkills := []string{"lark-" + cmd.Name()}
	if declared, ok := r.guidance().DomainSkills(cmdmeta.Domain(cmd)); ok {
		canonicalSkills = declared
	}
	writeDomainSkills(&b, canonicalSkills, r.skillContent(), r.skillReferences())
	if len(flat) > 0 {
		b.WriteString("\n\nAPI methods (append --help for params")
		if hasShortcuts {
			// cobra's help template renders Long before UsageString, so this
			// listing physically precedes the +shortcut rows in Available
			// Commands — the reverse of the preference stated above. The pointer
			// is positional only: the "prefer a shortcut" judgement is made once
			// on the boundary line, and restating it here would dilute it.
			// Omitted when the domain has no shortcuts (nothing to point at).
			b.WriteString("; +shortcuts are listed under Available Commands below")
		}
		b.WriteString("):\n")
		b.WriteString(strings.Join(flat, "\n"))
	}
	cmd.Long = b.String()
	return true
}

// flattenedAPIMethods renders one line per visible Meta API method under the
// domain: "  <resource> <method>  <first-sentence description>". The resource
// intermediate commands are hidden from the listing (they stay invocable), so
// this flattened block is the domain help's whole Meta API surface — a reader
// picks a full command path in one hop instead of stopping at a resource row
// that names no methods. Descriptions run through the same first-sentence and
// sanitize pipeline as the schema method index, so both surfaces render one
// method identically.
//
// Each row is the command's own path segments joined by spaces — the exact form
// that runs. An earlier revision listed the dotted form and asked the reader to
// convert it, which readers (agents especially) do not do: they copy the row
// verbatim and get unknown_subcommand. Note that a segment may itself contain
// dots (a flat resource like "chat.members" is one command), so the executable
// form is not derivable from the dotted string by any single substitution —
// which is exactly why it is rendered here rather than explained.
func flattenedAPIMethods(domainCmd *cobra.Command) []string {
	// sortKey is the dotted path, so a resource's methods stay grouped together
	// regardless of how the executable form spaces them.
	type row struct{ sortKey, exec, desc string }
	var rows []row
	var walk func(c *cobra.Command, path []string)
	walk = func(c *cobra.Command, path []string) {
		for _, ch := range c.Commands() {
			name := ch.Name()
			if strings.HasPrefix(name, "+") || name == "help" || name == "completion" {
				continue
			}
			segs := append(append([]string{}, path...), name)
			if ch.Annotations[schemaPathAnnotation] != "" { // a method leaf
				// A hidden method leaf is one a policy layer took away, and it
				// still carries method-schema-path: strict mode swaps in a stub
				// that copies every annotation off the original
				// (cmd/prune.go::strictModeStubFrom) and cmdpolicy hides its deny
				// stub in place (internal/cmdpolicy/apply.go::installDenyStub).
				// Listing on the annotation alone would advertise a path that
				// rejects even --help. The check sits on this branch rather than
				// at the top of the loop because resource groups are hidden by
				// design (service.go) — skipping every hidden child would drop
				// the whole API surface.
				if !ch.Hidden {
					rows = append(rows, row{
						sortKey: strings.Join(segs, "."),
						exec:    strings.Join(segs, " "),
						desc:    schema.SanitizeIndexDesc(schema.FirstSentence(ch.Short)),
					})
				}
				continue
			}
			walk(ch, segs) // a (hidden) resource group
		}
	}
	walk(domainCmd, nil)
	sort.Slice(rows, func(i, j int) bool { return rows[i].sortKey < rows[j].sortKey })

	width := 0
	for _, r := range rows {
		if len(r.exec) > width { // paths are ASCII; byte length == display width
			width = len(r.exec)
		}
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, fmt.Sprintf("  %-*s  %s", width, r.exec, r.desc))
	}
	return out
}

// PrepareMethodHelp rebuilds a generated method command's Long with the agent
// guidance at the TOP (Risk, then the affordance block, then the schema
// pointer), returning false for non-method commands. The overlay is resolved
// here — only when help is rendered. Related-skill pointers are emitted only
// when they resolve in the skill tree (see affordance.SkillStatPath), so a typo
// or a build without embedded skills never prints a `skills read` that cannot
// be opened.
func (r *HelpRenderer) PrepareMethodHelp(cmd *cobra.Command) bool {
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
	if a, ok := r.parsedAffordance(cmd); ok {
		if block := renderAffordanceValue(a); block != "" {
			b.WriteString("\n\n")
			b.WriteString(block)
		}
		skills = a.Skills
	}

	// The body skeleton is self-contained, so it stays regardless of whether
	// the schema command survives projection. The pointer below is a reference
	// to that command and is emitted only while it remains referenceable.
	if body := ann[bodyHelpAnnotation]; body != "" {
		b.WriteString("\n\n" + strings.TrimRight(body, "\n"))
	}
	if r == nil || r.CanReferenceSchema == nil || r.CanReferenceSchema() {
		fmt.Fprintf(&b, "\n\nFull parameter schema:\n  lark-cli schema %s", schemaPath)
	}
	b.WriteString(ann[paramsOnlyAnnotation])

	writeRelatedSkills(&b, skills, r.skillContent(), r.skillReferences())

	cmd.Long = b.String()
	return true
}

// PrepareShortcutHelp composes a +-prefixed shortcut's Long from its affordance
// overlay — the same top layout as method help (description, Risk, guidance
// block, related skills) minus the schema pointer, which shortcuts have none
// of. Returns false when the command is not a shortcut or carries no overlay
// entry, so shortcuts without guidance keep the default help plus the bottom
// risk/tips append.
//
// The lead is the command's pristine base (captureHelpBase): a shortcut with a
// hand-authored Long keeps it, while structured affordance guidance is
// appended below without clobbering the business description.
//
// Tips precedence (intentional, not a bug): the overlay's ### Tips win. The
// shortcut's declarative Tips (the Go Tips field) are only a fallback used when
// the overlay declares none; when the overlay has tips, the Go tips are dropped
// (replaced, not merged) so tips never render twice. Authoring a ### Tips block
// therefore silently retires that shortcut's Go Tips — consolidate into one.
func (r *HelpRenderer) PrepareShortcutHelp(cmd *cobra.Command) bool {
	if src, _ := cmdmeta.SourceOf(cmd); src != cmdmeta.SourceShortcut {
		return false
	}
	a, ok := r.parsedAffordance(cmd)
	if !ok {
		return false
	}
	if len(a.Tips) == 0 {
		a.Tips = cmdutil.GetTips(cmd)
	}

	var b strings.Builder
	b.WriteString(captureHelpBase(cmd, shortcutBaseAnnotation))
	writeRisk(&b, cmd)
	if block := renderAffordanceValue(a); block != "" {
		b.WriteString("\n\n")
		b.WriteString(block)
	}
	writeRelatedSkills(&b, a.Skills, r.skillContent(), r.skillReferences())

	cmd.Long = b.String()
	return true
}

// parsedAffordance resolves and parses the overlay recorded on cmd.
func (r *HelpRenderer) parsedAffordance(cmd *cobra.Command) (meta.Affordance, bool) {
	service, methodID, ok := cmdmeta.AffordanceRef(cmd)
	if !ok {
		return meta.Affordance{}, false
	}
	raw, ok := r.guidance().For(service, methodID)
	if !ok {
		return meta.Affordance{}, false
	}
	return (meta.Method{Affordance: raw}).ParsedAffordance()
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
func methodLong(description, schemaPath, paramsOnly, body string) string {
	var b strings.Builder
	b.WriteString(description)
	if body != "" {
		b.WriteString("\n\n" + strings.TrimRight(body, "\n"))
	}
	fmt.Fprintf(&b, "\n\nFull parameter schema:\n  lark-cli schema %s", schemaPath)
	b.WriteString(paramsOnly)
	return b.String()
}

// Annotation keys PrepareMethodHelp reads to rebuild a method command's Long.
// The affordance overlay coordinates live in cmdmeta (shared with shortcuts).
const (
	schemaPathAnnotation   = "method-schema-path"
	paramsOnlyAnnotation   = "method-params-only"
	bodyHelpAnnotation     = "method-body-help"
	domainBaseAnnotation   = "affordance-domain-base"
	shortcutBaseAnnotation = "affordance-shortcut-base"
)

// setMethodHelpData records the coordinates PrepareMethodHelp needs (storing a
// few strings is the only build-time cost; the overlay stays untouched).
func setMethodHelpData(cmd *cobra.Command, service, methodID, schemaPath, paramsOnly, body string) {
	if cmd.Annotations == nil {
		cmd.Annotations = map[string]string{}
	}
	cmdmeta.SetAffordanceRef(cmd, service, methodID)
	cmd.Annotations[schemaPathAnnotation] = schemaPath
	if paramsOnly != "" {
		cmd.Annotations[paramsOnlyAnnotation] = paramsOnly
	}
	if body != "" {
		cmd.Annotations[bodyHelpAnnotation] = body
	}
}

// writeRisk appends the "Risk: <level>" line, warning agents not to self-approve
// high-risk-write commands. A no-op when the command has no risk annotation.
func writeRisk(b *strings.Builder, cmd *cobra.Command) {
	level, ok := cmdutil.GetRisk(cmd)
	if !ok {
		return
	}
	// --yes asserts the USER confirmed; the agent must not self-approve.
	if level == cmdutil.RiskHighRiskWrite {
		fmt.Fprintf(b, "\n\nRisk: %s (requires explicit user confirmation to execute; the agent must NOT add --yes on its own — only pass --yes after the user has confirmed)", level)
	} else {
		fmt.Fprintf(b, "\n\nRisk: %s", level)
	}
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
