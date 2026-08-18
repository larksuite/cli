// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

// The two observability commands must route their API failures through
// withObservabilityHint, not the generic withAppsHint. common_test.go proves the
// helper in isolation, but that stays green if a call site reverts to
// withAppsHint — so these command-level tests drive the real Execute path with a
// mocked "Container not exists" (400002655) envelope and assert the
// container-specific rewrite. A revert to withAppsHint would leave the raw
// message and swap in the app-id hint, failing both assertions.

func TestAppsMetricList_NoContainerRewriteThroughExecute(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    metricListPath("app_x"),
		Body:   map[string]interface{}{"code": appNoContainerCode, "msg": "Container not exists"},
	})

	err := runAppsShortcut(t, AppsMetricList, []string{
		"+metric-list", "--app-id", "app_x", "--metric", "requests", "--as", "user",
	}, factory, stdout)

	p := requireAppsValidationProblem(t, err)
	if p.Code != appNoContainerCode {
		t.Errorf("Code = %d, want %d", p.Code, appNoContainerCode)
	}
	if p.Message != appNoContainerMessage {
		t.Errorf("Message = %q, want container rewrite %q (call site may have reverted to withAppsHint)", p.Message, appNoContainerMessage)
	}
	if p.Hint != appNoContainerHint {
		t.Errorf("Hint = %q, want deploy hint %q (call site may have reverted to withAppsHint)", p.Hint, appNoContainerHint)
	}
}

func TestAppsAnalyticsList_NoContainerRewriteThroughExecute(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    analyticsListPath("app_x"),
		Body:   map[string]interface{}{"code": appNoContainerCode, "msg": "Container not exists"},
	})

	err := runAppsShortcut(t, AppsAnalyticsList, []string{
		"+analytics-list", "--app-id", "app_x", "--analytics", "users", "--as", "user",
	}, factory, stdout)

	p := requireAppsValidationProblem(t, err)
	if p.Code != appNoContainerCode {
		t.Errorf("Code = %d, want %d", p.Code, appNoContainerCode)
	}
	if p.Message != appNoContainerMessage {
		t.Errorf("Message = %q, want container rewrite %q (call site may have reverted to withAppsHint)", p.Message, appNoContainerMessage)
	}
	if p.Hint != appNoContainerHint {
		t.Errorf("Hint = %q, want deploy hint %q (call site may have reverted to withAppsHint)", p.Hint, appNoContainerHint)
	}
}
