// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package platformhost is the bootstrap-time orchestrator that turns the
// global plugin registry (extension/platform.RegisteredPlugins) into:
//
//   - a populated internal/hook.Registry (Observer / Wrapper / Lifecycle)
//   - a list of cmdpolicy.PluginRule contributions (one per plugin that
//     called r.Restrict)
//
// Two key invariants:
//
//   - **Atomic install.** A plugin's Install() runs against a staging
//     Registrar; only when Install returns nil AND validateSelf passes
//     does the host commit the staged hooks/rule. Partial install never
//     reaches the live Registry, so a half-loaded plugin cannot leave
//     stale Observer / Wrap entries behind.
//
//   - **FailurePolicy honoured.** Each plugin declares FailOpen or
//     FailClosed. FailOpen plugins are skipped on error (warning to
//     stderr); FailClosed plugins abort the whole bootstrap. The
//     framework also enforces the Restricts↔FailClosed consistency
//     contract (a Restricts=true plugin with FailOpen would be a
//     silent security hole and is rejected during install).
//
// The host returns:
//
//   - a *hook.Registry ready to install on the command tree
//   - a []cmdpolicy.PluginRule for the pruning resolver
//   - an error when a FailClosed plugin failed
package internalplatform
