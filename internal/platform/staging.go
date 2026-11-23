// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package internalplatform

import (
	"fmt"
	"regexp"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/hook"
)

// hookNamePattern is the grammar both Plugin.Name() and hookName must
// match -- design hard-constraint #9. The "." character is forbidden so
// the namespace join "{plugin}.{hook}" is unambiguous.
var hookNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// stagingRegistrar buffers every Registrar call so the platformhost can
// commit them atomically (or discard them all) once Install returns.
//
// All validation happens here at staging time -- bad hookName, nil
// handler, duplicate names, etc. produce typed errors that surface in
// validateSelf and are translated into PluginInstallError by the host
// loop.
type stagingRegistrar struct {
	pluginName string

	stagedObservers  []hook.ObserverEntry
	stagedWrappers   []hook.WrapperEntry
	stagedLifecycles []hook.LifecycleEntry

	// rules holds the staged Restrict contributions, captured for the
	// host to feed into the resolver later. Empty means the plugin did
	// not call r.Restrict. A plugin may call Restrict more than once;
	// each call adds one scoped Rule (OR-combined by the engine).
	rules []*platform.Rule

	// actuallyRestricted records whether r.Restrict was called at all
	// (even Restrict(nil)), so the Restricts-vs-actual consistency check
	// can detect the call.
	actuallyRestricted bool

	// skillsOverlay holds the staged Skills contribution, captured for the
	// host to feed into the skill resolver later. nil means the plugin
	// did not call r.EmbeddedSkills. overlaySet records that the call happened so a
	// second call in the same plugin can be rejected.
	skillsOverlay *platform.SkillsOverlay
	overlaySet    bool

	// seenHookNames detects duplicate hookName within this plugin's
	// Install call.
	seenHookNames map[string]bool

	// stagingErrs accumulates per-call validation errors. A single
	// Install can violate the grammar multiple times; collecting all
	// of them lets diagnostic output show the full picture.
	stagingErrs []stagingErr
}

// stagingErr is the per-call buffered validation failure.
type stagingErr struct {
	reasonCode string
	message    string
}

func newStagingRegistrar(pluginName string) *stagingRegistrar {
	return &stagingRegistrar{
		pluginName:    pluginName,
		seenHookNames: map[string]bool{},
	}
}

// --- Registrar interface ---

func (r *stagingRegistrar) Observe(when platform.When, name string, sel platform.Selector, fn platform.Observer) {
	if !r.validateName(name) {
		return
	}
	if !r.validateNonNilSelector(name, sel) {
		return
	}
	if fn == nil {
		r.bufferErr(ReasonInvalidHookRegister, fmt.Sprintf("observe %q: handler is nil", name))
		return
	}
	if !isValidWhen(when) {
		r.bufferErr(ReasonInvalidHookRegister, fmt.Sprintf("observe %q: invalid When value %d", name, when))
		return
	}
	r.stagedObservers = append(r.stagedObservers, hook.ObserverEntry{
		Name:     r.namespaced(name),
		When:     when,
		Selector: sel,
		Fn:       fn,
	})
}

func (r *stagingRegistrar) Wrap(name string, sel platform.Selector, w platform.Wrapper) {
	if !r.validateName(name) {
		return
	}
	if !r.validateNonNilSelector(name, sel) {
		return
	}
	if w == nil {
		r.bufferErr(ReasonInvalidHookRegister, fmt.Sprintf("wrap %q: handler is nil", name))
		return
	}
	r.stagedWrappers = append(r.stagedWrappers, hook.WrapperEntry{
		Name:     r.namespaced(name),
		Selector: sel,
		Fn:       w,
	})
}

func (r *stagingRegistrar) On(event platform.LifecycleEvent, name string, fn platform.LifecycleHandler) {
	if !r.validateName(name) {
		return
	}
	if fn == nil {
		r.bufferErr(ReasonInvalidHookRegister, fmt.Sprintf("on %q: handler is nil", name))
		return
	}
	if !isValidLifecycleEvent(event) {
		r.bufferErr(ReasonInvalidHookRegister, fmt.Sprintf("on %q: invalid LifecycleEvent value %d", name, event))
		return
	}
	r.stagedLifecycles = append(r.stagedLifecycles, hook.LifecycleEntry{
		Name:  r.namespaced(name),
		Event: event,
		Fn:    fn,
	})
}

func (r *stagingRegistrar) Restrict(rule *platform.Rule) {
	r.actuallyRestricted = true
	if rule == nil {
		r.bufferErr(ReasonInvalidRule, "Restrict(nil)")
		return
	}
	// Defensive clone: retaining the caller's *Rule directly would let
	// the plugin mutate Allow/Deny/Identities (or even the whole rule)
	// after Install returns, bypassing the validation we run on the
	// stored copy in validateSelf. Take an independent snapshot of
	// every slice field so the post-validation rule is frozen.
	cp := *rule
	cp.Allow = append([]string(nil), rule.Allow...)
	cp.Deny = append([]string(nil), rule.Deny...)
	cp.Identities = append([]platform.Identity(nil), rule.Identities...)
	r.rules = append(r.rules, &cp)
}

