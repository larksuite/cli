// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

const sessionMessagesListURL = "/open-apis/spark/v1/apps/app_x/sessions/sess_1/turns/turn_9/reply_message"

func TestAppsSessionMessagesList_Success(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    sessionMessagesListURL,
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"messages": []interface{}{
					map[string]interface{}{"message_id": "m1", "role": "assistant", "content": "hi"},
				},
				"next_page_token": "tok_next",
				"has_more":        true,
			},
		},
	})
	if err := runAppsShortcut(t, AppsSessionMessagesList,
		[]string{"+session-messages-list", "--app-id", "app_x", "--session-id", "sess_1", "--turn-id", "turn_9", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"message_id": "m1"`) || !strings.Contains(got, `"next_page_token": "tok_next"`) {
		t.Fatalf("stdout missing messages/next_page_token: %s", got)
	}
}

func TestAppsSessionMessagesList_PageTokenOnlyWhenSet(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsSessionMessagesList,
		[]string{"+session-messages-list", "--app-id", "app_x", "--session-id", "sess_1", "--turn-id", "turn_9", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	if strings.Contains(stdout.String(), "page_token") {
		t.Fatalf("page_token must be absent when --page-token not set: %s", stdout.String())
	}

	factory2, stdout2, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsSessionMessagesList,
		[]string{"+session-messages-list", "--app-id", "app_x", "--session-id", "sess_1", "--turn-id", "turn_9", "--page-token", "tok_5", "--dry-run", "--as", "user"},
		factory2, stdout2); err != nil {
		t.Fatalf("dry-run err=%v", err)
	}
	got := stdout2.String()
	if !strings.Contains(got, "page_token") || !strings.Contains(got, "tok_5") {
		t.Fatalf("dry-run missing page_token=tok_5: %s", got)
	}
}

func TestAppsSessionMessagesList_RequiresIDs(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		wantParam string
	}{
		{"no app-id", []string{"+session-messages-list", "--app-id", "", "--session-id", "s", "--turn-id", "t", "--as", "user"}, "--app-id"},
		{"no session-id", []string{"+session-messages-list", "--app-id", "a", "--session-id", "", "--turn-id", "t", "--as", "user"}, "--session-id"},
		{"no turn-id", []string{"+session-messages-list", "--app-id", "a", "--session-id", "s", "--turn-id", "", "--as", "user"}, "--turn-id"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			factory, stdout, _ := newAppsExecuteFactory(t)
			err := runAppsShortcut(t, AppsSessionMessagesList, c.args, factory, stdout)
			if err == nil {
				t.Fatalf("expected validation error for %s, got nil", c.wantParam)
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected a typed Problem error, got %T: %v", err, err)
			}
			if p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeInvalidArgument {
				t.Fatalf("error type=%v subtype=%v, want %v/%v (err=%v)",
					p.Category, p.Subtype, errs.CategoryValidation, errs.SubtypeInvalidArgument, err)
			}
			var ve *errs.ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected *errs.ValidationError, got %T: %v", err, err)
			}
			if ve.Param != c.wantParam {
				t.Fatalf("Param = %q, want %q (err=%v)", ve.Param, c.wantParam, err)
			}
		})
	}
}

func TestAppsSessionMessagesList_APIErrorSurfacesHint(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    sessionMessagesListURL,
		Body:   map[string]interface{}{"code": 1254043, "msg": "permission denied"},
	})
	err := runAppsShortcut(t, AppsSessionMessagesList,
		[]string{"+session-messages-list", "--app-id", "app_x", "--session-id", "sess_1", "--turn-id", "turn_9", "--as", "user"},
		factory, stdout)
	if err == nil {
		t.Fatalf("expected API error, got nil")
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed Problem error, got %T: %v", err, err)
	}
	if !strings.Contains(p.Hint, "+session-get") {
		t.Fatalf("error should carry domain hint pointing at +session-get, got hint=%q (err=%v)", p.Hint, err)
	}
}
