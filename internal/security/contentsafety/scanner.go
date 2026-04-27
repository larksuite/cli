// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentsafety

import (
	"context"
	"regexp"
)

const (
	maxStringBytes = 1 << 17 // 128 KiB per string
	maxDepth       = 64
)

type rule struct {
	ID      string
	Pattern *regexp.Regexp
}

type scanner struct {
	rules []rule
}

func (s *scanner) walk(ctx context.Context, v any, hits map[string]struct{}, depth int) {
	if depth > maxDepth {
		return
	}
	if ctx.Err() != nil {
		return
	}
	switch t := v.(type) {
	case string:
		s.scanString(t, hits)
	case map[string]any:
		for _, child := range t {
			s.walk(ctx, child, hits, depth+1)
		}
	case []any:
		for _, child := range t {
			s.walk(ctx, child, hits, depth+1)
		}
	}
}

func (s *scanner) scanString(text string, hits map[string]struct{}) {
	if len(text) > maxStringBytes {
		text = text[:maxStringBytes]
	}
	for _, r := range s.rules {
		if _, already := hits[r.ID]; already {
			continue
		}
		if r.Pattern.MatchString(text) {
			hits[r.ID] = struct{}{}
		}
	}
}
