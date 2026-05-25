// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
)

// ── registration / declared contract ────────────────────────────────────────

func TestWikiShortcutsIncludesMembers(t *testing.T) {
	t.Parallel()

	commands := map[string]bool{}
	for _, s := range Shortcuts() {
		commands[s.Command] = true
	}
	for _, want := range []string{"+member-add", "+member-remove", "+member-list"} {
		if !commands[want] {
			t.Errorf("Shortcuts() missing %q", want)
		}
	}
}

// TestWikiMemberShortcutsDeclareNarrowScopes pins the per-endpoint scope so a
// future broadening (e.g. wiki:wiki) doesn't silently reject tokens that
// carry only the narrow scope the API accepts.
func TestWikiMemberShortcutsDeclareNarrowScopes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		shortcut common.Shortcut
		want     []string
	}{
		{"+member-add", WikiMemberAdd, []string{"wiki:member:create"}},
		{"+member-remove", WikiMemberRemove, []string{"wiki:member:update"}},
		{"+member-list", WikiMemberList, []string{"wiki:member:retrieve"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !reflect.DeepEqual(tc.shortcut.Scopes, tc.want) {
				t.Fatalf("%s scopes = %v, want %v", tc.name, tc.shortcut.Scopes, tc.want)
			}
		})
	}
}

func TestWikiMemberShortcutsDeclareRiskAndAuth(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		shortcut common.Shortcut
		risk     string
	}{
		{"+member-add", WikiMemberAdd, "write"},
		{"+member-remove", WikiMemberRemove, "write"},
		{"+member-list", WikiMemberList, "read"},
	}
	wantAuth := []string{"user", "bot"}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.shortcut.Risk != tc.risk {
				t.Errorf("Risk = %q, want %q", tc.shortcut.Risk, tc.risk)
			}
			if !reflect.DeepEqual(tc.shortcut.AuthTypes, wantAuth) {
				t.Errorf("AuthTypes = %v, want %v", tc.shortcut.AuthTypes, wantAuth)
			}
		})
	}
}

// ── +member-add ──────────────────────────────────────────────────────────────

func TestWikiMemberAddRequestBodyOmitsQueryWhenNotificationFlagUnset(t *testing.T) {
	t.Parallel()

	spec := wikiMemberAddSpec{
		SpaceID:    "space_1",
		MemberID:   "ou_x",
		MemberType: "openid",
		MemberRole: "member",
	}
	if got := spec.QueryParams(); got != nil {
		t.Fatalf("QueryParams() = %v, want nil when --need-notification was not set", got)
	}
	body := spec.RequestBody()
	want := map[string]interface{}{"member_id": "ou_x", "member_type": "openid", "member_role": "member"}
	if !reflect.DeepEqual(body, want) {
		t.Fatalf("RequestBody() = %v, want %v", body, want)
	}
}

func TestWikiMemberAddQueryParamsHonorsExplicitNotification(t *testing.T) {
	t.Parallel()

	spec := wikiMemberAddSpec{
		NotificationSet:  true,
		NeedNotification: true,
	}
	if got := spec.QueryParams(); !reflect.DeepEqual(got, map[string]interface{}{"need_notification": true}) {
		t.Fatalf("QueryParams() = %v, want need_notification=true", got)
	}
}

func TestWikiMemberAddQueryParamsHonorsExplicitFalse(t *testing.T) {
	t.Parallel()

	// The three-state design (unset / true / false) must distinguish false from
	// unset so --need-notification=false reaches the server instead of being
	// dropped along with the param block.
	spec := wikiMemberAddSpec{
		NotificationSet:  true,
		NeedNotification: false,
	}
	if got := spec.QueryParams(); !reflect.DeepEqual(got, map[string]interface{}{"need_notification": false}) {
		t.Fatalf("QueryParams() = %v, want need_notification=false", got)
	}
}

