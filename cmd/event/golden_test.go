// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files instead of comparing")

// goldenSchemaKeys picks one key per rendering path so every branch of the
// list/schema output stays pinned: a processed key with a flat custom schema,
// a native key with field overrides, a callback key with a single consumer,
// and a key with a required parameter plus a pre-consume hook.
var goldenSchemaKeys = map[string]string{
	"schema_im_message_receive":  "im.message.receive_v1",
	"schema_im_chat_updated":     "im.chat.updated_v1",
	"schema_card_action_trigger": "card.action.trigger",
	"schema_board_whiteboard":    "board.whiteboard.updated_v1",
}

// The golden files pin stdout byte-for-byte. The output is deterministic:
// the snapshot keeps keys sorted, encoding/json sorts object keys, and nothing
// on the rendering path reads the clock or randomness. Regenerate with:
//
//	go test ./cmd/event/ -run TestGolden -update
func TestGolden_ListOutput(t *testing.T) {
	snap := compileCatalog()
	for name, asJSON := range map[string]bool{"list_text": false, "list_json": true} {
		t.Run(name, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test"})
			if err := runList(f, snap, "", asJSON); err != nil {
				t.Fatalf("runList: %v", err)
			}
			assertGolden(t, name, stdout.String())
		})
	}
}

func TestGolden_SchemaOutput(t *testing.T) {
	snap := compileCatalog()
	for name, key := range goldenSchemaKeys {
		for suffix, asJSON := range map[string]bool{"_text": false, "_json": true} {
			t.Run(name+suffix, func(t *testing.T) {
				f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test"})
				if err := runSchema(f, snap, key, asJSON); err != nil {
					t.Fatalf("runSchema(%s): %v", key, err)
				}
				assertGolden(t, name+suffix, stdout.String())
			})
		}
	}
}

func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", "golden", name+".golden")
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (regenerate with -update): %v", name, err)
	}
	if string(want) != got {
		t.Errorf("output drifted from golden %s\n--- want\n%s\n--- got\n%s", name, want, got)
	}
}
