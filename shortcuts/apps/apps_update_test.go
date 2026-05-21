// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestAppsUpdate_PartialFields(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/spark/v1/apps/app_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"app_id":     "app_x",
				"name":       "renamed",
				"updated_at": "2026-05-18T10:05:00Z",
			},
		},
	}
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "app_x", "--name", "renamed", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}

	var sent map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &sent); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if sent["name"] != "renamed" {
		t.Fatalf("body.name = %v", sent["name"])
	}
	if _, present := sent["description"]; present {
		t.Fatalf("description should not be in body when not provided: %v", sent)
	}
}

func TestAppsUpdate_RequiresAppID(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--name", "renamed", "--as", "user"}, factory, stdout)
	// cobra Required:true may match "app-id" instead of "--app-id"
	if err == nil || !strings.Contains(err.Error(), "app-id") {
		t.Fatalf("expected --app-id required, got %v", err)
	}
}

func TestAppsUpdate_RequiresAtLeastOneField(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "app_x", "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatalf("expected error when no field provided")
	}
}

func TestAppsUpdate_TrimsAppIDInPath(t *testing.T) {
	// 钉死 --app-id 在拼进 URL 前要先 TrimSpace —— 与 create / access-scope-* 等保持一致，
	// 避免 " app_x " 这种取值被原样 EncodePathSegment 编进 path 出现空格转义。
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/spark/v1/apps/app_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"app_id": "app_x"},
		},
	}
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "  app_x  ", "--name", "renamed", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
}
