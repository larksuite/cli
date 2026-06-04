// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"fmt"
	"io"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
)

// holderMismatchWarning carries the structured shape of a soft holder
// mismatch — the operator did not declare an explicit target user (no
// --user, no env), but the AppConfig.CurrentUser left over from a prior
// login disagrees with the freshly-authorized identity. The Message field
// is the human-readable stderr WARN (still emitted for operators tailing
// 2>&1), while the typed fields let JSON-mode consumers key on the
// mismatch programmatically without parsing the message string.
//
// nil means "no soft mismatch" — either the holder matched (clean login)
// or the mismatch was hard-aborted upstream (flag/env source).
type holderMismatchWarning struct {
	HolderOpenId   string
	HolderUserName string
	FreshOpenId    string
	FreshUserName  string
	Message        string
}

// enforceLoginHolderGate is the single entry point both authLoginRun and
// authLoginPollDeviceCode use to gate a fresh authorization against the
// operator-named (or operator-implied) holder. Centralizing the wrapper
// keeps the soft-advisory wiring from drifting between the immediate-login
// path and the --no-wait/--device-code resume path: both paths must hard-
// abort on flag/env mismatches, both must emit the soft advisory to stderr
// on AppConfig.CurrentUser mismatches, and both must let login proceed in
// that soft case.
//
// Returns (warning, abortErr):
//   - abortErr non-nil → caller MUST return it (no warning emitted)
//   - abortErr nil, warning non-nil → soft mismatch; the human-readable
//     Message has already been written to errOut and the typed fields
//     are threaded through writeLoginSuccess / handleLoginScopeIssue so
//     the JSON success payload carries a structured
//     `holder_mismatch_warning` field for non-human consumers.
//   - both nil → clean login, no holder concern.
//
// freshUserName is the display name returned by the user-info call (may be
// empty); it is threaded through so the soft-advisory branch can render
// "Alice (ou_alice)" instead of two opaque open_ids.
func enforceLoginHolderGate(f *cmdutil.Factory, profileName, freshOpenId, freshUserName string, errOut io.Writer) (*holderMismatchWarning, error) {
	holderOpenId, holderSource, holderUserName := resolveLoginHolder(
		f.Invocation.UserOpenId, f.Invocation.UserSource, lookupAppConfig(profileName))
	warning, abortErr := verifyHolder(holderOpenId, holderUserName, holderSource, freshOpenId, freshUserName)
	if abortErr != nil {
		return nil, abortErr
	}
	if warning != nil {
		fmt.Fprintln(errOut, warning.Message)
	}
	return warning, nil
}

