// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

const fileQuotaURL = "/open-apis/spark/v1/apps/app_x/storage/file_quota"

// TestAppsFileQuotaGet_QuotaConnectedShowsAllFields 验证配额已对接时输出 storage_quota_bytes/usage_percent/files 全字段。
func TestAppsFileQuotaGet_QuotaConnectedShowsAllFields(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: fileQuotaURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"storage_used_bytes":  157286400,
			"storage_quota_bytes": 1073741824,
			"usage_percent":       14.6,
			"files":               42,
		}},
	})
	if err := runAppsShortcut(t, AppsFileQuotaGet,
		[]string{"+file-quota-get", "--app-id", "app_x", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, want := range []string{`"storage_quota_bytes"`, `"usage_percent"`, `"files"`} {
		if !strings.Contains(got, want) {
			t.Errorf("quota json missing %q:\n%s", want, got)
		}
	}
}

// 配额未对接（=0）：storage_quota_bytes / usage_percent 不输出。
func TestAppsFileQuotaGet_UnconnectedOmitsQuotaFields(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET", URL: fileQuotaURL,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{
			"storage_used_bytes":  157286400,
			"storage_quota_bytes": 0,
			"usage_percent":       0,
			"files":               42,
		}},
	})
	if err := runAppsShortcut(t, AppsFileQuotaGet,
		[]string{"+file-quota-get", "--app-id", "app_x", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	for _, banned := range []string{"storage_quota_bytes", "usage_percent"} {
		if strings.Contains(got, banned) {
			t.Errorf("unconnected quota should omit %q:\n%s", banned, got)
		}
	}
	if !strings.Contains(got, `"storage_used_bytes"`) || !strings.Contains(got, `"files"`) {
		t.Errorf("should still show used/files:\n%s", got)
	}
}

// TestProjectFileQuota_OmitsZeroQuotaAndDropsUnknownFields 验证 projectFileQuota 白名单投影：
// quota=0 时不输出 storage_quota_bytes/usage_percent，非零时保留；后端额外字段不透传。
func TestProjectFileQuota_OmitsZeroQuotaAndDropsUnknownFields(t *testing.T) {
	out := projectFileQuota(map[string]interface{}{
		"storage_used_bytes": 100, "storage_quota_bytes": float64(0), "usage_percent": float64(0),
		"files": 3, "tenant_key": "leak", "request_id": "rid",
	})
	if _, ok := out["storage_quota_bytes"]; ok {
		t.Errorf("zero quota should be omitted: %v", out)
	}
	if _, ok := out["usage_percent"]; ok {
		t.Errorf("usage_percent should be omitted when quota=0: %v", out)
	}
	if out["storage_used_bytes"] != 100 || out["files"] != 3 {
		t.Errorf("whitelisted fields should be kept: %v", out)
	}
	// 白名单外的字段必须被丢弃，避免无用字段消耗 agent 上下文。
	for _, leaked := range []string{"tenant_key", "request_id"} {
		if _, ok := out[leaked]; ok {
			t.Errorf("non-whitelisted field %q must be dropped: %v", leaked, out)
		}
	}

	out2 := projectFileQuota(map[string]interface{}{"storage_used_bytes": 100, "storage_quota_bytes": float64(1024), "usage_percent": float64(9.8), "files": 3})
	if _, ok := out2["storage_quota_bytes"]; !ok {
		t.Errorf("non-zero quota should be kept: %v", out2)
	}
	if _, ok := out2["usage_percent"]; !ok {
		t.Errorf("usage_percent should be kept when quota>0: %v", out2)
	}
}
