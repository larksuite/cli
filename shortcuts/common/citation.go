// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"fmt"
	"io"
	"net/url"

	"github.com/larksuite/cli/internal/citation"
)

// CitationDefinition declares a read command's citation capability.
// Legacy shortcuts mount it on Shortcut.Citation with Build set; typed
// shortcuts mount it on Output.Citation with SourceTypes only (the typed
// builder lives in Hooks.BuildCitation so it can see *Args and Data).
//
// Source constraint: every citation text field (title, snippet, and any
// future free-text field) must be drawn from material the command already
// has in hand — the output payload (data), the command's own input
// parameters, and Brand. A builder must never introduce text that isn't
// already present in one of those. Content-safety scanning runs on data
// before the citations closure runs; a builder that respects this
// constraint can only ever surface text that has already been scanned, so
// citation text carries the same safety guarantee as the payload it was
// built from. Pulling in new text from elsewhere (a fresh API call, a
// hardcoded string, anything outside data/args/Brand) breaks that
// guarantee and must not be done.
type CitationDefinition struct {
	// SourceTypes bounds what the builder may return; entries outside this
	// set are dropped at runtime with a stderr warning.
	SourceTypes []citation.SourceType
	// Build is the legacy builder. It sees the final output payload just
	// before serialization; it must not call any API.
	Build func(rt *RuntimeContext, data any) []citation.Citation
}

// validateCitationDeclaration is the registration-time contract shared by the
// legacy mount path and the typed compiler: citations are a read-only-command
// capability with an explicitly bounded scene set.
func validateCitationDeclaration(def *CitationDefinition, risk string) error {
	if def == nil {
		return nil
	}
	if risk != string(RiskRead) {
		return fmt.Errorf("citation requires explicit Risk %q, got %q", RiskRead, risk)
	}
	if len(def.SourceTypes) == 0 {
		return fmt.Errorf("citation requires a non-empty SourceTypes set")
	}
	for _, st := range def.SourceTypes {
		if !citation.IsAllocated(st) {
			return fmt.Errorf("citation SourceTypes contains unallocated value %d", st)
		}
	}
	return nil
}

// wrapCitationBuilder is the single runtime chokepoint shared by both command
// paths: gate short-circuit, then build, then per-entry validation with a
// stderr warning per dropped entry, then Normalize. A nil return means this
// invocation carries no citations at all.
func wrapCitationBuilder(errOut io.Writer, commandPath string,
	declared []citation.SourceType, build func() []citation.Citation) func() []citation.Citation {
	if build == nil || !citation.Enabled() {
		return nil
	}
	declaredSet := make(map[citation.SourceType]struct{}, len(declared))
	for _, st := range declared {
		declaredSet[st] = struct{}{}
	}
	return func() []citation.Citation {
		items := build()
		kept := make([]citation.Citation, 0, len(items))
		for _, item := range items {
			if reason := invalidCitationReason(declaredSet, item); reason != "" {
				fmt.Fprintf(errOut, "warning: %s: dropped citation: %s\n", commandPath, reason)
				continue
			}
			kept = append(kept, item)
		}
		return citation.Normalize(kept)
	}
}

// invalidCitationReason reports why an entry must be dropped, or "" to keep
// it. An empty URL is not an error here: Normalize drops it silently as the
// protocol's normal degradation.
func invalidCitationReason(declared map[citation.SourceType]struct{}, item citation.Citation) string {
	if item.SourceType == citation.SourceUnset {
		return "source_type is unset"
	}
	if _, ok := declared[item.SourceType]; !ok {
		return fmt.Sprintf("source_type %d is not declared by this command", item.SourceType)
	}
	if item.URL != "" {
		parsed, err := url.Parse(item.URL)
		if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
			return "url is not an absolute https URL"
		}
	}
	return ""
}
