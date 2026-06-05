// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"bytes"
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestGetUser_BotCurrentUserValidationTyped(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, searchUserDefaultConfig())

	err := mountAndRun(t, ContactGetUser, []string{"+get-user", "--as", "bot"}, f, stdout)
	if err == nil {
		t.Fatalf("expected validation error")
	}
	var validation *errs.ValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("expected validation error, got %T: %v", err, err)
	}
	if validation.Param != "--user-id" {
		t.Fatalf("param: got %q, want --user-id", validation.Param)
	}
}

func TestGetUser_DryRunShapes(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "current user",
			args: []string{"+get-user", "--dry-run", "--as", "user"},
			want: []string{"GET", "/authen/v1/user_info", "current_user"},
		},
		{
			name: "bot specific user",
			args: []string{"+get-user", "--user-id", "ou_a", "--dry-run", "--as", "bot"},
			want: []string{"GET", "/contact/v3/users/ou_a", "ou_a", "open_id"},
		},
		{
			name: "user basic batch",
			args: []string{"+get-user", "--user-id", "ou_a", "--dry-run", "--as", "user"},
			want: []string{"POST", "/contact/v3/users/basic_batch", "ou_a", "open_id"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, stdout, _, _ := cmdutil.TestFactory(t, searchUserDefaultConfig())
			if err := mountAndRun(t, ContactGetUser, tc.args, f, stdout); err != nil {
				t.Fatalf("dry-run: %v", err)
			}
			out := stdout.String()
			for _, want := range tc.want {
				if !bytes.Contains(stdout.Bytes(), []byte(want)) {
					t.Fatalf("dry-run output missing %q: %s", want, out)
				}
			}
		})
	}
}

func TestGetUser_CurrentUserAPIFailureTyped(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, searchUserDefaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/authen/v1/user_info",
		Body:   map[string]interface{}{"code": 123456, "msg": "upstream rejected contact request"},
	})

	err := mountAndRun(t, ContactGetUser, []string{"+get-user", "--as", "user"}, f, stdout)
	if err == nil {
		t.Fatalf("expected API error")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T: %v", err, err)
	}
	if p.Code != 123456 {
		t.Fatalf("code: got %d, want 123456", p.Code)
	}
	if p.Category != errs.CategoryAPI {
		t.Fatalf("category: got %q, want %q", p.Category, errs.CategoryAPI)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should stay empty on API failure, got %q", stdout.String())
	}
}

func TestGetUser_UserBasicBatchUsesTypedAPI(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, searchUserDefaultConfig())
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/contact/v3/users/basic_batch?user_id_type=open_id",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"users": []interface{}{
					map[string]interface{}{"user_id": "ou_a", "name": "Alice"},
				},
			},
		},
	}
	reg.Register(stub)

	err := mountAndRun(t, ContactGetUser, []string{"+get-user", "--user-id", "ou_a", "--as", "user", "--format", "json"}, f, stdout)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !bytes.Contains(stub.CapturedBody, []byte(`"ou_a"`)) {
		t.Fatalf("request body should include user id, got %s", string(stub.CapturedBody))
	}
	if !bytes.Contains(stdout.Bytes(), []byte(`"user"`)) {
		t.Fatalf("stdout should include user object, got %s", stdout.String())
	}
}
