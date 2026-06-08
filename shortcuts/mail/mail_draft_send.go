// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// MaxBatchSendDrafts caps the number of draft IDs accepted in a single
// +draft-send invocation. The limit is purely client-side: it bounds command-
// line length comfortably below ARG_MAX and keeps the failure blast radius of
// a single batch small. It is intentionally local to this shortcut (rather
// than living in limits.go) because no other shortcut shares the semantics.
const MaxBatchSendDrafts = 50

// sentDraft is the per-draft success entry in the +draft-send aggregated
// output. message_id and thread_id come from the server response of
// POST /drafts/:draft_id/send.
type sentDraft struct {
	DraftID   string `json:"draft_id"`
	MessageID string `json:"message_id"`
	ThreadID  string `json:"thread_id,omitempty"`
}

// failedDraft is the per-draft failure entry. error is the
// human-readable err.Error() string (typically including ClassifyLarkError
// hints); v2 may surface a structured errno field separately once the server-
// side mapping stabilises (see tech-design "待确认事项").
type failedDraft struct {
	DraftID string `json:"draft_id"`
	Error   string `json:"error"`
}

// batchSendOutput is the JSON envelope data shape:
//
//	{
//	  "mailbox_id":     "me",
//	  "total":          3,
//	  "success_count":  2,
//	  "failure_count":  1,
//	  "sent":  [{"draft_id":..., "message_id":..., "thread_id":...}, ...],
//	  "failed":[{"draft_id":..., "error":...}],
//	  "aborted":     true,
//	  "abort_error": {"type":..., "subtype":..., "code":..., "message":..., "hint":...}
//	}
//
// failed is marked omitempty so a fully successful batch returns a clean shape
// without an empty array.
//
// aborted reports an account-level abort: the failure repeats identically for
// every draft, so the remaining drafts were not attempted and retrying the
// batch as-is fails the same way. abort_error carries the typed error that
// triggered the abort (same wire shape as a stderr error envelope's error
// object) so callers can route recovery from stdout alone. A --stop-on-error
// stop does NOT set aborted: there the failure is draft-level and the caller
// chose to stop early.
type batchSendOutput struct {
	MailboxID    string        `json:"mailbox_id"`
	Total        int           `json:"total"`
	SuccessCount int           `json:"success_count"`
	FailureCount int           `json:"failure_count"`
	Sent         []sentDraft   `json:"sent"`
	Failed       []failedDraft `json:"failed,omitempty"`
	Aborted      bool          `json:"aborted,omitempty"`
	AbortError   interface{}   `json:"abort_error,omitempty"`
}

// MailDraftSend is the `+draft-send` shortcut: send N existing drafts
// sequentially via POST /drafts/:draft_id/send, isolating per-draft failures.
// Risk is "high-risk-write"; callers must pass --yes. User identity only —
// drafts are user-owned resources and bot has no coherent semantics here.
//
// Output schema is the batchSendOutput type above. Partial failures (any
// failed[]) emit an ok:false multi-status envelope so that agents can
// distinguish "all sent" from "some sent" without parsing the success_count
// field.
var MailDraftSend = common.Shortcut{
	Service: "mail",
	Command: "+draft-send",
	Description: "Send one or more existing mail drafts sequentially. Calls " +
		"POST /drafts/:draft_id/send for each input ID, isolates per-draft " +
		"failures, and aggregates the results. Use after the drafts have " +
		"already been created (via the Lark client, +draft-create, or the " +
		"drafts.create API).",
	Risk:      "high-risk-write",
	Scopes:    []string{"mail:user_mailbox.message:send"},
	AuthTypes: []string{"user"},
	HasFormat: true,
	Flags: []common.Flag{
		{Name: "mailbox", Desc: "Mailbox email address that owns the drafts (default: me)."},
		{Name: "draft-id", Type: "string_slice", Required: true,
			Desc: "Draft IDs to send; comma-separated or repeat the flag (max 50)."},
		{Name: "stop-on-error", Type: "bool",
			Desc: "Stop at the first recoverable per-draft failure (default: continue and aggregate). " +
				"Fatal errors (auth, permission, network, mailbox-level quota) always abort immediately " +
				"regardless of this flag."},
	},
	Validate: validateDraftSend,
	DryRun:   dryRunDraftSend,
	Execute:  executeDraftSend,
}

