// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// mustMarshalDryRun marshals v to a JSON string, calling t.Fatalf on error.
func mustMarshalDryRun(t *testing.T, v interface{}) string {
	t.Helper()

	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return string(b)
}

// newTestRuntimeContext builds a *common.RuntimeContext backed by a cobra
// command whose flags are populated from the provided string and bool maps,
// for unit-testing shortcut bodies, validators, and dry-run shapes.
func newTestRuntimeContext(t *testing.T, stringFlags map[string]string, boolFlags map[string]bool) *common.RuntimeContext {
	t.Helper()

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Int("page-limit", 20, "")
	for name := range stringFlags {
		if name == "page-limit" {
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

// newMessagesSearchTestRuntimeContext is the messages-search variant of
// newTestRuntimeContext: registers the search-specific --page-size flag
// before applying caller-provided values.
func newMessagesSearchTestRuntimeContext(t *testing.T, stringFlags map[string]string, boolFlags map[string]bool) *common.RuntimeContext {
	t.Helper()

	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Int("page-size", 20, "")
	cmd.Flags().Int("page-limit", 20, "")
	for name := range stringFlags {
		if name == "page-size" || name == "page-limit" {
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

// TestBuildCreateChatBody verifies the request body assembled when every
// flag is populated, including the default chat_mode="group".
func TestBuildCreateChatBody(t *testing.T) {
	runtime := newTestRuntimeContext(t, map[string]string{
		"type":        "public",
		"name":        "Team Chat",
		"description": "daily sync",
		"users":       "ou_1, ou_2",
		"bots":        "cli_1, cli_2",
		"owner":       "ou_owner",
		"chat-mode":   "group",
	}, nil)

	got := buildCreateChatBody(runtime)
	want := map[string]interface{}{
		"chat_type":   "public",
		"chat_mode":   "group",
		"name":        "Team Chat",
		"description": "daily sync",
		"user_id_list": []string{
			"ou_1",
			"ou_2",
		},
		"bot_id_list": []string{
			"cli_1",
			"cli_2",
		},
		"owner_id": "ou_owner",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildCreateChatBody() = %#v, want %#v", got, want)
	}
}

// TestBuildCreateChatBody_TopicMode verifies that --chat-mode topic produces
// chat_mode="topic" in the request body, the topic-chat creation path.
func TestBuildCreateChatBody_TopicMode(t *testing.T) {
	runtime := newTestRuntimeContext(t, map[string]string{
		"type":      "public",
		"name":      "Topic Group",
		"chat-mode": "topic",
	}, nil)

	got := buildCreateChatBody(runtime)
	want := map[string]interface{}{
		"chat_type": "public",
		"chat_mode": "topic",
		"name":      "Topic Group",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildCreateChatBody() = %#v, want %#v", got, want)
	}
}

// TestBuildCreateChatBody_EmptyChatModeFallsBack pins the defensive fallback:
// explicit `--chat-mode ""` slips past validateEnumFlags (which skips empty
// values), but buildCreateChatBody must still emit chat_mode="group" rather
// than an empty string with unspecified server semantics.
func TestBuildCreateChatBody_EmptyChatModeFallsBack(t *testing.T) {
	runtime := newTestRuntimeContext(t, map[string]string{
		"type":      "public",
		"name":      "Fallback Test",
		"chat-mode": "",
	}, nil)

	got := buildCreateChatBody(runtime)
	if got["chat_mode"] != "group" {
		t.Fatalf("buildCreateChatBody() chat_mode = %#v, want \"group\"", got["chat_mode"])
	}
}

// TestSplitMembers verifies the delegation wrapper; core logic is tested in TestSplitCSV. [#17]
func TestSplitMembers(t *testing.T) {
	got := common.SplitCSV(" ou_1, ,ou_2 ,, ou_3 ")
	want := []string{"ou_1", "ou_2", "ou_3"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitMembers() = %#v, want %#v", got, want)
	}
}

func TestBuildSearchChatBody(t *testing.T) {
	runtime := newTestRuntimeContext(t, map[string]string{
		"query":      "team-alpha",
		"page-size":  "50",
		"page-token": "next_page",
	}, nil)

	got := buildSearchChatBody(runtime)
	want := map[string]interface{}{
		"query": `"team-alpha"`,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildSearchChatBody() = %#v, want %#v", got, want)
	}
}

func TestSplitAndTrimChat(t *testing.T) {
	got := common.SplitCSV(" private, , public_joined ,, external ")
	want := []string{"private", "public_joined", "external"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("common.SplitCSV() = %#v, want %#v", got, want)
	}
}

func TestBuildUpdateChatBody(t *testing.T) {
	runtime := newTestRuntimeContext(t, map[string]string{
		"name":        "New Name",
		"description": "New Description",
	}, nil)

	got := buildUpdateChatBody(runtime)
	want := map[string]interface{}{
		"name":        "New Name",
		"description": "New Description",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildUpdateChatBody() = %#v, want %#v", got, want)
	}
}

func TestIsMediaKey(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{value: "img_123", want: true},
		{value: "file_123", want: true},
		{value: "/tmp/image.png", want: false},
		{value: "video.mp4", want: false},
	}

	for _, tt := range tests {
		if got := isMediaKey(tt.value); got != tt.want {
			t.Fatalf("isMediaKey(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
}

func TestShortcutValidateBranches(t *testing.T) {

	t.Run("ImChatCreate valid", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"type":  "public",
			"name":  "Team Room",
			"users": "ou_1,ou_2",
			"bots":  "cli_1",
			"owner": "ou_owner",
		}, nil)
		if err := ImChatCreate.Validate(context.Background(), runtime); err != nil {
			t.Fatalf("ImChatCreate.Validate() unexpected error = %v", err)
		}
	})

	t.Run("ImChatCreate name too long", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"name": strings.Repeat("长", 61),
		}, nil)
		err := ImChatCreate.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "--name exceeds the maximum of 60 characters") {
			t.Fatalf("ImChatCreate.Validate() error = %v", err)
		}
	})

	t.Run("ImChatCreate description too long", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"description": strings.Repeat("d", 101),
		}, nil)
		err := ImChatCreate.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "--description exceeds the maximum of 100 characters") {
			t.Fatalf("ImChatCreate.Validate() error = %v", err)
		}
	})

	t.Run("ImChatCreate invalid user id", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"users": "ou_1,user_2",
		}, nil)
		err := ImChatCreate.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "invalid user ID format") {
			t.Fatalf("ImChatCreate.Validate() error = %v", err)
		}
	})

	t.Run("ImChatCreate too many bots", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"bots": "cli_1,cli_2,cli_3,cli_4,cli_5,cli_6",
		}, nil)
		err := ImChatCreate.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "--bots exceeds the maximum of 5") {
			t.Fatalf("ImChatCreate.Validate() error = %v", err)
		}
	})

	t.Run("ImChatCreate invalid owner id", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"owner": "user_1",
		}, nil)
		err := ImChatCreate.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "invalid user ID format") {
			t.Fatalf("ImChatCreate.Validate() error = %v", err)
		}
	})

	t.Run("ImChatSearch invalid page size", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"query":     "ok",
			"page-size": "0",
		}, nil)
		err := ImChatSearch.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "--page-size must be an integer between 1 and 100") {
			t.Fatalf("ImChatSearch.Validate() error = %v", err)
		}
	})

	t.Run("ImChatSearch query too long", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"query": strings.Repeat("q", 65),
		}, nil)
		err := ImChatSearch.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "--query exceeds the maximum of 64 characters") {
			t.Fatalf("ImChatSearch.Validate() error = %v", err)
		}
	})

	t.Run("ImChatUpdate requires fields", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"chat-id": "oc_123",
		}, nil)
		err := ImChatUpdate.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "at least one field must be specified") {
			t.Fatalf("ImChatUpdate.Validate() error = %v", err)
		}
	})

	t.Run("ImChatUpdate invalid chat id", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"chat-id": "bad_chat",
			"name":    "new",
		}, nil)
		err := ImChatUpdate.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "invalid chat ID format") {
			t.Fatalf("ImChatUpdate.Validate() error = %v", err)
		}
	})

	t.Run("ImChatUpdate description too long", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"chat-id":     "oc_123",
			"description": strings.Repeat("x", 101),
		}, nil)
		err := ImChatUpdate.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "--description exceeds the maximum of 100 characters") {
			t.Fatalf("ImChatUpdate.Validate() error = %v", err)
		}
	})

	t.Run("ImMessagesSend conflicting target", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"chat-id": "oc_123",
			"user-id": "ou_123",
			"text":    "hello",
		}, nil)
		err := ImMessagesSend.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "--chat-id and --user-id are mutually exclusive") {
			t.Fatalf("ImMessagesSend.Validate() error = %v", err)
		}
	})

	t.Run("ImMessagesSend invalid content json", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"chat-id": "oc_123",
			"content": "{invalid",
		}, nil)
		err := ImMessagesSend.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "--content is not valid JSON") {
			t.Fatalf("ImMessagesSend.Validate() error = %v", err)
		}
	})

	t.Run("ImMessagesSend media with text", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"chat-id": "oc_123",
			"text":    "hello",
			"image":   "img_123",
		}, nil)
		err := ImMessagesSend.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "--image/--file/--video/--audio cannot be used with --text, --markdown, or --content") {
			t.Fatalf("ImMessagesSend.Validate() error = %v", err)
		}
	})

	t.Run("ImMessagesSend valid text", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"chat-id": "oc_123",
			"text":    "hello",
		}, nil)
		if err := ImMessagesSend.Validate(context.Background(), runtime); err != nil {
			t.Fatalf("ImMessagesSend.Validate() unexpected error = %v", err)
		}
	})

	t.Run("ImMessagesSend video with video-cover passes validate", func(t *testing.T) {
		// Previously broken: the deleted check used imageKey instead of videoCoverKey,
		// so --video + --video-cover would incorrectly fail at Validate.
		runtime := newTestRuntimeContext(t, map[string]string{
			"chat-id":     "oc_123",
			"video":       "file_456",
			"video-cover": "img_789",
		}, nil)
		if err := ImMessagesSend.Validate(context.Background(), runtime); err != nil {
			t.Fatalf("ImMessagesSend.Validate() unexpected error = %v", err)
		}
	})

	t.Run("ImMessagesSend video without video-cover fails validate", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"chat-id": "oc_123",
			"video":   "file_456",
		}, nil)
		err := ImMessagesSend.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "--video-cover is required when using --video") {
			t.Fatalf("ImMessagesSend.Validate() error = %v", err)
		}
	})

	t.Run("ImMessagesSend video-cover without video fails validate", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"chat-id":     "oc_123",
			"video-cover": "img_789",
		}, nil)
		err := ImMessagesSend.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "--video-cover can only be used with --video") {
			t.Fatalf("ImMessagesSend.Validate() error = %v", err)
		}
	})

	t.Run("ImMessagesSend conflicting explicit msg-type", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"chat-id":  "oc_123",
			"msg-type": "file",
			"image":    "img_123",
		}, nil)
		err := ImMessagesSend.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "conflicts with the inferred message type") {
			t.Fatalf("ImMessagesSend.Validate() error = %v", err)
		}
	})

	t.Run("ImMessagesReply invalid message id", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"message-id": "bad_id",
			"text":       "hello",
		}, nil)
		err := ImMessagesReply.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "must start with om_") {
			t.Fatalf("ImMessagesReply.Validate() error = %v", err)
		}
	})

	t.Run("ImThreadsMessagesList invalid thread", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"thread": "bad_thread",
		}, nil)
		err := ImThreadsMessagesList.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "must start with om_ or omt_") {
			t.Fatalf("ImThreadsMessagesList.Validate() error = %v", err)
		}
	})

	t.Run("ImChatMessageList requires one target", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{}, nil)
		err := ImChatMessageList.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "specify at least one of --chat-id or --user-id") {
			t.Fatalf("ImChatMessageList.Validate() error = %v", err)
		}
	})

	t.Run("ImChatMessageList valid user target", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"user-id": "ou_123",
		}, nil)
		if err := ImChatMessageList.Validate(context.Background(), runtime); err != nil {
			t.Fatalf("ImChatMessageList.Validate() unexpected error = %v", err)
		}
	})

	t.Run("ImChatMessageList rejects both targets", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"chat-id": "oc_abc",
			"user-id": "ou_123",
		}, nil)
		err := ImChatMessageList.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
			t.Fatalf("ImChatMessageList.Validate() error = %v, want mutually exclusive", err)
		}
	})

	t.Run("ImChatMessageList rejects user target for bot identity", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"user-id": "ou_123",
		}, nil)
		setRuntimeField(t, runtime, "resolvedAs", core.AsBot)
		err := ImChatMessageList.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "requires user identity") {
			t.Fatalf("ImChatMessageList.Validate() error = %v, want requires user identity", err)
		}
	})

	t.Run("ImMessagesMGet empty ids", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"message-ids": " , ",
		}, nil)
		err := ImMessagesMGet.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "--message-ids is required") {
			t.Fatalf("ImMessagesMGet.Validate() error = %v", err)
		}
	})

	t.Run("ImMessagesMGet invalid id", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"message-ids": "om_1,bad_2",
		}, nil)
		err := ImMessagesMGet.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "invalid message ID") {
			t.Fatalf("ImMessagesMGet.Validate() error = %v", err)
		}
	})

	t.Run("ImMessagesResourcesDownload invalid message id", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"message-id": "bad_id",
			"file-key":   "img_123",
			"type":       "image",
		}, nil)
		err := ImMessagesResourcesDownload.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "must start with om_") {
			t.Fatalf("ImMessagesResourcesDownload.Validate() error = %v", err)
		}
	})

	t.Run("ImThreadsMessagesList valid omt thread", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"thread": "omt_123",
		}, nil)
		if err := ImThreadsMessagesList.Validate(context.Background(), runtime); err != nil {
			t.Fatalf("ImThreadsMessagesList.Validate() unexpected error = %v", err)
		}
	})

	t.Run("ImMessagesSearch invalid page size", func(t *testing.T) {
		runtime := newMessagesSearchTestRuntimeContext(t, map[string]string{
			"query":     "incident",
			"page-size": "0",
		}, nil)
		err := ImMessagesSearch.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "--page-size must be an integer between 1 and 50") {
			t.Fatalf("ImMessagesSearch.Validate() error = %v", err)
		}
	})

	t.Run("ImMessagesSearch invalid page limit", func(t *testing.T) {
		runtime := newMessagesSearchTestRuntimeContext(t, map[string]string{
			"query":      "incident",
			"page-limit": "41",
		}, nil)
		err := ImMessagesSearch.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "--page-limit must be an integer between 1 and 40") {
			t.Fatalf("ImMessagesSearch.Validate() error = %v", err)
		}
	})

	t.Run("ImMessagesSearch invalid sender id", func(t *testing.T) {
		runtime := newMessagesSearchTestRuntimeContext(t, map[string]string{
			"sender": "user_1",
		}, nil)
		err := ImMessagesSearch.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "invalid user ID") {
			t.Fatalf("ImMessagesSearch.Validate() error = %v", err)
		}
	})

	t.Run("ImMessagesSearch invalid chat id", func(t *testing.T) {
		runtime := newMessagesSearchTestRuntimeContext(t, map[string]string{
			"chat-id": "bad_chat",
		}, nil)
		err := ImMessagesSearch.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "invalid chat ID") {
			t.Fatalf("ImMessagesSearch.Validate() error = %v", err)
		}
	})

	t.Run("ImMessagesSearch invalid time range", func(t *testing.T) {
		runtime := newMessagesSearchTestRuntimeContext(t, map[string]string{
			"start": "2025-01-02T00:00:00Z",
			"end":   "2025-01-01T00:00:00Z",
		}, nil)
		err := ImMessagesSearch.Validate(context.Background(), runtime)
		if err == nil || !strings.Contains(err.Error(), "--start cannot be later than --end") {
			t.Fatalf("ImMessagesSearch.Validate() error = %v", err)
		}
	})
}

