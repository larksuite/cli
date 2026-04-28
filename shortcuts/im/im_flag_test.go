// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/output"
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
		{name: "empty string defaults to default", input: "", want: ItemTypeDefault},
		{name: "thread", input: "thread", want: ItemTypeThread},
		{name: "msg_thread", input: "msg_thread", want: ItemTypeMsgThread},
		{name: "case insensitive", input: "THREAD", want: ItemTypeThread},
		{name: "with whitespace", input: " thread ", want: ItemTypeThread},
		{name: "invalid", input: "invalid_type", wantErr: true},
		{name: "message is not a valid item type", input: "message", wantErr: true},
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
		{name: "case insensitive", input: "FEED", want: FlagTypeFeed},
		{name: "with whitespace", input: " feed ", want: FlagTypeFeed},
		{name: "invalid", input: "invalid_type", wantErr: true},
		{name: "unknown is not a valid flag type", input: "unknown", wantErr: true},
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
		{name: "with whitespace", input: " om_abc123 ", wantIT: ItemTypeDefault, wantFT: FlagTypeMessage},
		{name: "empty string", input: "", wantErr: true, errContain: "cannot be empty"},
		{name: "unknown prefix", input: "oc_xxx", wantErr: true, errContain: "cannot infer"},
		{name: "omt prefix is not special", input: "omt_xyz789", wantErr: true, errContain: "cannot infer"},
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
			itemID:   "om_xyz789",
			it:       ItemTypeThread,
			ft:       FlagTypeFeed,
			wantJSON: `{"item_id":"om_xyz789","item_type":"4","flag_type":"1"}`,
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

