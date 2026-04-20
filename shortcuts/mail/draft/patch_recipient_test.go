// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package draft

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// add_recipient — adding individual recipients
// ---------------------------------------------------------------------------

func TestAddRecipientAppendsNewAddress(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:      "add_recipient",
			Field:   "to",
			Name:    "Carol",
			Address: "carol@example.com",
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(snapshot.To) != 2 {
		t.Fatalf("To len = %d, want 2", len(snapshot.To))
	}
	if snapshot.To[1].Address != "carol@example.com" {
		t.Fatalf("To[1] = %#v", snapshot.To[1])
	}
}

func TestAddRecipientDeduplicatesExistingAddress(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:      "add_recipient",
			Field:   "to",
			Name:    "Bobby",
			Address: "BOB@example.com", // case-insensitive match
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(snapshot.To) != 1 {
		t.Fatalf("To len = %d, want 1 (dedup)", len(snapshot.To))
	}
}

func TestAddRecipientToCcField(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:      "add_recipient",
			Field:   "cc",
			Name:    "Carol",
			Address: "carol@example.com",
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(snapshot.Cc) != 1 || snapshot.Cc[0].Address != "carol@example.com" {
		t.Fatalf("Cc = %#v", snapshot.Cc)
	}
}

func TestAddRecipientToBccField(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:      "add_recipient",
			Field:   "bcc",
			Name:    "Hidden",
			Address: "hidden@example.com",
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(snapshot.Bcc) != 1 || snapshot.Bcc[0].Address != "hidden@example.com" {
		t.Fatalf("Bcc = %#v", snapshot.Bcc)
	}
}

func TestAddRecipientRejectsEmptyAddress(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:      "add_recipient",
			Field:   "to",
			Name:    "Nobody",
			Address: "  ",
		}},
	})
	if err == nil {
		t.Fatalf("expected error for empty address")
	}
}

// ---------------------------------------------------------------------------
// remove_recipient — removing individual recipients
// ---------------------------------------------------------------------------

func TestRemoveRecipientByAddress(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>, Carol <carol@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:      "remove_recipient",
			Field:   "to",
			Address: "bob@example.com",
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(snapshot.To) != 1 {
		t.Fatalf("To len = %d, want 1", len(snapshot.To))
	}
	if snapshot.To[0].Address != "carol@example.com" {
		t.Fatalf("To[0] = %#v", snapshot.To[0])
	}
}

func TestRemoveRecipientCaseInsensitive(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>, Carol <carol@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:      "remove_recipient",
			Field:   "to",
			Address: "BOB@EXAMPLE.COM",
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(snapshot.To) != 1 {
		t.Fatalf("To len = %d, want 1", len(snapshot.To))
	}
}

func TestRemoveRecipientNotFoundReturnsError(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:      "remove_recipient",
			Field:   "to",
			Address: "nonexistent@example.com",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("error = %v, want not found error", err)
	}
}

func TestRemoveRecipientFromEmptyFieldReturnsError(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:      "remove_recipient",
			Field:   "cc",
			Address: "bob@example.com",
		}},
	})
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("error = %v, want empty header error", err)
	}
}

func TestRemoveLastRecipientClearsHeader(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
Cc: Carol <carol@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:      "remove_recipient",
			Field:   "cc",
			Address: "carol@example.com",
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(snapshot.Cc) != 0 {
		t.Fatalf("Cc = %#v, want empty", snapshot.Cc)
	}
	if got := headerValue(snapshot.Headers, "Cc"); got != "" {
		t.Fatalf("Cc header still present: %q", got)
	}
}

// ---------------------------------------------------------------------------
// set_recipients to empty list clears header
// ---------------------------------------------------------------------------

func TestSetRecipientsEmptyListClearsHeader(t *testing.T) {
	snapshot := mustParseFixtureDraft(t, `Subject: Test
From: Alice <alice@example.com>
To: Bob <bob@example.com>
Cc: Carol <carol@example.com>
MIME-Version: 1.0
Content-Type: text/plain; charset=UTF-8

hello
`)
	err := Apply(&DraftCtx{FIO: testFIO}, snapshot, Patch{
		Ops: []PatchOp{{
			Op:        "set_recipients",
			Field:     "cc",
			Addresses: []Address{},
		}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if len(snapshot.Cc) != 0 {
		t.Fatalf("Cc = %#v, want empty", snapshot.Cc)
	}
}