func (r *stagingRegistrar) EmbeddedSkills(spec *platform.SkillsOverlay) {
	if r.overlaySet {
		r.bufferErr(ReasonInvalidSkillsOverlay, "EmbeddedSkills() called more than once in the same plugin")
		return
	}
	r.overlaySet = true
	if spec == nil {
		r.bufferErr(ReasonInvalidSkillsOverlay, "EmbeddedSkills(nil)")
		return
	}
	// Defensive clone: freeze selection and reference-remap slices so a plugin
	// cannot mutate them after Install returns. Overlay/Base are read-only
	// fs.FS views retained by reference.
	cp := *spec
	cp.Allow = append([]string(nil), spec.Allow...)
	cp.Remove = append([]string(nil), spec.Remove...)
	cp.ReferenceRemaps = append([]platform.SkillRefRemap(nil), spec.ReferenceRemaps...)
	r.skillsOverlay = &cp
}

// --- helpers ---

func (r *stagingRegistrar) namespaced(name string) string {
	return r.pluginName + "." + name
}

func (r *stagingRegistrar) validateName(name string) bool {
	if !hookNamePattern.MatchString(name) {
		r.bufferErr(ReasonInvalidHookName, fmt.Sprintf("hookName %q must match ^[a-z0-9][a-z0-9-]*$", name))
		return false
	}
	if r.seenHookNames[name] {
		r.bufferErr(ReasonDuplicateHookName, fmt.Sprintf("hookName %q registered twice in same plugin", name))
		return false
	}
	r.seenHookNames[name] = true
	return true
}

func (r *stagingRegistrar) validateNonNilSelector(name string, sel platform.Selector) bool {
	if sel == nil {
		r.bufferErr(ReasonInvalidHookRegister, fmt.Sprintf("hook %q: selector is nil", name))
		return false
	}
	return true
}

func (r *stagingRegistrar) bufferErr(reasonCode, message string) {
	r.stagingErrs = append(r.stagingErrs, stagingErr{
		reasonCode: reasonCode,
		message:    message,
	})
}

// validateSelf runs after Install returns. It checks:
//
//   - any buffered staging error -> abort
//   - Restricts declared but Install did not call r.Restrict -> abort
//   - Restricts NOT declared but Install did call r.Restrict -> abort
//   - EmbeddedSkills contributed under FailOpen -> abort unconditionally
//
// Returns the first PluginInstallError encountered (callers can use
// errors.As to inspect it). Nil means staging is clean.
func (r *stagingRegistrar) validateSelf(caps platform.Capabilities) error {
	// Check this before ordinary registration faults. Once a plugin has
	// successfully staged distribution assets, FailOpen is itself an invalid
	// capability declaration: skipping the plugin would silently fall back to
	// host skills. A later duplicate/bad registration must not downgrade that
	// build-integrity violation into an ordinary fail-open staging error.
	if r.overlaySet && caps.FailurePolicy != platform.FailClosed {
		return r.failOpenSkillsError()
	}
	if len(r.stagingErrs) > 0 {
		first := r.stagingErrs[0]
		return &PluginInstallError{
			PluginName: r.pluginName,
			ReasonCode: first.reasonCode,
			Reason:     first.message,
		}
	}
	if caps.Restricts && !r.actuallyRestricted {
		return &PluginInstallError{
			PluginName: r.pluginName,
			ReasonCode: ReasonRestrictsMismatch,
			Reason:     "Capabilities.Restricts=true but Install did not call r.Restrict",
		}
	}
	if !caps.Restricts && r.actuallyRestricted {
		return &PluginInstallError{
			PluginName: r.pluginName,
			ReasonCode: ReasonRestrictsMismatch,
			Reason:     "Capabilities.Restricts=false but Install called r.Restrict",
		}
	}
	return nil
}

func (r *stagingRegistrar) failOpenSkillsError() error {
	return &PluginInstallError{
		PluginName: r.pluginName,
		ReasonCode: ReasonInvalidCapability,
		Reason:     "Install contributed EmbeddedSkills but FailurePolicy=FailOpen; EmbeddedSkills requires FailClosed",
	}
}

func isValidWhen(w platform.When) bool {
	return w == platform.Before || w == platform.After
}

func isValidLifecycleEvent(e platform.LifecycleEvent) bool {
	switch e {
	case platform.Startup, platform.Shutdown:
		return true
	}
	return false
}
