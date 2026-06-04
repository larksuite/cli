// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package ratelimit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"github.com/larksuite/cli/internal/core"
)

type Request struct {
	Brand  core.LarkBrand
	AppID  string
	Method string
	Path   string
}

type Limiter struct {
	store    *stateFile
	compiled []compiledRule
	now      func() time.Time
}

var (
	defaultLimiterMu       sync.Mutex
	defaultLimiterOverride *Limiter
)

func newLimiter(store *stateFile, rules []Rule, now func() time.Time) *Limiter {
	return newLimiterWithCompiled(store, compileRules(rules), now)
}

func newLimiterWithCompiled(store *stateFile, compiled []compiledRule, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	return &Limiter{store: store, compiled: compiled, now: now}
}

func NewLimiterForDir(dir string, rules []Rule, now func() time.Time) *Limiter {
	return newLimiter(newStateFile(dir), rules, now)
}

func Allow(ctx context.Context, req Request) error {
	defaultLimiterMu.Lock()
	limiter := defaultLimiterOverride
	defaultLimiterMu.Unlock()
	if limiter == nil {
		limiter = newLimiterWithCompiled(defaultStateFile(), compiledBuiltinRules, time.Now)
	}
	return limiter.Allow(ctx, req)
}

func SetDefaultLimiterForTest(limiter *Limiter) func() {
	defaultLimiterMu.Lock()
	previous := defaultLimiterOverride
	defaultLimiterOverride = limiter
	defaultLimiterMu.Unlock()
	return func() {
		defaultLimiterMu.Lock()
		defaultLimiterOverride = previous
		defaultLimiterMu.Unlock()
	}
}

func (l *Limiter) Allow(ctx context.Context, req Request) error {
	if l == nil {
		return nil
	}
	if l.store == nil {
		l.store = defaultStateFile()
	}
	rules, canonical, ok := l.match(req.Method, req.Path)
	if !ok {
		return nil
	}
	rules = usableRules(rules)
	if len(rules) == 0 {
		return nil
	}
	if req.AppID == "" {
		return nil
	}
	nowFn := l.now
	if nowFn == nil {
		nowFn = time.Now
	}
	key := buildKey(req.Brand, req.AppID, normalizeMethod(req.Method), canonical)
	return l.store.WithKeyLock(ctx, key, func(entries []int64) ([]int64, error) {
		now := nowFn()
		cutoff := now.Add(-maxMatchedWindow(rules)).UnixMilli()
		kept := prune(entries, cutoff)
		for _, rule := range rules {
			if retryAfter, limited := retryAfterForRule(kept, now, rule); limited {
				return nil, newRateLimitError(rule, retryAfter)
			}
		}
		return append(kept, now.UnixMilli()), nil
	})
}

func usableRules(rules []*Rule) []*Rule {
	// Fail open on local rule mistakes: bad built-in rules must not block user requests.
	usable := rules[:0]
	for _, rule := range rules {
		if rule.Scope != ScopeApp || rule.Limit <= 0 || rule.Window <= 0 {
			continue
		}
		usable = append(usable, rule)
	}
	return usable
}

func (l *Limiter) match(method, rawPath string) ([]*Rule, string, bool) {
	method = normalizeMethod(method)
	path := normalizePath(rawPath)
	if path == "" {
		return nil, "", false
	}
	var rules []*Rule
	var canonical string
	for _, entry := range l.compiled {
		if method != entry.method {
			continue
		}
		if path == entry.rule.CanonicalPath || entry.pattern.MatchString(path) {
			if canonical == "" {
				canonical = entry.rule.CanonicalPath
			}
			if entry.rule.CanonicalPath == canonical {
				rules = append(rules, entry.rule)
			}
		}
	}
	if len(rules) == 0 {
		return nil, "", false
	}
	return rules, canonical, true
}

func prune(values []int64, cutoff int64) []int64 {
	if len(values) == 0 {
		return nil
	}
	kept := values[:0]
	for _, value := range values {
		if value > cutoff {
			kept = append(kept, value)
		}
	}
	return kept
}

func buildKey(brand core.LarkBrand, appID, method, canonicalPath string) string {
	sum := sha256.Sum256([]byte(string(brand) + "\x00" + appID + "\x00" + method + "\x00" + canonicalPath))
	return hex.EncodeToString(sum[:])
}

func maxMatchedWindow(rules []*Rule) time.Duration {
	var max time.Duration
	for _, rule := range rules {
		if rule.Window > max {
			max = rule.Window
		}
	}
	return max
}

func retryAfterForRule(entries []int64, now time.Time, rule *Rule) (time.Duration, bool) {
	cutoff := now.Add(-rule.Window).UnixMilli()
	count := 0
	var oldest int64
	for _, entry := range entries {
		if entry <= cutoff {
			continue
		}
		if count == 0 || entry < oldest {
			oldest = entry
		}
		count++
	}
	if count < rule.Limit {
		return 0, false
	}
	return time.UnixMilli(oldest).Add(rule.Window).Sub(now), true
}
