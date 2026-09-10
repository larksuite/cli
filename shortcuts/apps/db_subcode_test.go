// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

// TestSubcodeOf covers the extraction contract: which messages yield a subcode,
// and what the stripped remainder looks like.
func TestSubcodeOf(t *testing.T) {
	cases := []struct {
		name     string
		message  string
		subcode  string
		stripped string
		ok       bool
	}{
		{
			name:     "full-width colon is the wire form",
			message:  "k_dl_4000004：forbid set audit log operation in online env",
			subcode:  "k_dl_4000004",
			stripped: "forbid set audit log operation in online env",
			ok:       true,
		},
		{
			name:     "ascii colon tolerated",
			message:  "k_dl_4000019:Audit is already enabled for table 'orders'",
			subcode:  "k_dl_4000019",
			stripped: "Audit is already enabled for table 'orders'",
			ok:       true,
		},
		{
			name:     "unmapped subcode still parses; classification decides later",
			message:  "k_dl_9999999：some future failure",
			subcode:  "k_dl_9999999",
			stripped: "some future failure",
			ok:       true,
		},
		{
			// Server sent the code with no wording after it. Still a valid subcode;
			// applyDBSubcode is what decides the message (see the empty-remainder test).
			name:     "subcode with empty remainder",
			message:  "k_dl_4000004：",
			subcode:  "k_dl_4000004",
			stripped: "",
			ok:       true,
		},
		{
			name:    "no marker",
			message: "System error, please contact the administrator",
			ok:      false,
		},
		{
			name:    "marker without digits",
			message: "k_dl_：malformed",
			ok:      false,
		},
		{
			name:    "digits without colon",
			message: "k_dl_4000004 forbid set audit log operation",
			ok:      false,
		},
		{
			// The decisive one: import errors echo user cell values verbatim, so a
			// row whose text merely contains the marker must not be classified.
			name:    "marker mid-message is not a prefix",
			message: "Row 1, column 'note': value 'k_dl_4000004：x' is not a valid integer",
			ok:      false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			subcode, stripped, ok := subcodeOf(tc.message)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (subcode=%q)", ok, tc.ok, subcode)
			}
			if !tc.ok {
				if stripped != tc.message {
					t.Fatalf("stripped = %q, want message unchanged on miss", stripped)
				}
				return
			}
			if subcode != tc.subcode {
				t.Fatalf("subcode = %q, want %q", subcode, tc.subcode)
			}
			if stripped != tc.stripped {
				t.Fatalf("stripped = %q, want %q", stripped, tc.stripped)
			}
		})
	}
}

// TestWithAppsHint_OnlineAuditBan is the reproduced defect: enabling audit on the
// online branch of a multi-env app. It must stop reading as an unknown API error
// advising the caller to check --app-id and --table, both of which are correct.
func TestWithAppsHint_OnlineAuditBan(t *testing.T) {
	cause := errors.New("upstream transport failure")
	in := errs.NewAPIError(errs.SubtypeUnknown, "k_dl_4000004：forbid set audit log operation in online env").
		WithCode(400002476).WithLogID("log_audit").WithCause(cause)
	out := withAppsHint(in, dbAuditSetHint)

	if !errors.Is(out, cause) {
		t.Errorf("cause chain lost: errors.Is(out, cause) = false")
	}
	p, ok := errs.ProblemOf(out)
	if !ok {
		t.Fatalf("withAppsHint returned untyped error: %T", out)
	}
	if p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("category/subtype = %s/%s, want validation/failed_precondition", p.Category, p.Subtype)
	}
	if !strings.Contains(p.Hint, "--environment dev") {
		t.Fatalf("hint = %q, must name the dev branch as the fix", p.Hint)
	}
	// The generic hint sends the caller to inspect two correct things.
	if strings.Contains(p.Hint, "verify --app-id") {
		t.Fatalf("hint = %q, must not fall back to the generic db hint", p.Hint)
	}
	// The ban is multi-env-only, so the hint must say which apps it applies to
	// rather than reading as "audit never works on online".
	if !strings.Contains(p.Hint, "multi-env") {
		t.Fatalf("hint = %q, must scope the ban to multi-env apps", p.Hint)
	}
	if strings.Contains(p.Message, dbSubcodePrefix) {
		t.Fatalf("message = %q, internal subcode prefix must be stripped once consumed", p.Message)
	}
	// Server-reported identity is preserved: the troubleshooter URL in the same
	// envelope is keyed on this code.
	if p.Code != 400002476 || p.LogID != "log_audit" {
		t.Fatalf("server-reported code/log_id mutated: %+v", p)
	}
}

