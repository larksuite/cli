// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func TestBuildAppFeedCardCreateBody(t *testing.T) {
	runtime := newTestRuntimeContext(t, map[string]string{
		"user-ids":          "ou_1, ou_2",
		"title":             "Order ready",
		"preview":           "Tap to view details",
		"link":              "https://example.com/orders/1",
		"status-label-text": "Open",
		"button-text":       "View",
		"button-url":        "https://example.com/orders/1/action",
		"custom-sound-text": "New order",
	}, map[string]bool{
		"time-sensitive": true,
		"close-notify":   true,
	})

	body, err := buildAppFeedCardCreateBody(runtime)
	if err != nil {
		t.Fatalf("buildAppFeedCardCreateBody() error = %v", err)
	}
	userIDs, _ := body["user_ids"].([]string)
	if len(userIDs) != 2 || userIDs[0] != "ou_1" || userIDs[1] != "ou_2" {
		t.Fatalf("user_ids = %#v", body["user_ids"])
	}

	card, _ := body["app_feed_card"].(map[string]interface{})
	if card["title"] != "Order ready" || card["preview"] != "Tap to view details" || card["time_sensitive"] != true {
		t.Fatalf("app_feed_card = %#v", card)
	}
	status, _ := card["status_label"].(map[string]interface{})
	if status["text"] != "Open" || status["type"] != "primary" {
		t.Fatalf("status_label = %#v", status)
	}
	notify, _ := card["notify"].(map[string]interface{})
	if notify["close_notify"] != true || notify["custom_sound_text"] != "New order" {
		t.Fatalf("notify = %#v", notify)
	}
	buttons, _ := card["buttons"].(map[string]interface{})
	rawButtons, _ := buttons["buttons"].([]interface{})
	if len(rawButtons) != 1 {
		t.Fatalf("buttons = %#v", buttons)
	}
	button, _ := rawButtons[0].(map[string]interface{})
	if button["action_type"] != "url_page" {
		t.Fatalf("button = %#v", button)
	}
}

func TestBuildAppFeedCardCreateBodyAcceptsRawJSON(t *testing.T) {
	runtime := newTestRuntimeContext(t, map[string]string{
		"user-ids": "ou_1",
		"card-json": `{
			"app_feed_card": {
				"title": "From JSON",
				"link": {"link": "https://example.com/card"}
			}
		}`,
		"buttons-json": `[
			{
				"action_type": "url_page",
				"text": {"text": "Open"},
				"multi_url": {"url": "https://example.com/open"}
			}
		]`,
	}, nil)

	body, err := buildAppFeedCardCreateBody(runtime)
	if err != nil {
		t.Fatalf("buildAppFeedCardCreateBody() error = %v", err)
	}
	card, _ := body["app_feed_card"].(map[string]interface{})
	if card["title"] != "From JSON" {
		t.Fatalf("card title = %#v", card["title"])
	}
	buttons, _ := card["buttons"].(map[string]interface{})
	if rawButtons, _ := buttons["buttons"].([]interface{}); len(rawButtons) != 1 {
		t.Fatalf("buttons = %#v", buttons)
	}
}

