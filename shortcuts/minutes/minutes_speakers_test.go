// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package minutes

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

func TestResolveSpeakerIDByName(t *testing.T) {
	speakers := []minuteSpeaker{
		{SpeakerID: "id_a", Name: "Alice"},
		{SpeakerID: "id_b", Name: "Bob"},
		{SpeakerID: "id_c", Name: "Alice"},
	}

	id, err := resolveSpeakerIDByName(speakers, "Bob")
	if err != nil || id != "id_b" {
		t.Fatalf("resolve Bob: id=%q err=%v", id, err)
	}

	_, err = resolveSpeakerIDByName(speakers, "Carol")
	if err == nil {
		t.Fatal("expected not found error")
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) || ve.Subtype != errs.SubtypeNotFound {
		t.Fatalf("want not-found validation error, got %T: %v", err, err)
	}

	_, err = resolveSpeakerIDByName(speakers, "Alice")
	if err == nil {
		t.Fatal("expected duplicate name error")
	}
	if !errors.As(err, &ve) || ve.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("want failed-precondition validation error, got %T: %v", err, err)
	}
	if !strings.Contains(ve.Hint, "id_a") || !strings.Contains(ve.Hint, "id_c") {
		t.Errorf("hint should list matching speaker_ids, got: %s", ve.Hint)
	}
}