func TestParseFlagTypeFromRaw(t *testing.T) {
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
			if got := parseFlagTypeFromRaw(tt.input); got != tt.want {
				t.Fatalf("parseFlagTypeFromRaw(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

// helper
func stringInt(v int) string {
	return strconv.Itoa(v)
}

func setFlag(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	if err := cmd.Flags().Set(name, value); err != nil {
		t.Fatalf("Set %s error = %v", name, err)
	}
}

func newFlagScopeTestCmd(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("message-id", "", "")
	cmd.Flags().String("item-type", "", "")
	cmd.Flags().String("flag-type", "", "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	return cmd
}

type scopedTokenResolver struct {
	scopes string
}

func (r scopedTokenResolver) ResolveToken(_ context.Context, _ credential.TokenSpec) (*credential.TokenResult, error) {
	return &credential.TokenResult{Token: "user-token", Scopes: r.scopes}, nil
}

type errorTokenResolver struct {
	err error
}

func (r errorTokenResolver) ResolveToken(_ context.Context, _ credential.TokenSpec) (*credential.TokenResult, error) {
	return nil, r.err
}

func setRuntimeScopes(t *testing.T, rt *common.RuntimeContext, scopes string) {
	t.Helper()
	rt.Factory.Credential = credential.NewCredentialProvider(nil, nil, scopedTokenResolver{scopes: scopes}, nil)
}

func setRuntimeTokenError(t *testing.T, rt *common.RuntimeContext, err error) {
	t.Helper()
	rt.Factory.Credential = credential.NewCredentialProvider(nil, nil, errorTokenResolver{err: err}, nil)
}

func TestFlagMessageID(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		want       string
		wantErr    bool
		errContain string
	}{
		{name: "trims message id", id: " om_abc ", want: "om_abc"},
		{name: "missing message id", id: "", wantErr: true, errContain: "--message-id is required"},
		{name: "thread id rejected", id: "omt_abc", wantErr: true, errContain: "omt_ prefix is a thread ID"},
		{name: "non message id rejected", id: "oc_abc", wantErr: true, errContain: "must start with om_"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newFlagScopeTestCmd(t)
			setFlag(t, cmd, "message-id", tt.id)
			got, err := flagMessageID(&common.RuntimeContext{Cmd: cmd})
			if tt.wantErr {
				if err == nil {
					t.Fatalf("flagMessageID() expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errContain) {
					t.Fatalf("flagMessageID() error = %q, want %q", err.Error(), tt.errContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("flagMessageID() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("flagMessageID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseExplicitFlagCombo(t *testing.T) {
	tests := []struct {
		name       string
		itemType   string
		flagType   string
		wantItem   ItemType
		wantFlag   FlagType
		wantItemOK bool
		wantFlagOK bool
		wantErr    bool
		errContain string
	}{
		{name: "no overrides"},
		{name: "valid feed thread", itemType: "thread", flagType: "feed", wantItem: ItemTypeThread, wantFlag: FlagTypeFeed, wantItemOK: true, wantFlagOK: true},
		{name: "valid message default", itemType: "default", flagType: "message", wantItem: ItemTypeDefault, wantFlag: FlagTypeMessage, wantItemOK: true, wantFlagOK: true},
		{name: "flag only", flagType: "feed", wantFlag: FlagTypeFeed, wantFlagOK: true},
		{name: "item only requires flag", itemType: "thread", wantErr: true, errContain: "requires --flag-type=feed"},
		{name: "invalid pair", itemType: "thread", flagType: "message", wantErr: true, errContain: "invalid --item-type=thread --flag-type=message combination"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseExplicitFlagCombo(tt.itemType, tt.flagType)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseExplicitFlagCombo() expected error, got nil")
				}
				if !strings.Contains(err.Error(), tt.errContain) {
					t.Fatalf("parseExplicitFlagCombo() error = %q, want %q", err.Error(), tt.errContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseExplicitFlagCombo() error = %v", err)
			}
			if got.ItemTypeSet != tt.wantItemOK {
				t.Fatalf("ItemTypeSet = %v, want %v", got.ItemTypeSet, tt.wantItemOK)
			}
			if got.FlagTypeSet != tt.wantFlagOK {
				t.Fatalf("FlagTypeSet = %v, want %v", got.FlagTypeSet, tt.wantFlagOK)
			}
			if got.ItemTypeSet && got.ItemType != tt.wantItem {
				t.Fatalf("ItemType = %v, want %v", got.ItemType, tt.wantItem)
			}
			if got.FlagTypeSet && got.FlagType != tt.wantFlag {
				t.Fatalf("FlagType = %v, want %v", got.FlagType, tt.wantFlag)
			}
		})
	}
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
			name: "explicit item-type and flag-type",
			flags: map[string]string{
				"message-id": "om_xyz789",
				"item-type":  "thread",
				"flag-type":  "feed",
			},
			wantItem: newFlagItem("om_xyz789", ItemTypeThread, FlagTypeFeed),
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
			name: "explicit flag-type message",
			flags: map[string]string{
				"message-id": "om_abc",
				"flag-type":  "message",
			},
			wantItem: newFlagItem("om_abc", ItemTypeDefault, FlagTypeMessage),
		},
		{
			name: "missing message-id",
			flags: map[string]string{
				"item-type": "default",
			},
			wantErr:    true,
			errContain: "--message-id is required",
		},
		{
			name: "only item-type thread without flag-type should error",
			flags: map[string]string{
				"message-id": "om_abc",
				"item-type":  "thread",
			},
			wantErr:    true,
			errContain: "--item-type=thread requires --flag-type=feed",
		},
		{
			name: "only item-type msg_thread without flag-type should error",
			flags: map[string]string{
				"message-id": "om_abc",
				"item-type":  "msg_thread",
			},
			wantErr:    true,
			errContain: "--item-type=msg_thread requires --flag-type=feed",
		},
		{
			name: "only item-type default without flag-type should error",
			flags: map[string]string{
				"message-id": "om_abc",
				"item-type":  "default",
			},
			wantErr:    true,
			errContain: "--item-type=default requires --flag-type=message",
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

func TestFlagShortcutStaticScopesIncludeLookupRequirements(t *testing.T) {
	wantWriteLookup := append([]string{flagWriteScope}, flagLookupScopes...)
	if got := ImFlagCreate.ScopesForIdentity("user"); strings.Join(got, ",") != strings.Join(wantWriteLookup, ",") {
		t.Fatalf("ImFlagCreate scopes = %#v, want %#v", got, wantWriteLookup)
	}
	if got := ImFlagCancel.ScopesForIdentity("user"); strings.Join(got, ",") != strings.Join(wantWriteLookup, ",") {
		t.Fatalf("ImFlagCancel scopes = %#v, want %#v", got, wantWriteLookup)
	}
	if got := ImFlagList.ScopesForIdentity("user"); len(got) != 1 || got[0] != flagReadScope {
		t.Fatalf("ImFlagList scopes = %#v, want only %s", got, flagReadScope)
	}
}

func TestFlagCreateExplicitFeedTypeDoesNotRequireLookupScopes(t *testing.T) {
	var calls int
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, fmt.Errorf("explicit type should not call lookup API: %s", req.URL.Path)
	}))
	setRuntimeScopes(t, rt, flagWriteScope)

	cmd := newFlagScopeTestCmd(t)
	setFlag(t, cmd, "message-id", "om_123")
	setFlag(t, cmd, "flag-type", "feed")
	setFlag(t, cmd, "item-type", "msg_thread")
	setRuntimeField(t, rt, "Cmd", cmd)

	got, err := buildCreateItem(rt)
	if err != nil {
		t.Fatalf("buildCreateItem() error = %v", err)
	}
	if got.ItemType != "11" || got.FlagType != "1" {
		t.Fatalf("buildCreateItem() = %+v, want msg_thread/feed", got)
	}
	if calls != 0 {
		t.Fatalf("buildCreateItem() made %d lookup call(s), want 0", calls)
	}
}

func TestFlagCreateAutoDetectReliesOnDeclaredLookupScopes(t *testing.T) {
	var calls int
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, fmt.Errorf("should fail scope check before lookup API: %s", req.URL.Path)
	}))
	setRuntimeScopes(t, rt, flagWriteScope)

	cmd := newFlagScopeTestCmd(t)
	setFlag(t, cmd, "message-id", "om_123")
	setFlag(t, cmd, "flag-type", "feed")
	setRuntimeField(t, rt, "Cmd", cmd)

	_, err := buildCreateItem(rt)
	if err == nil || !strings.Contains(err.Error(), "should fail scope check before lookup API") {
		t.Fatalf("buildCreateItem() error = %v, want lookup API attempt", err)
	}
	if calls != 1 {
		t.Fatalf("buildCreateItem() made %d lookup call(s), want 1", calls)
	}
}

func TestCheckFlagRequiredScopesReportsTokenResolutionError(t *testing.T) {
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("checkFlagRequiredScopes should not call API")
		return nil, nil
	}))
	setRuntimeTokenError(t, rt, errors.New("token cache unavailable"))

	err := checkFlagRequiredScopes(context.Background(), rt, flagMessageReadScopes)
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("checkFlagRequiredScopes() error = %T %v, want ExitError", err, err)
	}
	if exitErr.Code != output.ExitAuth || exitErr.Detail.Type != "auth" {
		t.Fatalf("checkFlagRequiredScopes() detail = %+v code=%d, want auth exit", exitErr.Detail, exitErr.Code)
	}
	if !strings.Contains(exitErr.Detail.Message, "cannot verify required scope") {
		t.Fatalf("message = %q, want scope verification context", exitErr.Detail.Message)
	}
	if !strings.Contains(exitErr.Detail.Hint, strings.Join(flagMessageReadScopes, " ")) {
		t.Fatalf("hint = %q, want required scopes", exitErr.Detail.Hint)
	}
}

func TestCheckFlagRequiredScopesAllowsMissingScopeMetadata(t *testing.T) {
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("checkFlagRequiredScopes should not call API")
		return nil, nil
	}))
	setRuntimeScopes(t, rt, "")

	err := checkFlagRequiredScopes(context.Background(), rt, flagMessageReadScopes)
	if err != nil {
		t.Fatalf("checkFlagRequiredScopes() error = %v, want nil for missing scope metadata", err)
	}
	errOut := rt.Factory.IOStreams.ErrOut.(*bytes.Buffer).String()
	if !strings.Contains(errOut, "warning: cannot verify required scope(s)") {
		t.Fatalf("stderr = %q, want scope metadata warning", errOut)
	}
	if !strings.Contains(errOut, strings.Join(flagMessageReadScopes, " ")) {
		t.Fatalf("stderr = %q, want required scopes", errOut)
	}
}

