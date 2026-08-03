// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentsafety

import (
	"context"
	"errors"
	"fmt"
	"regexp"
)

const (
	maxStringBytes = 1 << 17 // 128 KiB per string
	maxDepth       = 64
)

var errScanIncomplete = errors.New("content safety scan incomplete")

type rule struct {
	ID      string
	Pattern *regexp.Regexp
}

type scanner struct {
	rules    []rule
	fullText bool
}

func (s *scanner) walk(ctx context.Context, v any, hits map[string]struct{}, depth int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if depth > maxDepth {
		if s.fullText {
			return fmt.Errorf("%w: maximum depth %d exceeded", errScanIncomplete, maxDepth)
		}
		return nil
	}
	switch t := v.(type) {
	case string:
		s.scanString(t, hits)
	case map[string]any:
		for k, child := range t {
			// Scan the key too: JSON/NDJSON/table/CSV all emit map keys, so a
			// rule match hiding in a key must not slip past block mode.
			s.scanString(k, hits)
			if err := s.walk(ctx, child, hits, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range t {
			if err := s.walk(ctx, child, hits, depth+1); err != nil {
				return err
			}
		}
	}
	return ctx.Err()
}

func (s *scanner) scanString(text string, hits map[string]struct{}) {
	if !s.fullText && len(text) > maxStringBytes {
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
