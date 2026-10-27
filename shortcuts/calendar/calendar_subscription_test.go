// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func runCalendarSubscription(t *testing.T, shortcut common.Shortcut, f *cmdutil.Factory, args ...string) error {
	t.Helper()
	cmd := &cobra.Command{Use: "calendar", SilenceErrors: true, SilenceUsage: true}
	shortcut.Mount(cmd, f)
	cmd.SetArgs(append([]string{shortcut.Command}, args...))
	return cmd.Execute()
}

func TestCalendarSubscriptionSuccess(t *testing.T) {
	for _, shortcut := range []common.Shortcut{CalendarSubscribe, CalendarUnsubscribe} {
		for _, identity := range []string{"user", "bot"} {
			t.Run(shortcut.Command+"/"+identity, func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				f, stdout, _, reg := cmdutil.TestFactory(t, calDefaultConfig())
				// Accepted input must be forwarded and echoed verbatim.
				calendarID := " cal/test@group.calendar.example "
				// Subscribe returns a calendar entity upstream; the shortcut must
				// ignore it and emit no extra payload on success.
				data := map[string]any{
					"calendar": map[string]any{
						"calendar_id": calendarID, "summary": "Shared calendar",
						"type": "shared", "permissions": "public", "role": "reader",
						"is_deleted": false, "color": -1,
					},
				}
				stub := &httpmock.Stub{
					Method: "POST",
					URL:    "/open-apis/calendar/v4/calendars/" + url.PathEscape(calendarID) + "/" + shortcut.Command[1:],
					Body:   map[string]any{"code": 0, "msg": "success", "data": data},
					OnMatch: func(req *http.Request) {
						require.Empty(t, req.URL.RawQuery)
					},
				}
				reg.Register(stub)
				require.NoError(t, runCalendarSubscription(t, shortcut, f, "--calendar-id", calendarID, "--as", identity))
				require.Empty(t, stub.CapturedBody)
				require.Len(t, stub.CapturedBodies, 1)
				var response struct {
					OK       bool           `json:"ok"`
					Identity string         `json:"identity"`
					Data     map[string]any `json:"data"`
				}
				require.NoError(t, json.Unmarshal(stdout.Bytes(), &response))
				require.True(t, response.OK)
				require.Equal(t, identity, response.Identity)
				require.Empty(t, response.Data)
			})
		}
	}
}

func TestCalendarSubscriptionInvalidID(t *testing.T) {
	for _, shortcut := range []common.Shortcut{CalendarSubscribe, CalendarUnsubscribe} {
		for _, calendarID := range []string{"", "  ", "../calendar", "cal?x=1", "cal%2Fother", "cal\x00", "cal\u202e"} {
			t.Run(shortcut.Command+"/"+calendarID, func(t *testing.T) {
				t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
				f, stdout, _, _ := cmdutil.TestFactory(t, calDefaultConfig())
				err := runCalendarSubscription(t, shortcut, f, "--calendar-id", calendarID, "--as", "user", "--dry-run")
				var validationErr *errs.ValidationError
				require.ErrorAs(t, err, &validationErr)
				require.Equal(t, errs.SubtypeInvalidArgument, validationErr.Subtype)
				require.Equal(t, "--calendar-id", validationErr.Param)
				require.Empty(t, stdout.String())
			})
		}
	}
}

func TestCalendarSubscriptionRejectsAutoBotFallback(t *testing.T) {
	for _, shortcut := range []common.Shortcut{CalendarSubscribe, CalendarUnsubscribe} {
		t.Run(shortcut.Command, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			f, _, _, _ := cmdutil.TestFactory(t, calDefaultConfig())
			f.IdentityAutoDetected = true
			cmd := &cobra.Command{}
			cmd.Flags().String("calendar-id", "cal_test", "")
			cmd.Flags().String("as", "auto", "")
			runtime := common.TestNewRuntimeContextForAPI(context.Background(), cmd, calDefaultConfig(), f, core.AsBot)
			var authErr *errs.AuthenticationError
			require.ErrorAs(t, shortcut.Validate(context.Background(), runtime), &authErr)
			require.Equal(t, errs.SubtypeTokenMissing, authErr.Subtype)
			require.Contains(t, authErr.Hint, "--as bot")

			require.NoError(t, cmd.Flags().Set("as", "bot"))
			require.NoError(t, shortcut.Validate(context.Background(), runtime))
		})
	}
}