func TestWikiMemberAddDryRunSingleStep(t *testing.T) {
	t.Parallel()

	dry := buildWikiMemberAddDryRun(wikiMemberAddSpec{
		SpaceID:    "space_42",
		MemberID:   "ou_x",
		MemberType: "openid",
		MemberRole: "admin",
	})
	api := dryRunAPIList(t, dry)
	if len(api) != 1 || api[0].Method != "POST" || api[0].URL != "/open-apis/wiki/v2/spaces/space_42/members" {
		t.Fatalf("dry-run api = %#v", api)
	}
	if api[0].Body["member_id"] != "ou_x" || api[0].Body["member_role"] != "admin" {
		t.Fatalf("dry-run body = %#v", api[0].Body)
	}
}

func TestWikiMemberAddDryRunMyLibraryIsTwoStep(t *testing.T) {
	t.Parallel()

	dry := buildWikiMemberAddDryRun(wikiMemberAddSpec{
		SpaceID:    wikiMyLibrarySpaceID,
		MemberID:   "ou_x",
		MemberType: "openid",
		MemberRole: "member",
	})
	api := dryRunAPIList(t, dry)
	if len(api) != 2 {
		t.Fatalf("dry-run api count = %d, want 2", len(api))
	}
	if api[0].Method != "GET" || !strings.Contains(api[0].URL, "/spaces/my_library") {
		t.Fatalf("dry-run step 1 = %#v", api[0])
	}
	if api[1].Method != "POST" || !strings.Contains(api[1].URL, "<resolved_space_id>/members") {
		t.Fatalf("dry-run step 2 = %#v", api[1])
	}
}

func TestWikiMemberAddRejectsMyLibraryForBot(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, _, _, _ := cmdutil.TestFactory(t, wikiTestConfig())
	err := mountAndRunWiki(t, WikiMemberAdd, []string{
		"+member-add",
		"--space-id", "my_library",
		"--member-id", "ou_x",
		"--member-type", "openid",
		"--member-role", "member",
		"--as", "bot",
	}, factory, nil)
	if err == nil || !strings.Contains(err.Error(), "bot identity does not support --space-id my_library") {
		t.Fatalf("expected my_library bot rejection, got %v", err)
	}
}

func TestWikiMemberAddRejectsBotWithDepartment(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, _, _, _ := cmdutil.TestFactory(t, wikiTestConfig())
	err := mountAndRunWiki(t, WikiMemberAdd, []string{
		"+member-add",
		"--space-id", "space_42",
		"--member-id", "od_dept_1",
		"--member-type", "opendepartmentid",
		"--member-role", "member",
		"--as", "bot",
	}, factory, nil)
	if err == nil || !strings.Contains(err.Error(), "--as bot does not support --member-type opendepartmentid") {
		t.Fatalf("expected bot+opendepartmentid rejection, got %v", err)
	}
}

func TestWikiMemberAddMountedExecuteFlattensMember(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, stderr, reg := cmdutil.TestFactory(t, wikiTestConfig())

	var capturedQuery string
	stub := &httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/wiki/v2/spaces/space_42/members",
		OnMatch: func(req *http.Request) { capturedQuery = req.URL.RawQuery },
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"member": map[string]interface{}{
					"member_id":   "ou_abc",
					"member_type": "openid",
					"member_role": "admin",
					"type":        "user",
				},
			},
			"msg": "success",
		},
	}
	reg.Register(stub)

	err := mountAndRunWiki(t, WikiMemberAdd, []string{
		"+member-add",
		"--space-id", "space_42",
		"--member-id", "ou_abc",
		"--member-type", "openid",
		"--member-role", "admin",
		"--need-notification",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	data := decodeWikiEnvelope(t, stdout)
	if data["space_id"] != "space_42" {
		t.Fatalf("space_id = %#v", data["space_id"])
	}
	if data["member_id"] != "ou_abc" || data["member_role"] != "admin" || data["type"] != "user" {
		t.Fatalf("flattened envelope = %#v", data)
	}

	// Captured body must carry the three required fields; query must include the
	// notification flag because the caller passed --need-notification.
	var captured map[string]interface{}
	if err := json.Unmarshal(stub.CapturedBody, &captured); err != nil {
		t.Fatalf("unmarshal captured body: %v", err)
	}
	if captured["member_id"] != "ou_abc" || captured["member_type"] != "openid" || captured["member_role"] != "admin" {
		t.Fatalf("captured request body = %#v", captured)
	}
	if !strings.Contains(capturedQuery, "need_notification=true") {
		t.Fatalf("captured query = %q, want need_notification=true", capturedQuery)
	}
	if !strings.Contains(stderr.String(), "Added wiki space member") {
		t.Fatalf("stderr = %q, want success log", stderr.String())
	}
}

func TestWikiMemberAddFallsBackToSpecWhenMemberEchoIsEmpty(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	// Server returns an empty member object: scripts must still see the three
	// identifying fields, restored from the caller's spec.
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/wiki/v2/spaces/space_42/members",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"member": map[string]interface{}{},
			},
			"msg": "success",
		},
	})

	err := mountAndRunWiki(t, WikiMemberAdd, []string{
		"+member-add",
		"--space-id", "space_42",
		"--member-id", "ou_abc",
		"--member-type", "openid",
		"--member-role", "admin",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}
	data := decodeWikiEnvelope(t, stdout)
	if data["space_id"] != "space_42" ||
		data["member_id"] != "ou_abc" ||
		data["member_type"] != "openid" ||
		data["member_role"] != "admin" {
		t.Fatalf("fallback envelope = %#v", data)
	}
}

