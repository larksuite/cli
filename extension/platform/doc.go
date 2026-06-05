// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package platform is the single public extension contract for lark-cli.
//
// External integrators (plugin authors, embedding platforms) only import this
// package; everything else under internal/ is off-limits.
//
// Plugin lifecycle:
//
//   - Plugin         - the interface every plugin implements (Name / Version / Capabilities / Install)
//   - Registrar      - what Install receives; the four registration verbs (Observe / Wrap / On / Restrict)
//   - Capabilities   - declared up front: FailurePolicy (FailOpen | FailClosed) and Restricts
//   - Register       - process-wide entry point; plugins call this from init()
//
// Hook surface (what Install hangs off Registrar):
//
//   - Observer       - side-effect-only callback, panic-safe, runs Before / After RunE
//   - Wrapper        - middleware that can short-circuit via AbortError
//   - LifecycleHandler - reacts to Startup / Shutdown / etc. (LifecycleEvent + When)
//   - Selector       - chooses which commands a hook applies to (ByDomain / ByWrite / ByReadOnly / ByExactRisk / And / Or / Not, etc.)
//   - Handler        - the inner "run the command" function Wrappers compose around
//   - Invocation     - per-call context passed to handlers (Cmd view + DeniedByPolicy / DenialLayer / DenialPolicySource)
//   - AbortError     - structured short-circuit error from a Wrapper; framework namespaces HookName
//
// Policy surface (what Restrict contributes, also consumable from yaml policy):
//
//   - Rule              - declarative policy rule (Allow / Deny / MaxRisk / Identities / AllowUnannotated)
//   - CommandView       - read-only command metadata view (Path / Domain / Risk / Identities)
//   - Risk / Identity   - defined string types with closed taxonomies; ParseRisk / ParseIdentity
//     convert raw strings (yaml, cobra annotation) into typed values; r.Rank()
//     gives a comparable rank for the read < write < high-risk-write ordering
//   - CommandDeniedError - structured error returned to denied callers
//
// Stability: every exported symbol here is part of the contract. Internal
// orchestration (staging, validation, RunE wrapping, denial guard) lives
// under internal/platform, internal/hook and internal/cmdpolicy and is not
// importable by third parties.
package platform
