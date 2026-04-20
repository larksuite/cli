// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// set_reply_to / clear_reply_to
// ---------------------------------------------------------------------------

func TestApplySetReplyTo(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:        "set_reply_to",
			Addresses: []Address{{Name: "Support", Address: "support@example.com"}},
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(snapshot.ReplyTo) != 1 || snapshot.ReplyTo[0].Address != "support@example.com" {
		t.Fatalf("ReplyTo = %#v", snapshot.ReplyTo)
	}
}

func TestApplyClearReplyTo(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
Reply-To: Support <support@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	if len(snapshot.ReplyTo) == 0 {
		t.Fatalf("ReplyTo should be set before clear")
	}
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "clear_reply_to"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(snapshot.ReplyTo) != 0 {
		t.Fatalf("ReplyTo = %#v, want empty", snapshot.ReplyTo)
	}
	if got := headerValue(snapshot.Headers, "Reply-To"); got != "" {
		t.Fatalf("Reply-To header still present: %q", got)
	}
}

// ---------------------------------------------------------------------------
// remove_header
// ---------------------------------------------------------------------------

func TestApplyRemoveHeader(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
X-Priority: 1
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "remove_header", Name: "X-Priority"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := headerValue(snapshot.Headers, "X-Priority"); got != "" {
		t.Fatalf("X-Priority still present: %q", got)
	}
}

func TestApplyRemoveProtectedHeaderBlocked(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "remove_header", Name: "Content-Type"}},
	})
	if err == nil || !strings.Contains(err.Error(), "protected") {
		t.Fatalf("error = %v, want protected header error", err)
	}
}

func TestApplyRemoveProtectedHeaderWithOverride(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
Reply-To: Old <old@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops:     []PatchOp{{Op: "remove_header", Name: "Reply-To"}},
		Options: PatchOptions{AllowProtectedHeaderEdits: true},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if got := headerValue(snapshot.Headers, "Reply-To"); got != "" {
		t.Fatalf("Reply-To still present: %q", got)
	}
}

// ---------------------------------------------------------------------------
// set_header validation — name with colon, CR/LF
// ---------------------------------------------------------------------------

func TestApplySetHeaderRejectsNameWithColon(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_header", Name: "Bad:Name", Value: "value"}},
	})
	if err == nil || !strings.Contains(err.Error(), "must not contain") {
		t.Fatalf("error = %v, want header name rejection", err)
	}
}

func TestApplySetHeaderRejectsValueWithCRLF(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_header", Name: "X-Custom", Value: "val\r\ninjected"}},
	})
	if err == nil || !strings.Contains(err.Error(), "must not contain") {
		t.Fatalf("error = %v, want header value rejection", err)
	}
}

// ---------------------------------------------------------------------------
// set_subject with CR/LF
// ---------------------------------------------------------------------------

func TestApplySetSubjectRejectsCRLF(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "set_subject", Value: "Subject\ninjection"}},
	})
	if err == nil || !strings.Contains(err.Error(), "must not contain") {
		t.Fatalf("error = %v, want subject rejection", err)
	}
}

// ---------------------------------------------------------------------------
// unsupported op
// ---------------------------------------------------------------------------

func TestApplyUnsupportedOpReturnsError(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{Op: "unknown_op"}},
	})
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("error = %v, want unsupported op error", err)
	}
}
