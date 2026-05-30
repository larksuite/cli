// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package internalplatform

import (
	"errors"
	"fmt"
	"io"

	"github.com/larksuite/cli/extension/platform"
	"github.com/larksuite/cli/internal/cmdpolicy"
	"github.com/larksuite/cli/internal/hook"
)

// PluginInfo is the metadata of a successfully-installed plugin,
// captured at install time so diagnostic commands (config plugins show)
// can enumerate plugins without re-calling potentially panic-prone
// plugin methods at display time.
type PluginInfo struct {
	Name         string
	Version      string
	Capabilities platform.Capabilities
}

// InstallResult is the output of InstallAll. Registry is ready for
// hook.Install; PluginRules feeds into cmdpolicy.Resolve as the
// "plugin contribution" half of the resolver input. Plugins lists
// every plugin that committed successfully (FailOpen-skipped plugins
// are absent), for downstream diagnostics.
type InstallResult struct {
	Registry    *hook.Registry
	PluginRules []cmdpolicy.PluginRule
	Plugins     []PluginInfo
}

// InstallAll runs every registered plugin through the staging
// Registrar, validates, and commits the survivors. FailOpen plugins
// that fail are skipped with a warning; the first FailClosed failure
// stops the loop and returns the error.
//
// Plugins are processed in registration order so the result is
// deterministic.
//
// errOut receives warnings about FailOpen plugin skips. nil errOut
// means warnings are dropped (useful in tests).
func InstallAll(plugins []platform.Plugin, errOut io.Writer) (*InstallResult, error) {
	if errOut == nil {
		errOut = io.Discard
	}
	result := &InstallResult{
		Registry: hook.NewRegistry(),
	}

	// Detect duplicate Plugin.Name. We do this up-front so the error
	// surfaces before any Install runs; design hard-constraint #7
	// treats this as configuration error (fail-closed regardless of
	// individual FailurePolicy).
	if err := detectDuplicateNames(plugins); err != nil {
		return nil, err
	}

	for _, p := range plugins {
		name, nameErr := safeCallName(p)
		if nameErr != nil {
			// Fail-closed on bad Name: we don't know the plugin's
			// FailurePolicy yet (it's behind Capabilities, and we
			// cannot trust Capabilities() before Name() succeeds).
			return nil, nameErr
		}
		if err := installOne(name, p, result); err != nil {
			// Some errors must abort regardless of FailurePolicy
			// because they imply the plugin's FailurePolicy itself
			// cannot be trusted (e.g. the consistency check between
			// Restricts and FailClosed failed).
			if isUntrustedConfigError(err) {
				return nil, err
			}
			policy := readFailurePolicy(p)
			switch policy {
			case platform.FailClosed:
				return nil, err
			default:
				fmt.Fprintf(errOut, "warning: plugin %q skipped: %v\n", name, err)
				continue
			}
		}
	}

	return result, nil
}

// isUntrustedConfigError flags errors where the plugin's declared
// FailurePolicy is itself part of the misconfiguration. For these the
// host MUST abort unconditionally; honouring an FailOpen declaration on
// a misconfigured Restricts plugin would defeat the whole point of the
// consistency check.
func isUntrustedConfigError(err error) bool {
	var pi *PluginInstallError
	if !errors.As(err, &pi) {
		return false
	}
	return pi.ReasonCode == ReasonRestrictsMismatch ||
		pi.ReasonCode == ReasonInvalidPluginName ||
		pi.ReasonCode == ReasonPluginNamePanic ||
		pi.ReasonCode == ReasonDuplicatePluginName ||
		pi.ReasonCode == ReasonInvalidCapability
}

