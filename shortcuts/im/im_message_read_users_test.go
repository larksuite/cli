// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strconv"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

func newMessageReadUsersTestRuntime(t *testing.T, transport http.RoundTripper, stringsMap map[string]string, bools map[string]bool, ints map[string]int) *common.RuntimeContext {
	t.Helper()

	runtime := newUserShortcutRuntime(t, transport)
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("message-id", "", "")
	cmd.Flags().String("user-id-type", "open_id", "")
	cmd.Flags().Int("page-size", 100, "")
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().Bool("page-all", false, "")
	cmd.Flags().Int("page-limit", 10, "")
	cmd.Flags().Int("page-delay", 200, "")
	for name, value := range stringsMap {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("Flags().Set(%q) error = %v", name, err)
		}
	}
	for name, value := range bools {
		if err := cmd.Flags().Set(name, strconv.FormatBool(value)); err != nil {
			t.Fatalf("Flags().Set(%q) error = %v", name, err)
		}
	}
	for name, value := range ints {
		if err := cmd.Flags().Set(name, strconv.Itoa(value)); err != nil {
			t.Fatalf("Flags().Set(%q) error = %v", name, err)
		}
	}
	runtime.Cmd = cmd
	return runtime
}

func TestMessageReadUsersScopesByIdentity(t *testing.T) {
	if got := ImMessageReadUsers.ScopesForIdentity("user"); !reflect.DeepEqual(got, []string{"im:message:readonly"}) {
		t.Fatalf("user scopes = %v, want [im:message:readonly]", got)
	}
	if got := ImMessageReadUsers.DeclaredScopesForIdentity("user"); !reflect.DeepEqual(got, []string{"im:message:readonly"}) {
		t.Fatalf("declared user scopes = %v, want [im:message:readonly]", got)
	}
	if !reflect.DeepEqual(ImMessageReadUsers.ScopesForIdentity("bot"), []string{"im:message:readonly"}) {
		t.Fatalf("bot scopes = %v", ImMessageReadUsers.ScopesForIdentity("bot"))
	}
	if !reflect.DeepEqual(ImMessageReadUsers.AuthTypes, []string{"user", "bot"}) {
		t.Fatalf("AuthTypes = %v", ImMessageReadUsers.AuthTypes)
	}
}

func TestMessageReadUsersUsesSharedPaginationFlags(t *testing.T) {
	flags := make(map[string]common.Flag, len(ImMessageReadUsers.Flags))
	for _, flag := range ImMessageReadUsers.Flags {
		flags[flag.Name] = flag
	}
	for _, shared := range common.PageAllFlags() {
		got, ok := flags[shared.Name]
		if !ok {
			t.Fatalf("missing shared pagination flag %q", shared.Name)
		}
		if !reflect.DeepEqual(got, shared) {
			t.Fatalf("pagination flag %q = %#v, want %#v", shared.Name, got, shared)
		}
	}
}

func TestBuildMessageReadUsersParams(t *testing.T) {
	runtime := newMessageReadUsersTestRuntime(t, shortcutRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, fmt.Errorf("unexpected request")
	}), map[string]string{"message-id": "om_test"}, nil, nil)

	got, err := buildMessageReadUsersParams(runtime, "cursor")
	if err != nil {
		t.Fatalf("buildMessageReadUsersParams() error = %v", err)
	}
	want := map[string]interface{}{
		"user_id_type": "open_id",
		"page_size":    100,
		"page_token":   "cursor",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildMessageReadUsersParams() = %#v, want %#v", got, want)
	}
}

func TestFetchMessageReadUsersAggregatesPages(t *testing.T) {
	calls := 0
	transport := shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/open-apis/im/v1/messages/om_test/read_users" {
			return nil, fmt.Errorf("unexpected path: %s", req.URL.Path)
		}
		calls++
		if req.URL.Query().Get("page_token") == "" {
			return shortcutJSONResponse(200, map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"items":      []interface{}{map[string]interface{}{"user_id": "ou_one", "timestamp": "1"}},
					"has_more":   true,
					"page_token": "next",
				},
			}), nil
		}
		return shortcutJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":    []interface{}{map[string]interface{}{"user_id": "ou_two", "tenant_key": "tenant"}},
				"has_more": false,
			},
		}), nil
	})
	runtime := newMessageReadUsersTestRuntime(t, transport, map[string]string{"message-id": "om_test"}, map[string]bool{"page-all": true}, map[string]int{"page-limit": 10, "page-delay": 0})

	got, pagination, err := fetchMessageReadUsers(runtime)
	if err != nil {
		t.Fatalf("fetchMessageReadUsers() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	items, _ := got["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
	if got["has_more"] != false || got["page_token"] != "" || got["total"] != 2 {
		t.Fatalf("result metadata = %#v", got)
	}
	if !pagination.Complete || pagination.Pages != 2 {
		t.Fatalf("pagination = %#v, want complete two-page result", pagination)
	}
}

func TestFetchMessageReadUsersStopsAtPageLimit(t *testing.T) {
	calls := 0
	transport := shortcutRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return shortcutJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":      []interface{}{map[string]interface{}{"user_id": fmt.Sprintf("ou_%d", calls)}},
				"has_more":   true,
				"page_token": fmt.Sprintf("page_%d", calls),
			},
		}), nil
	})
	runtime := newMessageReadUsersTestRuntime(t, transport, map[string]string{"message-id": "om_test"}, map[string]bool{"page-all": true}, map[string]int{"page-limit": 2, "page-delay": 0})

	got, pagination, err := fetchMessageReadUsers(runtime)
	if err != nil {
		t.Fatalf("fetchMessageReadUsers() error = %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	if pagination.Complete || pagination.Pages != 2 || pagination.NextToken != "page_2" {
		t.Fatalf("pagination = %#v, want incomplete two-page result", pagination)
	}
	items, _ := got["items"].([]interface{})
	if len(items) != 2 {
		t.Fatalf("len(items) = %d, want 2", len(items))
	}
}

