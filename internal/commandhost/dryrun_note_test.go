// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package commandhost

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/extension/command"
)

// convertDryRun writes the bounded-repeat note into the projection it builds,
// never back into the DryRun the business hook returned. A hook may cache and
// return the same *DryRun on every invocation, so converting twice must not
// accumulate the note: the preview would then claim the command pages more
// times than it does.
func TestConvertDryRunNoteDoesNotAccumulateAcrossCalls(t *testing.T) {
	preview := command.NewDryRun(command.GET("/open-apis/im/v1/chats"))

	first := convertedDryRunJSON(t, preview)
	second := convertedDryRunJSON(t, preview)

	if first != second {
		t.Fatalf("second conversion differs:\nfirst:  %s\nsecond: %s", first, second)
	}
	if got := strings.Count(second, pageAllRepeatNote); got != 1 {
		t.Fatalf("repeat note appears %d times, want 1:\n%s", got, second)
	}
}

// The same holds when the hook supplied its own description: the note is joined
// onto a copy, so the business description must survive unchanged and must not
// grow a second note on the next conversion.
func TestConvertDryRunKeepsBusinessDescriptionIntact(t *testing.T) {
	preview := command.NewDryRun(command.GET("/open-apis/im/v1/chats").Desc("lists the first page"))

	first := convertedDryRunJSON(t, preview)
	second := convertedDryRunJSON(t, preview)

	if first != second {
		t.Fatalf("second conversion differs:\nfirst:  %s\nsecond: %s", first, second)
	}
	if got := strings.Count(second, "lists the first page"); got != 1 {
		t.Fatalf("business description appears %d times, want 1:\n%s", got, second)
	}

	view := command.InspectDryRun(preview)
	if got := view.Requests[len(view.Requests)-1].Description; got != "lists the first page" {
		t.Fatalf("business description was rewritten to %q", got)
	}
}

func convertedDryRunJSON(t *testing.T, preview *command.DryRun) string {
	t.Helper()
	converted, err := convertDryRun(preview, true)
	if err != nil {
		t.Fatal(err)
	}
	if converted == nil {
		t.Fatal("convertDryRun returned nil")
	}
	encoded, err := json.Marshal(converted)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