// installOne handles a single plugin: build a staging Registrar, call
// Install, run validateSelf, and on success commit to the live
// Registry / PluginRules. Any error means staged data is discarded.
func installOne(name string, p platform.Plugin, result *InstallResult) error {
	caps, capsErr := safeCallCapabilities(p)
	if capsErr != nil {
		return capsErr
	}

	// FailurePolicy is a closed enum. An out-of-range value almost
	// always means the plugin author shipped FailurePolicy(2)/etc. by
	// mistake, and the host's switch on caps.FailurePolicy below would
	// silently treat the unknown value as FailOpen — defeating the
	// security boundary the policy was meant to express. Reject up
	// front with ReasonInvalidCapability (classified as
	// untrusted-config, so the abort is unconditional).
	if caps.FailurePolicy != platform.FailOpen && caps.FailurePolicy != platform.FailClosed {
		return &PluginInstallError{
			PluginName: name,
			ReasonCode: ReasonInvalidCapability,
			Reason: fmt.Sprintf("FailurePolicy=%d is not a recognised value (expected FailOpen or FailClosed)",
				caps.FailurePolicy),
		}
	}

	// Strict consistency check: Restricts=true must pair with
	// FailClosed (design hard-constraint #6).
	if caps.Restricts && caps.FailurePolicy != platform.FailClosed {
		return &PluginInstallError{
			PluginName: name,
			ReasonCode: ReasonRestrictsMismatch,
			Reason:     "Restricts=true requires FailurePolicy=FailClosed",
		}
	}

	// Version compatibility check. Two distinct failure modes:
	//
	//   1. Parse error (constraint is malformed, e.g. ">=abc")
	//      -> ReasonInvalidCapability, classified as untrusted-config
	//         so the host aborts unconditionally. This is a plugin
	//         authoring bug; FailurePolicy must NOT mask it.
	//
	//   2. Legitimate version mismatch (constraint parses fine but
	//      current CLI does not satisfy it)
	//      -> ReasonCapabilityUnmet, honours FailurePolicy. A FailOpen
	//         plugin announcing ">=2.0" against a 1.x CLI is skipped
	//         with a warning; a FailClosed plugin aborts.
	if ok, err := satisfiesRequiredCLIVersion(currentCLIVersion(), caps.RequiredCLIVersion); err != nil {
		return &PluginInstallError{
			PluginName: name,
			ReasonCode: ReasonInvalidCapability,
			Reason:     err.Error(),
		}
	} else if !ok {
		return &PluginInstallError{
			PluginName: name,
			ReasonCode: ReasonCapabilityUnmet,
			Reason: fmt.Sprintf("CLI version %q does not satisfy plugin requirement %q",
				currentCLIVersion(), caps.RequiredCLIVersion),
		}
	}

	staging := newStagingRegistrar(name)
	if err := safeCallInstall(p, staging); err != nil {
		// Don't double-wrap typed PluginInstallError -- safeCallInstall
		// already produces install_panic for recovered panics, and a
		// re-wrap would bury the precise reason_code under
		// install_failed.
		var pi *PluginInstallError
		if errors.As(err, &pi) {
			return err
		}
		return &PluginInstallError{
			PluginName: name,
			ReasonCode: ReasonInstallFailed,
			Reason:     "Install returned error",
			Cause:      err,
		}
	}

	if err := staging.validateSelf(caps); err != nil {
		return err
	}

	// Commit staged data atomically.
	for _, e := range staging.stagedObservers {
		result.Registry.AddObserver(e)
	}
	for _, e := range staging.stagedWrappers {
		result.Registry.AddWrapper(e)
	}
	for _, e := range staging.stagedLifecycles {
		result.Registry.AddLifecycle(e)
	}
	for _, rule := range staging.rules {
		result.PluginRules = append(result.PluginRules, cmdpolicy.PluginRule{
			PluginName: name,
			Rule:       rule,
		})
	}

	// Record the plugin in the inventory. Version is fetched here under
	// a recover-wrapped helper so a plugin's Version() panic does not
	// abort the install we just committed.
	result.Plugins = append(result.Plugins, PluginInfo{
		Name:         name,
		Version:      safeCallVersion(p),
		Capabilities: caps,
	})
	return nil
}