func TestFlagCancelExplicitTypeDoesNotRequireLookupScopes(t *testing.T) {
	var calls int
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, fmt.Errorf("explicit type should not call lookup API: %s", req.URL.Path)
	}))
	setRuntimeScopes(t, rt, flagWriteScope)

	cmd := newFlagScopeTestCmd(t)
	setFlag(t, cmd, "message-id", "om_123")
	setFlag(t, cmd, "flag-type", "feed")
	setFlag(t, cmd, "item-type", "msg_thread")
	setRuntimeField(t, rt, "Cmd", cmd)

	got, err := buildCancelItems(rt)
	if err != nil {
		t.Fatalf("buildCancelItems() error = %v", err)
	}
	if len(got) != 1 || got[0].ItemType != "11" || got[0].FlagType != "1" {
		t.Fatalf("buildCancelItems() = %+v, want single msg_thread/feed item", got)
	}
	if calls != 0 {
		t.Fatalf("buildCancelItems() made %d lookup call(s), want 0", calls)
	}
}

func TestFlagCancelDefaultReliesOnDeclaredLookupScopes(t *testing.T) {
	var calls int
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, fmt.Errorf("should fail scope check before lookup API: %s", req.URL.Path)
	}))
	setRuntimeScopes(t, rt, flagWriteScope)

	cmd := newFlagScopeTestCmd(t)
	setFlag(t, cmd, "message-id", "om_123")
	setRuntimeField(t, rt, "Cmd", cmd)

	got, err := buildCancelItems(rt)
	if err != nil {
		t.Fatalf("buildCancelItems() error = %v", err)
	}
	if len(got) != 1 || got[0].ItemType != "0" || got[0].FlagType != "2" {
		t.Fatalf("buildCancelItems() = %+v, want default/message fallback", got)
	}
	if calls != 1 {
		t.Fatalf("buildCancelItems() made %d lookup call(s), want 1", calls)
	}
}

func TestFlagCreateDryRunFeedDoesNotCallAPI(t *testing.T) {
	var calls int
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		return nil, fmt.Errorf("dry-run must not call API: %s", req.URL.Path)
	}))

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("message-id", "", "")
	cmd.Flags().String("item-type", "", "")
	cmd.Flags().String("flag-type", "", "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	if err := cmd.Flags().Set("message-id", "om_123"); err != nil {
		t.Fatalf("Set message-id error = %v", err)
	}
	if err := cmd.Flags().Set("flag-type", "feed"); err != nil {
		t.Fatalf("Set flag-type error = %v", err)
	}
	setRuntimeField(t, rt, "Cmd", cmd)

	got := ImFlagCreate.DryRun(context.Background(), rt).Format()
	if calls != 0 {
		t.Fatalf("DryRun made %d API call(s), want 0", calls)
	}
	if !strings.Contains(got, "auto:thread|msg_thread") {
		t.Fatalf("DryRun output = %s, want auto-detect placeholder", got)
	}
}

