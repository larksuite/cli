// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func testRuntimeWithNameDesc(t *testing.T, name, desc string) *common.RuntimeContext {
	t.Helper()
	cmd := &cobra.Command{Use: "update"}
	cmd.Flags().String("name", name, "")
	cmd.Flags().String("description", desc, "")
	return common.TestNewRuntimeContext(cmd, nil)
}

func TestBuildAppsUpdateBody_FieldCombos(t *testing.T) {
	t.Run("both empty -> empty body", func(t *testing.T) {
		if body := buildAppsUpdateBody(testRuntimeWithNameDesc(t, "  ", "")); len(body) != 0 {
			t.Errorf("empty inputs should yield empty body, got %v", body)
		}
	})
	t.Run("name only", func(t *testing.T) {
		body := buildAppsUpdateBody(testRuntimeWithNameDesc(t, "App", ""))
		if body["name"] != "App" || len(body) != 1 {
			t.Errorf("name-only body=%v", body)
		}
	})
	t.Run("description only", func(t *testing.T) {
		body := buildAppsUpdateBody(testRuntimeWithNameDesc(t, "", "desc"))
		if body["description"] != "desc" || len(body) != 1 {
			t.Errorf("desc-only body=%v", body)
		}
	})
	t.Run("both set and trimmed", func(t *testing.T) {
		body := buildAppsUpdateBody(testRuntimeWithNameDesc(t, "  App  ", "  d  "))
		if body["name"] != "App" || body["description"] != "d" {
			t.Errorf("both body=%v", body)
		}
	})
}

func TestAppsUpdate_PartialFields(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/spark/v1/apps/app_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"app": map[string]interface{}{
					"app_id":     "app_x",
					"name":       "renamed",
					"updated_at": "2026-05-18T10:05:00Z",
				},
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
			"data": map[string]interface{}{
				"app": map[string]interface{}{"app_id": "app_x"},
			},
		},
	}
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "  app_x  ", "--name", "renamed", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
}

// TestAppsUpdate_PrettyOutputReadsNestedAppID exercises the prettyFn callback
// passed to OutFormat (only invoked under --format pretty) so the new
// data.app.app_id nesting is actually read by the text writer.
func TestAppsUpdate_PrettyOutputReadsNestedAppID(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/spark/v1/apps/app_x",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"app": map[string]interface{}{"app_id": "app_x", "name": "renamed"},
			},
		},
	})

	if err := runAppsShortcut(t, AppsUpdate,
		[]string{"+update", "--app-id", "app_x", "--name", "renamed", "--format", "pretty", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, "updated: app_x") {
		t.Fatalf("pretty output should read app_id from data.app.app_id, got: %q", got)
	}
}