func TestMessagesSearchPaginationConfig(t *testing.T) {
	t.Run("default single page", func(t *testing.T) {
		runtime := newMessagesSearchTestRuntimeContext(t, nil, nil)
		autoPaginate, pageLimit := messagesSearchPaginationConfig(runtime)
		if autoPaginate {
			t.Fatal("messagesSearchPaginationConfig() autoPaginate = true, want false")
		}
		if pageLimit != messagesSearchDefaultPageLimit {
			t.Fatalf("messagesSearchPaginationConfig() pageLimit = %d, want %d", pageLimit, messagesSearchDefaultPageLimit)
		}
	})

	t.Run("page all uses max limit", func(t *testing.T) {
		runtime := newMessagesSearchTestRuntimeContext(t, nil, map[string]bool{
			"page-all": true,
		})
		autoPaginate, pageLimit := messagesSearchPaginationConfig(runtime)
		if !autoPaginate {
			t.Fatal("messagesSearchPaginationConfig() autoPaginate = false, want true")
		}
		if pageLimit != messagesSearchMaxPageLimit {
			t.Fatalf("messagesSearchPaginationConfig() pageLimit = %d, want %d", pageLimit, messagesSearchMaxPageLimit)
		}
	})

	t.Run("explicit page limit enables auto pagination", func(t *testing.T) {
		runtime := newMessagesSearchTestRuntimeContext(t, map[string]string{
			"query":      "incident",
			"page-limit": "3",
		}, nil)
		if err := ImMessagesSearch.Validate(context.Background(), runtime); err != nil {
			t.Fatalf("ImMessagesSearch.Validate() error = %v, want valid explicit --page-limit", err)
		}
		autoPaginate, pageLimit := messagesSearchPaginationConfig(runtime)
		if !autoPaginate {
			t.Fatal("messagesSearchPaginationConfig() autoPaginate = false, want true")
		}
		if pageLimit != 3 {
			t.Fatalf("messagesSearchPaginationConfig() pageLimit = %d, want 3", pageLimit)
		}
	})
}