// executeDraftSend runs the +draft-send command:
//
//  1. Resolve mailbox ID (defaults to "me" via resolveComposeMailboxID).
//  2. Validate the draft-id slice (non-empty, under MaxBatchSendDrafts cap,
//     no empty elements).
//  3. Loop over each draft ID, calling POST .../drafts/:id/send directly via
//     runtime.CallAPITyped. Per-draft outcomes:
//     - fatal err (isFatalSendErr) → abort immediately (bypasses
//     --stop-on-error): with earlier progress, emit the aborted ledger as the
//     single failure result; with none, return the typed error directly.
//     - recoverable err → append to failed[]; honor --stop-on-error.
//     - success + automation_send_disable signal → abort the same way with a
//     failed-precondition error.
//     - success → append to sent[].
//  4. Emit batchSendOutput via runtime.Out.
//  5. If any draft failed, emit ok:false via runtime.OutPartialFailure.
func executeDraftSend(ctx context.Context, rt *common.RuntimeContext) error {
	mailboxID := resolveComposeMailboxID(rt)
	draftIDs, err := normalizedDraftSendIDs(rt)
	if err != nil {
		return err
	}

	out := batchSendOutput{MailboxID: mailboxID, Total: len(draftIDs)}
	stopOnErr := rt.Bool("stop-on-error")
	for i, id := range draftIDs {
		idx := i + 1
		writeDraftSendProgressf(rt, "[%d/%d] sending draft %s",
			idx, len(draftIDs), sanitizeForSingleLine(id))
		// Direct CallAPITyped rather than draftpkg.Send: this shortcut never sends
		// a body, so the helper's send_time-aware envelope would add no value.
		data, err := rt.CallAPITyped("POST",
			mailboxPath(mailboxID, "drafts", id, "send"), nil, nil)
		if err != nil {
			if isFatalSendErr(err) {
				writeDraftSendProgressf(rt, "[%d/%d] aborting after draft %s: %s",
					idx, len(draftIDs), sanitizeForSingleLine(id), sanitizeForSingleLine(err.Error()))
				hadProgress := out.hasProgress()
				out.Failed = append(out.Failed, failedDraft{DraftID: id, Error: err.Error()})
				// Account- / mailbox-level failures (auth, permission, network,
				// quota) will repeat identically for every remaining draft —
				// abort immediately so the caller sees a single clear error
				// instead of 100 redundant failed[] entries. With earlier
				// progress the aborted ledger is the single failure result;
				// with none, stdout stays empty and the typed error envelope is.
				if hadProgress {
					return emitDraftSendAborted(rt, &out, err)
				}
				return err
			}
			writeDraftSendProgressf(rt, "[%d/%d] failed draft %s: %s",
				idx, len(draftIDs), sanitizeForSingleLine(id), sanitizeForSingleLine(err.Error()))
			out.Failed = append(out.Failed, failedDraft{DraftID: id, Error: err.Error()})
			if stopOnErr {
				break
			}
			continue
		}
		if reason := extractAutomationDisabledReason(data); reason != "" {
			err := mailFailedPreconditionError(
				"automation send is disabled for this mailbox: %s", reason).
				WithHint("enable automation send for this mailbox, or send the draft from the Lark client")
			writeDraftSendProgressf(rt, "[%d/%d] aborting after draft %s: %s",
				idx, len(draftIDs), sanitizeForSingleLine(id), sanitizeForSingleLine(err.Error()))
			// HTTP success (code: 0) but the backend signaled automation send
			// is disabled — every subsequent send will fail the same way, so
			// abort the batch with a single failure result: the aborted ledger
			// when earlier drafts made progress, the typed error otherwise.
			if out.hasProgress() {
				out.Failed = append(out.Failed, failedDraft{DraftID: id, Error: err.Error()})
				return emitDraftSendAborted(rt, &out, err)
			}
			return err
		}
		s := sentDraft{DraftID: id}
		if v, ok := data["message_id"].(string); ok {
			s.MessageID = v
		}
		if v, ok := data["thread_id"].(string); ok {
			s.ThreadID = v
		}
		out.Sent = append(out.Sent, s)
		if s.MessageID != "" {
			writeDraftSendProgressf(rt, "[%d/%d] sent draft %s message_id=%s",
				idx, len(draftIDs), sanitizeForSingleLine(id), sanitizeForSingleLine(s.MessageID))
		} else {
			writeDraftSendProgressf(rt, "[%d/%d] sent draft %s",
				idx, len(draftIDs), sanitizeForSingleLine(id))
		}
	}
	if len(out.Failed) == 0 {
		emitDraftSendOutput(rt, &out)
		return nil
	}
	return emitDraftSendPartialFailure(rt, &out)
}

// dryRunDraftSend builds the --dry-run preview: one POST call per draft ID,
// in input order, with a header description summarising the batch size.
func dryRunDraftSend(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
	mailboxID := resolveComposeMailboxID(rt)
	draftIDs, _ := normalizedDraftSendIDs(rt)
	api := common.NewDryRunAPI().Desc(fmt.Sprintf(
		"Send %d existing drafts sequentially", len(draftIDs)))
	for _, id := range draftIDs {
		api = api.POST(mailboxPath(mailboxID, "drafts", id, "send"))
	}
	return api
}

func validateDraftSend(ctx context.Context, rt *common.RuntimeContext) error {
	_, err := normalizedDraftSendIDs(rt)
	return err
}

func normalizedDraftSendIDs(rt *common.RuntimeContext) ([]string, error) {
	return normalizeDraftSendIDs(rt.StrSlice("draft-id"))
}