// TestWithAppsHint_SubcodeTable covers the remaining observed subcodes, each of
// which shares code 400002476 with the ban above and with each other.
func TestWithAppsHint_SubcodeTable(t *testing.T) {
	cases := []struct {
		name     string
		message  string
		category errs.Category
		subtype  errs.Subtype
		hintHas  string
	}{
		{
			name:     "already enabled",
			message:  "k_dl_4000019：Audit is already enabled for table 'orders'",
			category: errs.CategoryAPI,
			subtype:  errs.SubtypeAlreadyExists,
			hintHas:  "+db-audit-status",
		},
		{
			name:     "not enabled",
			message:  "k_dl_4000020：Audit is not enabled for table 'orders'",
			category: errs.CategoryValidation,
			subtype:  errs.SubtypeFailedPrecondition,
			hintHas:  "nothing to disable",
		},
		{
			name:     "multi-env already initialized",
			message:  "k_dl_4000023：Multi-env is already initialized",
			category: errs.CategoryAPI,
			subtype:  errs.SubtypeAlreadyExists,
			hintHas:  "one-time",
		},
		{
			// Non-date column, to pin that the entry is type-agnostic.
			name:     "import type mismatch (integer)",
			message:  "k_dl_4000012：Row 1, column 'qty': value '不是数字' is not a valid integer",
			category: errs.CategoryValidation,
			subtype:  errs.SubtypeInvalidArgument,
			hintHas:  "re-import",
		},
		{
			// The date variant reaches the same subcode once the server maps the
			// invalid-datetime SQLSTATE alongside the invalid-text one.
			name:     "import type mismatch (date)",
			message:  "k_dl_4000012：Row 1, column 'ship_date': value '按铅改锂送仓时间' is not a valid date",
			category: errs.CategoryValidation,
			subtype:  errs.SubtypeInvalidArgument,
			hintHas:  "re-import",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Every entry rewrites Message wholesale, which is exactly when an
			// implementation that swapped the error value instead of mutating the
			// Problem in place would silently break errors.Is for callers. Mirrors
			// the guard on the no-database rewrite.
			cause := errors.New("upstream transport failure")
			in := errs.NewAPIError(errs.SubtypeUnknown, "%s", tc.message).WithCode(400002476).WithCause(cause)
			out := withAppsHint(in, dbAuditSetHint)
			if !errors.Is(out, cause) {
				t.Errorf("cause chain lost: errors.Is(out, cause) = false")
			}
			p, _ := errs.ProblemOf(out)
			if p.Category != tc.category || p.Subtype != tc.subtype {
				t.Fatalf("category/subtype = %s/%s, want %s/%s", p.Category, p.Subtype, tc.category, tc.subtype)
			}
			if !strings.Contains(p.Hint, tc.hintHas) {
				t.Fatalf("hint = %q, want it to mention %q", p.Hint, tc.hintHas)
			}
			if strings.Contains(p.Message, dbSubcodePrefix) {
				t.Fatalf("message = %q, prefix must be stripped", p.Message)
			}
		})
	}
}