// TestShortcutDryRunShapes verifies that each shortcut's DryRun function
// produces the expected API path, query parameters, and request body.
func TestShortcutDryRunShapes(t *testing.T) {
	t.Run("ImChatCreate dry run includes params and body", func(t *testing.T) {
		cmd := &cobra.Command{Use: "test"}
		for _, name := range []string{"type", "name", "users", "owner", "chat-mode"} {
			cmd.Flags().String(name, "", "")
		}
		cmd.Flags().Bool("set-bot-manager", false, "")
		_ = cmd.ParseFlags(nil)
		_ = cmd.Flags().Set("type", "public")
		_ = cmd.Flags().Set("name", "Team Room")
		_ = cmd.Flags().Set("users", "ou_1,ou_2")
		_ = cmd.Flags().Set("owner", "ou_owner")
		_ = cmd.Flags().Set("set-bot-manager", "true")
		_ = cmd.Flags().Set("chat-mode", "group")
		runtime := common.TestNewRuntimeContextWithIdentity(cmd, nil, "bot")
		got := mustMarshalDryRun(t, ImChatCreate.DryRun(context.Background(), runtime))
		if !strings.Contains(got, `"/open-apis/im/v1/chats"`) || !strings.Contains(got, `"set_bot_manager":true`) || !strings.Contains(got, `"chat_type":"public"`) || !strings.Contains(got, `"chat_mode":"group"`) {
			t.Fatalf("ImChatCreate.DryRun() = %s", got)
		}
	})

	t.Run("ImChatSearch dry run includes built params", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"query":      "team-alpha",
			"page-size":  "50",
			"page-token": "next_page",
		}, nil)
		got := mustMarshalDryRun(t, ImChatSearch.DryRun(context.Background(), runtime))
		if !strings.Contains(got, `"/open-apis/im/v2/chats/search"`) || !strings.Contains(got, `"page_size":20`) || !strings.Contains(got, `"query":"\"team-alpha\""`) {
			t.Fatalf("ImChatSearch.DryRun() = %s", got)
		}
	})

	t.Run("ImMessagesSearch dry run uses messages search endpoint", func(t *testing.T) {
		runtime := newMessagesSearchTestRuntimeContext(t, map[string]string{
			"query":      "incident",
			"page-size":  "51",
			"page-token": "next_page",
		}, nil)
		got := mustMarshalDryRun(t, ImMessagesSearch.DryRun(context.Background(), runtime))
		if !strings.Contains(got, `"/open-apis/im/v1/messages/search"`) || !strings.Contains(got, `"page_size":"50"`) || !strings.Contains(got, `"query":"incident"`) {
			t.Fatalf("ImMessagesSearch.DryRun() = %s", got)
		}
	})

	t.Run("ImChatUpdate dry run resolves path", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"chat-id":     "oc_123",
			"name":        "New Name",
			"description": "New Description",
		}, nil)
		got := mustMarshalDryRun(t, ImChatUpdate.DryRun(context.Background(), runtime))
		if !strings.Contains(got, `"/open-apis/im/v1/chats/oc_123"`) || !strings.Contains(got, `"user_id_type":"open_id"`) || !strings.Contains(got, `"name":"New Name"`) {
			t.Fatalf("ImChatUpdate.DryRun() = %s", got)
		}
	})

	t.Run("ImMessagesSend dry run resolves open_id target", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"user-id":         "ou_123",
			"image":           "img_123",
			"idempotency-key": "uuid-2",
		}, nil)
		got := mustMarshalDryRun(t, ImMessagesSend.DryRun(context.Background(), runtime))
		if !strings.Contains(got, `"receive_id_type":"open_id"`) || !strings.Contains(got, `"msg_type":"image"`) || !strings.Contains(got, `"uuid":"uuid-2"`) || !strings.Contains(got, `\"image_key\":\"img_123\"`) {
			t.Fatalf("ImMessagesSend.DryRun() = %s", got)
		}
	})

	t.Run("ImMessagesSend dry run uses placeholder media key for url input", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"chat-id": "oc_123",
			"image":   "https://example.com/a.png",
		}, nil)
		got := mustMarshalDryRun(t, ImMessagesSend.DryRun(context.Background(), runtime))
		if !strings.Contains(got, `"description":"dry-run uses placeholder media keys for --image URL input; execution uploads it before sending"`) ||
			!strings.Contains(got, `"msg_type":"image"`) ||
			!strings.Contains(got, `\"image_key\":\"img_dryrun_upload\"`) {
			t.Fatalf("ImMessagesSend.DryRun() = %s", got)
		}
	})

	t.Run("ImMessagesMGet dry run expands message ids", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"message-ids": "om_1,om_2",
		}, nil)
		got := mustMarshalDryRun(t, ImMessagesMGet.DryRun(context.Background(), runtime))
		if !strings.Contains(got, `"/open-apis/im/v1/messages/mget?card_msg_content_type=raw_card_content\u0026message_ids=om_1\u0026message_ids=om_2"`) {
			t.Fatalf("ImMessagesMGet.DryRun() = %s", got)
		}
	})

	t.Run("ImMessagesResourcesDownload dry run resolves path", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"message-id": "om_123",
			"file-key":   "img_123",
			"type":       "image",
			"output":     "downloads/out.png",
		}, nil)
		got := mustMarshalDryRun(t, ImMessagesResourcesDownload.DryRun(context.Background(), runtime))
		if !strings.Contains(got, `"/open-apis/im/v1/messages/om_123/resources/img_123"`) || !strings.Contains(got, `"type":"image"`) || !strings.Contains(got, `"output":"downloads/out.png"`) {
			t.Fatalf("ImMessagesResourcesDownload.DryRun() = %s", got)
		}
	})

	t.Run("ImThreadsMessagesList dry run keeps requested thread params", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"thread":    "omt_123",
			"sort":      "desc",
			"page-size": "10",
		}, nil)
		got := mustMarshalDryRun(t, ImThreadsMessagesList.DryRun(context.Background(), runtime))
		if !strings.Contains(got, `"container_id":"omt_123"`) || !strings.Contains(got, `"sort_type":"ByCreateTimeDesc"`) || !strings.Contains(got, `"page_size":10`) {
			t.Fatalf("ImThreadsMessagesList.DryRun() = %s", got)
		}
	})

	t.Run("ImMessagesReply dry run resolves message path and body", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"message-id":      "om_123",
			"text":            "hi <at id=ou_1/>",
			"idempotency-key": "uuid-1",
		}, map[string]bool{
			"reply-in-thread": true,
		})
		got := mustMarshalDryRun(t, ImMessagesReply.DryRun(context.Background(), runtime))
		if !strings.Contains(got, "/open-apis/im/v1/messages/om_123/reply") || !strings.Contains(got, `"reply_in_thread":true`) || !strings.Contains(got, `"uuid":"uuid-1"`) {
			t.Fatalf("ImMessagesReply.DryRun() = %s", got)
		}
	})

	t.Run("ImMessagesReply dry run uses markdown image placeholders", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"message-id": "om_123",
			"markdown":   "![alt](https://example.com/a.png)",
		}, nil)
		got := mustMarshalDryRun(t, ImMessagesReply.DryRun(context.Background(), runtime))
		if !strings.Contains(got, `"description":"dry-run uses placeholder image keys for markdown image URLs; execution downloads and uploads them before sending"`) ||
			!strings.Contains(got, `"msg_type":"post"`) ||
			!strings.Contains(got, `img_dryrun_1`) {
			t.Fatalf("ImMessagesReply.DryRun() = %s", got)
		}
	})

	t.Run("ImChatMessageList dry run notes p2p resolution", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"user-id":   "ou_123",
			"page-size": "10",
			"sort":      "asc",
		}, nil)
		d := ImChatMessageList.DryRun(context.Background(), runtime)
		formatted := d.Format()
		if !strings.Contains(formatted, "resolve P2P chat_id") || !strings.Contains(formatted, "container_id=%3Cresolved_chat_id%3E") {
			t.Fatalf("ImChatMessageList.DryRun().Format() = %s", formatted)
		}
	})

	t.Run("ImChatMessageList dry run includes root-only query", func(t *testing.T) {
		runtime := newTestRuntimeContext(t, map[string]string{
			"chat-id":   "oc_123",
			"page-size": "20",
			"sort":      "desc",
		}, nil)
		formatted := ImChatMessageList.DryRun(context.Background(), runtime).Format()
		if !strings.Contains(formatted, "only_thread_root_messages=true") {
			t.Fatalf("ImChatMessageList.DryRun().Format() = %s, want only_thread_root_messages=true", formatted)
		}
	})
}

func TestChatMessageListOnlyThreadRootMessagesDryRun(t *testing.T) {
	runtime := newTestRuntimeContext(t, map[string]string{
		"chat-id":   "oc_123",
		"page-size": "20",
		"sort":      "desc",
	}, nil)

	formatted := ImChatMessageList.DryRun(context.Background(), runtime).Format()
	if !strings.Contains(formatted, "only_thread_root_messages=true") {
		t.Fatalf("ImChatMessageList.DryRun().Format() = %s, want only_thread_root_messages=true", formatted)
	}
}