func TestBuildAppFeedCardCreateBodyValidation(t *testing.T) {
	invalidOpenIDErr := invalidOpenIDError(t)
	tests := []struct {
		name      string
		strFlags  map[string]string
		boolFlags map[string]bool
		wantErr   string
	}{
		{
			name: "missing link",
			strFlags: map[string]string{
				"user-ids": "ou_1",
				"title":    "Missing link",
			},
			wantErr: "--link is required",
		},
		{
			name: "invalid recipient open id",
			strFlags: map[string]string{
				"user-ids": "bad_user",
				"link":     "https://example.com/card",
			},
			wantErr: invalidOpenIDErr,
		},
		{
			name: "http link rejected",
			strFlags: map[string]string{
				"user-ids": "ou_1",
				"link":     "http://example.com/card",
			},
			wantErr: "must use HTTPS or applink",
		},
		{
			name: "hostless https link rejected",
			strFlags: map[string]string{
				"user-ids": "ou_1",
				"link":     "https:example.com/card",
			},
			wantErr: "must be a valid HTTPS URL",
		},
		{
			name: "too many buttons",
			strFlags: map[string]string{
				"user-ids":     "ou_1",
				"link":         "https://example.com/card",
				"buttons-json": `[{"action_type":"url_page","text":{"text":"1"},"multi_url":{"url":"https://example.com/1"}},{"action_type":"url_page","text":{"text":"2"},"multi_url":{"url":"https://example.com/2"}},{"action_type":"url_page","text":{"text":"3"},"multi_url":{"url":"https://example.com/3"}}]`,
			},
			wantErr: "at most 2 buttons",
		},
		{
			name: "url page button requires non-empty url",
			strFlags: map[string]string{
				"user-ids":     "ou_1",
				"link":         "https://example.com/card",
				"buttons-json": `[{"action_type":"url_page","text":{"text":"Open"},"multi_url":{"url":""}}]`,
			},
			wantErr: "must contain at least one HTTPS URL",
		},
		{
			name: "url page button rejects hostless https",
			strFlags: map[string]string{
				"user-ids":     "ou_1",
				"link":         "https://example.com/card",
				"buttons-json": `[{"action_type":"url_page","text":{"text":"Open"},"multi_url":{"url":"https:example.com/open"}}]`,
			},
			wantErr: "must be a valid HTTPS URL",
		},
		{
			name: "url page button url must be string",
			strFlags: map[string]string{
				"user-ids":     "ou_1",
				"link":         "https://example.com/card",
				"buttons-json": `[{"action_type":"url_page","text":{"text":"Open"},"multi_url":{"url":123}}]`,
			},
			wantErr: "must be a string",
		},
		{
			name: "status type needs text",
			strFlags: map[string]string{
				"user-ids":          "ou_1",
				"link":              "https://example.com/card",
				"status-label-type": "success",
			},
			wantErr: "--status-label-type requires --status-label-text",
		},
		{
			name: "union id bypasses open id prefix validation",
			strFlags: map[string]string{
				"user-ids":     "onion_1",
				"user-id-type": "union_id",
				"link":         "https://example.com/card",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := newTestRuntimeContext(t, tt.strFlags, tt.boolFlags)
			_, err := buildAppFeedCardCreateBody(runtime)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestBuildAppFeedCardUpdateBody(t *testing.T) {
	runtime := newTestRuntimeContext(t, map[string]string{
		"user-ids": "ou_1,ou_2",
		"biz-id":   "biz_123",
		"title":    "Updated title",
		"preview":  "Updated preview",
		"link":     "https://example.com/updated",
	}, nil)

	body, err := buildAppFeedCardUpdateBody(runtime)
	if err != nil {
		t.Fatalf("buildAppFeedCardUpdateBody() error = %v", err)
	}
	feedCards, _ := body["feed_cards"].([]interface{})
	if len(feedCards) != 2 {
		t.Fatalf("feed_cards = %#v", body["feed_cards"])
	}
	first, _ := feedCards[0].(map[string]interface{})
	card, _ := first["app_feed_card"].(map[string]interface{})
	if card["biz_id"] != "biz_123" || card["title"] != "Updated title" {
		t.Fatalf("app_feed_card = %#v", card)
	}
	updateFields, _ := first["update_fields"].([]string)
	if strings.Join(updateFields, ",") != "1,3,12" {
		t.Fatalf("update_fields = %#v", first["update_fields"])
	}
	second, _ := feedCards[1].(map[string]interface{})
	secondCard, _ := second["app_feed_card"].(map[string]interface{})
	card["title"] = "mutated title"
	cardLink, _ := card["link"].(map[string]interface{})
	cardLink["link"] = "https://example.com/mutated"
	secondLink, _ := secondCard["link"].(map[string]interface{})
	if secondCard["title"] != "Updated title" || secondLink["link"] != "https://example.com/updated" {
		t.Fatalf("app_feed_card maps should not be shared across feed_cards: first=%#v second=%#v", card, secondCard)
	}
}

func TestBuildAppFeedCardUpdateBodyAcceptsRawJSON(t *testing.T) {
	runtime := newTestRuntimeContext(t, map[string]string{
		"feed-cards-json": `[
			{
				"user_id": "ou_1",
				"app_feed_card": {"biz_id": "biz_123", "title": "Updated"},
				"update_fields": ["title"]
			}
		]`,
	}, nil)

	body, err := buildAppFeedCardUpdateBody(runtime)
	if err != nil {
		t.Fatalf("buildAppFeedCardUpdateBody() error = %v", err)
	}
	feedCards, _ := body["feed_cards"].([]interface{})
	if len(feedCards) != 1 {
		t.Fatalf("feed_cards = %#v", body["feed_cards"])
	}
	first, _ := feedCards[0].(map[string]interface{})
	updateFields, _ := first["update_fields"].([]string)
	if strings.Join(updateFields, ",") != "1" {
		t.Fatalf("update_fields = %#v", first["update_fields"])
	}
}

func TestBuildAppFeedCardUpdateBodyAcceptsUpdateFieldNamesAndCodes(t *testing.T) {
	runtime := newTestRuntimeContext(t, map[string]string{
		"user-ids":      "ou_1",
		"biz-id":        "biz_123",
		"preview":       "Updated preview",
		"link":          "https://example.com/updated",
		"update-fields": "preview,12,notify",
	}, nil)

	body, err := buildAppFeedCardUpdateBody(runtime)
	if err != nil {
		t.Fatalf("buildAppFeedCardUpdateBody() error = %v", err)
	}
	feedCards, _ := body["feed_cards"].([]interface{})
	first, _ := feedCards[0].(map[string]interface{})
	updateFields, _ := first["update_fields"].([]string)
	if strings.Join(updateFields, ",") != "3,12,103" {
		t.Fatalf("update_fields = %#v", first["update_fields"])
	}
}

func TestBuildAppFeedCardDeleteBody(t *testing.T) {
	runtime := newTestRuntimeContext(t, map[string]string{
		"user-ids": "ou_1,ou_2",
		"biz-id":   "biz_123",
	}, nil)

	body, err := buildAppFeedCardDeleteBody(runtime)
	if err != nil {
		t.Fatalf("buildAppFeedCardDeleteBody() error = %v", err)
	}
	feedCards, _ := body["feed_cards"].([]interface{})
	if len(feedCards) != 2 {
		t.Fatalf("feed_cards = %#v", body["feed_cards"])
	}
	first, _ := feedCards[0].(map[string]interface{})
	if first["biz_id"] != "biz_123" || first["user_id"] != "ou_1" {
		t.Fatalf("feed_cards[0] = %#v", first)
	}
}

func TestBuildAppFeedCardBatchBodyValidation(t *testing.T) {
	invalidOpenIDErr := invalidOpenIDError(t)
	tests := []struct {
		name    string
		build   func(*common.RuntimeContext) (map[string]interface{}, error)
		flags   map[string]string
		wantErr string
	}{
		{
			name:    "update missing biz id",
			build:   buildAppFeedCardUpdateBody,
			flags:   map[string]string{"user-ids": "ou_1", "title": "Updated"},
			wantErr: "--biz-id is required",
		},
		{
			name:    "update missing fields",
			build:   buildAppFeedCardUpdateBody,
			flags:   map[string]string{"user-ids": "ou_1", "biz-id": "biz_123"},
			wantErr: "--update-fields is required",
		},
		{
			name:    "delete missing biz id",
			build:   buildAppFeedCardDeleteBody,
			flags:   map[string]string{"user-ids": "ou_1"},
			wantErr: "--biz-id is required",
		},
		{
			name:    "delete raw missing user id",
			build:   buildAppFeedCardDeleteBody,
			flags:   map[string]string{"feed-cards-json": `[{"biz_id":"biz_123"}]`},
			wantErr: "user_id is required",
		},
		{
			name:    "update raw invalid open id",
			build:   buildAppFeedCardUpdateBody,
			flags:   map[string]string{"feed-cards-json": `[{"user_id":"bad_user","app_feed_card":{"biz_id":"biz_123","title":"Updated"},"update_fields":["title"]}]`},
			wantErr: invalidOpenIDErr,
		},
		{
			name:    "delete raw invalid open id",
			build:   buildAppFeedCardDeleteBody,
			flags:   map[string]string{"feed-cards-json": `[{"user_id":"bad_user","biz_id":"biz_123"}]`},
			wantErr: invalidOpenIDErr,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := newTestRuntimeContext(t, tt.flags, nil)
			_, err := tt.build(runtime)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want substring %q", err, tt.wantErr)
			}
		})
	}
}

func TestAppFeedCardCreateDryRunShape(t *testing.T) {
	runtime := newTestRuntimeContext(t, map[string]string{
		"user-ids":    "ou_1",
		"title":       "Dry run card",
		"preview":     "Preview",
		"link":        "https://example.com/card",
		"button-text": "Open",
		"button-url":  "https://example.com/open",
	}, nil)

	got := mustMarshalDryRun(t, ImAppFeedCardCreate.DryRun(context.Background(), runtime))
	if !strings.Contains(got, `"/open-apis/im/v2/app_feed_card"`) ||
		!strings.Contains(got, `"user_id_type":"open_id"`) ||
		!strings.Contains(got, `"user_ids":["ou_1"]`) ||
		!strings.Contains(got, `"title":"Dry run card"`) ||
		!strings.Contains(got, `"action_type":"url_page"`) {
		t.Fatalf("ImAppFeedCardCreate.DryRun() = %s", got)
	}
}

func TestAppFeedCardBatchDryRunShape(t *testing.T) {
	updateRuntime := newTestRuntimeContext(t, map[string]string{
		"user-ids": "ou_1",
		"biz-id":   "biz_123",
		"title":    "Updated title",
	}, nil)
	updateGot := mustMarshalDryRun(t, ImAppFeedCardUpdate.DryRun(context.Background(), updateRuntime))
	if !strings.Contains(updateGot, `"method":"PUT"`) ||
		!strings.Contains(updateGot, `"/open-apis/im/v2/app_feed_card/batch"`) ||
		!strings.Contains(updateGot, `"update_fields":["1"]`) {
		t.Fatalf("ImAppFeedCardUpdate.DryRun() = %s", updateGot)
	}

	deleteRuntime := newTestRuntimeContext(t, map[string]string{
		"user-ids": "ou_1",
		"biz-id":   "biz_123",
	}, nil)
	deleteGot := mustMarshalDryRun(t, ImAppFeedCardDelete.DryRun(context.Background(), deleteRuntime))
	if !strings.Contains(deleteGot, `"method":"DELETE"`) ||
		!strings.Contains(deleteGot, `"/open-apis/im/v2/app_feed_card/batch"`) ||
		!strings.Contains(deleteGot, `"biz_id":"biz_123"`) {
		t.Fatalf("ImAppFeedCardDelete.DryRun() = %s", deleteGot)
	}
}

func TestAppFeedCardCreateExecute(t *testing.T) {
	factory, stdout, reg := newIMExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    appFeedCardCreatePath,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"biz_id":       "biz_123",
				"failed_cards": []interface{}{},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRunIMShortcut(t, ImAppFeedCardCreate, []string{
		"+app-feed-card-create",
		"--user-ids", "ou_1",
		"--title", "Execute card",
		"--link", "https://example.com/card",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunIMShortcut() error = %v", err)
	}

	got := stdout.String()
	if !strings.Contains(got, `"biz_id": "biz_123"`) || !strings.Contains(got, `"failed_count": 0`) {
		t.Fatalf("stdout = %s", got)
	}
	var captured map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &captured); err != nil {
		t.Fatalf("captured body JSON = %s, err=%v", string(stub.CapturedBody), err)
	}
	if _, ok := captured["app_feed_card"].(map[string]interface{}); !ok {
		t.Fatalf("captured body missing app_feed_card: %#v", captured)
	}
}

func TestAppFeedCardUpdateExecute(t *testing.T) {
	factory, stdout, reg := newIMExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PUT",
		URL:    appFeedCardBatchPath,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"failed_cards": []interface{}{},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRunIMShortcut(t, ImAppFeedCardUpdate, []string{
		"+app-feed-card-update",
		"--user-ids", "ou_1",
		"--biz-id", "biz_123",
		"--title", "Updated title",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunIMShortcut() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"requested_card_count": 1`) {
		t.Fatalf("stdout = %s", stdout.String())
	}

	var captured map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &captured); err != nil {
		t.Fatalf("captured body JSON = %s, err=%v", string(stub.CapturedBody), err)
	}
	feedCards, _ := captured["feed_cards"].([]interface{})
	if len(feedCards) != 1 {
		t.Fatalf("captured body missing feed_cards: %#v", captured)
	}
}

func TestAppFeedCardDeleteExecute(t *testing.T) {
	factory, stdout, reg := newIMExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "DELETE",
		URL:    appFeedCardBatchPath,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"failed_cards": []interface{}{},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRunIMShortcut(t, ImAppFeedCardDelete, []string{
		"+app-feed-card-delete",
		"--user-ids", "ou_1",
		"--biz-id", "biz_123",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunIMShortcut() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"requested_card_count": 1`) {
		t.Fatalf("stdout = %s", stdout.String())
	}

	var captured map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &captured); err != nil {
		t.Fatalf("captured body JSON = %s, err=%v", string(stub.CapturedBody), err)
	}
	feedCards, _ := captured["feed_cards"].([]interface{})
	first, _ := feedCards[0].(map[string]interface{})
	if first["biz_id"] != "biz_123" || first["user_id"] != "ou_1" {
		t.Fatalf("captured feed_cards = %#v", feedCards)
	}
}

func invalidOpenIDError(t *testing.T) string {
	t.Helper()
	_, err := common.ValidateUserID("bad_user")
	if err == nil {
		t.Fatal("common.ValidateUserID(bad_user) returned nil")
	}
	return err.Error()
}

func newIMExecuteFactory(t *testing.T) (*cmdutil.Factory, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()
	config := &core.CliConfig{
		AppID:     "test-app-" + strings.ReplaceAll(strings.ToLower(t.Name()), "/", "-"),
		AppSecret: "test-secret",
		Brand:     core.BrandFeishu,
	}
	factory, stdout, _, reg := cmdutil.TestFactory(t, config)
	return factory, stdout, reg
}

func mountAndRunIMShortcut(t *testing.T, shortcut common.Shortcut, args []string, factory *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "im"}
	shortcut.Mount(parent, factory)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	stdout.Reset()
	return parent.ExecuteContext(context.Background())
}
