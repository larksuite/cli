// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"

	"github.com/larksuite/cli/errs"
)

// The db OpenAPI collapses every dataloom business failure into one of two codes.
// paas_integration classifies by the FIRST DIGIT of dataloom's 7-digit code —
// 4xxxxxx ("caller's problem", not counted against SLA) becomes 400002476, and
// 5xxxxxx becomes 500002776. That split exists for SLA attribution, and it is the
// only thing the numeric code carries: a dozen unrelated business states share one
// number. The specific reason survives only as a `k_dl_<7 digits>` prefix on the
// server message.
//
// So the CLI cannot classify these on p.Code — every one of them would land on the
// same subtype. It classifies on the subcode instead, which is a backend contract
// (one code, one scenario) rather than the free-text English that follows it.
//
// This table is deliberately keyed on the subcode STRING, not on where the subcode
// was read from. Today the only channel is the message prefix; when the gateway
// exposes the subcode as a structured field, only subcodeOf changes — the table,
// the classifications and the hints stay put.
//
// Entries are added from observed wire behaviour, not from reading dataloom's
// constant list: an unmapped subcode degrades to today's behaviour (subtype
// unknown, generic hint, prefix left visible for debugging), which is honest,
// whereas a guessed classification is not.
type dbSubcodeMeta struct {
	Category errs.Category
	Subtype  errs.Subtype
	Hint     string

	// Retryable marks a failure the caller can resolve by repeating the same
	// request unchanged. Only for genuinely transient server-side conditions —
	// never for anything the caller must change first, which would send an agent
	// into a loop that cannot succeed.
	Retryable bool

	// Message is used only when the server sends the subcode with no wording after
	// it (`k_dl_4000004：` and nothing else) — seen if an i18n lookup resolves to an
	// empty string. It cannot be skipped by simply assigning the empty remainder:
	// errs.Problem documents Message as REQUIRED, and an empty one makes Error()
	// return "", which silently swallows the failure in any fmt.Errorf("...: %v").
	// Leaving the raw prefix instead would put internal vocabulary in front of
	// users, so each entry carries its own wording, phrased to match what the
	// server normally sends for that subcode.
	Message string
}

// dbSubcodeTable maps stable dataloom subcodes to a classification and a hint that
// names the actual fix.
//
// Hints state the product rule rather than framing the working environment as a
// retry trick, matching dbSyncOnlineDDLHint: the request is not malformed, the
// target environment is what is wrong, so "verify --app-id and --table" (the
// generic db hint) sends the caller to inspect two things that are both correct.
var dbSubcodeTable = map[string]dbSubcodeMeta{
	// Multi-env apps forbid changing audit settings on the online branch
	// (dataloom ErrOnlineEnvForbidAuditLog, thrown only when
	// env==online && expert mode && multi-env workspace).
	//
	// FailedPrecondition, mirroring the sibling online-DDL ban: the request is
	// valid, the environment state is not, and retrying is guaranteed to fail.
	// Exit 2 rather than 1 tells an agent to change the request, not to back off.
	//
	// "a multi-env app" is the scope qualifier, not filler: the ban never fires for
	// single-env apps, whose only branch is online and where audit works normally.
	"k_dl_4000004": {
		Category: errs.CategoryValidation,
		Subtype:  errs.SubtypeFailedPrecondition,
		Hint:     "a multi-env app only allows audit settings to be changed on the dev branch. Rerun with `--environment dev`.",
		Message:  "changing audit settings is not allowed on the online branch",
	},

	// Enabling audit on a table that already has it on. Idempotency violation:
	// nothing changed, and the caller usually wants to read the current setting.
	"k_dl_4000019": {
		Category: errs.CategoryAPI,
		Subtype:  errs.SubtypeAlreadyExists,
		Hint:     "audit is already enabled for this table and nothing was changed. Inspect the current retention with `lark-cli apps +db-audit-status --app-id <app_id> --table <table>`.",
		Message:  "audit is already enabled for this table",
	},

	// Disabling audit on a table that never had it on.
	"k_dl_4000020": {
		Category: errs.CategoryValidation,
		Subtype:  errs.SubtypeFailedPrecondition,
		Hint:     "audit is not enabled for this table, so there is nothing to disable. List the tables that do have it on with `lark-cli apps +db-audit-status --app-id <app_id>`.",
		Message:  "audit is not enabled for this table",
	},

	// +db-data-import hit a cell whose value does not fit the target column's type.
	// The server already names the row, column, value and expected type in the
	// message, so the hint only has to point at it.
	//
	// InvalidArgument, not FailedPrecondition: nothing about the database state is
	// wrong — the file the caller supplied is. Exit 2 says "fix the input", which is
	// exactly the action, and separates it from the generic api/unknown exit 1 that
	// reads as "something went wrong server-side, maybe retry". Retrying an
	// unchanged file always fails.
	//
	// Covers every column type, not just dates: the server maps both the
	// invalid-text and invalid-datetime cases here.
	"k_dl_4000012": {
		Category: errs.CategoryValidation,
		Subtype:  errs.SubtypeInvalidArgument,
		Hint:     "the message names the row, column and expected type. Correct that value in the data file and re-import.",
		Message:  "a value in the import file does not match its column type",
	},

	// The app's data-sync (DTS) task could not be initialized while turning table
	// audit on or off. Two distinct causes share this subcode: a concurrent request
	// holding the initialization lock, and the remote sync task sitting in a
	// terminal state. Both are server-side and transient from the caller's side —
	// the request itself is well-formed — so the classification covers both and the
	// hint does not tell the caller to change anything about it.
	//
	// The lock variant is retried in-process first (see db_dts_retry.go); reaching
	// this classification means either that retry was exhausted or the cause was the
	// terminal-state one.
	//
	// NOT marked retryable at the table level, even though one of its two causes is.
	// Retryable is a machine-readable promise that repeating the call unchanged can
	// succeed, and only the lock variant has been observed to behave that way — it is
	// promoted in applyDBSubcode once the message identifies it. A task stuck in a
	// terminal state is not fixed by calling again, and claiming otherwise would walk
	// an agent through a round of attempts that cannot work, against a write endpoint.
	// Asserting it for a variant whose wire message this package has never seen would
	// be the guessed classification the table exists to avoid.
	"k_dl_1600039": {
		Category: errs.CategoryAPI,
		Subtype:  errs.SubtypeServerError,
		Hint: "the app's data-sync task could not be initialized. This is server-side and not a problem with the request — try once more, " +
			"and if it repeats, the app's data-sync task needs attention rather than another attempt.",
		Message: "the app's data-sync task could not be initialized",
	},

	// +db-env-create on an app whose dev/online split already exists. The split is
	// one-time and irreversible, so this is a no-op, not a retryable failure.
	"k_dl_4000023": {
		Category: errs.CategoryAPI,
		Subtype:  errs.SubtypeAlreadyExists,
		Hint:     "this app already has dev and online branches; splitting is a one-time operation. Inspect the dev branch with `lark-cli apps +db-table-list --app-id <app_id> --environment dev`.",
		Message:  "multi-env is already initialized for this app",
	},
}

