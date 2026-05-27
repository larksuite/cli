// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
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
//	  "failed":[{"draft_id":..., "error":...}]
//	}
//
// failed is marked omitempty so a fully successful batch returns a clean shape
// without an empty array.
type batchSendOutput struct {
	MailboxID    string        `json:"mailbox_id"`
	Total        int           `json:"total"`
	SuccessCount int           `json:"success_count"`
	FailureCount int           `json:"failure_count"`
	Sent         []sentDraft   `json:"sent"`
	Failed       []failedDraft `json:"failed,omitempty"`
}

// MailDraftSend is the `+draft-send` shortcut: send N existing drafts
// sequentially via POST /drafts/:draft_id/send, isolating per-draft failures.
// Risk is "high-risk-write"; callers must pass --yes. User identity only —
// drafts are user-owned resources and bot has no coherent semantics here.
//
// Output schema is the batchSendOutput type above. Partial failures (any
// failed[]) return exit 1 with envelope.error.type="partial_failure" so that
// agents can distinguish "all sent" from "some sent" without parsing the
// success_count field.
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
//     runtime.CallAPI. Per-draft outcomes:
//     - fatal err (isFatalSendErr) → return immediately (bypasses --stop-on-error).
//     - recoverable err → append to failed[]; honor --stop-on-error.
//     - success + automation_send_disable signal → return immediately with
//     ExitAPI/"automation_send_disabled".
//     - success → append to sent[].
//  4. Emit batchSendOutput via runtime.Out.
//  5. If any draft failed, return ExitAPI/"partial_failure" so exit code = 1.
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
		// Direct CallAPI rather than draftpkg.Send: this shortcut never sends
		// a body, so the helper's send_time-aware envelope would add no value.
		data, err := rt.CallAPI("POST",
			mailboxPath(mailboxID, "drafts", id, "send"), nil, nil)
		if err != nil {
			if isFatalSendErr(err) {
				writeDraftSendProgressf(rt, "[%d/%d] aborting after draft %s: %s",
					idx, len(draftIDs), sanitizeForSingleLine(id), sanitizeForSingleLine(err.Error()))
				hadProgress := out.hasProgress()
				out.Failed = append(out.Failed, failedDraft{DraftID: id, Error: err.Error()})
				if hadProgress {
					emitDraftSendOutput(rt, &out)
				}
				// Account- / mailbox-level failures (auth, permission, network,
				// quota) will repeat identically for every remaining draft —
				// abort immediately so the caller sees a single clear error
				// instead of 100 redundant failed[] entries.
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
			err := output.Errorf(output.ExitAPI, "automation_send_disabled",
				"automation send is disabled for this mailbox: %s", reason)
			writeDraftSendProgressf(rt, "[%d/%d] aborting after draft %s: %s",
				idx, len(draftIDs), sanitizeForSingleLine(id), sanitizeForSingleLine(err.Error()))
			if out.hasProgress() {
				out.Failed = append(out.Failed, failedDraft{DraftID: id, Error: err.Error()})
				emitDraftSendOutput(rt, &out)
			}
			// HTTP success (code: 0) but the backend signaled automation send
			// is disabled — every subsequent send will fail the same way, so
			// abort the batch with a single descriptive error.
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
	emitDraftSendOutput(rt, &out)

	if out.FailureCount == 0 {
		return nil
	}
	return output.Errorf(output.ExitAPI, "partial_failure",
		"%d of %d drafts failed to send", out.FailureCount, out.Total)
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
		return nil, output.ErrValidation("--draft-id is required")
	}

	normalized := make([]string, 0, len(draftIDs))
	seen := make(map[string]struct{}, len(draftIDs))
	for _, id := range draftIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			return nil, output.ErrValidation("--draft-id contains empty value")
		}
		if _, ok := seen[trimmed]; ok {
			return nil, output.ErrValidation("--draft-id contains duplicate value: %s", trimmed)
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	if len(normalized) > MaxBatchSendDrafts {
		return nil, output.ErrValidation(
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
//   - err does not unwrap to an *output.ExitError, or its Detail is missing:
//     unknown shapes are treated as fatal so they cannot accidentally
//     accumulate into failed[] for every remaining draft.
//   - Detail.Type ∈ {"auth", "app_status", "config", "permission",
//     "rate_limit", "network"}: token, scope, app-installation problems,
//     throttling, and connectivity are account-level.
//   - Code == output.ExitNetwork: connectivity loss is account-level.
//   - Detail.Code ∈ {LarkErrMailboxNotFound, LarkErrMailSendQuotaUser,
//     LarkErrMailSendQuotaUserExt, LarkErrMailSendQuotaTenantExt,
//     LarkErrMailQuota, LarkErrTenantStorageLimit}: mailbox / quota
//     exhaustion is account-level.
func isFatalSendErr(err error) bool {
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Detail == nil {
		return true
	}
	switch exitErr.Detail.Type {
	case "auth", "app_status", "config":
		return true
	case "permission", "rate_limit", "network":
		return true
	}
	if exitErr.Code == output.ExitNetwork || wrapsExitCode(err, output.ExitNetwork) {
		return true
	}
	switch exitErr.Detail.Code {
	case output.LarkErrMailboxNotFound,
		output.LarkErrMailSendQuotaUser,
		output.LarkErrMailSendQuotaUserExt,
		output.LarkErrMailSendQuotaTenantExt,
		output.LarkErrMailQuota,
		output.LarkErrTenantStorageLimit:
		return true
	}
	return false
}

func wrapsExitCode(err error, code int) bool {
	for unwrapped := errors.Unwrap(err); unwrapped != nil; unwrapped = errors.Unwrap(unwrapped) {
		if exitErr, ok := unwrapped.(*output.ExitError); ok && exitErr.Code == code {
			return true
		}
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
