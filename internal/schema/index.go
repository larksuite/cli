// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"regexp"
	"sort"
	"strings"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/meta"
)

// Kind values distinguish the two index shapes from a full schema envelope, so
// a consumer never has to guess which one it received.
const (
	KindServiceIndex = "service_index"
	KindMethodIndex  = "method_index"
)

const (
	serviceIndexHint = "run `lark-cli schema <service>` for that service's method index, " +
		"or `lark-cli schema <service>.<resource>.<method>` for one method's full parameter contract"
	// The hint names what each field is for; it no longer teaches a dot-to-space
	// rule. Describing the transformation is what forced a reader to first work
	// out which surface a string came from — this index's rows are fully
	// qualified (both the service and the method boundary need a space), while
	// the domain-help listing's rows are service-relative (only the method
	// boundary is left). One method reachable through two contradictory rules is
	// the defect; rendering the runnable form on every row removes both rules.
	methodIndexHint = "run `lark-cli schema <path>` for one method's full parameter contract; " +
		"`command` runs the method as written, `path` is the identifier `schema` takes"
)

// commandPrefix is the argv0 a rendered command form carries so a caller can run
// the string without composing anything around it. Shared by the index and the
// detail view: one naming system means one prefix.
const commandPrefix = "lark-cli "

// ServiceIndex is the output of a bare `schema`: which services exist, and what
// each one is for. It deliberately carries no method count — counting would
// force parsing every service's metadata, which is the exact cost this index
// exists to avoid.
type ServiceIndex struct {
	Kind     string             `json:"kind"`
	Services []ServiceIndexItem `json:"services"`
	Hint     string             `json:"hint"`
}

type ServiceIndexItem struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// MethodIndex is the output of `schema <service>` / `schema <service>.<resource>`:
// one line per method, enough to pick one. Service names the owning service and
// mirrors the service index's "services"; each methods[].path stays the full
// dotted path.
type MethodIndex struct {
	Kind    string            `json:"kind"`
	Service string            `json:"service"`
	Methods []MethodIndexItem `json:"methods"`
	Hint    string            `json:"hint"`
}

// MethodIndexItem field names mirror the schema envelope's _meta (snake_case) so
// the index and the detail view speak one naming system. Path and Command are
// two renderings of the same method, not a redundancy: Path is the dotted
// identifier that feeds straight back into `schema`, Command is the argv that
// runs it. This index is the highest-volume copy source in the CLI — one call
// returns every method of a service — so it is the surface where making the
// reader derive the runnable form costs the most.
type MethodIndexItem struct {
	Path         string   `json:"path"`
	Command      string   `json:"command"`
	Description  string   `json:"description"`
	Risk         string   `json:"risk"`
	AccessTokens []string `json:"access_tokens"`
}

// BuildServiceIndex renders the service listing. descFor supplies the curated
// description; the caller injects it so this package keeps no registry
// dependency. When there is no curated text the metadata's own description is
// used — the same override-then-fall-back chain the command tree's own service
// summaries use, so this machine-readable index never carries less than the
// human-facing help.
func BuildServiceIndex(services []meta.Service, descFor func(string) string) ServiceIndex {
	items := make([]ServiceIndexItem, 0, len(services))
	for _, svc := range services {
		desc := descFor(svc.Name)
		if desc == "" {
			desc = svc.Description
		}
		items = append(items, ServiceIndexItem{
			Name:        svc.Name,
			Description: SanitizeIndexDesc(desc),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return ServiceIndex{Kind: KindServiceIndex, Services: items, Hint: serviceIndexHint}
}

// BuildMethodIndex renders one line per method ref, sorted by dotted path.
// Methods is never nil — an empty slice means "this path is valid but the
// current identity sees no methods here", which is distinct both from an error
// and from a bare null.
func BuildMethodIndex(service string, refs []apicatalog.MethodRef) MethodIndex {
	items := make([]MethodIndexItem, 0, len(refs))
	for _, ref := range refs {
		items = append(items, MethodIndexItem{
			Path:         ref.SchemaPath(),
			Command:      commandPrefix + strings.Join(ref.CommandPath(), " "),
			Description:  SanitizeIndexDesc(FirstSentence(ref.Method.Description)),
			Risk:         riskOf(ref.Method),
			AccessTokens: ref.Method.Identities(),
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Path < items[j].Path })
	return MethodIndex{Kind: KindMethodIndex, Service: service, Methods: items, Hint: methodIndexHint}
}

// riskOf mirrors the envelope's fallback so the index and the detail view report
// the same risk for the same method.
func riskOf(m meta.Method) string {
	if m.Risk != "" {
		return m.Risk
	}
	return core.RiskRead
}

var (
	// Upstream appends an identity note after the summary. Chinese descriptions
	// delimit it with 。 (cut below), but English ones run it on with ASCII
	// punctuation, so the marker itself is the only reliable cut point —
	// truncating at the first ASCII "." instead would cut abbreviations like
	// "e.g." mid-word.
	identityTailRe = regexp.MustCompile(`\s*Identity:.*$`)
	// Markdown links in upstream text target skill-internal paths, which mean
	// nothing to someone reading CLI output: keep the text, drop the target.
	markdownLinkRe = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
)

// FirstSentence keeps the leading summary of an upstream description and drops
// the identity note that follows it — identity is already a separate field, so
// carrying the tail only inflates the line. Exported so the flattened
// domain-help listing renders descriptions the same way.
func FirstSentence(s string) string {
	if i := strings.IndexRune(s, '。'); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	s = identityTailRe.ReplaceAllString(s, "")
	s = markdownLinkRe.ReplaceAllString(s, "$1")
	return strings.TrimRight(strings.TrimSpace(s), " .;；,，、")
}

var (
	ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	// Zero-width and bidirectional-control characters. The bidi set covers both
	// the embedding/override controls (202a-202e) and the isolates (2066-2069,
	// 061c) — reordering text visually needs only one of the two groups, so
	// covering one without the other would leave the same trick available.
	zeroWidthRe = regexp.MustCompile(`[\x{200b}-\x{200f}\x{2028}\x{2029}\x{feff}\x{202a}-\x{202e}\x{2066}-\x{2069}\x{061c}]`)
)

// SanitizeIndexDesc is the rendering-side defence in depth for upstream
// descriptions that reach a consumer's context: a description that carried ANSI
// escapes, Bidi overrides or raw newlines could otherwise redraw or reorder the
// surrounding listing. It strips ANSI escapes and zero-width/Bidi characters,
// folds remaining control characters to spaces, and collapses the resulting
// runs. Exported for the same reason as FirstSentence.
func SanitizeIndexDesc(s string) string {
	if s == "" {
		return ""
	}
	s = ansiRe.ReplaceAllString(s, "")
	s = zeroWidthRe.ReplaceAllString(s, "")
	var b strings.Builder
	for _, r := range s {
		// C0 (incl. DEL) and C1: C1 carries a second set of escape introducers,
		// so folding only C0 would let those through.
		if r == 0x7f || r < 0x20 || (r >= 0x80 && r <= 0x9f) {
			b.WriteRune(' ')
			continue
		}
		b.WriteRune(r)
	}
	return strings.TrimSpace(spaceRunRe.ReplaceAllString(b.String(), " "))
}