func TestFlagListSkillDocUsesExistingMessageFetchShortcut(t *testing.T) {
	b, err := os.ReadFile("../../skills/lark-im/references/lark-im-flag-list.md")
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	doc := string(b)
	if strings.Contains(doc, "+message-get") {
		t.Fatalf("flag-list skill doc references nonexistent +message-get shortcut")
	}
	if !strings.Contains(doc, "+messages-mget") {
		t.Fatalf("flag-list skill doc should mention +messages-mget")
	}
}

func TestBuildCancelItemsForPreview(t *testing.T) {
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

			got, isDouble, err := buildCancelItemsForPreview(runtime)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("buildCancelItemsForPreview() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildCancelItemsForPreview() unexpected error: %v", err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("buildCancelItemsForPreview() returned %d items, want %d", len(got), tt.wantLen)
			}
			if isDouble != tt.wantDouble {
				t.Fatalf("buildCancelItemsForPreview() isDouble = %v, want %v", isDouble, tt.wantDouble)
			}
		})
	}
}

func TestFlagCancelDryRunReportsValidationError(t *testing.T) {
	cmd := newFlagScopeTestCmd(t)
	setFlag(t, cmd, "message-id", "om_123")
	setFlag(t, cmd, "flag-type", "invalid")

	runtime := &common.RuntimeContext{Cmd: cmd}
	got := ImFlagCancel.DryRun(context.Background(), runtime).Format()

	if !strings.Contains(got, "invalid --flag-type") {
		t.Fatalf("DryRun output = %q, want validation error", got)
	}
	if strings.Contains(got, "flag_items") {
		t.Fatalf("DryRun output = %q, should not include request body for invalid input", got)
	}
}

func TestFlagCancelDryRunUsesAutoDetectPlaceholderForFeedLayer(t *testing.T) {
	cmd := newFlagScopeTestCmd(t)
	setFlag(t, cmd, "message-id", "om_123")

	runtime := &common.RuntimeContext{Cmd: cmd}
	got := ImFlagCancel.DryRun(context.Background(), runtime).Format()

	if !strings.Contains(got, "auto:thread|msg_thread") {
		t.Fatalf("DryRun output = %q, want auto-detect placeholder", got)
	}
	if strings.Contains(got, `"item_type":"11"`) {
		t.Fatalf("DryRun output = %q, should not hard-code msg_thread item_type", got)
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
			name:       "explicit override",
			id:         "om_xyz",
			itOverride: "msg_thread",
			ftOverride: "feed",
			wantIT:     ItemTypeMsgThread,
			wantFT:     FlagTypeFeed,
		},
		{
			name:       "only item-type override",
			id:         "om_xyz",
			itOverride: "msg_thread",
			wantErr:    true, // msg_thread + message (inferred from om_) is invalid
		},
		{
			name:       "only flag-type override",
			id:         "om_xyz",
			ftOverride: "feed",
			wantErr:    true, // default (inferred from om_) + feed is invalid
		},
		{
			name:       "invalid combo: thread+message",
			id:         "om_abc",
			itOverride: "thread",
			wantErr:    true, // thread + message (inferred from om_) is invalid
		},
		{
			name:       "invalid combo: default+feed",
			id:         "om_xyz",
			ftOverride: "feed",
			wantErr:    true, // default (inferred) + feed is invalid
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

func TestFlagListRejectsInvalidPageLimit(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Int("page-size", 50, "")
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().Bool("page-all", false, "")
	cmd.Flags().Int("page-limit", 20, "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	if err := cmd.Flags().Set("page-limit", "0"); err != nil {
		t.Fatalf("Set page-limit error = %v", err)
	}
	runtime := &common.RuntimeContext{Cmd: cmd}

	if err := ImFlagList.Validate(context.Background(), runtime); err == nil {
		t.Fatalf("Validate() expected page-limit error, got nil")
	}

	got := ImFlagList.DryRun(context.Background(), runtime).Format()
	if !strings.Contains(got, "--page-limit") {
		t.Fatalf("DryRun output = %q, want page-limit validation error", got)
	}
	if strings.Contains(got, "/open-apis/im/v1/flags") {
		t.Fatalf("DryRun output = %q, should not include request for invalid input", got)
	}
}

func TestFlagListDryRunMentionsConditionalEnrichmentScopes(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Int("page-size", 50, "")
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().Bool("page-all", false, "")
	cmd.Flags().Int("page-limit", 20, "")
	cmd.Flags().Bool("enrich-feed-thread", true, "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	runtime := &common.RuntimeContext{Cmd: cmd}

	got := ImFlagList.DryRun(context.Background(), runtime).Format()
	for _, want := range []string{
		"im:message.group_msg:get_as_user",
		"im:message.p2p_msg:get_as_user",
		"--enrich-feed-thread=false",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("DryRun output = %q, want %q", got, want)
		}
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

func TestEnrichFeedThreadItems(t *testing.T) {
	tests := []struct {
		name       string
		data       map[string]any
		wantMsg    bool // whether message should be attached
		wantErr    bool
		mockMGet   map[string]any // mock mget response
		mockMGetOK bool
	}{
		{
			name: "empty flag_items",
			data: map[string]any{
				"flag_items": []any{},
			},
			wantErr: false,
		},
		{
			name: "non-feed item skipped",
			data: map[string]any{
				"flag_items": []any{
					map[string]any{
						"item_id":   "om_123",
						"item_type": "0",
						"flag_type": "2", // message type
					},
				},
			},
			wantErr: false,
		},
		{
			name: "feed-thread item with inlined message",
			data: map[string]any{
				"flag_items": []any{
					map[string]any{
						"item_id":   "omt_123",
						"item_type": "4",
						"flag_type": "1", // feed type
					},
				},
				"messages": []any{
					map[string]any{
						"message_id": "omt_123",
						"content":    "hello",
					},
				},
			},
			wantErr: false,
			wantMsg: true,
		},
		{
			name: "feed-thread item needs mget",
			data: map[string]any{
				"flag_items": []any{
					map[string]any{
						"item_id":   "omt_456",
						"item_type": "4",
						"flag_type": "1",
					},
				},
			},
			mockMGetOK: true,
			mockMGet: map[string]any{
				"items": []any{
					map[string]any{
						"message_id": "omt_456",
						"content":    "fetched content",
					},
				},
			},
			wantErr: false,
			wantMsg: true,
		},
		{
			name: "msg_thread item needs mget",
			data: map[string]any{
				"flag_items": []any{
					map[string]any{
						"item_id":   "om_789",
						"item_type": "11", // msg_thread
						"flag_type": "1",
					},
				},
			},
			mockMGetOK: true,
			mockMGet: map[string]any{
				"items": []any{
					map[string]any{
						"message_id": "om_789",
						"content":    "msg_thread content",
					},
				},
			},
			wantErr: false,
			wantMsg: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/mget") {
					if !tt.mockMGetOK {
						return nil, fmt.Errorf("unexpected mget call")
					}
					return shortcutJSONResponse(200, map[string]any{
						"code": 0,
						"data": tt.mockMGet,
					}), nil
				}
				return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
			}))
			setRuntimeScopes(t, rt, strings.Join(flagMessageReadScopes, " "))

			err := enrichFeedThreadItems(rt, tt.data)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("enrichFeedThreadItems() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("enrichFeedThreadItems() unexpected error: %v", err)
			}

			if tt.wantMsg {
				items := tt.data["flag_items"].([]any)
				if len(items) == 0 {
					t.Fatalf("expected flag_items")
				}
				item := items[0].(map[string]any)
				if _, ok := item["message"]; !ok {
					t.Fatalf("expected message to be attached to item")
				}
			}
		})
	}
}

