// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"encoding/json"
	"net/url"
	"reflect"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
)

func TestIntegrationContactUpdateSelfDryRun(t *testing.T) {
	for _, kind := range []core.CredentialKind{core.CredentialUAT, core.CredentialAAT} {
		t.Run(string(kind), func(t *testing.T) {
			testIntegrationContactUpdateSelfDryRun(t, kind)
		})
	}
}

func testIntegrationContactUpdateSelfDryRun(t *testing.T, kind core.CredentialKind) {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, stderr, _ := cmdutil.TestFactory(t, &core.CliConfig{
		AppID: "dry-run", AppSecret: "dry-run", Brand: core.BrandFeishu,
		UserOpenId: "ou_fixture", DefaultAs: core.AsUser,
		CredentialKind: kind, PrincipalOpenID: "ou_fixture",
	})
	root := buildIntegrationRootCmd(t, f)
	code := executeRootIntegration(t, f, root, []string{
		"contact", "+update-self", "--avatar-file", "missing.png", "--as", "user", "--dry-run",
	})
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("exit=%d stderr=%s", code, stderr.String())
	}
	var envelope struct {
		OK       bool   `json:"ok"`
		Identity string `json:"identity"`
		Data     struct {
			API []struct {
				Method string            `json:"method"`
				URL    string            `json:"url"`
				Params map[string]string `json:"params"`
				Body   json.RawMessage   `json:"body"`
			} `json:"api"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil || !envelope.OK || envelope.Identity != "user" {
		t.Fatalf("invalid dry-run envelope: %s err=%v", stdout.String(), err)
	}
	calls := envelope.Data.API
	if len(calls) != 3 || calls[0].Method != "GET" || calls[0].URL != "/open-apis/authen/v1/user_info" ||
		calls[1].Method != "POST" || calls[1].URL != "/open-apis/im/v1/images" ||
		calls[2].Method != "PATCH" || calls[2].URL != "/open-apis/directory/v1/employees/"+url.PathEscape("<current_user_open_id>") {
		t.Fatalf("unexpected request plan: %s", stdout.String())
	}
	if !reflect.DeepEqual(calls[2].Params, map[string]string{"employee_id_type": "open_id", "user_auth_type": "user"}) {
		t.Fatalf("PATCH params = %v", calls[2].Params)
	}
	var upload struct {
		ImageType string `json:"image_type"`
	}
	if err := json.Unmarshal(calls[1].Body, &upload); err != nil || upload.ImageType != "avatar" {
		t.Fatalf("upload preview = %s err=%v", calls[1].Body, err)
	}
	var patch struct {
		Employee map[string]string `json:"employee"`
	}
	if err := json.Unmarshal(calls[2].Body, &patch); err != nil ||
		!reflect.DeepEqual(patch.Employee, map[string]string{"avatar_key": "<image_key from avatar upload>"}) {
		t.Fatalf("PATCH preview = %s err=%v", calls[2].Body, err)
	}
}
