// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"regexp"
	"strings"

	"github.com/larksuite/cli/internal/meta"
)

// The affordance source is a narrow, fixed markdown subset (see src/*.md):
//
//	# domain            optional `> skill: <name>` applied to every method
//	## command          e.g. `instances get`
//	<lead paragraph>    -> use_when (when this command is right)
//	### Avoid when      -> avoid_when (links become prefer/alternative edges)
//	### Prerequisites   -> prerequisites   (a "…来自 [[x]]" link is a sequence edge)
//	### Tips            -> tips
//	### Examples        -> examples: **description** + a ```fenced``` command
//	### Skills          -> skills: bullet skill names, added to the domain default
//	### <other>         -> extensions[] (custom section, flows through verbatim)
//	[[cmd]]             -> a command reference, rendered as `cmd`
//
// Parsing is lazy and cached (see For), so the constrained grammar is read at
// most once per domain.

var mdLink = regexp.MustCompile(`\[\[(.+?)\]\]`)

// standardSection maps a section heading to its typed Affordance field; any
// other heading becomes an extension.
var standardSection = map[string]string{
	"Avoid when":    "avoid_when",
	"Prerequisites": "prerequisites",
	"Tips":          "tips",
	"Examples":      "examples",
	"Skills":        "skills",
}

// mergeSkills returns the domain-default skill followed by a command's own skill
// entries, de-duplicated in author order and empties dropped. Backticks (left by
// the shared bullet parse) are stripped so each entry is a bare skill name.
func mergeSkills(domain string, extra []string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.Trim(strings.TrimSpace(s), "`")
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add(domain)
	for _, s := range extra {
		add(s)
	}
	return out
}

func linkToBacktick(s string) string { return mdLink.ReplaceAllString(s, "`$1`") }

// SkillStatPath maps a `### Skills` entry to the path (relative to the skill
// tree) whose existence gates it: a bare skill name resolves to its SKILL.md,
// while an entry containing a slash is a name/relative-path reference (e.g.
// "lark-contact/references/lark-contact-search-user.md") and resolves to that
// path directly. Both render as `lark-cli skills read <entry>` — the slash form
// skills read already accepts — so a per-command entry can point at that
// command's own reference file, not just re-point the domain skill.
func SkillStatPath(entry string) string {
	if strings.Contains(entry, "/") {
		return entry
	}
	return entry + "/SKILL.md"
}

// headingToKey maps a command heading ("instances get") to its affordance key
// ("instances.get"). The space→dot rule holds where the command form matches
// the method id; domains whose resource names differ (e.g. plural "messages"
// vs id segment "message") need the registry's authoritative resource↔id table.
func headingToKey(h string) string {
	h = strings.TrimSpace(h)
	if strings.HasPrefix(h, "+") { // shortcut command: key is the command verbatim
		return h
	}
	return strings.ReplaceAll(h, " ", ".")
}

type mdSection struct {
	label string
	items []string
	cases []meta.AffordanceCase
}

// parseDomainMD parses one domain's markdown into per-method Affordance values,
// keyed by method id. resolve maps a command-form heading ("user_mailbox.messages
// list") to its method id ("user_mailbox.message.list"); nil falls back to the
// space→dot rule (valid only where the command form already equals the id).
func parseDomainMD(src []byte, resolve func(string) string) map[string]meta.Affordance {
	if resolve == nil {
		resolve = headingToKey
	}
	out := map[string]meta.Affordance{}

	var skill, curKey string
	var useWhen, para []string // lead paragraphs -> use_when entries (blank line separates)
	var secs []*mdSection
	var sec *mdSection
	var pending string
	var fence []string
	inFence := false

	assemble := func() {
		if curKey == "" {
			return
		}
		if len(para) > 0 {
			useWhen = append(useWhen, strings.TrimSpace(strings.Join(para, " ")))
			para = nil
		}
		var a meta.Affordance
		if len(useWhen) > 0 {
			a.UseWhen = useWhen
		}
		var perCmdSkills []string
		for _, s := range secs {
			switch standardSection[s.label] {
			case "avoid_when":
				a.AvoidWhen = s.items
			case "prerequisites":
				a.Prerequisites = s.items
			case "tips":
				a.Tips = s.items
			case "examples":
				a.Examples = s.cases
			case "skills":
				perCmdSkills = s.items
			default:
				a.Extensions = append(a.Extensions, meta.AffordanceSection{Label: s.label, Items: s.items})
			}
		}
		if s := mergeSkills(skill, perCmdSkills); len(s) > 0 {
			a.Skills = s
		}
		out[curKey] = a
	}

	reset := func() { useWhen, para, secs, sec, pending, fence, inFence = nil, nil, nil, nil, "", nil, false }

	// flushPending appends a non-bullet paragraph line that was not consumed as
	// an example description (i.e. no fence followed) to the current section's
	// items, so prose under any section is preserved rather than dropped.
	flushPending := func() {
		if sec != nil && pending != "" {
			sec.items = append(sec.items, linkToBacktick(pending))
			pending = ""
		}
	}

	for _, raw := range strings.Split(string(src), "\n") {
		line := strings.TrimRight(raw, "\r")
		t := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(line, "## "):
			flushPending()
			assemble()
			curKey = resolve(line[3:])
			reset()
			continue
		case strings.HasPrefix(line, "# "):
			continue
		case strings.HasPrefix(t, "> skill:"):
			skill = strings.TrimSpace(t[len("> skill:"):])
			continue
		case strings.HasPrefix(line, "### "):
			flushPending()
			sec = &mdSection{label: strings.TrimSpace(line[4:])}
			secs = append(secs, sec)
			pending, fence, inFence = "", nil, false
			continue
		}
		if curKey == "" {
			continue
		}
		if sec == nil { // lead paragraphs before any section -> use_when (blank line separates entries)
			if t == "" {
				if len(para) > 0 {
					useWhen = append(useWhen, strings.Join(para, " "))
					para = nil
				}
			} else {
				para = append(para, t)
			}
			continue
		}
		// inside a section: a fenced block is an example command; otherwise the
		// shape follows the writing (bullet item vs **description** before a fence).
		if strings.HasPrefix(t, "```") {
			if !inFence {
				inFence, fence = true, nil
			} else {
				inFence = false
				sec.cases = append(sec.cases, meta.AffordanceCase{Description: linkToBacktick(pending), Command: strings.Join(fence, "\n")})
				pending = ""
			}
			continue
		}
		if inFence {
			fence = append(fence, line)
			continue
		}
		if strings.HasPrefix(t, "-") {
			flushPending()
			sec.items = append(sec.items, linkToBacktick(strings.TrimSpace(t[1:])))
		} else if t != "" {
			flushPending()
			pending = strings.Trim(t, "* ")
		}
	}
	flushPending()
	assemble()
	return out
}