func TestWikiMemberAddResolvesMyLibraryForUser(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/my_library",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"space": map[string]interface{}{"space_id": "space_personal_7", "name": "My Library", "space_type": "my_library"},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/wiki/v2/spaces/space_personal_7/members",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"member": map[string]interface{}{
					"member_id":   "ou_x",
					"member_type": "openid",
					"member_role": "member",
				},
			},
		},
	})

	err := mountAndRunWiki(t, WikiMemberAdd, []string{
		"+member-add",
		"--space-id", "my_library",
		"--member-id", "ou_x",
		"--member-type", "openid",
		"--member-role", "member",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}
	data := decodeWikiEnvelope(t, stdout)
	if data["space_id"] != "space_personal_7" {
		t.Fatalf("space_id = %#v, want space_personal_7", data["space_id"])
	}
}

// ── +member-remove ───────────────────────────────────────────────────────────

func TestWikiMemberRemoveSpecRequiresMemberID(t *testing.T) {
	t.Parallel()

	cmd := newMemberRemoveCmd("space_1", "", "openid", "member")
	runtime := common.TestNewRuntimeContext(cmd, nil)
	if _, err := readWikiMemberRemoveSpec(runtime); err == nil || !strings.Contains(err.Error(), "--member-id is required") {
		t.Fatalf("expected --member-id rejection, got %v", err)
	}
}

func TestWikiMemberRemoveDryRunIncludesBody(t *testing.T) {
	t.Parallel()

	dry := buildWikiMemberRemoveDryRun(wikiMemberRemoveSpec{
		SpaceID:    "space_42",
		MemberID:   "ou_x",
		MemberType: "openid",
		MemberRole: "admin",
	})
	api := dryRunAPIList(t, dry)
	if len(api) != 1 || api[0].Method != "DELETE" {
		t.Fatalf("dry-run api = %#v", api)
	}
	if api[0].URL != "/open-apis/wiki/v2/spaces/space_42/members/ou_x" {
		t.Fatalf("dry-run url = %q", api[0].URL)
	}
	if api[0].Body["member_type"] != "openid" || api[0].Body["member_role"] != "admin" {
		t.Fatalf("dry-run body = %#v", api[0].Body)
	}
}

func TestWikiMemberRemoveDryRunMyLibraryIsTwoStep(t *testing.T) {
	t.Parallel()

	dry := buildWikiMemberRemoveDryRun(wikiMemberRemoveSpec{
		SpaceID:    wikiMyLibrarySpaceID,
		MemberID:   "ou_x",
		MemberType: "openid",
		MemberRole: "member",
	})
	api := dryRunAPIList(t, dry)
	if len(api) != 2 {
		t.Fatalf("dry-run api count = %d, want 2", len(api))
	}
	if api[1].Method != "DELETE" || !strings.Contains(api[1].URL, "<resolved_space_id>/members/ou_x") {
		t.Fatalf("dry-run step 2 = %#v", api[1])
	}
}

