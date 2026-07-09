// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

// TestAppsList_503IsRetryableTypedError pins that a 5xx response from the apps
// list endpoint surfaces as a typed errs.Problem with Retryable == true (via
// CallAPITyped → httpStatusError).
func TestAppsList_503IsRetryableTypedError(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps",
		Status: 503,
		// A gateway-style non-JSON body (text/html) forces the status-based
		// classifier (httpStatusError) rather than the API-envelope path.
		Headers: http.Header{"Content-Type": []string{"text/html"}},
		RawBody: []byte("<html><body>503 Service Unavailable</body></html>"),
	})

	err := runAppsShortcut(t, AppsList,
		[]string{"+list", "--as", "user"}, factory, stdout)
	if err == nil {
		t.Fatalf("expected an error on 503, got nil; stdout:\n%s", stdout.String())
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed errs.Problem on 503, got %T: %v", err, err)
	}
	if !p.Retryable {
		t.Fatalf("expected Retryable == true on 503, got Problem=%+v", p)
	}
}

// TestAppsList_SuccessShapeUnchanged pins that the success path is
// output-shape-neutral after migration: a 200 envelope still yields a success
// stdout envelope carrying the app_id.
func TestAppsList_SuccessShapeUnchanged(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"app_id": "a", "name": "n"},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsList,
		[]string{"+list", "--as", "user"}, factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"app_id": "a"`) {
		t.Fatalf("stdout missing app_id: %s", got)
	}
}
