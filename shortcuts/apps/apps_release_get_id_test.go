// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
)

// sparkRequestProbe catches any request the command sends to the apps OpenAPI, so
// a test can assert that a rejected argument never reached the server.
func sparkRequestProbe(reg *httpmock.Registry) *httpmock.Stub {
	probe := &httpmock.Stub{
		URL: "/open-apis/spark/v1/", Reusable: true, Optional: true,
		Body: map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	}
	reg.Register(probe)
	return probe
}

// TestAppsReleaseGet_RejectsNonNumericReleaseID pins the local --release-id check.
//
// The gateway types the release id as a 64-bit integer and answers anything else
// with code 9499 "Invalid parameter type in param: ReleaseID". Each value below is
// a shape callers actually sent in one week: placeholders, a release status, the
// whole +release-create output, the id still wrapped in quotes, several ids at
// once, a prefixed id, and digit strings longer than a 64-bit integer. None of them
// can succeed, so the command must reject them before the request and say where a
// valid id comes from.
func TestAppsReleaseGet_RejectsNonNumericReleaseID(t *testing.T) {
	for _, tc := range []struct{ name, raw string }{
		{"placeholder x", "x"},
		{"null", "null"},
		{"latest", "latest"},
		{"status", "finished"},
		{"doc placeholder", "release_yyy"},
		{"quoted id", `"7712345678901234567"`},
		{"whole create output", "{\n  \"ok\": true,\n  \"data\": {\n    \"release_id\": \"7712345678901234567\""},
		{"two ids", "7712345678901234567 7712345678901234568"},
		{"prefixed id", "release7712345678901234567"},
		{"trailing comma", "7712345678901234567,"},
		{"above int64", "9999999999999999999"},
		{"twenty digits", "77123456789012345678"},
		{"negative", "-1"},
		{"signed", "+7712345678901234567"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			factory, stdout, reg := newAppsExecuteFactory(t)
			probe := sparkRequestProbe(reg)

			err := runAppsShortcut(t, AppsReleaseGet,
				[]string{"+release-get", "--app-id", "app_x", "--release-id", tc.raw, "--as", "user"}, factory, stdout)
			p := requireAppsValidationProblem(t, err)
			if p.Subtype != errs.SubtypeInvalidArgument {
				t.Errorf("subtype = %q, want %q", p.Subtype, errs.SubtypeInvalidArgument)
			}
			if got := validationParamOf(t, err); got != "--release-id" {
				t.Errorf("param = %q, want --release-id", got)
			}
			for _, want := range []string{"-q '.data.release_id'", "lark-cli apps +release-list --app-id app_x"} {
				if !strings.Contains(p.Hint, want) {
					t.Errorf("hint = %q, want it to mention %q", p.Hint, want)
				}
			}
			if n := len(probe.CapturedBodies); n != 0 {
				t.Errorf("sent %d request(s) for a release id the server cannot accept", n)
			}
		})
	}
}

// TestAppsReleaseGet_DryRunRejectsNonNumericReleaseID keeps the preview honest: a
// dry run must not print a request the real run would refuse.
func TestAppsReleaseGet_DryRunRejectsNonNumericReleaseID(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsReleaseGet,
		[]string{"+release-get", "--app-id", "app_x", "--release-id", "latest", "--dry-run", "--as", "user"}, factory, stdout)
	requireAppsValidationProblem(t, err)
	if got := validationParamOf(t, err); got != "--release-id" {
		t.Errorf("param = %q, want --release-id", got)
	}
}

// TestAppsReleaseGet_AcceptsNumericReleaseID guards the other direction: a real
// release id, including one with surrounding blanks, still reaches the server.
func TestAppsReleaseGet_AcceptsNumericReleaseID(t *testing.T) {
	for _, raw := range []string{"7712345678901234567", " 7712345678901234567 ", "9223372036854775807", "5"} {
		t.Run(strings.TrimSpace(raw), func(t *testing.T) {
			factory, stdout, _ := newAppsExecuteFactory(t)
			if err := runAppsShortcut(t, AppsReleaseGet,
				[]string{"+release-get", "--app-id", "app_x", "--release-id", raw, "--dry-run", "--as", "user"}, factory, stdout); err != nil {
				t.Fatalf("dry-run err = %v", err)
			}
			var env dryRunAPIEnvelope
			if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
				t.Fatalf("unmarshal dry-run: %v\n%s", err, stdout.String())
			}
			want := "/open-apis/spark/v1/apps/app_x/releases/" + strings.TrimSpace(raw)
			if len(env.API) != 1 || env.API[0].Method != "GET" || env.API[0].URL != want {
				t.Fatalf("dry-run = %+v, want GET %s", env.API, want)
			}
		})
	}
}

// TestAppsReleaseGet_ExecuteTrimsReleaseID covers the execution path, which reads
// --release-id on its own: a padded id must request the trimmed release, not a
// path with encoded blanks that the stub below would not match.
func TestAppsReleaseGet_ExecuteTrimsReleaseID(t *testing.T) {
	rctx, stdout, reg := newStatusRuntimeContext(t, "app_x", " 7712345678901234567 ")
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/spark/v1/apps/app_x/releases/7712345678901234567",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "",
			"data": map[string]interface{}{
				"release": map[string]interface{}{"release_id": "7712345678901234567", "status": "finished"},
			},
		},
	})

	if err := AppsReleaseGet.Execute(context.Background(), rctx); err != nil {
		t.Fatalf("Execute() = %v", err)
	}
	var env struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal output: %v\n%s", err, stdout.String())
	}
	if !env.OK || env.Data["release_id"] != "7712345678901234567" {
		t.Fatalf("output = %s, want ok with release_id 7712345678901234567", stdout.String())
	}
}