// TestWithAppsHint_EmptyRemainderUsesTableMessage covers the server sending a
// subcode with nothing after it. Two things must hold at once: the internal prefix
// must not survive into the user-facing message, and the message must not be blanked
// — errs.Problem requires it, and an empty one makes Error() return "", which
// silently swallows the failure in any fmt.Errorf("...: %v", err).
func TestWithAppsHint_EmptyRemainderUsesTableMessage(t *testing.T) {
	for _, colon := range []string{"：", ":"} {
		t.Run("colon="+colon, func(t *testing.T) {
			cause := errors.New("upstream transport failure")
			in := errs.NewAPIError(errs.SubtypeUnknown, "%s", "k_dl_4000004"+colon).WithCode(400002476).WithCause(cause)
			out := withAppsHint(in, dbAuditSetHint)

			if !errors.Is(out, cause) {
				t.Errorf("cause chain lost: errors.Is(out, cause) = false")
			}
			p, _ := errs.ProblemOf(out)
			if p.Message == "" {
				t.Fatal("message was blanked; errs.Problem requires a non-empty Message")
			}
			if strings.Contains(p.Message, dbSubcodePrefix) {
				t.Fatalf("message = %q, internal prefix must not survive", p.Message)
			}
			if p.Message != dbSubcodeTable["k_dl_4000004"].Message {
				t.Fatalf("message = %q, want the table's fallback wording", p.Message)
			}
			// Classification still applies — the missing wording says nothing about
			// which failure this is.
			if p.Subtype != errs.SubtypeFailedPrecondition {
				t.Fatalf("subtype = %s, want failed_precondition", p.Subtype)
			}
		})
	}
}

// TestDBSubcodeTable_EveryEntryHasFallbackMessage stops a new entry from being added
// without one: without it, a server message of just `k_dl_<code>：` would leave the
// raw prefix in front of the user, which is the case this fallback exists to remove.
func TestDBSubcodeTable_EveryEntryHasFallbackMessage(t *testing.T) {
	for subcode, meta := range dbSubcodeTable {
		if strings.TrimSpace(meta.Message) == "" {
			t.Errorf("%s: Message is empty; add fallback wording for the no-remainder case", subcode)
		}
		if strings.Contains(meta.Message, dbSubcodePrefix) {
			t.Errorf("%s: Message %q must not embed the internal subcode prefix", subcode, meta.Message)
		}
	}
}

// TestWithAppsHint_UnmappedSubcodeDegrades pins the failure mode for a subcode the
// CLI does not know: today's behaviour, prefix intact. The prefix is the only clue
// left for diagnosing a case with no table entry, so stripping it would make an
// unknown failure harder to investigate, not easier.
func TestWithAppsHint_UnmappedSubcodeDegrades(t *testing.T) {
	in := errs.NewAPIError(errs.SubtypeUnknown, "k_dl_9999999：some future failure").WithCode(400002476)
	out := withAppsHint(in, dbAuditSetHint)

	p, _ := errs.ProblemOf(out)
	if p.Subtype != errs.SubtypeUnknown {
		t.Fatalf("subtype = %s, unmapped subcode must not be given a guessed classification", p.Subtype)
	}
	if p.Hint != dbAuditSetHint {
		t.Fatalf("hint = %q, want the per-command fallback", p.Hint)
	}
	if !strings.Contains(p.Message, "k_dl_9999999") {
		t.Fatalf("message = %q, unmapped subcode must stay visible for debugging", p.Message)
	}
}

// TestWithAppsHint_NoSubcodeUnaffected guards the blast radius: apps commands that
// never see a k_dl_ message keep the existing fallback behaviour exactly.
func TestWithAppsHint_NoSubcodeUnaffected(t *testing.T) {
	in := errs.NewAPIError(errs.SubtypeUnknown, "some unrelated failure").WithCode(400002476)
	out := withAppsHint(in, dbAuditSetHint)

	p, _ := errs.ProblemOf(out)
	if p.Subtype != errs.SubtypeUnknown || p.Hint != dbAuditSetHint {
		t.Fatalf("unrelated failure was altered: %+v", p)
	}
}

// TestWithAppsHint_NoDatabaseStillWins keeps the pre-existing override ahead of the
// subcode branch. The no-database failure carries its own code and message markers
// and must not be reinterpreted.
func TestWithAppsHint_NoDatabaseStillWins(t *testing.T) {
	in := errs.NewAPIError(errs.SubtypeUnknown, "get workspace id failed by app id").WithCode(400002465)
	out := withAppsHint(in, dbAuditSetHint)

	p, _ := errs.ProblemOf(out)
	if p.Message != appNoDatabaseMessage || p.Hint != appNoDatabaseHint {
		t.Fatalf("no-database override regressed: %+v", p)
	}
}
