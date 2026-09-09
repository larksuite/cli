// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package profile

import (
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

func TestProfileListRunShowsCurrentUser(t *testing.T) {
	setupProfileConfigDir(t)
	multi := &core.MultiAppConfig{
		CurrentApp: "default",
		Apps: []core.AppConfig{{
			Name:        "default",
			AppId:       "app-default",
			AppSecret:   core.PlainSecret("secret"),
			Brand:       core.BrandFeishu,
			CurrentUser: "ou_second",
			Users: []core.AppUser{
				{UserOpenId: "ou_first", UserName: "first"},
				{UserOpenId: "ou_second", UserName: "second"},
			},
		}},
	}
	if err := core.SaveMultiAppConfig(multi); err != nil {
		t.Fatalf("SaveMultiAppConfig() error = %v", err)
	}

	f, stdout, _, _ := cmdutil.TestFactory(t, nil)
	if err := profileListRun(f); err != nil {
		t.Fatalf("profileListRun() error = %v", err)
	}
	var rows []profileListItem
	if err := json.Unmarshal(stdout.Bytes(), &rows); err != nil {
		t.Fatalf("stdout must be JSON: %v\n%s", err, stdout.String())
	}
	if len(rows) != 1 || rows[0].User != "second" {
		t.Fatalf("rows = %#v, want current user second", rows)
	}
}