func TestFetchMessageReadUsersRejectsInvalidItemsType(t *testing.T) {
	transport := shortcutRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return shortcutJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":    "not-an-array",
				"has_more": false,
			},
		}), nil
	})
	runtime := newMessageReadUsersTestRuntime(t, transport, map[string]string{"message-id": "om_test"}, nil, nil)

	_, _, err := fetchMessageReadUsers(runtime)
	requireMessageReadUsersInvalidResponse(t, err)
}

func TestFetchMessageReadUsersRejectsMissingNextToken(t *testing.T) {
	transport := shortcutRoundTripFunc(func(*http.Request) (*http.Response, error) {
		return shortcutJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":    []interface{}{},
				"has_more": true,
			},
		}), nil
	})
	runtime := newMessageReadUsersTestRuntime(t, transport, map[string]string{"message-id": "om_test"}, map[string]bool{"page-all": true}, map[string]int{"page-delay": 0})

	_, _, err := fetchMessageReadUsers(runtime)
	requireMessageReadUsersInvalidResponse(t, err)
}

func TestFetchMessageReadUsersRejectsRepeatedNextToken(t *testing.T) {
	calls := 0
	transport := shortcutRoundTripFunc(func(*http.Request) (*http.Response, error) {
		calls++
		return shortcutJSONResponse(200, map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items":      []interface{}{},
				"has_more":   true,
				"page_token": "repeat",
			},
		}), nil
	})
	runtime := newMessageReadUsersTestRuntime(t, transport, map[string]string{"message-id": "om_test"}, map[string]bool{"page-all": true}, map[string]int{"page-delay": 0})

	_, _, err := fetchMessageReadUsers(runtime)
	requireMessageReadUsersInvalidResponse(t, err)
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
}

func requireMessageReadUsersInvalidResponse(t *testing.T, err error) {
	t.Helper()
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeInvalidResponse {
		t.Fatalf("problem = %#v, err = %v; want invalid_response", problem, err)
	}
}

func TestMessageReadUsersValidationRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name       string
		stringsMap map[string]string
		ints       map[string]int
		wantParam  string
	}{
		{name: "invalid message", stringsMap: map[string]string{"message-id": "oc_invalid"}, wantParam: "--message-id"},
		{name: "invalid id type", stringsMap: map[string]string{"message-id": "om_test", "user-id-type": "email"}, wantParam: "--user-id-type"},
		{name: "page size too large", stringsMap: map[string]string{"message-id": "om_test"}, ints: map[string]int{"page-size": 101}, wantParam: "--page-size"},
		{name: "zero page limit", stringsMap: map[string]string{"message-id": "om_test"}, ints: map[string]int{"page-limit": 0}, wantParam: "--page-limit"},
		{name: "negative page limit", stringsMap: map[string]string{"message-id": "om_test"}, ints: map[string]int{"page-limit": -1}, wantParam: "--page-limit"},
		{name: "negative page delay", stringsMap: map[string]string{"message-id": "om_test"}, ints: map[string]int{"page-delay": -1}, wantParam: "--page-delay"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := newMessageReadUsersTestRuntime(t, shortcutRoundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, fmt.Errorf("unexpected request")
			}), tt.stringsMap, nil, tt.ints)
			err := validateMessageReadUsers(runtime)
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("errors.As(*errs.ValidationError) = false, err = %v", err)
			}
			if validationErr.Category != errs.CategoryValidation || validationErr.Subtype != errs.SubtypeInvalidArgument || validationErr.Param != tt.wantParam {
				t.Fatalf("validation error = %#v, want category=%q subtype=%q param=%q", validationErr, errs.CategoryValidation, errs.SubtypeInvalidArgument, tt.wantParam)
			}
		})
	}
}
