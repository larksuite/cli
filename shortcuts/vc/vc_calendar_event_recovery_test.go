// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	keyring "github.com/zalando/go-keyring"

	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/recovery"
	"github.com/larksuite/cli/internal/surface"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestCalendarEventResolutionMissingScopeProjectsInlineHint(t *testing.T) {
	// Prime the package test credential cache before subtests isolate keyring and
	// data directories for their real stored-user-token setup.
	warmTokenCache(t)

	const (
		calendarID   = "cal_scope"
		instanceID   = "evt_scope"
		missingScope = "calendar:calendar.event:read"
		wantError    = "unauthorized: user authorization does not cover the required scope(s): " + missingScope
	)

	sinks := []struct {
		name      string
		shortcut  common.Shortcut
		command   string
		resultKey string
	}{
		{name: "notes", shortcut: VCNotes, command: "+notes", resultKey: "notes"},
		{name: "recording", shortcut: VCRecording, command: "+recording", resultKey: "recordings"},
	}
	projections := []struct {
		name string
		plan *surface.Plan
	}{
		{name: "visible"},
		{
			name: "concealed",
			plan: surface.NewPlan(map[surface.CommandID]surface.CommandState{
				surface.CommandAuthLogin: surface.CommandConcealed,
			}),
		},
	}

	for _, sink := range sinks {
		for _, projection := range projections {
			t.Run(sink.name+"/"+projection.name, func(t *testing.T) {
				keyring.MockInit()
				t.Setenv("HOME", t.TempDir())
				t.Setenv("LARKSUITE_CLI_DATA_DIR", t.TempDir())
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

				cfg := defaultConfig()
				now := time.Now()
				stored := &auth.StoredUAToken{
					UserOpenId:       cfg.UserOpenId,
					AppId:            cfg.AppID,
					AccessToken:      "test-user-access-token",
					RefreshToken:     "test-refresh-token",
					ExpiresAt:        now.Add(time.Hour).UnixMilli(),
					RefreshExpiresAt: now.Add(24 * time.Hour).UnixMilli(),
					GrantedAt:        now.Add(-time.Hour).UnixMilli(),
					Scope: strings.Join([]string{
						"vc:note:read",
						"vc:record:readonly",
						"vc:meeting.meetingevent:read",
						"calendar:calendar:read",
						missingScope,
					}, " "),
				}
				if err := auth.SetStoredToken(stored); err != nil {
					t.Fatalf("SetStoredToken() error = %v", err)
				}
				t.Cleanup(func() { _ = auth.RemoveStoredToken(cfg.AppID, cfg.UserOpenId) })

				f, stdout, _, registry := cmdutil.TestFactory(t, cfg)
				f.Recovery = recovery.NewProjector(func() *surface.Plan { return projection.plan })
				registry.Register(primaryCalendarStub(calendarID))
				registry.Register(&httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/calendar/v4/calendars/" + calendarID + "/events/mget_instance_relation_info",
					Body: map[string]interface{}{
						"code": 99991679,
						"msg":  "missing scope",
						"error": map[string]interface{}{
							"permission_violations": []interface{}{
								map[string]interface{}{"subject": missingScope},
							},
						},
					},
				})

				err := mountAndRun(t, sink.shortcut, []string{
					sink.command,
					"--calendar-event-ids", instanceID,
					"--as", "user",
					"--format", "json",
				}, f, stdout)
				var partial *output.PartialFailureError
				if !errors.As(err, &partial) || partial.Code != output.ExitAPI {
					t.Fatalf("exit error = %T %v, want PartialFailureError(%d)", err, err, output.ExitAPI)
				}
				registry.Verify(t)

				var envelope struct {
					OK   bool                   `json:"ok"`
					Data map[string]interface{} `json:"data"`
				}
				if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
					t.Fatalf("unmarshal stdout: %v\n%s", err, stdout.String())
				}
				if envelope.OK {
					t.Fatalf("envelope.ok = true, want false: %s", stdout.String())
				}
				items, ok := envelope.Data[sink.resultKey].([]interface{})
				if !ok || len(items) != 1 {
					t.Fatalf("data.%s = %#v, want one result", sink.resultKey, envelope.Data[sink.resultKey])
				}
				result, ok := items[0].(map[string]interface{})
				if !ok {
					t.Fatalf("result = %T, want object", items[0])
				}
				if got := result["calendar_event_id"]; got != instanceID {
					t.Errorf("calendar_event_id = %#v, want %q", got, instanceID)
				}
				if got := result["error"]; got != wantError {
					t.Errorf("error = %#v, want unchanged %q", got, wantError)
				}
				hint, ok := result["hint"].(string)
				if !ok {
					t.Fatalf("hint = %#v, want string", result["hint"])
				}
				wantHint := recovery.UserAuthorization(missingScope).Render(projection.plan)
				if hint != wantHint {
					t.Errorf("hint = %q, want %q", hint, wantHint)
				}
				if projection.plan != nil && strings.Contains(hint, "auth login") {
					t.Errorf("concealed hint leaked auth command: %q", hint)
				}
			})
		}
	}
}
