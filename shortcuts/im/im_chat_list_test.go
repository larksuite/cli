// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// newChatListTestRuntimeContext mirrors newMessagesSearchTestRuntimeContext —
// it registers page-size as Int (the existing newTestRuntimeContext registers
// it as String, which would short-circuit our buildChatListParams logic).
func newChatListTestRuntimeContext(t *testing.T, stringFlags map[string]string, boolFlags map[string]bool) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Int("page-size", 20, "")
	for name := range stringFlags {
		if name == "page-size" {
			continue
		}
		cmd.Flags().String(name, "", "")
	}
	for name := range boolFlags {
		cmd.Flags().Bool(name, false, "")
	}
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	for name, val := range stringFlags {
		if err := cmd.Flags().Set(name, val); err != nil {
			t.Fatalf("Flags().Set(%q) error = %v", name, err)
		}
	}
	for name, val := range boolFlags {
		if err := cmd.Flags().Set(name, map[bool]string{true: "true", false: "false"}[val]); err != nil {
			t.Fatalf("Flags().Set(%q) error = %v", name, err)
		}
	}
	return &common.RuntimeContext{Cmd: cmd}
}

func TestBuildChatListParams_Defaults(t *testing.T) {
	rt := newChatListTestRuntimeContext(t, map[string]string{
		"user-id-type": "open_id",
		"sort-type":    "ByCreateTimeAsc",
	}, nil)
	got := buildChatListParams(rt)
	if got["user_id_type"] != "open_id" {
		t.Fatalf("user_id_type = %v", got["user_id_type"])
	}
	if got["sort_type"] != "ByCreateTimeAsc" {
		t.Fatalf("sort_type = %v", got["sort_type"])
	}
	if got["page_size"] != 20 {
		t.Fatalf("page_size = %v, want 20", got["page_size"])
	}
	if _, present := got["page_token"]; present {
		t.Fatalf("page_token should be omitted when empty")
	}
}

func TestBuildChatListParams_Overrides(t *testing.T) {
	rt := newChatListTestRuntimeContext(t, map[string]string{
		"user-id-type": "user_id",
		"sort-type":    "ByActiveTimeDesc",
		"page-size":    "50",
		"page-token":   "tok_xyz",
	}, nil)
	got := buildChatListParams(rt)
	if got["user_id_type"] != "user_id" {
		t.Fatalf("user_id_type = %v", got["user_id_type"])
	}
	if got["sort_type"] != "ByActiveTimeDesc" {
		t.Fatalf("sort_type = %v", got["sort_type"])
	}
	if got["page_size"] != 50 {
		t.Fatalf("page_size = %v, want 50", got["page_size"])
	}
	if got["page_token"] != "tok_xyz" {
		t.Fatalf("page_token = %v", got["page_token"])
	}
}

func TestImChatList_Validate_PageSizeBounds(t *testing.T) {
	cases := []struct {
		name     string
		pageSize string
		wantErr  bool
	}{
		{"zero rejected", "0", true},
		{"negative rejected", "-1", true},
		{"one ok", "1", false},
		{"hundred ok", "100", false},
		{"oneoone rejected", "101", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := newChatListTestRuntimeContext(t, map[string]string{"page-size": c.pageSize}, nil)
			err := ImChatList.Validate(context.Background(), rt)
			if (err != nil) != c.wantErr {
				t.Fatalf("Validate() err = %v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}

func TestImChatList_DryRun_IncludesEndpoint(t *testing.T) {
	rt := newChatListTestRuntimeContext(t, map[string]string{
		"user-id-type": "open_id",
		"sort-type":    "ByActiveTimeDesc",
		"page-size":    "30",
	}, nil)
	got := mustMarshalDryRun(t, ImChatList.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"/open-apis/im/v1/chats"`) {
		t.Fatalf("DryRun missing endpoint: %s", got)
	}
	if !strings.Contains(got, `"sort_type":"ByActiveTimeDesc"`) {
		t.Fatalf("DryRun missing sort_type: %s", got)
	}
	if !strings.Contains(got, `"page_size":30`) {
		t.Fatalf("DryRun missing page_size: %s", got)
	}
}