// verifyHolder gates a fresh authorization against the user the operator
// named (or implied) before the device flow ran. Without this gate a stale
// LARKSUITE_CLI_OPEN_ID, a --user typo, or even a phishing redirect could
// silently overwrite a different user's record.
//
// The function distinguishes the holder source and treats them differently:
//
//   - holderSource == "flag" or "env" — the operator EXPLICITLY declared
//     the target user. A mismatch here is either a typo or a phishing/
//     redirect guard: we ABORT before any keychain / config write and
//     return a structured *errs.ConfigError so the dispatcher renders a
//     clean message.
//
//   - holderSource == "" — the holder was IMPLIED by AppConfig.CurrentUser
//     (the active user from the last `auth users use` or login). A mismatch
//     is the legacy "logout-and-login-as-someone-else" workflow: pre-
//     multi-user lark-cli let `auth login` silently replace the single
//     stored user there. Aborting here would break that workflow with no
//     security benefit (the operator did not declare an intent to lock
//     to the implied user). We allow the login to proceed and return a
//     stderr advisory naming the new fallback semantics — Bob is appended
//     to Users[] and the active user stays Alice; switch via `auth users
//     use` if that is what was intended.
//
//   - any other holderSource — fail-closed internal error. The producer
//     (resolveLoginHolder) only emits {"", "flag", "env"} today, so a new
//     value means a refactor introduced a label without thinking about
//     this gate. We refuse to guess whether to abort or advise.
//
// Empty holder is a no-op (legacy single-user install with no CurrentUser
// stamped, or a fresh init).
//
// Returns (warning, abortErr) — error last per Go convention, matching
// enforceLoginHolderGate's signature so the two helpers compose cleanly:
//   - abortErr non-nil  → caller MUST return it (no warning emitted)
//   - abortErr nil, warning non-nil → caller writes warning.Message to
//     stderr and proceeds with the login. The typed fields are
//     threaded through writeLoginSuccess / authorizationCompletePayload
//     so JSON consumers see a structured `holder_mismatch_warning`.
//   - both nil → no holder concern, proceed silently
//
// holderUserName / freshUserName are display names (possibly empty). The
// hard-abort branches stay open_id-only — the operator just typed an
// open_id (flag) or set an env var, and the failure attribution is to a
// machine-readable identifier they can edit. The soft-advisory branch
// renders "Alice (ou_alice)" when both names are present so a human
// reading stderr does not have to mentally map two opaque ou_* strings.
//
// The switch is fail-closed on unknown sources: anything not in the
// {"", "flag", "env"} allow-list aborts with an internal error rather
// than silently falling through to the soft path. The producer
// (resolveLoginHolder) only ever emits those three values today, so any
// other source is a programmer bug — adding a new "profile" or
// "keychain" label without thinking about the gate's contract should
// fail loudly at the gate, not quietly leak a fresh token.
func verifyHolder(holderOpenId, holderUserName, holderSource, freshOpenId, freshUserName string) (*holderMismatchWarning, error) {
	if holderOpenId == "" || holderOpenId == freshOpenId {
		return nil, nil
	}

	switch holderSource {
	case "flag":
		hint := "you passed --user " + holderOpenId + " but the device you authorized was " + freshOpenId +
			". Re-run with `--user " + freshOpenId + "` to register the user you actually authorized," +
			" or re-authorize on a device signed in as " + holderOpenId + "."
		return nil, errs.NewConfigError(errs.SubtypeInvalidArgument,
			"login holder mismatch: requested user %q but authorized user is %q",
			holderOpenId, freshOpenId).
			WithHint(hint)
	case "env":
		hint := "LARKSUITE_CLI_OPEN_ID is set to " + holderOpenId + " but the device you authorized was " + freshOpenId +
			" — unset the env var or re-run with `--user " + freshOpenId + "`."
		return nil, errs.NewConfigError(errs.SubtypeInvalidArgument,
			"login holder mismatch: requested user %q but authorized user is %q",
			holderOpenId, freshOpenId).
			WithHint(hint)
	case "":
		// Implied holder (AppConfig.CurrentUser). Soft mismatch — emit a
		// warning and let login proceed. The Message must:
		//   1. NOT recommend "re-run without --user" (the operator already
		//      did not pass --user; that hint is self-contradictory).
		//   2. Tell the operator the new active-user semantics: the fresh
		//      user is appended, the active user is unchanged, and the
		//      explicit switch command.
		//   3. Render "Alice (ou_alice)" when display names are available
		//      so the human reader can map open_ids to people; fall back
		//      to bare open_id when the name is unknown.
		holderLabel := formatHolderLabel(holderUserName, holderOpenId)
		freshLabel := formatHolderLabel(freshUserName, freshOpenId)
		message := "[lark-cli] [WARN] auth login: the active profile's currentUser is " + holderLabel +
			" but the device you authorized was " + freshLabel +
			". Login will proceed and add " + freshLabel + " to the profile;" +
			" the active user stays " + holderLabel +
			". Run `lark-cli auth users use " + freshOpenId + "` to switch the active user."
		return &holderMismatchWarning{
			HolderOpenId:   holderOpenId,
			HolderUserName: holderUserName,
			FreshOpenId:    freshOpenId,
			FreshUserName:  freshUserName,
			Message:        message,
		}, nil
	default:
		// Fail-closed: an unknown source is a programmer bug at the
		// resolver, not the operator's fault. Surface it as an internal
		// error so it gets fixed instead of silently downgrading the
		// gate's safety property.
		return nil, errs.NewInternalError(errs.SubtypeUnknown,
			"verifyHolder: unknown holderSource %q (expected \"\", \"flag\", or \"env\") — cannot decide whether to abort or advise; refusing to proceed",
			holderSource)
	}
}