func TestEnrichFeedThreadItems_BatchMGet(t *testing.T) {
	// Test that batched mget works when > 50 items
	var feedItems []any
	for i := 0; i < 60; i++ {
		feedItems = append(feedItems, map[string]any{
			"item_id":   fmt.Sprintf("omt_%d", i),
			"item_type": "4",
			"flag_type": "1",
		})
	}

	callCount := 0
	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/mget") {
			callCount++
			return shortcutJSONResponse(200, map[string]any{
				"code": 0,
				"data": map[string]any{
					"items": []any{},
				},
			}), nil
		}
		return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
	}))
	setRuntimeScopes(t, rt, strings.Join(flagMessageReadScopes, " "))

	data := map[string]any{"flag_items": feedItems}
	err := enrichFeedThreadItems(rt, data)
	if err != nil {
		t.Fatalf("enrichFeedThreadItems() error = %v", err)
	}
	// Should make 2 calls: 50 + 10 items
	if callCount != 2 {
		t.Fatalf("expected 2 mget calls, got %d", callCount)
	}
}

func TestEnrichFeedThreadItems_MGetError(t *testing.T) {
	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/mget") {
			return nil, fmt.Errorf("mget failed")
		}
		return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
	}))
	setRuntimeScopes(t, rt, strings.Join(flagMessageReadScopes, " "))

	data := map[string]any{
		"flag_items": []any{
			map[string]any{
				"item_id":   "omt_123",
				"item_type": "4",
				"flag_type": "1",
			},
		},
	}

	err := enrichFeedThreadItems(rt, data)
	if err == nil {
		t.Fatalf("enrichFeedThreadItems() expected error, got nil")
	}
}