func TestCalendarSubscribeFailureRecovery(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, calDefaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test/subscribe",
		Headers: http.Header{
			"X-Tt-Logid": []string{"subscribe-limit-log"},
		},
		Body: map[string]any{
			"code": 191005,
			"msg":  "calendar subscription limit reached",
			"error": map[string]any{
				"details": []any{map[string]any{"value": "upstream-limit-detail"}},
			},
			"data": map[string]any{},
		},
	})
	err := runCalendarSubscription(t, CalendarSubscribe, f, "--calendar-id", "cal_test", "--as", "user")
	var apiErr *errs.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, errs.CategoryAPI, apiErr.Category)
	require.Equal(t, errs.SubtypeUnknown, apiErr.Subtype)
	require.Equal(t, 191005, apiErr.Code)
	require.Equal(t, "subscribe-limit-log", apiErr.LogID)
	require.Contains(t, apiErr.Hint, "upstream-limit-detail")
	require.Contains(t, apiErr.Hint, "1000-calendar")
	require.Contains(t, apiErr.Hint, "lark-cli calendar +unsubscribe")
	require.Empty(t, stdout.String())
}

func TestCalendarUnsubscribeAlreadyUnsubscribed(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	f, stdout, _, reg := cmdutil.TestFactory(t, calDefaultConfig())
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/calendar/v4/calendars/cal_test/unsubscribe",
		Headers: http.Header{
			"X-Tt-Logid": []string{"subscription-log"},
		},
		Body: map[string]any{"code": 191007, "msg": "target calendar is already unsubscribed", "data": map[string]any{}},
	})
	err := runCalendarSubscription(t, CalendarUnsubscribe, f, "--calendar-id", "cal_test", "--as", "user")
	var apiErr *errs.APIError
	require.ErrorAs(t, err, &apiErr)
	require.Equal(t, 191007, apiErr.Code)
	require.Equal(t, "subscription-log", apiErr.LogID)
	require.Contains(t, apiErr.Hint, "already unsubscribed")
	require.Empty(t, stdout.String())
}

func TestCalendarSubscriptionRecoveryPreservesError(t *testing.T) {
	cause := errors.New("upstream cause")
	cases := []struct {
		name     string
		recover  func(error) error
		codeHint map[int]string
		// fallthrough codes belong to the other endpoint and must pass through
		// untouched (no hint attached).
		fallthrough_ []int
	}{
		{
			name:    "subscribe",
			recover: withCalendarSubscribeRecovery,
			codeHint: map[int]string{
				190007: "bot capability",
				191000: "calendar not found",
				191001: "invalid --calendar-id",
				191002: "cannot access",
				191003: "has been deleted",
				191004: "shared or primary",
				191005: "1000-calendar",
				191006: "cannot be subscribed",
				195100: "left the tenant",
			},
			fallthrough_: []int{191007, 191008},
		},
		{
			name:    "unsubscribe",
			recover: withCalendarUnsubscribeRecovery,
			codeHint: map[int]string{
				190007: "bot capability",
				191000: "calendar not found",
				191001: "invalid --calendar-id",
				191002: "cannot access",
				191003: "has been deleted",
				191004: "shared or primary",
				191007: "already unsubscribed",
				191008: "primary calendar cannot be unsubscribed",
				195100: "left the tenant",
			},
			fallthrough_: []int{191005, 191006},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			for code, hint := range tc.codeHint {
				apiErr := errs.NewAPIError(errs.SubtypeFailedPrecondition, "upstream failure").
					WithCode(code).WithLogID("log").WithHint("upstream detail").WithCause(cause)
				got := tc.recover(apiErr)
				require.Same(t, apiErr, got)
				require.ErrorIs(t, got, cause)
				require.Equal(t, errs.SubtypeFailedPrecondition, apiErr.Subtype)
				require.Equal(t, code, apiErr.Code)
				require.Equal(t, "log", apiErr.LogID)
				require.Contains(t, apiErr.Hint, "upstream detail")
				require.Contains(t, apiErr.Hint, hint)
			}
			for _, code := range tc.fallthrough_ {
				apiErr := errs.NewAPIError(errs.SubtypeFailedPrecondition, "upstream failure").WithCode(code)
				require.Same(t, apiErr, tc.recover(apiErr))
				require.Empty(t, apiErr.Hint)
			}
			// Non-API errors and unmapped codes pass through untouched.
			require.Same(t, cause, tc.recover(cause))
			unknown := errs.NewAPIError(errs.SubtypeFailedPrecondition, "unknown").WithCode(199999)
			require.Same(t, unknown, tc.recover(unknown))
			require.Empty(t, unknown.Hint)
		})
	}
}
