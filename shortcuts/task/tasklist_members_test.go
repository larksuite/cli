// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package task

import (
	"errors"
	"testing"

	"github.com/smartystreets/goconvey/convey"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
)

func TestBuildTlMembersBody(t *testing.T) {
	convey.Convey("Build with ids", t, func() {
		body := buildTlMembersBody("u1, u2 , ")
		members := body["members"].([]map[string]interface{})
		convey.So(len(members), convey.ShouldEqual, 2)
	})
}

// TestMembersTasklist_SetCombinedWithAddRejected covers the Validate guard:
// --set is mutually exclusive with --add/--remove. It must surface a typed
// *errs.ValidationError (exit 2) before any API call is made.
func TestMembersTasklist_SetCombinedWithAddRejected(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	s := MembersTasklist
	s.AuthTypes = []string{"bot", "user"}

	args := []string{"+tasklist-members", "--tasklist-id", "tl-123", "--set", "ou_a", "--add", "ou_b", "--as", "bot", "--format", "json"}
	err := runMountedTaskShortcut(t, s, args, f, stdout)

	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T, want *errs.ValidationError; err = %v", err, err)
	}
	if ve.Subtype != errs.SubtypeInvalidArgument {
		t.Errorf("subtype = %q, want %q", ve.Subtype, errs.SubtypeInvalidArgument)
	}
	if got := output.ExitCodeOf(err); got != output.ExitValidation {
		t.Errorf("exit code = %d, want %d (ExitValidation)", got, output.ExitValidation)
	}
}

// TestMembersTasklist_ListMalformedResponse covers the list arm (no
// set/add/remove): a 200 with a non-JSON body must surface a typed
// *errs.InternalError(invalid_response) (exit 5) from the json.Unmarshal guard,
// not a silent success.
func TestMembersTasklist_ListMalformedResponse(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/task/v2/tasklists/tl-123",
		RawBody: []byte("not json"),
	})

	s := MembersTasklist
	s.AuthTypes = []string{"bot", "user"}

	args := []string{"+tasklist-members", "--tasklist-id", "tl-123", "--as", "bot", "--format", "json"}
	err := runMountedTaskShortcut(t, s, args, f, stdout)

	assertInvalidResponse(t, err)
}

// TestMembersTasklist_SetMalformedResponse covers the --set arm: the diff path
// first GETs the tasklist; a non-JSON body there must surface the typed
// internal invalid_response error.
func TestMembersTasklist_SetMalformedResponse(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	reg.Register(&httpmock.Stub{
		Method:  "GET",
		URL:     "/open-apis/task/v2/tasklists/tl-123",
		RawBody: []byte("not json"),
	})

	s := MembersTasklist
	s.AuthTypes = []string{"bot", "user"}

	args := []string{"+tasklist-members", "--tasklist-id", "tl-123", "--set", "ou_a", "--as", "bot", "--format", "json"}
	err := runMountedTaskShortcut(t, s, args, f, stdout)

	assertInvalidResponse(t, err)
}

// TestMembersTasklist_AddMalformedResponse covers the add/remove arm: the POST
// to add_members returns a non-JSON body, which must surface the typed internal
// invalid_response error.
func TestMembersTasklist_AddMalformedResponse(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	reg.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/task/v2/tasklists/tl-123/add_members",
		RawBody: []byte("not json"),
	})

	s := MembersTasklist
	s.AuthTypes = []string{"bot", "user"}

	args := []string{"+tasklist-members", "--tasklist-id", "tl-123", "--add", "ou_a", "--as", "bot", "--format", "json"}
	err := runMountedTaskShortcut(t, s, args, f, stdout)

	assertInvalidResponse(t, err)
}

// TestMembersTasklist_SetRemoveDiffMalformedResponse covers the --set diff's
// remove_members arm: the GET returns an existing member absent from the target
// set, so the shortcut issues a remove_members POST whose 200 carries a
// non-JSON body, which must surface the typed internal invalid_response error.
// The target equals one existing member, so no add_members call precedes it.
func TestMembersTasklist_SetRemoveDiffMalformedResponse(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/task/v2/tasklists/tl-123",
		Status: 200,
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "success",
			"data": map[string]interface{}{
				"tasklist": map[string]interface{}{
					"url": "https://example.com/tl-123",
					"members": []interface{}{
						map[string]interface{}{"id": "ou_keep"},
						map[string]interface{}{"id": "ou_drop"},
					},
				},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/task/v2/tasklists/tl-123/remove_members",
		Status:  200,
		RawBody: []byte("not json"),
	})

	s := MembersTasklist
	s.AuthTypes = []string{"bot", "user"}

	args := []string{"+tasklist-members", "--tasklist-id", "tl-123", "--set", "ou_keep", "--as", "bot", "--format", "json"}
	err := runMountedTaskShortcut(t, s, args, f, stdout)

	assertInvalidResponse(t, err)
}

// TestMembersTasklist_RemoveMalformedResponse covers the add/remove mode's
// remove_members arm: with only --remove set, the add arm is skipped and the
// remove_members POST returns a non-JSON body, which must surface the typed
// internal invalid_response error.
func TestMembersTasklist_RemoveMalformedResponse(t *testing.T) {
	f, stdout, _, reg := taskShortcutTestFactory(t)
	warmTenantToken(t, f, reg)

	reg.Register(&httpmock.Stub{
		Method:  "POST",
		URL:     "/open-apis/task/v2/tasklists/tl-123/remove_members",
		Status:  200,
		RawBody: []byte("not json"),
	})

	s := MembersTasklist
	s.AuthTypes = []string{"bot", "user"}

	args := []string{"+tasklist-members", "--tasklist-id", "tl-123", "--remove", "ou_a", "--as", "bot", "--format", "json"}
	err := runMountedTaskShortcut(t, s, args, f, stdout)

	assertInvalidResponse(t, err)
}

// assertInvalidResponse asserts a typed *errs.InternalError(invalid_response)
// with exit 5 — the contract for a parse-response failure across the members
// arms.
func assertInvalidResponse(t *testing.T, err error) {
	t.Helper()
	var ie *errs.InternalError
	if !errors.As(err, &ie) {
		t.Fatalf("err = %T, want *errs.InternalError; err = %v", err, err)
	}
	if ie.Subtype != errs.SubtypeInvalidResponse {
		t.Errorf("subtype = %q, want %q", ie.Subtype, errs.SubtypeInvalidResponse)
	}
	if got := output.ExitCodeOf(err); got != output.ExitInternal {
		t.Errorf("exit code = %d, want %d (ExitInternal)", got, output.ExitInternal)
	}
}