func TestWikiMemberRemoveMountedExecuteFlattensMember(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/wiki/v2/spaces/space_42/members/ou_abc",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"member": map[string]interface{}{
					"member_id":   "ou_abc",
					"member_type": "openid",
					"member_role": "admin",
				},
			},
			"msg": "success",
		},
	})

	err := mountAndRunWiki(t, WikiMemberRemove, []string{
		"+member-remove",
		"--space-id", "space_42",
		"--member-id", "ou_abc",
		"--member-type", "openid",
		"--member-role", "admin",
		"--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}
	data := decodeWikiEnvelope(t, stdout)
	if data["space_id"] != "space_42" || data["member_id"] != "ou_abc" || data["member_role"] != "admin" {
		t.Fatalf("envelope = %#v", data)
	}
}

// ── +member-list ─────────────────────────────────────────────────────────────

func TestWikiMemberListRequiresSpaceID(t *testing.T) {
	t.Parallel()

	factory, _, _, _ := cmdutil.TestFactory(t, wikiTestConfig())
	err := mountAndRunWiki(t, WikiMemberList, []string{"+member-list", "--as", "user"}, factory, nil)
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("expected required flag error, got %v", err)
	}
}

func TestWikiMemberListRejectsMyLibraryForBot(t *testing.T) {
	t.Parallel()

	factory, _, _, _ := cmdutil.TestFactory(t, wikiTestConfig())
	err := mountAndRunWiki(t, WikiMemberList, []string{
		"+member-list", "--space-id", "my_library", "--as", "bot",
	}, factory, nil)
	if err == nil || !strings.Contains(err.Error(), "bot identity does not support --space-id my_library") {
		t.Fatalf("expected my_library bot rejection, got %v", err)
	}
}

