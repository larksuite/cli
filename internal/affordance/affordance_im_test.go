// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package affordance

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// The 21 im raw-API methods that affordance/im.md must cover: 17 first-batch
// methods plus 4 "prefer the shortcut" entries. Keys follow the parsed heading
// form (spaces become dots), same as TestFor's fixture keys.
var imAffordanceMethods = []string{
	"chat.members.create", "chat.members.delete", "chat.members.get", "chat.members.bots",
	"messages.forward", "messages.delete", "messages.merge_forward", "messages.read_users",
	"reactions.create", "reactions.delete", "reactions.list", "reactions.batch_query",
	"pins.create", "pins.delete", "pins.list",
	"images.create",
	"threads.forward",
	"chats.get", "chats.update", "chats.create", "chats.link",
}

type parsedAffordance struct {
	UseWhen       []string `json:"use_when"`
	AvoidWhen     []string `json:"avoid_when"`
	Prerequisites []string `json:"prerequisites"`
	Examples      []struct {
		Command string `json:"command"`
	} `json:"examples"`
}

// TestForIMRealFile parses the real affordance/im.md through the production
// parser and asserts coverage plus depth on the showcase method.
func TestForIMRealFile(t *testing.T) {
	prev := mdSource
	t.Cleanup(func() { SetSource(prev) })
	SetSource(os.DirFS("../../affordance"))

	for _, m := range imAffordanceMethods {
		raw, ok := For("im", m)
		if !ok {
			t.Errorf("For(\"im\", %q) ok=false, want an overlay section in affordance/im.md", m)
			continue
		}
		var a parsedAffordance
		if err := json.Unmarshal(raw, &a); err != nil {
			t.Errorf("%s: overlay is not valid affordance JSON: %v", m, err)
			continue
		}
		if len(a.UseWhen) == 0 {
			t.Errorf("%s: missing lead paragraph (use_when)", m)
		}
		if len(a.AvoidWhen) == 0 {
			t.Errorf("%s: missing Avoid when section", m)
		}
		if len(a.Examples) == 0 || a.Examples[0].Command == "" {
			t.Errorf("%s: missing fenced example command", m)
			continue
		}
		// Each example must invoke the section's own command, so a heading
		// can't silently drift apart from the command its examples show.
		// Normalize the example's command words (before the first flag) the
		// same way headings become keys: spaces join with dots.
		words := strings.Fields(strings.TrimPrefix(a.Examples[0].Command, "lark-cli im "))
		var cmdWords []string
		for _, w := range words {
			if strings.HasPrefix(w, "-") {
				break
			}
			cmdWords = append(cmdWords, w)
		}
		if got := strings.Join(cmdWords, "."); got != m {
			t.Errorf("%s: first example %q invokes %q, want the section's own command", m, a.Examples[0].Command, got)
		}
	}

	// Showcase depth: messages forward (the deepest overlay section).
	raw, ok := For("im", "messages.forward")
	if !ok {
		t.Fatal("messages.forward overlay missing")
	}
	var fwd parsedAffordance
	if err := json.Unmarshal(raw, &fwd); err != nil {
		t.Fatalf("messages.forward overlay invalid: %v", err)
	}
	if len(fwd.AvoidWhen) < 3 {
		t.Errorf("messages.forward: want >=3 avoid_when entries, got %d", len(fwd.AvoidWhen))
	}
	if len(fwd.Prerequisites) < 2 {
		t.Errorf("messages.forward: want >=2 prerequisites, got %d", len(fwd.Prerequisites))
	}
	if len(fwd.Examples) < 1 || fwd.Examples[0].Command == "" {
		t.Errorf("messages.forward: want >=1 fenced example command")
	}
}
