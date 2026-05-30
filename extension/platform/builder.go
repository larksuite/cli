// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package platform

import (
	"errors"
	"fmt"
	"regexp"
)

// Builder is the ergonomic constructor for Plugin. Use it from init():
//
//	func init() {
//	    platform.Register(
//	        platform.NewPlugin("audit", "0.1.0").
//	            Observer(platform.After, "log", platform.All(), auditFn).
//	            FailOpen().
//	            MustBuild())
//	}
//
// The lower-level Plugin interface remains available for cases that
// need finer control (state on a struct, complex Install logic). The
// Builder enforces:
//
//   - Name format (^[a-z0-9][a-z0-9-]*$)
//   - hookName format and uniqueness within a plugin
//   - Restricts ↔ FailClosed consistency (calling Restrict() implies
//     FailClosed, so plugin authors cannot accidentally ship a policy
//     plugin under FailOpen)
//   - Rule validation via ValidateRule analogues (delegated to
//     internal/cmdpolicy at install time; Builder only fast-fails
//     blatantly bad input)
type Builder struct {
	name    string
	version string
	caps    Capabilities

	actions []func(Registrar)
	rules   []*Rule

	hookNames map[string]bool
	errs      []error
}

var pluginNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// NewPlugin starts a Builder. Name format is validated lazily — errors
// surface at Build()/MustBuild() time, allowing chained calls without
// intermediate error handling.
func NewPlugin(name, version string) *Builder {
	b := &Builder{
		name:      name,
		version:   version,
		hookNames: map[string]bool{},
	}
	if !pluginNamePattern.MatchString(name) {
		b.errs = append(b.errs, fmt.Errorf("invalid plugin name %q: must match ^[a-z0-9][a-z0-9-]*$", name))
	}
	return b
}

// RequireCLI sets Capabilities.RequiredCLIVersion (semver constraint,
// e.g. ">=1.1.0"). Empty string means no requirement.
func (b *Builder) RequireCLI(constraint string) *Builder {
	b.caps.RequiredCLIVersion = constraint
	return b
}

// FailOpen sets Capabilities.FailurePolicy = FailOpen. Default when
// neither FailOpen nor FailClosed is called and Restrict is not used.
func (b *Builder) FailOpen() *Builder {
	b.caps.FailurePolicy = FailOpen
	return b
}

// FailClosed sets Capabilities.FailurePolicy = FailClosed. Implicit
// when Restrict() is called.
func (b *Builder) FailClosed() *Builder {
	b.caps.FailurePolicy = FailClosed
	return b
}

// Observer registers an Observer. Multiple calls accumulate.
func (b *Builder) Observer(when When, hookName string, sel Selector, fn Observer) *Builder {
	if !b.validateHookName(hookName, "observer") {
		return b
	}
	// Capture by value so the action closure doesn't share state with
	// subsequent Observer() calls (Go ≥1.22 already gives each call
	// its own copies of parameter values, but pinning is explicit).
	w, n, s, f := when, hookName, sel, fn
	b.actions = append(b.actions, func(r Registrar) {
		r.Observe(w, n, s, f)
	})
	return b
}

// Wrap registers a Wrapper. Multiple calls accumulate; the host
// composes them in registration order (outermost first).
func (b *Builder) Wrap(hookName string, sel Selector, wrap Wrapper) *Builder {
	if !b.validateHookName(hookName, "wrap") {
		return b
	}
	n, s, w := hookName, sel, wrap
	b.actions = append(b.actions, func(r Registrar) {
		r.Wrap(n, s, w)
	})
	return b
}

// On registers a LifecycleHandler.
func (b *Builder) On(event LifecycleEvent, hookName string, fn LifecycleHandler) *Builder {
	if !b.validateHookName(hookName, "on") {
		return b
	}
	e, n, f := event, hookName, fn
	b.actions = append(b.actions, func(r Registrar) {
		r.On(e, n, f)
	})
	return b
}

// Restrict contributes a pruning Rule. Calling Restrict implicitly
// sets Restricts=true and FailurePolicy=FailClosed (the framework
// requires both to coexist; the builder enforces the pairing so the
// plugin author cannot accidentally ship a policy plugin under
// FailOpen). It may be called more than once; each call adds one scoped
// Rule and the engine OR-combines them.
func (b *Builder) Restrict(rule *Rule) *Builder {
	if rule == nil {
		b.errs = append(b.errs, errors.New("Restrict(nil): rule must not be nil"))
		return b
	}
	b.caps.Restricts = true
	b.caps.FailurePolicy = FailClosed
	// Defensive clone: capture an independent snapshot so a caller that
	// reuses and mutates the same *Rule across multiple Restrict calls
	// gets distinct entries (mirrors the staging registrar's clone).
	cp := *rule
	cp.Allow = append([]string(nil), rule.Allow...)
	cp.Deny = append([]string(nil), rule.Deny...)
	cp.Identities = append([]Identity(nil), rule.Identities...)
	b.rules = append(b.rules, &cp)
	return b
}

// Build returns the configured Plugin, or an error if any builder
// step found a fault. MustBuild panics on the same error.
//
// The Restrict + FailOpen mismatch is checked here, not in the chained
// setters, because the two methods may be called in either order.
func (b *Builder) Build() (Plugin, error) {
	if len(b.rules) > 0 && b.caps.FailurePolicy == FailOpen {
		b.errs = append(b.errs, errors.New(
			"Restrict() requires FailClosed; do not call FailOpen() after Restrict()"))
	}
	if len(b.errs) > 0 {
		return nil, errors.Join(b.errs...)
	}
	return &builtPlugin{
		name:    b.name,
		version: b.version,
		caps:    b.caps,
		actions: b.actions,
		rules:   b.rules,
	}, nil
}

// MustBuild panics if Build() would return an error. Designed for
// init():
//
//	func init() { platform.Register(platform.NewPlugin(...).MustBuild()) }
//
// A panic in init runs before the framework's recover guard is
// installed and will crash the binary. That is the intended
// behaviour: a misconfigured plugin must NOT be silently registered.
func (b *Builder) MustBuild() Plugin {
	p, err := b.Build()
	if err != nil {
		panic(fmt.Sprintf("plugin %q: %v", b.name, err))
	}
	return p
}

// validateHookName checks the grammar and uniqueness; returns false
// when the name was rejected (caller skips the action).
func (b *Builder) validateHookName(hookName, kind string) bool {
	if !pluginNamePattern.MatchString(hookName) {
		b.errs = append(b.errs, fmt.Errorf(
			"%s %q: hookName must match ^[a-z0-9][a-z0-9-]*$", kind, hookName))
		return false
	}
	if b.hookNames[hookName] {
		b.errs = append(b.errs, fmt.Errorf(
			"%s %q: hookName already used in this plugin", kind, hookName))
		return false
	}
	b.hookNames[hookName] = true
	return true
}

// builtPlugin is the Plugin implementation the builder emits.
type builtPlugin struct {
	name    string
	version string
	caps    Capabilities
	actions []func(Registrar)
	rules   []*Rule
}

func (p *builtPlugin) Name() string               { return p.name }
func (p *builtPlugin) Version() string            { return p.version }
func (p *builtPlugin) Capabilities() Capabilities { return p.caps }
func (p *builtPlugin) Install(r Registrar) error {
	for _, rule := range p.rules {
		r.Restrict(rule)
	}
	for _, action := range p.actions {
		action(r)
	}
	return nil
}
