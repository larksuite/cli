// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentsafety

import "context"

const (
	maxStringBytes = 1 << 17 // 128 KiB per string
	maxDepth       = 64
)

type regexProvider struct {
	rules []injectionRule
}

// walk does a depth-first traversal over generic JSON data, scanning all
// string leaves against injection rules. Fail-open design: if depth exceeds
// maxDepth or the context is cancelled, the node is silently skipped — the
// caller (runContentSafety) treats missing hits as "no issue detected" and
// lets the response pass through.
func (p *regexProvider) walk(ctx context.Context, v any, hits map[string]struct{}, depth int) {
	if depth > maxDepth {
		return // fail-open: skip nodes beyond max depth
	}
	if ctx.Err() != nil {
		return // fail-open: deadline exceeded or cancelled, stop traversal
	}
	switch t := v.(type) {
	case string:
		p.scanString(t, hits)
	case map[string]any:
		for _, child := range t {
			p.walk(ctx, child, hits, depth+1)
		}
	case []any:
		for _, child := range t {
			p.walk(ctx, child, hits, depth+1)
		}
		// json.Number, bool, nil — no injection attack surface, skip silently
	}
}

func (p *regexProvider) scanString(text string, hits map[string]struct{}) {
	if len(text) > maxStringBytes {
		text = text[:maxStringBytes] // fail-open: truncate oversized strings, payload beyond this limit is not scanned
	}
	for _, rule := range p.rules {
		if _, already := hits[rule.ID]; already {
			continue
		}
		if rule.Pattern.MatchString(text) {
			hits[rule.ID] = struct{}{}
		}
	}
}
