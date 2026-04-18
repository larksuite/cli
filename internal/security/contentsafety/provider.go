// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentsafety

import (
	"context"
	"sort"

	extcs "github.com/larksuite/cli/extension/contentsafety"
)

func init() {
	extcs.Register(&regexProvider{rules: compiledInjectionRules()})
}

func (p *regexProvider) Name() string { return "regex" }

func (p *regexProvider) Scan(ctx context.Context, req extcs.ScanRequest) (*extcs.Alert, error) {
	normalized := normalize(req.Data)
	hits := make(map[string]struct{}, 4)
	p.walk(ctx, normalized, hits, 0)
	if len(hits) == 0 {
		return nil, nil
	}
	matches := make([]extcs.RuleMatch, 0, len(hits))
	for rule := range hits {
		matches = append(matches, extcs.RuleMatch{Rule: rule})
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Rule < matches[j].Rule })
	return &extcs.Alert{Provider: p.Name(), Matches: matches}, nil
}