func normalizeDraftSendIDs(draftIDs []string) ([]string, error) {
	if len(draftIDs) == 0 {
		return nil, mailValidationParamError("--draft-id", "--draft-id is required")
	}

	normalized := make([]string, 0, len(draftIDs))
	seen := make(map[string]struct{}, len(draftIDs))
	for _, id := range draftIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			return nil, mailValidationParamError("--draft-id", "--draft-id contains empty value")
		}
		if _, ok := seen[trimmed]; ok {
			return nil, mailValidationParamError("--draft-id", "--draft-id contains duplicate value: %s", trimmed)
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) > MaxBatchSendDrafts {
		return nil, mailValidationParamError("--draft-id",
			"too many drafts: %d > %d (split into multiple batches)",
			len(normalized), MaxBatchSendDrafts)
	}
	return normalized, nil
}

func (out *batchSendOutput) hasProgress() bool {
	return len(out.Sent) > 0 || len(out.Failed) > 0
}

func emitDraftSendOutput(rt *common.RuntimeContext, out *batchSendOutput) {
	out.SuccessCount = len(out.Sent)
	out.FailureCount = len(out.Failed)
	rt.Out(*out, nil)
}

func emitDraftSendPartialFailure(rt *common.RuntimeContext, out *batchSendOutput) error {
	out.SuccessCount = len(out.Sent)
	out.FailureCount = len(out.Failed)
	return rt.OutPartialFailure(*out, nil)
}

// emitDraftSendAborted emits the batch ledger as the single failure result for
// an account-level abort: the ledger carries aborted/abort_error and the
// returned partial-failure signal sets the exit code without a second error
// envelope on stderr.
func emitDraftSendAborted(rt *common.RuntimeContext, out *batchSendOutput, cause error) error {
	out.Aborted = true
	if typed, ok := errs.UnwrapTypedError(errs.WrapInternal(cause)); ok {
		out.AbortError = typed
	}
	return emitDraftSendPartialFailure(rt, out)
}

func writeDraftSendProgressf(rt *common.RuntimeContext, format string, args ...interface{}) {
	if rt == nil || rt.Factory == nil || rt.Factory.IOStreams == nil || rt.Factory.IOStreams.ErrOut == nil {
		return
	}
	fmt.Fprintf(rt.Factory.IOStreams.ErrOut, "mail +draft-send: "+format+"\n", args...)
}

// isFatalSendErr reports whether err is an account- or mailbox-level failure
// that will repeat identically for every subsequent draft. Fatal errors
// bypass --stop-on-error and immediately abort the batch.
//
// Trigger conditions:
//
//   - err does not expose a typed Problem:
//     unknown shapes are treated as fatal so they cannot accidentally
//     accumulate into failed[] for every remaining draft.
//   - Problem.Category ∈ {authentication, authorization, config, network,
//     internal}: token, scope, app-installation problems, throttling,
//     connectivity, SDK, and invalid-response failures are account-level.
//   - Problem.Subtype ∈ {rate_limit, quota_exceeded}: throttling and quota
//     exhaustion are account-level.
//   - Problem.Code ∈ {1234013, 1236007, 1236008, 1236009, 1236010, 1236013}:
//     mailbox missing / quota exhaustion is account-level. Mailbox-not-found
//     stays code-scoped (1234013) rather than matching subtype not_found, so
//     an unrelated not_found — e.g. a single bad draft ID — remains a
//     per-draft recoverable failure.
func isFatalSendErr(err error) bool {
	p, ok := errs.ProblemOf(err)
	if !ok {
		return true
	}
	switch p.Category {
	case errs.CategoryAuthentication, errs.CategoryAuthorization, errs.CategoryConfig, errs.CategoryNetwork, errs.CategoryInternal:
		return true
	}
	if p.Subtype == errs.SubtypeRateLimit || p.Subtype == errs.SubtypeQuotaExceeded {
		return true
	}
	switch p.Code {
	case 1234013, 1236007, 1236008, 1236009, 1236010, 1236013:
		return true
	}
	return false
}

// extractAutomationDisabledReason returns the human-readable reason when the
// send succeeded at HTTP level (code: 0) but the backend reports that
// automation send is disabled for this mailbox. An empty return value means
// automation send is enabled.
//
// The data["automation_send_disable"] payload is best-effort: a malformed
// shape or missing reason still produces a generic non-empty message so the
// caller can surface the disabled status to the user instead of silently
// continuing.
func extractAutomationDisabledReason(data map[string]interface{}) string {
	ad, ok := data["automation_send_disable"]
	if !ok {
		return ""
	}
	m, ok := ad.(map[string]interface{})
	if !ok {
		return "automation send disabled (no reason provided)"
	}
	if reason, ok := m["reason"].(string); ok && strings.TrimSpace(reason) != "" {
		return strings.TrimSpace(reason)
	}
	return "automation send disabled (no reason provided)"
}
