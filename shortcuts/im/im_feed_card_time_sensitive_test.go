// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestBuildFeedCardTimeSensitiveBody(t *testing.T) {
	runtime := newTestRuntimeContext(t, map[string]string{
		"user-ids":       "ou_1, ou_2",
		"time-sensitive": "true",
	}, nil)

	body, err := buildFeedCardTimeSensitiveBody(runtime)
	if err != nil {
		t.Fatalf("buildFeedCardTimeSensitiveBody() error = %v", err)
	}
	userIDs, _ := body["user_ids"].([]string)
	if len(userIDs) != 2 || userIDs[0] != "ou_1" || userIDs[1] != "ou_2" {
		t.Fatalf("user_ids = %#v", body["user_ids"])
	}
	if body["time_sensitive"] != true {
		t.Fatalf("time_sensitive = %#v", body["time_sensitive"])
	}
}

func TestBuildFeedCardTimeSensitiveBodyAcceptsFalse(t *testing.T) {
	runtime := newTestRuntimeContext(t, map[string]string{
		"user-ids":       "ou_1",
		"time-sensitive": "false",
	}, nil)

	body, err := buildFeedCardTimeSensitiveBody(runtime)
	if err != nil {
		t.Fatalf("buildFeedCardTimeSensitiveBody() error = %v", err)
	}
	if body["time_sensitive"] != false {
		t.Fatalf("time_sensitive = %#v", body["time_sensitive"])
	}
}

func TestBuildFeedCardTimeSensitiveBodyValidation(t *testing.T) {
	invalidOpenIDErr := invalidOpenIDError(t)
	tests := []struct {
		name      string
		strFlags  map[string]string
		boolFlags map[string]bool
		wantErr   string
	}{
		{
			name:     "missing time sensitive",
			strFlags: map[string]string{"user-ids": "ou_1"},
			wantErr:  "--time-sensitive is required",
		},
		{
			name:     "invalid time sensitive",
			strFlags: map[string]string{"user-ids": "ou_1", "time-sensitive": "maybe"},
			wantErr:  "--time-sensitive must be true or false",
		},
		{
			name:     "invalid recipient open id",
			strFlags: map[string]string{"user-ids": "bad_user", "time-sensitive": "true"},
			wantErr:  invalidOpenIDErr,
		},
		{
			name:     "union id bypasses open id prefix validation",
			strFlags: map[string]string{"user-ids": "onion_1", "user-id-type": "union_id", "time-sensitive": "true"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := newTestRuntimeContext(t, tt.strFlags, tt.boolFlags)
			_, err := buildFeedCardTimeSensitiveBody(runtime)
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

func TestFeedCardIDForTimeSensitiveValidation(t *testing.T) {
	runtime := newTestRuntimeContext(t, map[string]string{"feed-card-id": "  "}, nil)
	if _, err := feedCardIDForTimeSensitive(runtime); err == nil || !strings.Contains(err.Error(), "--feed-card-id is required") {
		t.Fatalf("feedCardIDForTimeSensitive() error = %v", err)
	}

	runtime = newTestRuntimeContext(t, map[string]string{"feed-card-id": "om_123"}, nil)
	if _, err := feedCardIDForTimeSensitive(runtime); err == nil ||
		!strings.Contains(err.Error(), `starting with "oc_"`) {
		t.Fatalf("feedCardIDForTimeSensitive() non-group feed card error = %v", err)
	}
}

func TestFeedCardTimeSensitiveDryRunShape(t *testing.T) {
	cardRuntime := newTestRuntimeContext(t, map[string]string{
		"feed-card-id":   "oc_dryrun",
		"user-ids":       "ou_1",
		"time-sensitive": "false",
	}, nil)
	cardGot := mustMarshalDryRun(t, ImFeedCardTimeSensitive.DryRun(context.Background(), cardRuntime))
	if !strings.Contains(cardGot, `"method":"PATCH"`) ||
		!strings.Contains(cardGot, `"/open-apis/im/v2/feed_cards/oc_dryrun"`) ||
		!strings.Contains(cardGot, `"time_sensitive":false`) {
		t.Fatalf("ImFeedCardTimeSensitive.DryRun() = %s", cardGot)
	}
}

func TestFeedCardTimeSensitiveExecute(t *testing.T) {
	factory, stdout, reg := newIMExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/im/v2/feed_cards/oc_123",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{},
		},
	}
	reg.Register(stub)

	err := mountAndRunIMShortcut(t, ImFeedCardTimeSensitive, []string{
		"+feed-card-time-sensitive",
		"--feed-card-id", "oc_123",
		"--user-ids", "ou_1",
		"--time-sensitive", "false",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunIMShortcut() error = %v", err)
	}
	if !strings.Contains(stdout.String(), `"feed_card_id": "oc_123"`) || !strings.Contains(stdout.String(), `"time_sensitive": false`) {
		t.Fatalf("stdout = %s", stdout.String())
	}

	var captured map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &captured); err != nil {
		t.Fatalf("captured body JSON = %s, err=%v", string(stub.CapturedBody), err)
	}
	userIDs, _ := captured["user_ids"].([]interface{})
	if len(userIDs) != 1 || captured["time_sensitive"] != false {
		t.Fatalf("captured body = %#v", captured)
	}
}