func TestWikiMemberListReturnsMembers(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/space_42/members",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"has_more": false,
				"members": []interface{}{
					map[string]interface{}{
						"member_id":   "ou_1",
						"member_type": "openid",
						"member_role": "admin",
					},
					map[string]interface{}{
						"member_id":   "ou_2",
						"member_type": "openid",
						"member_role": "member",
					},
				},
			},
		},
	})

	err := mountAndRunWiki(t, WikiMemberList, []string{
		"+member-list", "--space-id", "space_42", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	var envelope struct {
		OK   bool `json:"ok"`
		Data struct {
			SpaceID   string                   `json:"space_id"`
			Members   []map[string]interface{} `json:"members"`
			HasMore   bool                     `json:"has_more"`
			PageToken string                   `json:"page_token"`
		} `json:"data"`
		Meta struct {
			Count float64 `json:"count"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if !envelope.OK {
		t.Fatalf("expected ok=true, got %s", stdout.String())
	}
	if envelope.Meta.Count != 2 {
		t.Fatalf("meta.count = %v, want 2", envelope.Meta.Count)
	}
	if envelope.Data.SpaceID != "space_42" {
		t.Fatalf("data.space_id = %q, want space_42", envelope.Data.SpaceID)
	}
	if envelope.Data.Members[0]["member_role"] != "admin" {
		t.Fatalf("members[0].member_role = %v", envelope.Data.Members[0]["member_role"])
	}
}

func TestWikiMemberListResolvesMyLibraryForUser(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/my_library",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"space": map[string]interface{}{"space_id": "space_personal_7", "name": "My Library", "space_type": "my_library"},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/space_personal_7/members",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"has_more": false,
				"members":  []interface{}{},
			},
		},
	})

	err := mountAndRunWiki(t, WikiMemberList, []string{
		"+member-list", "--space-id", "my_library", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}
	var envelope struct {
		Data struct {
			SpaceID string `json:"space_id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if envelope.Data.SpaceID != "space_personal_7" {
		t.Fatalf("data.space_id = %q, want space_personal_7", envelope.Data.SpaceID)
	}
}

func TestWikiMemberListAutoPaginatesAcrossPages(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	// Page 1: has_more=true, page_token set. Loop must continue.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/space_42/members",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"has_more":   true,
				"page_token": "tok_page2",
				"members": []interface{}{
					map[string]interface{}{"member_id": "ou_1", "member_type": "openid", "member_role": "admin"},
				},
			},
		},
	})
	// Page 2: must carry page_token=tok_page2 in the query. Captured to verify.
	var page2Query string
	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/wiki/v2/spaces/space_42/members",
		OnMatch: func(req *http.Request) { page2Query = req.URL.RawQuery },
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"has_more":   false,
				"page_token": "",
				"members": []interface{}{
					map[string]interface{}{"member_id": "ou_2", "member_type": "openid", "member_role": "member"},
				},
			},
		},
	})

	err := mountAndRunWiki(t, WikiMemberList, []string{
		"+member-list", "--space-id", "space_42", "--page-all", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	var envelope struct {
		Data struct {
			Members   []map[string]interface{} `json:"members"`
			HasMore   bool                     `json:"has_more"`
			PageToken string                   `json:"page_token"`
		} `json:"data"`
		Meta struct {
			Count float64 `json:"count"`
		} `json:"meta"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if envelope.Meta.Count != 2 || len(envelope.Data.Members) != 2 {
		t.Fatalf("merged members = %d / count=%v, want 2 / 2", len(envelope.Data.Members), envelope.Meta.Count)
	}
	if envelope.Data.HasMore || envelope.Data.PageToken != "" {
		t.Fatalf("natural end should clear has_more/page_token, got has_more=%v page_token=%q",
			envelope.Data.HasMore, envelope.Data.PageToken)
	}
	q, _ := url.ParseQuery(page2Query)
	if q.Get("page_token") != "tok_page2" {
		t.Fatalf("page2 page_token = %q, want tok_page2", q.Get("page_token"))
	}
}

func TestWikiMemberListPageLimitTruncatesAndExposesNextCursor(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	// Only stub page 1; with --page-limit=1 the loop must stop BEFORE page 2 —
	// and the response must surface has_more/page_token so the caller can resume.
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/space_42/members",
		Body: map[string]interface{}{
			"code": 0, "msg": "success",
			"data": map[string]interface{}{
				"has_more":   true,
				"page_token": "tok_next",
				"members": []interface{}{
					map[string]interface{}{"member_id": "ou_only", "member_type": "openid", "member_role": "admin"},
				},
			},
		},
	})

	err := mountAndRunWiki(t, WikiMemberList, []string{
		"+member-list", "--space-id", "space_42", "--page-all", "--page-limit", "1", "--as", "user",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	var envelope struct {
		Data struct {
			Members   []map[string]interface{} `json:"members"`
			HasMore   bool                     `json:"has_more"`
			PageToken string                   `json:"page_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal stdout: %v", err)
	}
	if len(envelope.Data.Members) != 1 {
		t.Fatalf("members = %d, want 1 (capped)", len(envelope.Data.Members))
	}
	if !envelope.Data.HasMore || envelope.Data.PageToken != "tok_next" {
		t.Fatalf("truncated state = has_more=%v page_token=%q, want true / tok_next",
			envelope.Data.HasMore, envelope.Data.PageToken)
	}
}

// ── helpers ──────────────────────────────────────────────────────────────────

func newMemberRemoveCmd(spaceID, memberID, memberType, memberRole string) *cobra.Command {
	cmd := &cobra.Command{Use: "wiki +member-remove"}
	cmd.Flags().String("space-id", spaceID, "")
	cmd.Flags().String("member-id", memberID, "")
	cmd.Flags().String("member-type", memberType, "")
	cmd.Flags().String("member-role", memberRole, "")
	return cmd
}

// dryRunAPIList serializes a DryRunAPI through JSON to match how the framework
// exposes it to callers — same approach used by +space-create's tests.
func dryRunAPIList(t *testing.T, dry *common.DryRunAPI) []struct {
	Method string                 `json:"method"`
	URL    string                 `json:"url"`
	Body   map[string]interface{} `json:"body"`
	Params map[string]interface{} `json:"params"`
} {
	t.Helper()
	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}
	var got struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Body   map[string]interface{} `json:"body"`
			Params map[string]interface{} `json:"params"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run: %v", err)
	}
	return got.API
}