// dbSubcodePrefix is the marker dataloom stamps on the message ahead of its
// business code.
const dbSubcodePrefix = "k_dl_"

// subcodeOf extracts the dataloom subcode from a server message and returns it
// along with the message stripped of the `k_dl_<digits><colon>` prefix.
//
// Both colon forms are accepted: the wire uses the full-width "：" (U+FF1A), and
// the ASCII ":" is tolerated so a server-side punctuation change does not silently
// drop every classification below.
//
// Only a genuine prefix counts. Scanning the whole message for the marker would
// misfire on messages that quote user input — table names and imported cell values
// are echoed back verbatim, so a row containing the literal text "k_dl_4000004"
// must not be classified as the online-audit ban.
func subcodeOf(message string) (subcode, stripped string, ok bool) {
	rest, found := strings.CutPrefix(message, dbSubcodePrefix)
	if !found {
		return "", message, false
	}
	digits := 0
	for digits < len(rest) && rest[digits] >= '0' && rest[digits] <= '9' {
		digits++
	}
	if digits == 0 {
		return "", message, false
	}
	subcode = dbSubcodePrefix + rest[:digits]
	tail := rest[digits:]
	for _, colon := range []string{"：", ":"} {
		if after, cut := strings.CutPrefix(tail, colon); cut {
			return subcode, strings.TrimSpace(after), true
		}
	}
	return "", message, false
}

// applyDBSubcode reclassifies a typed failure whose message carries a known
// dataloom subcode, and reports whether it did.
//
// On a match it rewrites four things and returns true:
//   - Category / Subtype — so the failure stops reading as "unknown" and the
//     process exit code reflects what the caller should do about it;
//   - Hint — the subcode names one scenario, so its guidance is strictly better
//     than the per-command fallback and replaces it even when one is already set;
//   - Message — the `k_dl_...：` prefix is internal vocabulary; it is dropped once
//     it has been consumed, matching how the no-database failure rewrites the
//     server's internal wording for users.
//
// p.Code is deliberately NOT rewritten to the subcode's digits. 400002476 is what
// the server actually returned and what its troubleshooter URL in the same
// envelope is keyed on; replacing it would make the two disagree and would report
// a code the server never sent.
//
// An unmapped subcode is left completely alone — including its prefix, which stays
// visible precisely because it is the only remaining clue for a case the CLI does
// not yet understand.
func applyDBSubcode(p *errs.Problem) bool {
	if p == nil {
		return false
	}
	subcode, stripped, ok := subcodeOf(p.Message)
	if !ok {
		return false
	}
	meta, known := dbSubcodeTable[subcode]
	if !known {
		return false
	}
	p.Category = meta.Category
	p.Subtype = meta.Subtype
	p.Hint = meta.Hint
	p.Retryable = meta.Retryable

	// Server wording wins when there is any; otherwise the table's own (see
	// dbSubcodeMeta.Message). Never assign the empty remainder — Message is
	// required, and blanking it would make Error() return "".
	if stripped == "" {
		stripped = meta.Message
	}
	p.Message = stripped

	// Applied last, after the message assignment above, so the variant wording is
	// not overwritten by the server text it is meant to replace.
	//
	// dtsInitFailedSubcode carries two causes with the same classification but
	// different things worth telling the caller. Naming the one the server's wording
	// identifies is the difference between a user thinking their command was
	// rejected and knowing it merely collided with a concurrent one. The server's
	// own text is replaced for the same reason the no-database failure's is —
	// "lock already held, workspace: …, branch: …" is internal vocabulary; the
	// original stays available via log_id / troubleshooter.
	// Retryable is promoted here rather than carried on the entry: contention is the
	// one cause of this subcode observed to clear on its own, so it is the only one
	// the flag may promise anything about. See the entry's comment.
	if subcode == dtsInitFailedSubcode && strings.Contains(stripped, dtsLockHeldMarker) {
		p.Message = dtsLockContentionMessage
		p.Hint = dtsLockContentionHint
		p.Retryable = true
	}
	return true
}
