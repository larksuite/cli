// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package event

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"

	_ "github.com/larksuite/cli/events"
)

func TestRunList_TextOutput(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test"})

	if err := runList(f, false); err != nil {
		t.Fatalf("runList: %v", err)
	}

	out := stdout.String()
	for _, want := range []string{
		"KEY", "AUTH", "PARAMS", "DESCRIPTION",
		"im.message.receive_v1",
		"im.message.message_read_v1",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("list output missing %q; full output:\n%s", want, out)
		}
	}
}

func TestRunList_JSONOutput(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, &core.CliConfig{AppID: "test"})

	if err := runList(f, true); err != nil {
		t.Fatalf("runList json: %v", err)
	}

	var rows []map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, stdout.String())
	}
	if len(rows) == 0 {
		t.Fatal("expected at least one EventKey in JSON output")
	}

	for _, row := range rows {
		for _, field := range []string{"key", "event_type", "schema"} {
			if row[field] == nil {
				t.Errorf("row missing %q: %+v", field, row)
			}
		}
	}
}