// formatHolderLabel renders a holder for human-readable advisory text.
// When a display name is available it returns "Name (open_id)"; otherwise
// just the open_id. Avoids "(ou_xxx)" empty-name artifacts and keeps the
// advisory grep-friendly (the open_id always appears verbatim so existing
// `[lark-cli]` log filters can still extract the identity).
//
// userName is sanitized through validate.SanitizeForTerminal before it is
// woven into the stderr advisory: a stored UserName originates from the
// IdP user-info call but is then persisted in MultiAppConfig and could
// reach this label long after the original login. A maliciously-crafted
// (or accidentally-corrupted) name carrying ANSI escapes, C0 control
// bytes, or zero-width Unicode would otherwise poison the WARN line —
// rewriting prompts, masking output, or injecting fake brand prefixes.
// open_id stays verbatim by contract: it is grep-bait for `[lark-cli]`
// log filters and is regex-validated upstream at the IdP boundary.
//
// The typed JSON fields on holderMismatchWarning are NOT sanitized here:
// per validate.SanitizeForTerminal's contract, JSON / NDJSON consumers
// need raw bytes so they can render in their own escape-aware UI. Only
// the human-readable Message goes through this label, so the sanitizer
// fires exactly where it is safe to mutate.
func formatHolderLabel(userName, openId string) string {
	if userName == "" {
		return openId
	}
	return validate.SanitizeForTerminal(userName) + " (" + openId + ")"
}

// resolveLoginHolder picks the holder identity to verify the login
// against, in priority order: invocation --user/env, then active
// AppConfig.CurrentUser, then none. Falling back to CurrentUser when
// the operator did not pass --user lets the soft-mismatch advisory
// path nudge them — without locking the legacy `auth login` re-login
// workflow behind a hard abort.
//
// Critically, when the invocation override matches an existing user by
// UserName (the --user flag is documented at cmd/global_flags.go to
// accept "open_id or username"), we translate it to the stored
// UserOpenId here so verifyHolder's equality check sees apples-to-apples
// open_ids. Without this translation, `--user Alice` against a profile
// where Alice is stored as ou_alice would land in verifyHolder as
// ("Alice", "ou_alice") — a guaranteed mismatch and an aborted login.
//
// Also returns the matched holder's stored UserName when available — the
// caller threads it into verifyHolder's soft-advisory branch so a human
// reading stderr sees "Alice (ou_alice)" instead of bare open_ids. Empty
// when no matching row is found in the profile (e.g. operator passed a
// brand-new open_id, or the holder is implied from CurrentUser but the
// row was scrubbed).
func resolveLoginHolder(invUserOpenId, invUserSource string, app *core.AppConfig) (openId, source, userName string) {
	if invUserOpenId != "" {
		// Two-pass: exact UserOpenId, then UserName. If neither hits,
		// return verbatim so a brand-new open_id can still match the
		// fresh-authorization echo in verifyHolder.
		if app != nil {
			if hit := app.FindUser(invUserOpenId); hit != nil {
				return hit.UserOpenId, invUserSource, hit.UserName
			}
		}
		return invUserOpenId, invUserSource, ""
	}
	if app != nil && app.CurrentUser != "" {
		// Implied holder — try to recover its display name for the soft
		// advisory; fine to leave empty if the row was scrubbed.
		var name string
		if hit := app.FindUser(app.CurrentUser); hit != nil {
			name = hit.UserName
		}
		return app.CurrentUser, "", name
	}
	return "", "", ""
}