// safeCallVersion mirrors safeCallName but for Plugin.Version. Failures
// degrade to the empty string -- Version is informational, not a hard
// contract field, so we never want it to abort installation.
func safeCallVersion(p platform.Plugin) (v string) {
	defer func() {
		if r := recover(); r != nil {
			v = ""
		}
	}()
	return p.Version()
}

// readFailurePolicy reads Capabilities and returns the policy, falling
// back to FailClosed if Capabilities() panics. Defensive default: we
// assume the worst-case (safety-sensitive) when we cannot read the
// declaration.
//
// **Implementation note**: FailClosed must be the value set BEFORE the
// panic-prone call. The zero value of platform.FailurePolicy is
// FailOpen, so a "just return after recover" pattern would silently
// flip the safe-default to FailOpen on panic -- the opposite of what
// the comment claims.
func readFailurePolicy(p platform.Plugin) (policy platform.FailurePolicy) {
	policy = platform.FailClosed
	defer func() { _ = recover() }()
	policy = p.Capabilities().FailurePolicy
	return
}

// safeCallName recovers from a panic in Plugin.Name() and surfaces it
// as a typed PluginInstallError. Without recovery, a buggy plugin could
// crash the binary before main has a chance to emit a JSON envelope.
func safeCallName(p platform.Plugin) (string, error) {
	var (
		name string
		err  error
	)
	func() {
		defer func() {
			if r := recover(); r != nil {
				err = &PluginInstallError{
					PluginName: "<unknown>",
					ReasonCode: ReasonPluginNamePanic,
					Reason:     fmt.Sprintf("Plugin.Name() panicked: %v", r),
				}
			}
		}()
		name = p.Name()
	}()
	if err != nil {
		return "", err
	}
	if !hookNamePattern.MatchString(name) {
		return "", &PluginInstallError{
			PluginName: name,
			ReasonCode: ReasonInvalidPluginName,
			Reason:     fmt.Sprintf("Plugin.Name() %q must match ^[a-z0-9][a-z0-9-]*$ (no dots)", name),
		}
	}
	return name, nil
}

// safeCallCapabilities mirrors safeCallName for Capabilities().
func safeCallCapabilities(p platform.Plugin) (caps platform.Capabilities, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = &PluginInstallError{
				PluginName: pluginNameOrPlaceholder(p),
				ReasonCode: ReasonCapabilitiesPanic,
				Reason:     fmt.Sprintf("Plugin.Capabilities() panicked: %v", r),
			}
		}
	}()
	caps = p.Capabilities()
	return caps, nil
}

// safeCallInstall mirrors safeCallName for Install(). Install panics
// become install_panic errors, not crashes.
func safeCallInstall(p platform.Plugin, r platform.Registrar) (err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = &PluginInstallError{
				PluginName: pluginNameOrPlaceholder(p),
				ReasonCode: ReasonInstallPanic,
				Reason:     fmt.Sprintf("Install panicked: %v", rec),
			}
		}
	}()
	return p.Install(r)
}

func pluginNameOrPlaceholder(p platform.Plugin) string {
	defer func() { _ = recover() }()
	if n := p.Name(); n != "" {
		return n
	}
	return "<unknown>"
}

// detectDuplicateNames scans the plugin slice for repeated Plugin.Name
// values. Returns a typed PluginInstallError on the first duplicate so
// the bootstrap aborts.
func detectDuplicateNames(plugins []platform.Plugin) error {
	seen := map[string]bool{}
	for _, p := range plugins {
		name, err := safeCallName(p)
		if err != nil {
			// Don't double-report: let installOne handle naming
			// errors per-plugin so we get the same code path.
			continue
		}
		if seen[name] {
			return &PluginInstallError{
				PluginName: name,
				ReasonCode: ReasonDuplicatePluginName,
				Reason:     fmt.Sprintf("duplicate Plugin.Name() %q across plugins", name),
			}
		}
		seen[name] = true
	}
	return nil
}