func TestAsStringFloat(t *testing.T) {
	// Test float64 conversion specifically (JSON numbers come as float64)
	tests := []struct {
		input float64
		want  string
	}{
		{input: 1.0, want: "1"},
		{input: 123.456, want: "123.456"},
		{input: 0.0, want: "0"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := asString(tt.input); got != tt.want {
				t.Fatalf("asString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestAsStringInt(t *testing.T) {
	// Test int conversion
	tests := []struct {
		input int
		want  string
	}{
		{input: 1, want: "1"},
		{input: 123, want: "123"},
		{input: 0, want: "0"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := asString(tt.input); got != tt.want {
				t.Fatalf("asString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestBuildCancelItems_ExplicitOverride(t *testing.T) {
	// Test buildCancelItems with explicit item-type and flag-type override
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("message-id", "", "")
	cmd.Flags().String("item-type", "", "")
	cmd.Flags().String("flag-type", "", "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	if err := cmd.Flags().Set("message-id", "om_xyz"); err != nil {
		t.Fatalf("Set message-id error = %v", err)
	}
	if err := cmd.Flags().Set("item-type", "msg_thread"); err != nil {
		t.Fatalf("Set item-type error = %v", err)
	}
	if err := cmd.Flags().Set("flag-type", "feed"); err != nil {
		t.Fatalf("Set flag-type error = %v", err)
	}
	runtime := &common.RuntimeContext{Cmd: cmd}

	got, err := buildCancelItems(runtime)
	if err != nil {
		t.Fatalf("buildCancelItems() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("buildCancelItems() returned %d items, want 1", len(got))
	}
	if got[0].ItemID != "om_xyz" {
		t.Fatalf("buildCancelItems().ItemID = %q, want %q", got[0].ItemID, "om_xyz")
	}
	if got[0].ItemType != "11" {
		t.Fatalf("buildCancelItems().ItemType = %q, want 11", got[0].ItemType)
	}
	if got[0].FlagType != "1" {
		t.Fatalf("buildCancelItems().FlagType = %q, want 1", got[0].FlagType)
	}
}

func TestFlagCancelExecuteSummarizesPartialFailure(t *testing.T) {
	var cancelCalls int
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch {
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_123"):
			return shortcutJSONResponse(200, map[string]any{
				"code": 0,
				"data": map[string]any{
					"items": []any{
						map[string]any{
							"message_id": "om_123",
							"chat_id":    "oc_chat",
						},
					},
				},
			}), nil
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/chats/oc_chat"):
			return shortcutJSONResponse(200, map[string]any{
				"code": 0,
				"data": map[string]any{"chat_mode": "group"},
			}), nil
		case strings.Contains(req.URL.Path, "/open-apis/im/v1/flags/cancel"):
			cancelCalls++
			if cancelCalls == 1 {
				return shortcutJSONResponse(200, map[string]any{
					"code": 0,
					"data": map[string]any{"request_id": "message-ok"},
				}), nil
			}
			return shortcutJSONResponse(200, map[string]any{
				"code": 999,
				"msg":  "feed failed",
			}), nil
		default:
			return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
		}
	}))

	cmd := newFlagScopeTestCmd(t)
	setFlag(t, cmd, "message-id", "om_123")
	setRuntimeField(t, rt, "Cmd", cmd)

	err := ImFlagCancel.Execute(context.Background(), rt)
	if err == nil {
		t.Fatalf("Execute() expected partial failure error, got nil")
	}

	out := rt.Factory.IOStreams.Out.(*bytes.Buffer).String()
	for _, want := range []string{`"results"`, `"item_type": "default"`, `"flag_type": "message"`, `"status": "ok"`, `"item_type": "msg_thread"`, `"flag_type": "feed"`, `"status": "failed"`, "feed failed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout = %s, want %q", out, want)
		}
	}

	var envelope struct {
		Data struct {
			Results []map[string]any `json:"results"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &envelope); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out)
	}
	if len(envelope.Data.Results) != 2 {
		t.Fatalf("results len = %d, want 2", len(envelope.Data.Results))
	}
}

func TestBuildCancelItems_OnlyItemTypeOverride(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("message-id", "", "")
	cmd.Flags().String("item-type", "", "")
	cmd.Flags().String("flag-type", "", "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	if err := cmd.Flags().Set("message-id", "om_xyz"); err != nil {
		t.Fatalf("Set message-id error = %v", err)
	}
	if err := cmd.Flags().Set("item-type", "thread"); err != nil {
		t.Fatalf("Set item-type error = %v", err)
	}
	runtime := &common.RuntimeContext{Cmd: cmd}

	_, err := buildCancelItems(runtime)
	// om_xyz + thread -> inferred flag-type=message from om_ prefix
	// thread + message is invalid combo, should error
	if err == nil {
		t.Fatalf("buildCancelItems() expected error for invalid combo, got nil")
	}
}

func TestBuildCancelItems_OmPrefixThreadRoot(t *testing.T) {
	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_123") {
			return shortcutJSONResponse(200, map[string]any{
				"code": 0,
				"data": map[string]any{
					"items": []any{
						map[string]any{
							"message_id": "om_123",

							"chat_id": "oc_chat",
						},
					},
				},
			}), nil
		}
		if strings.Contains(req.URL.Path, "/open-apis/im/v1/chats/") {
			return shortcutJSONResponse(200, map[string]any{
				"code": 0,
				"data": map[string]any{
					"chat_mode": "group",
				},
			}), nil
		}
		return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
	}))

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("message-id", "", "")
	cmd.Flags().String("item-type", "", "")
	cmd.Flags().String("flag-type", "", "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	if err := cmd.Flags().Set("message-id", "om_123"); err != nil {
		t.Fatalf("Set message-id error = %v", err)
	}
	setRuntimeField(t, rt, "Cmd", cmd)

	got, err := buildCancelItems(rt)
	if err != nil {
		t.Fatalf("buildCancelItems() error = %v", err)
	}
	// Thread root message should produce double-cancel
	if len(got) != 2 {
		t.Fatalf("buildCancelItems() returned %d items, want 2", len(got))
	}
	// First item should be default+message
	if got[0].ItemType != "0" || got[0].FlagType != "2" {
		t.Fatalf("first item = %+v, want default+message", got[0])
	}
	// Second item should be msg_thread+feed (group chat)
	if got[1].ItemType != "11" || got[1].FlagType != "1" {
		t.Fatalf("second item = %+v, want msg_thread+feed", got[1])
	}
}

func TestBuildCancelItems_MessageQueryFails(t *testing.T) {
	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("API error")
	}))

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("message-id", "", "")
	cmd.Flags().String("item-type", "", "")
	cmd.Flags().String("flag-type", "", "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	if err := cmd.Flags().Set("message-id", "om_789"); err != nil {
		t.Fatalf("Set message-id error = %v", err)
	}
	setRuntimeField(t, rt, "Cmd", cmd)

	got, err := buildCancelItems(rt)
	if err != nil {
		t.Fatalf("buildCancelItems() error = %v", err)
	}
	// When message query fails, should still cancel message layer (best effort)
	// Feed layer is skipped since we can't determine chat_type
	if len(got) != 1 {
		t.Fatalf("buildCancelItems() returned %d items, want 1", len(got))
	}
	if got[0].ItemType != "0" || got[0].FlagType != "2" {
		t.Fatalf("item = %+v, want default+message", got[0])
	}
}

func TestBuildCancelItems_MissingID(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("message-id", "", "")
	cmd.Flags().String("item-type", "", "")
	cmd.Flags().String("flag-type", "", "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	runtime := &common.RuntimeContext{Cmd: cmd}

	_, err := buildCancelItems(runtime)
	if err == nil {
		t.Fatalf("buildCancelItems() expected error, got nil")
	}
}

func TestExecuteListAllPages(t *testing.T) {
	callCount := 0
	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/open-apis/im/v1/flags") {
			callCount++
			hasMore := callCount < 2
			return shortcutJSONResponse(200, map[string]any{
				"code": 0,
				"data": map[string]any{
					"flag_items": []any{
						map[string]any{"item_id": fmt.Sprintf("om_%d", callCount)},
					},
					"delete_flag_items": []any{},
					"messages":          []any{},
					"has_more":          hasMore,
					"page_token":        fmt.Sprintf("token_%d", callCount),
				},
			}), nil
		}
		return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
	}))

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Int("page-size", 50, "")
	cmd.Flags().Int("page-limit", 10, "")
	cmd.Flags().Bool("enrich-feed-thread", false, "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	setRuntimeField(t, rt, "Cmd", cmd)

	err := executeListAllPages(rt)
	if err != nil {
		t.Fatalf("executeListAllPages() error = %v", err)
	}
	if callCount != 2 {
		t.Fatalf("expected 2 API calls, got %d", callCount)
	}
}

func TestExecuteListAllPages_EnrichFeedThread(t *testing.T) {
	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/open-apis/im/v1/flags") {
			return shortcutJSONResponse(200, map[string]any{
				"code": 0,
				"data": map[string]any{
					"flag_items": []any{
						map[string]any{
							"item_id":   "omt_123",
							"item_type": "4",
							"flag_type": "1",
						},
					},
					"delete_flag_items": []any{},
					"messages":          []any{},
					"has_more":          false,
					"page_token":        "",
				},
			}), nil
		}
		if strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/mget") {
			return shortcutJSONResponse(200, map[string]any{
				"code": 0,
				"data": map[string]any{
					"items": []any{
						map[string]any{
							"message_id": "omt_123",
							"content":    "test content",
						},
					},
				},
			}), nil
		}
		return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
	}))

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Int("page-size", 50, "")
	cmd.Flags().Int("page-limit", 10, "")
	cmd.Flags().Bool("enrich-feed-thread", true, "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	setRuntimeField(t, rt, "Cmd", cmd)

	err := executeListAllPages(rt)
	if err != nil {
		t.Fatalf("executeListAllPages() error = %v", err)
	}
}

func TestExecuteListAllPages_PageLimit(t *testing.T) {
	callCount := 0
	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if strings.Contains(req.URL.Path, "/open-apis/im/v1/flags") {
			callCount++
			return shortcutJSONResponse(200, map[string]any{
				"code": 0,
				"data": map[string]any{
					"flag_items":        []any{},
					"delete_flag_items": []any{},
					"messages":          []any{},
					"has_more":          true, // always has more
					"page_token":        fmt.Sprintf("token_%d", callCount),
				},
			}), nil
		}
		return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
	}))

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Int("page-size", 50, "")
	cmd.Flags().Int("page-limit", 3, "") // limit to 3 pages
	cmd.Flags().Bool("enrich-feed-thread", false, "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	setRuntimeField(t, rt, "Cmd", cmd)

	err := executeListAllPages(rt)
	if err != nil {
		t.Fatalf("executeListAllPages() error = %v", err)
	}
	// Should stop at page-limit
	if callCount != 3 {
		t.Fatalf("expected 3 API calls (page limit), got %d", callCount)
	}
}

func TestExecuteListAllPages_APIError(t *testing.T) {
	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("API error")
	}))

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Int("page-size", 50, "")
	cmd.Flags().Int("page-limit", 10, "")
	cmd.Flags().Bool("enrich-feed-thread", false, "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	setRuntimeField(t, rt, "Cmd", cmd)

	err := executeListAllPages(rt)
	if err == nil {
		t.Fatalf("executeListAllPages() expected error, got nil")
	}
}

func TestBuildCreateItem_FeedAutoDetect(t *testing.T) {
	// Test --flag-type feed auto-detects item_type from chat_mode
	t.Run("topic-style chat", func(t *testing.T) {
		rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_123") {
				return shortcutJSONResponse(200, map[string]any{
					"code": 0,
					"data": map[string]any{
						"items": []any{
							map[string]any{
								"message_id": "om_123",
								"chat_id":    "oc_chat",
							},
						},
					},
				}), nil
			}
			if strings.Contains(req.URL.Path, "/open-apis/im/v1/chats/") {
				return shortcutJSONResponse(200, map[string]any{
					"code": 0,
					"data": map[string]any{
						"chat_mode": "topic",
					},
				}), nil
			}
			return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
		}))

		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("message-id", "", "")
		cmd.Flags().String("item-type", "", "")
		cmd.Flags().String("flag-type", "", "")
		if err := cmd.ParseFlags(nil); err != nil {
			t.Fatalf("ParseFlags() error = %v", err)
		}
		cmd.Flags().Set("message-id", "om_123")
		cmd.Flags().Set("flag-type", "feed")
		setRuntimeField(t, rt, "Cmd", cmd)

		got, err := buildCreateItem(rt)
		if err != nil {
			t.Fatalf("buildCreateItem() error = %v", err)
		}
		if got.ItemType != "4" {
			t.Fatalf("ItemType = %q, want 4 (thread)", got.ItemType)
		}
		if got.FlagType != "1" {
			t.Fatalf("FlagType = %q, want 1 (feed)", got.FlagType)
		}
	})

	t.Run("regular chat", func(t *testing.T) {
		rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/open-apis/im/v1/messages/om_456") {
				return shortcutJSONResponse(200, map[string]any{
					"code": 0,
					"data": map[string]any{
						"items": []any{
							map[string]any{
								"message_id": "om_456",
								"chat_id":    "oc_chat",
							},
						},
					},
				}), nil
			}
			if strings.Contains(req.URL.Path, "/open-apis/im/v1/chats/") {
				return shortcutJSONResponse(200, map[string]any{
					"code": 0,
					"data": map[string]any{
						"chat_mode": "group",
					},
				}), nil
			}
			return nil, fmt.Errorf("unexpected request: %s", req.URL.Path)
		}))

		cmd := &cobra.Command{Use: "test"}
		cmd.Flags().String("message-id", "", "")
		cmd.Flags().String("item-type", "", "")
		cmd.Flags().String("flag-type", "", "")
		if err := cmd.ParseFlags(nil); err != nil {
			t.Fatalf("ParseFlags() error = %v", err)
		}
		cmd.Flags().Set("message-id", "om_456")
		cmd.Flags().Set("flag-type", "feed")
		setRuntimeField(t, rt, "Cmd", cmd)

		got, err := buildCreateItem(rt)
		if err != nil {
			t.Fatalf("buildCreateItem() error = %v", err)
		}
		if got.ItemType != "11" {
			t.Fatalf("ItemType = %q, want 11 (msg_thread)", got.ItemType)
		}
		if got.FlagType != "1" {
			t.Fatalf("FlagType = %q, want 1 (feed)", got.FlagType)
		}
	})
}

func TestValidateExplicitCombo(t *testing.T) {
	tests := []struct {
		name       string
		itOverride string
		ftOverride string
		wantErr    bool
		errContain string
	}{
		{name: "no overrides", itOverride: "", ftOverride: "", wantErr: false},
		{name: "both valid", itOverride: "thread", ftOverride: "feed", wantErr: false},
		{name: "default+message", itOverride: "default", ftOverride: "message", wantErr: false},
		{name: "invalid combo", itOverride: "thread", ftOverride: "message", wantErr: true, errContain: "invalid"},
		{name: "item-type thread without flag-type", itOverride: "thread", ftOverride: "", wantErr: true, errContain: "requires --flag-type=feed"},
		{name: "item-type default without flag-type", itOverride: "default", ftOverride: "", wantErr: true, errContain: "requires --flag-type=message"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateExplicitCombo(tt.itOverride, tt.ftOverride)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("validateExplicitCombo() expected error, got nil")
				}
				if tt.errContain != "" && !strings.Contains(err.Error(), tt.errContain) {
					t.Fatalf("error = %q, want to contain %q", err.Error(), tt.errContain)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateExplicitCombo() unexpected error: %v", err)
			}
		})
	}
}

func TestItemTypeString(t *testing.T) {
	tests := []struct {
		input ItemType
		want  string
	}{
		{input: ItemTypeDefault, want: "default"},
		{input: ItemTypeThread, want: "thread"},
		{input: ItemTypeMsgThread, want: "msg_thread"},
		{input: ItemType(999), want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := itemTypeString(tt.input); got != tt.want {
				t.Fatalf("itemTypeString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFlagTypeString(t *testing.T) {
	tests := []struct {
		input FlagType
		want  string
	}{
		{input: FlagTypeMessage, want: "message"},
		{input: FlagTypeFeed, want: "feed"},
		{input: FlagTypeUnknown, want: "unknown"},
		{input: FlagType(999), want: "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := flagTypeString(tt.input); got != tt.want {
				t.Fatalf("flagTypeString(%v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
