// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"strconv"
	"strings"
	"testing"

	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestParseItemType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    ItemType
		wantErr bool
	}{
		{name: "default", input: "default", want: ItemTypeDefault},
		{name: "message alias", input: "message", want: ItemTypeDefault},
		{name: "empty string defaults to default", input: "", want: ItemTypeDefault},
		{name: "thread", input: "thread", want: ItemTypeThread},
		{name: "msg_thread", input: "msg_thread", want: ItemTypeMsgThread},
		{name: "case insensitive", input: "THREAD", want: ItemTypeThread},
		{name: "with whitespace", input: " thread ", want: ItemTypeThread},
		{name: "invalid", input: "invalid_type", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseItemType(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseItemType(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseItemType(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("parseItemType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseFlagType(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    FlagType
		wantErr bool
	}{
		{name: "message", input: "message", want: FlagTypeMessage},
		{name: "empty string defaults to message", input: "", want: FlagTypeMessage},
		{name: "feed", input: "feed", want: FlagTypeFeed},
		{name: "unknown", input: "unknown", want: FlagTypeUnknown},
		{name: "case insensitive", input: "FEED", want: FlagTypeFeed},
		{name: "with whitespace", input: " feed ", want: FlagTypeFeed},
		{name: "invalid", input: "invalid_type", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFlagType(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseFlagType(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFlagType(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("parseFlagType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseItemID(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantIT     ItemType
		wantFT     FlagType
		wantErr    bool
		errContain string
	}{
		{name: "om prefix", input: "om_abc123", wantIT: ItemTypeDefault, wantFT: FlagTypeMessage},
		{name: "omt prefix", input: "omt_xyz789", wantIT: ItemTypeThread, wantFT: FlagTypeFeed},
		{name: "with whitespace", input: " om_abc123 ", wantIT: ItemTypeDefault, wantFT: FlagTypeMessage},
		{name: "empty string", input: "", wantErr: true, errContain: "cannot be empty"},
		{name: "unknown prefix", input: "oc_xxx", wantErr: true, errContain: "cannot infer"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotIT, gotFT, err := parseItemID(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseItemID(%q) expected error, got nil", tt.input)
				}
				if tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
					t.Fatalf("parseItemID(%q) error = %q, want to contain %q", tt.input, err.Error(), tt.errContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseItemID(%q) unexpected error: %v", tt.input, err)
			}
			if gotIT != tt.wantIT {
				t.Fatalf("parseItemID(%q) itemType = %v, want %v", tt.input, gotIT, tt.wantIT)
			}
			if gotFT != tt.wantFT {
				t.Fatalf("parseItemID(%q) flagType = %v, want %v", tt.input, gotFT, tt.wantFT)
			}
		})
	}
}

func TestIsValidCombo(t *testing.T) {
	tests := []struct {
		name string
		it   ItemType
		ft   FlagType
		want bool
	}{
		{name: "default+message valid", it: ItemTypeDefault, ft: FlagTypeMessage, want: true},
		{name: "thread+feed valid", it: ItemTypeThread, ft: FlagTypeFeed, want: true},
		{name: "msg_thread+feed valid", it: ItemTypeMsgThread, ft: FlagTypeFeed, want: true},
		{name: "default+feed invalid", it: ItemTypeDefault, ft: FlagTypeFeed, want: false},
		{name: "thread+message invalid", it: ItemTypeThread, ft: FlagTypeMessage, want: false},
		{name: "msg_thread+message invalid", it: ItemTypeMsgThread, ft: FlagTypeMessage, want: false},
		{name: "unknown flag type", it: ItemTypeDefault, ft: FlagTypeUnknown, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isValidCombo(tt.it, tt.ft); got != tt.want {
				t.Fatalf("isValidCombo(%v, %v) = %v, want %v", tt.it, tt.ft, got, tt.want)
			}
		})
	}
}

func TestNewFlagItem(t *testing.T) {
	tests := []struct {
		name     string
		itemID   string
		it       ItemType
		ft       FlagType
		wantJSON string
	}{
		{
			name:     "default+message",
			itemID:   "om_abc123",
			it:       ItemTypeDefault,
			ft:       FlagTypeMessage,
			wantJSON: `{"item_id":"om_abc123","item_type":"0","flag_type":"2"}`,
		},
		{
			name:     "thread+feed",
			itemID:   "omt_xyz789",
			it:       ItemTypeThread,
			ft:       FlagTypeFeed,
			wantJSON: `{"item_id":"omt_xyz789","item_type":"4","flag_type":"1"}`,
		},
		{
			name:     "msg_thread+feed",
			itemID:   "om_123",
			it:       ItemTypeMsgThread,
			ft:       FlagTypeFeed,
			wantJSON: `{"item_id":"om_123","item_type":"11","flag_type":"1"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newFlagItem(tt.itemID, tt.it, tt.ft)
			if got.ItemID != tt.itemID {
				t.Fatalf("newFlagItem().ItemID = %q, want %q", got.ItemID, tt.itemID)
			}
			if got.ItemType != stringInt(int(tt.it)) {
				t.Fatalf("newFlagItem().ItemType = %q, want %q", got.ItemType, stringInt(int(tt.it)))
			}
			if got.FlagType != stringInt(int(tt.ft)) {
				t.Fatalf("newFlagItem().FlagType = %q, want %q", got.FlagType, stringInt(int(tt.ft)))
			}
		})
	}
}

func TestAlternateFeedItemType(t *testing.T) {
	tests := []struct {
		name     string
		items    []flagItem
		wantNil  bool
		wantItem flagItem // the swapped item, if any
	}{
		{
			name: "swap thread to msg_thread",
			items: []flagItem{
				newFlagItem("omt_123", ItemTypeThread, FlagTypeFeed),
			},
			wantNil: false,
			wantItem: flagItem{
				ItemID:   "omt_123",
				ItemType: "11", // ItemTypeMsgThread
				FlagType: "1",  // FlagTypeFeed
			},
		},
		{
			name: "swap msg_thread to thread",
			items: []flagItem{
				newFlagItem("om_123", ItemTypeMsgThread, FlagTypeFeed),
			},
			wantNil: false,
			wantItem: flagItem{
				ItemID:   "om_123",
				ItemType: "4", // ItemTypeThread
				FlagType: "1", // FlagTypeFeed
			},
		},
		{
			name: "no feed item to swap",
			items: []flagItem{
				newFlagItem("om_123", ItemTypeDefault, FlagTypeMessage),
			},
			wantNil: true,
		},
		{
			name: "multiple items with one swap",
			items: []flagItem{
				newFlagItem("om_123", ItemTypeDefault, FlagTypeMessage),
				newFlagItem("omt_456", ItemTypeThread, FlagTypeFeed),
			},
			wantNil: false,
			wantItem: flagItem{
				ItemID:   "omt_456",
				ItemType: "11", // ItemTypeMsgThread
				FlagType: "1",
			},
		},
		{
			name:    "empty list",
			items:   []flagItem{},
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := alternateFeedItemType(tt.items)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("alternateFeedItemType() = %v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("alternateFeedItemType() = nil, want non-nil")
			}
			// Find the swapped item
			for _, item := range got {
				if item.FlagType == "1" { // feed type
					if item.ItemID != tt.wantItem.ItemID ||
						item.ItemType != tt.wantItem.ItemType ||
						item.FlagType != tt.wantItem.FlagType {
						t.Fatalf("alternateFeedItemType() swapped item = %v, want %v", item, tt.wantItem)
					}
					return
				}
			}
			t.Fatalf("alternateFeedItemType() no feed item found in result")
		})
	}
}

func TestParseItemTypeFromRaw(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  ItemType
	}{
		{name: "default", input: "0", want: ItemTypeDefault},
		{name: "thread", input: "4", want: ItemTypeThread},
		{name: "msg_thread", input: "11", want: ItemTypeMsgThread},
		{name: "unknown defaults to default", input: "999", want: ItemTypeDefault},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseItemTypeFromRaw(tt.input); got != tt.want {
				t.Fatalf("parseItemTypeFromRaw(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestMustParseItemType(t *testing.T) {
	tests := []struct {
		input string
		want  ItemType
	}{
		{input: "0", want: ItemTypeDefault},
		{input: "4", want: ItemTypeThread},
		{input: "11", want: ItemTypeMsgThread},
		{input: "999", want: ItemTypeDefault}, // unknown defaults to default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := mustParseItemType(tt.input); got != tt.want {
				t.Fatalf("mustParseItemType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestMustParseFlagType(t *testing.T) {
	tests := []struct {
		input string
		want  FlagType
	}{
		{input: "1", want: FlagTypeFeed},
		{input: "2", want: FlagTypeMessage},
		{input: "999", want: FlagTypeUnknown}, // unknown
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := mustParseFlagType(tt.input); got != tt.want {
				t.Fatalf("mustParseFlagType(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// helper
func stringInt(v int) string {
	return strconv.Itoa(v)
}

func TestBuildCreateItem(t *testing.T) {
	tests := []struct {
		name       string
		flags      map[string]string
		wantItem   flagItem
		wantErr    bool
		errContain string
	}{
		{
			name: "message id defaults to message type",
			flags: map[string]string{
				"message-id": "om_abc123",
			},
			wantItem: newFlagItem("om_abc123", ItemTypeDefault, FlagTypeMessage),
		},
		{
			name: "thread id defaults to message type",
			flags: map[string]string{
				"thread-id": "omt_xyz789",
			},
			wantItem: newFlagItem("omt_xyz789", ItemTypeDefault, FlagTypeMessage),
		},
		{
			name: "explicit item-type and flag-type",
			flags: map[string]string{
				"message-id": "omt_xyz789",
				"item-type":  "thread",
				"flag-type":  "feed",
			},
			wantItem: newFlagItem("omt_xyz789", ItemTypeThread, FlagTypeFeed),
		},
		{
			name: "explicit msg_thread type",
			flags: map[string]string{
				"message-id": "om_abc",
				"item-type":  "msg_thread",
				"flag-type":  "feed",
			},
			wantItem: newFlagItem("om_abc", ItemTypeMsgThread, FlagTypeFeed),
		},
		{
			name: "missing message-id and thread-id",
			flags: map[string]string{
				"item-type": "default",
			},
			wantErr:    true,
			errContain: "--message-id or --thread-id is required",
		},
		{
			name: "only item-type without flag-type",
			flags: map[string]string{
				"message-id": "om_abc",
				"item-type":  "thread",
			},
			wantErr:    true,
			errContain: "--item-type and --flag-type must be provided together",
		},
		{
			name: "only flag-type without item-type",
			flags: map[string]string{
				"message-id": "om_abc",
				"flag-type":  "feed",
			},
			wantErr:    true,
			errContain: "--item-type and --flag-type must be provided together",
		},
		{
			name: "invalid item-type",
			flags: map[string]string{
				"message-id": "om_abc",
				"item-type":  "invalid",
				"flag-type":  "feed",
			},
			wantErr:    true,
			errContain: "invalid --item-type",
		},
		{
			name: "invalid flag-type",
			flags: map[string]string{
				"message-id": "om_abc",
				"item-type":  "thread",
				"flag-type":  "invalid",
			},
			wantErr:    true,
			errContain: "invalid --flag-type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			for name := range tt.flags {
				cmd.Flags().String(name, "", "")
			}
			if err := cmd.ParseFlags(nil); err != nil {
				t.Fatalf("ParseFlags() error = %v", err)
			}
			for name, val := range tt.flags {
				if err := cmd.Flags().Set(name, val); err != nil {
					t.Fatalf("Flags().Set(%q) error = %v", name, err)
				}
			}
			runtime := &common.RuntimeContext{Cmd: cmd}

			got, err := buildCreateItem(runtime)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("buildCreateItem() expected error, got nil")
				}
				if tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
					t.Fatalf("buildCreateItem() error = %q, want to contain %q", err.Error(), tt.errContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("buildCreateItem() unexpected error: %v", err)
			}
			if got.ItemID != tt.wantItem.ItemID {
				t.Fatalf("buildCreateItem().ItemID = %q, want %q", got.ItemID, tt.wantItem.ItemID)
			}
			if got.ItemType != tt.wantItem.ItemType {
				t.Fatalf("buildCreateItem().ItemType = %q, want %q", got.ItemType, tt.wantItem.ItemType)
			}
			if got.FlagType != tt.wantItem.FlagType {
				t.Fatalf("buildCreateItem().FlagType = %q, want %q", got.FlagType, tt.wantItem.FlagType)
			}
		})
	}
}

func TestBuildCancelItemsDryRun(t *testing.T) {
	tests := []struct {
		name       string
		flags      map[string]string
		wantLen    int
		wantDouble bool
		wantErr    bool
	}{
		{
			name:       "om prefix dry-run assumes double-cancel",
			flags:      map[string]string{"message-id": "om_abc"},
			wantLen:    2,
			wantDouble: true,
		},
		{
			name:       "omt prefix dry-run double-cancel",
			flags:      map[string]string{"thread-id": "omt_xyz"},
			wantLen:    2,
			wantDouble: true,
		},
		{
			name:       "explicit flag-type single cancel",
			flags:      map[string]string{"message-id": "om_abc", "flag-type": "message"},
			wantLen:    1,
			wantDouble: false,
		},
		{
			name:       "explicit item-type single cancel",
			flags:      map[string]string{"message-id": "om_abc", "item-type": "default"},
			wantLen:    1,
			wantDouble: false,
		},
		{
			name:    "missing id",
			flags:   map[string]string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{Use: "test"}
			for name := range tt.flags {
				cmd.Flags().String(name, "", "")
			}
			if err := cmd.ParseFlags(nil); err != nil {
				t.Fatalf("ParseFlags() error = %v", err)
			}
			for name, val := range tt.flags {
				if err := cmd.Flags().Set(name, val); err != nil {
					t.Fatalf("Flags().Set(%q) error = %v", name, err)
				}
			}
			runtime := &common.RuntimeContext{Cmd: cmd}

			got, isDouble, err := buildCancelItemsDryRun(runtime)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("buildCancelItemsDryRun() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildCancelItemsDryRun() unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("buildCancelItemsDryRun() returned %d items, want %d", len(got), tt.wantLen)
			}
			if isDouble != tt.wantDouble {
				t.Fatalf("buildCancelItemsDryRun() isDouble = %v, want %v", isDouble, tt.wantDouble)
			}
		})
	}
}

func TestBuildSingleCancelItem(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		itOverride string
		ftOverride string
		wantIT     ItemType
		wantFT     FlagType
		wantErr    bool
	}{
		{
			name:   "om id infers default+message",
			id:     "om_abc",
			wantIT: ItemTypeDefault,
			wantFT: FlagTypeMessage,
		},
		{
			name:   "omt id infers thread+feed",
			id:     "omt_xyz",
			wantIT: ItemTypeThread,
			wantFT: FlagTypeFeed,
		},
		{
			name:       "explicit override",
			id:         "omt_xyz",
			itOverride: "msg_thread",
			ftOverride: "feed",
			wantIT:     ItemTypeMsgThread,
			wantFT:     FlagTypeFeed,
		},
		{
			name:       "only item-type override infers flag-type",
			id:         "om_abc",
			itOverride: "thread",
			wantIT:     ItemTypeThread,
			wantFT:     FlagTypeMessage, // inferred from om_ prefix
		},
		{
			name:       "only flag-type override infers item-type",
			id:         "omt_xyz",
			ftOverride: "message",
			wantIT:     ItemTypeThread, // inferred from omt_ prefix
			wantFT:     FlagTypeMessage,
		},
		{
			name:    "empty id",
			id:      "",
			wantErr: true,
		},
		{
			name:       "invalid item-type override",
			id:         "om_abc",
			itOverride: "invalid",
			wantErr:    true,
		},
		{
			name:       "invalid flag-type override",
			id:         "om_abc",
			ftOverride: "invalid",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildSingleCancelItem(tt.id, tt.itOverride, tt.ftOverride)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("buildSingleCancelItem() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildSingleCancelItem() unexpected error: %v", err)
			}
			if got.ItemID != tt.id {
				t.Fatalf("buildSingleCancelItem().ItemID = %q, want %q", got.ItemID, tt.id)
			}
			if got.ItemType != stringInt(int(tt.wantIT)) {
				t.Fatalf("buildSingleCancelItem().ItemType = %q, want %q", got.ItemType, stringInt(int(tt.wantIT)))
			}
			if got.FlagType != stringInt(int(tt.wantFT)) {
				t.Fatalf("buildSingleCancelItem().FlagType = %q, want %q", got.FlagType, stringInt(int(tt.wantFT)))
			}
		})
	}
}

func TestListQuery(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Int("page-size", 50, "")
	cmd.Flags().String("page-token", "", "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	if err := cmd.Flags().Set("page-size", "20"); err != nil {
		t.Fatalf("Set page-size error = %v", err)
	}
	if err := cmd.Flags().Set("page-token", "next_token"); err != nil {
		t.Fatalf("Set page-token error = %v", err)
	}
	runtime := &common.RuntimeContext{Cmd: cmd}

	got := listQuery(runtime)
	if got["page_size"][0] != "20" {
		t.Fatalf("listQuery() page_size = %q, want %q", got["page_size"][0], "20")
	}
	if got["page_token"][0] != "next_token" {
		t.Fatalf("listQuery() page_token = %q, want %q", got["page_token"][0], "next_token")
	}
}

func TestAsString(t *testing.T) {
	tests := []struct {
		input any
		want  string
	}{
		{input: "hello", want: "hello"},
		{input: "", want: ""},
		{input: 123, want: "123"},
		{input: int(456), want: "456"},
		{input: float64(78.9), want: "78.9"},
		{input: nil, want: ""},
		{input: []string{"a"}, want: ""},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			if got := asString(tt.input); got != tt.want {
				t.Fatalf("asString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
