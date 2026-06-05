// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestWikiSpaceCreateDeclaredContract(t *testing.T) {
	t.Parallel()

	if WikiSpaceCreate.Command != "+space-create" {
		t.Fatalf("Command = %q, want +space-create", WikiSpaceCreate.Command)
	}
	if WikiSpaceCreate.Risk != "write" {
		t.Fatalf("Risk = %q, want write", WikiSpaceCreate.Risk)
	}
	if !reflect.DeepEqual(WikiSpaceCreate.AuthTypes, []string{"user"}) {
		t.Fatalf("AuthTypes = %v, want [user]", WikiSpaceCreate.AuthTypes)
	}
	if !reflect.DeepEqual(WikiSpaceCreate.Scopes, []string{"wiki:space:write_only"}) {
		t.Fatalf("Scopes = %v, want [wiki:space:write_only]", WikiSpaceCreate.Scopes)
	}
}

func TestReadWikiSpaceCreateSpecRejectsBlankName(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "wiki +space-create"}
	cmd.Flags().String("name", "   ", "")
	cmd.Flags().String("description", "", "")

	runtime := common.TestNewRuntimeContext(cmd, nil)
	if _, err := readWikiSpaceCreateSpec(runtime); err == nil || !strings.Contains(err.Error(), "--name is required") {
		t.Fatalf("expected blank-name rejection, got %v", err)
	}
}

func TestWikiSpaceCreateRequestBody(t *testing.T) {
	t.Parallel()

	nameOnly := wikiSpaceCreateSpec{Name: "Eng Wiki"}.RequestBody()
	if !reflect.DeepEqual(nameOnly, map[string]interface{}{"name": "Eng Wiki"}) {
		t.Fatalf("name-only body = %#v", nameOnly)
	}

	full := wikiSpaceCreateSpec{Name: "Eng Wiki", Description: "team docs"}.RequestBody()
	if full["name"] != "Eng Wiki" || full["description"] != "team docs" {
		t.Fatalf("full body = %#v", full)
	}
}

func TestWikiSpaceCreateDryRun(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "wiki +space-create"}
	cmd.Flags().String("name", "Eng Wiki", "")
	cmd.Flags().String("description", "team docs", "")
	runtime := common.TestNewRuntimeContext(cmd, nil)

	dry := WikiSpaceCreate.DryRun(nil, runtime)
	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}
	var got struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Body   map[string]interface{} `json:"body"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run: %v", err)
	}
	if len(got.API) != 1 || got.API[0].Method != "POST" || got.API[0].URL != "/open-apis/wiki/v2/spaces" {
		t.Fatalf("dry-run api = %#v", got.API)
	}
	if got.API[0].Body["name"] != "Eng Wiki" || got.API[0].Body["description"] != "team docs" {
		t.Fatalf("dry-run body = %#v", got.API[0].Body)
	}
}

func TestWikiSpaceCreateDryRunBlankNameSurfacesError(t *testing.T) {
	t.Parallel()

	cmd := &cobra.Command{Use: "wiki +space-create"}
	cmd.Flags().String("name", "", "")
	cmd.Flags().String("description", "", "")
	runtime := common.TestNewRuntimeContext(cmd, nil)

	dry := WikiSpaceCreate.DryRun(nil, runtime)
	data, _ := json.Marshal(dry)
	if !strings.Contains(string(data), "--name is required") {
		t.Fatalf("dry-run should surface validation error, got %s", data)
	}
}

func TestWikiSpaceCreateMountedExecuteFlattensSpace(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, stderr, reg := cmdutil.TestFactory(t, wikiTestConfig())

	createStub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/wiki/v2/spaces",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"space": map[string]interface{}{
					"space_id":     "7160145948494381236",
					"name":         "Eng Wiki",
					"description":  "team docs",
					"space_type":   "team",
					"visibility":   "private",
					"open_sharing": "closed",
				},
			},
			"msg": "success",
		},
	}
	reg.Register(createStub)

	err := mountAndRunWiki(t, WikiSpaceCreate, []string{
		"+space-create",
		"--name", "Eng Wiki",
		"--description", "team docs",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	data := decodeWikiEnvelope(t, stdout)
	if data["space_id"] != "7160145948494381236" {
		t.Fatalf("space_id = %#v", data["space_id"])
	}
	if data["name"] != "Eng Wiki" || data["description"] != "team docs" {
		t.Fatalf("name/description = %#v / %#v", data["name"], data["description"])
	}
	if data["space_type"] != "team" || data["visibility"] != "private" || data["open_sharing"] != "closed" {
		t.Fatalf("space_type/visibility/open_sharing = %#v", data)
	}
	if _, ok := data["url"]; ok {
		t.Fatalf("output must not include a url field, got %#v", data["url"])
	}

	var captured map[string]interface{}
	if err := json.Unmarshal(createStub.CapturedBody, &captured); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	if captured["name"] != "Eng Wiki" || captured["description"] != "team docs" {
		t.Fatalf("captured request body = %#v", captured)
	}
	if !strings.Contains(stderr.String(), "Created wiki space") {
		t.Fatalf("stderr = %q, want creation log", stderr.String())
	}
}

func TestWikiSpaceCreateRejectsBotIdentity(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, _ := cmdutil.TestFactory(t, wikiTestConfig())

	err := mountAndRunWiki(t, WikiSpaceCreate, []string{
		"+space-create",
		"--name", "Eng Wiki",
		"--as", "bot",
	}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "only supports: user") {
		t.Fatalf("expected bot identity rejection, got %v", err)
	}
}

func TestWikiSpaceCreateErrorsWhenNoSpaceReturned(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/wiki/v2/spaces",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
			"msg":  "success",
		},
	})

	err := mountAndRunWiki(t, WikiSpaceCreate, []string{
		"+space-create",
		"--name", "Eng Wiki",
		"--as", "user",
	}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "returned no space") {
		t.Fatalf("expected missing-space error, got %v", err)
	}
}
