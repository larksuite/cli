// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentsafety

import (
	"context"
	"io"
	"sort"
	"sync"

	extcs "github.com/larksuite/cli/extension/contentsafety"
	"github.com/larksuite/cli/internal/core"
)

// regexProvider implements extcs.Provider using regex rules from config file.
// Config is loaded on every Scan() call (no caching) so changes take
// effect immediately. mu serializes lazy config creation.
type regexProvider struct {
	configDir string
	mu        sync.Mutex
}

func (p *regexProvider) Name() string { return "regex" }

func (p *regexProvider) Scan(ctx context.Context, req extcs.ScanRequest) (*extcs.Alert, error) {
	cfg, err := p.loadOrCreate(req.ErrOut)
	if err != nil {
		return nil, err
	}

	if !IsAllowlisted(req.Path, cfg.Allowlist) {
		return nil, nil
	}
	if len(cfg.Rules) == 0 {
		return nil, nil
	}

	data := normalize(req.Data)
	s := &scanner{rules: cfg.Rules}
	hits := make(map[string]struct{})
	s.walk(ctx, data, hits, 0)

	if len(hits) == 0 {
		return nil, nil
	}
	matched := make([]string, 0, len(hits))
	for id := range hits {
		matched = append(matched, id)
	}
	sort.Strings(matched)
	return &extcs.Alert{Provider: p.Name(), MatchedRules: matched}, nil
}

// loadOrCreate loads config, creating the default on first use.
// mu serializes creation so concurrent Scan calls don't race on first-use.
func (p *regexProvider) loadOrCreate(errOut io.Writer) (*Config, error) {
	cfg, err := LoadConfig(p.configDir)
	if err == nil {
		return cfg, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Re-check after acquiring the lock (another goroutine may have created it).
	cfg, err = LoadConfig(p.configDir)
	if err == nil {
		return cfg, nil
	}
	if errC := EnsureDefaultConfig(p.configDir, errOut); errC != nil {
		return nil, err
	}
	return LoadConfig(p.configDir)
}

func init() {
	extcs.Register(&regexProvider{
		configDir: core.GetConfigDir(),
	})
}
