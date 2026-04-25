// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package appmeta

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/event/testutil"
)

const respFourVersions = `{
  "code": 0,
  "data": {
    "has_more": false,
    "items": [
      {"version_id": "oav_draft", "version": "1.0.3", "status": 4, "publish_time": null,
       "event_infos": [{"event_type": "im.message.receive_v1"}, {"event_type": "mail.user_mailbox.event.message_received_v1"}],
       "scopes": [{"scope": "draft:only", "token_types": ["tenant"]}]
      },
      {"version_id": "oav_latest", "version": "1.0.2", "status": 1, "publish_time": "1776684746",
       "event_infos": [
         {"event_type": "im.message.receive_v1"},
         {"event_type": "im.message.message_read_v1"}
       ],
       "scopes": [
         {"scope": "im:message", "token_types": ["tenant", "user"]},
         {"scope": "im:message.group_at_msg", "token_types": ["tenant"]},
         {"scope": "contact:user:readonly", "token_types": ["user"]}
       ]
      }
    ]
  }
}`

func TestFetchCurrentPublished_SelectsLatestPublished(t *testing.T) {
	c := &testutil.StubAPIClient{Body: respFourVersions}

	v, err := FetchCurrentPublished(context.Background(), c, "cli_test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v == nil {
		t.Fatal("expected a version, got nil")
	}
	if v.VersionID != "oav_latest" {
		t.Errorf("VersionID = %q, want oav_latest", v.VersionID)
	}
	if v.Version != "1.0.2" {
		t.Errorf("Version = %q, want 1.0.2", v.Version)
	}

	wantEvents := map[string]bool{"im.message.receive_v1": true, "im.message.message_read_v1": true}
	if len(v.EventTypes) != len(wantEvents) {
		t.Fatalf("EventTypes = %v, want %v", v.EventTypes, wantEvents)
	}
	for _, e := range v.EventTypes {
		if !wantEvents[e] {
			t.Errorf("unexpected event type %q in %v", e, v.EventTypes)
		}
	}

	wantTenant := map[string]bool{"im:message": true, "im:message.group_at_msg": true}
	if len(v.TenantScopes) != len(wantTenant) {
		t.Fatalf("TenantScopes = %v, want %v", v.TenantScopes, wantTenant)
	}
	for _, s := range v.TenantScopes {
		if !wantTenant[s] {
			t.Errorf("unexpected tenant scope %q in %v", s, v.TenantScopes)
		}
	}
}

func TestFetchCurrentPublished_PathContainsQuery(t *testing.T) {
	c := &testutil.StubAPIClient{Body: respFourVersions}
	_, _ = FetchCurrentPublished(context.Background(), c, "cli_x")
	for _, want := range []string{
		"/open-apis/application/v6/applications/cli_x/app_versions",
		"lang=zh_cn",
		"page_size=2",
	} {
		if !strings.Contains(c.GotPath, want) {
			t.Errorf("path %q missing %q", c.GotPath, want)
		}
	}
}

func TestFetchCurrentPublished_NoPublishedYet(t *testing.T) {
	c := &testutil.StubAPIClient{Body: `{"code":0,"data":{"items":[
    {"version_id":"oav_draft","status":4,"publish_time":null,"event_infos":[],"scopes":[]}
  ]}}`}
	v, err := FetchCurrentPublished(context.Background(), c, "cli_x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != nil {
		t.Errorf("want nil (app never published), got %+v", v)
	}
}

func TestFetchCurrentPublished_EmptyItems(t *testing.T) {
	c := &testutil.StubAPIClient{Body: `{"code":0,"data":{"items":[]}}`}
	v, err := FetchCurrentPublished(context.Background(), c, "cli_x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != nil {
		t.Errorf("want nil for empty items, got %+v", v)
	}
}

func TestFetchCurrentPublished_APIErrorPropagated(t *testing.T) {
	want := errors.New("insufficient permission level")
	c := &testutil.StubAPIClient{Err: want}
	v, err := FetchCurrentPublished(context.Background(), c, "cli_x")
	if !errors.Is(err, want) {
		t.Errorf("err = %v, want wrapping %v", err, want)
	}
	if v != nil {
		t.Errorf("want nil version on error, got %+v", v)
	}
}

func TestFetchCurrentPublished_PublishTimeEmptyStringTreatedAsUnpublished(t *testing.T) {
	c := &testutil.StubAPIClient{Body: `{"code":0,"data":{"items":[
    {"version_id":"oav_x","status":1,"publish_time":"","event_infos":[],"scopes":[]}
  ]}}`}
	v, err := FetchCurrentPublished(context.Background(), c, "cli_x")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v != nil {
		t.Errorf("want nil (empty publish_time), got %+v", v)
	}
}
