// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// Server messages below are the wording observed on the wire. Values that vary
// per request (database user, connection target, value size) use neutral
// placeholders in the same shape.
const (
	unpublishedAccessScopeMessage = "Application has not been published yet. Access scope operations require the app to be published first."
	expertModeOnlyMessageZH       = "请求参数不合法：env var operation is only supported for expert mode apps"
	expertModeOnlyMessageEN       = "Invalid Request: env var operation is only supported for expert mode apps"
	envValueTooLargeMessage       = "Invalid Request: value size 6059 is greater than max size 4096"
	releaseInternalMessageEN      = "Something went wrong. Please try again later."
	releaseInternalMessageZH      = "出错了，请稍后重试"
	dbPasswordAuthMessage         = "k_dl_000001：ERROR: password authentication failed for user 'app_user' (SQLSTATE XX000)"
	dbConnectionSlotsMessage      = "k_dl_000001：ERROR: remaining connection slots are reserved for roles with the SUPERUSER attribute (SQLSTATE XX000)"
	dbGetConnectionMessage        = "k_dl_1600052：Get DB connection failed"
)

type contextualHintCase struct {
	name, command, method, path, message string
	status, code                         int
	args                                 []string
	want, forbid                         []string
}

// appsShortcutByCommand resolves the real registered shortcut used by command-level hint tests.
func appsShortcutByCommand(t *testing.T, command string) common.Shortcut {
	t.Helper()
	for _, sc := range Shortcuts() {
		if sc.Command == command {
			return sc
		}
	}
	t.Fatalf("shortcut %s is not registered", command)
	return common.Shortcut{}
}

// runContextualHintCase executes one mocked command failure and pins its typed error contract.
func runContextualHintCase(t *testing.T, tc contextualHintCase) *errs.Problem {
	t.Helper()
	factory, stdout, reg := newAppsExecuteFactory(t)
	calls := 0
	reg.Register(&httpmock.Stub{
		Method: tc.method, URL: tc.path, Status: tc.status,
		Body:    map[string]interface{}{"code": tc.code, "msg": tc.message},
		OnMatch: func(*http.Request) { calls++ },
	})
	err := runAppsShortcut(t, appsShortcutByCommand(t, tc.command), tc.args, factory, stdout)
	if err == nil {
		t.Fatalf("expected failure, got nil; stdout=%s", stdout.String())
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T: %v", err, err)
	}
	if calls != 1 {
		t.Fatalf("HTTP calls = %d, want 1", calls)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on failure", stdout.String())
	}
	if p.Code != tc.code || p.Message != tc.message {
		t.Fatalf("code/message = %d/%q, want %d/%q", p.Code, p.Message, tc.code, tc.message)
	}
	wantCategory, wantSubtype, wantExit := errs.CategoryAPI, errs.SubtypeUnknown, 1
	if tc.code == 400002484 {
		wantCategory, wantSubtype, wantExit = errs.CategoryValidation, errs.SubtypeInvalidArgument, 2
	}
	if p.Category != wantCategory || p.Subtype != wantSubtype || output.ExitCodeOf(err) != wantExit || p.Retryable {
		t.Fatalf("category/subtype/exit/retryable = %q/%q/%d/%t, want %q/%q/%d/false", p.Category, p.Subtype, output.ExitCodeOf(err), p.Retryable, wantCategory, wantSubtype, wantExit)
	}
	lowerHint := strings.ToLower(p.Hint)
	for _, want := range tc.want {
		if !strings.Contains(lowerHint, strings.ToLower(want)) {
			t.Errorf("hint %q does not contain %q", p.Hint, want)
		}
	}
	for _, forbid := range tc.forbid {
		if strings.Contains(lowerHint, strings.ToLower(forbid)) {
			t.Errorf("hint %q unexpectedly contains %q", p.Hint, forbid)
		}
	}
	return p
}

// TestAppsContextualHints_RealCommands verifies every supported wire signature through registered commands.
func TestAppsContextualHints_RealCommands(t *testing.T) {
	appPath := "/open-apis/spark/v1/apps/app_x"
	cases := []contextualHintCase{
		{name: "01 unpublished get", command: "+access-scope-get", method: "GET", path: appPath + "/access-scope", status: 409, code: 3340002, message: unpublishedAccessScopeMessage, args: []string{"+access-scope-get", "--app-id", "app_x", "--as", "user"}, want: []string{"user confirmation", "+release-create --help"}, forbid: []string{"verify --app-id", "apps +list"}},
		{name: "02 creative get", command: "+access-scope-get", method: "GET", path: appPath + "/access-scope", status: 200, code: 40002, message: "创意模式应用不支持查询可用范围，请前往妙搭后台操作", args: []string{"+access-scope-get", "--app-id", "app_x", "--as", "user"}, want: []string{"creative", "web console", "do not retry"}, forbid: []string{"verify --app-id", "apps +list"}},
		{name: "03 creative set", command: "+access-scope-set", method: "PUT", path: appPath + "/access-scope", status: 200, code: 40002, message: "创意模式应用不支持设置可用范围，请前往妙搭后台操作", args: []string{"+access-scope-set", "--app-id", "app_x", "--scope", "tenant", "--as", "user"}, want: []string{"creative", "web console", "do not retry"}, forbid: []string{"--targets", "+access-scope-get"}},
		{name: "04 expert list", command: "+env-list", method: "POST", path: appPath + "/env_vars", status: 200, code: 400000072, message: expertModeOnlyMessageZH, args: []string{"+env-list", "--app-id", "app_x", "--as", "user"}, want: []string{"expert mode", "do not retry"}, forbid: []string{"verify --app-id", "apps +list"}},
		{name: "05a expert set", command: "+env-set", method: "POST", path: appPath + "/create_or_update_env_var", status: 200, code: 400000072, message: expertModeOnlyMessageZH, args: []string{"+env-set", "--app-id", "app_x", "--key", "FOO", "--value", "bar", "--as", "user"}, want: []string{"expert mode", "do not retry"}, forbid: []string{"apps +list"}},
		{name: "05b value size", command: "+env-set", method: "POST", path: appPath + "/create_or_update_env_var", status: 200, code: 400000072, message: envValueTooLargeMessage, args: []string{"+env-set", "--app-id", "app_x", "--key", "FOO", "--value", "bar", "--as", "user"}, want: []string{"shorten", "4096 bytes"}, forbid: []string{"apps +list"}},
		{name: "06 release in progress", command: "+release-create", method: "POST", path: appPath + "/releases", status: 200, code: 400002553, message: "发布任务已存在", args: []string{"+release-create", "--app-id", "app_x", "--as", "user"}, want: []string{"+release-list", "+release-get", "do not create another"}, forbid: []string{"git pull --rebase", "then retry"}},
		{name: "07 release internal", command: "+release-create", method: "POST", path: appPath + "/releases", status: 200, code: 500000049, message: releaseInternalMessageEN, args: []string{"+release-create", "--app-id", "app_x", "--as", "user"}, want: []string{"internal failure", "confirm whether", "request identifier"}, forbid: []string{"push was rejected", "git pull --rebase"}},
		{name: "08 session app unavailable", command: "+session-get", method: "GET", path: appPath + "/sessions/conv_x", status: 404, code: 2320008, message: "app tenant not found", args: []string{"+session-get", "--app-id", "app_x", "--session-id", "conv_x", "--as", "user"}, want: []string{"apps +list", "stop"}, forbid: []string{"session_id", "+session-list"}},
		{name: "09 create internal", command: "+create", method: "POST", path: "/open-apis/spark/v1/apps", status: 200, code: 50001, message: "upstream internal error", args: []string{"+create", "--name", "example", "--app-type", "html", "--as", "user"}, want: []string{"internal failure", "confirm whether", "request identifier"}, forbid: []string{"--app-type", "--name"}},
		{name: "10 app banned", command: "+db-execute", method: "POST", path: appPath + "/sql_commands", status: 200, code: 500232002, message: "[GetApp] app is banned for Miaoda", args: []string{"+db-execute", "--app-id", "app_x", "--sql", "select 1", "--yes", "--as", "user"}, want: []string{"banned", "stop", "owner", "administrator"}, forbid: []string{"+db-table-get", "--environment dev"}},
		{name: "11a connection auth", command: "+db-table-list", method: "GET", path: appPath + "/tables", status: 200, code: 500002776, message: dbPasswordAuthMessage, args: []string{"+db-table-list", "--app-id", "app_x", "--as", "user"}, want: []string{"database connection", "do not create", "request identifier"}, forbid: []string{"+db-env-create", "verify --app-id"}},
		{name: "11b connection slots", command: "+db-table-list", method: "GET", path: appPath + "/tables", status: 200, code: 500002776, message: dbConnectionSlotsMessage, args: []string{"+db-table-list", "--app-id", "app_x", "--as", "user"}, want: []string{"database connection", "do not create", "request identifier"}, forbid: []string{"+db-env-create"}},
		{name: "11c connection acquire", command: "+db-table-list", method: "GET", path: appPath + "/tables", status: 200, code: 500002776, message: dbGetConnectionMessage, args: []string{"+db-table-list", "--app-id", "app_x", "--as", "user"}, want: []string{"database connection", "do not create", "request identifier"}, forbid: []string{"+db-env-create"}},
		{name: "08 control env", command: "+env-list", method: "POST", path: appPath + "/env_vars", status: 404, code: 2320008, message: "app tenant not found", args: []string{"+env-list", "--app-id", "app_x", "--as", "user"}, want: []string{"apps +list", "stop"}, forbid: []string{"verify --app-id"}},
		{name: "10 control env", command: "+env-list", method: "POST", path: appPath + "/env_vars", status: 200, code: 500232002, message: "[GetApp] app is banned for Miaoda", args: []string{"+env-list", "--app-id", "app_x", "--as", "user"}, want: []string{"banned", "stop"}, forbid: []string{"apps +list"}},
		{name: "extra app not found", command: "+db-execute", method: "POST", path: appPath + "/sql_commands", status: 200, code: 400002484, message: "app not found", args: []string{"+db-execute", "--app-id", "app_x", "--sql", "select 1", "--yes", "--as", "user"}, want: []string{"apps +list", "cannot be identified"}, forbid: []string{"+db-table-get", "--environment dev", "banned"}},
		{name: "extra behind main", command: "+release-create", method: "POST", path: appPath + "/releases", status: 200, code: 400002552, message: "分支落后于主分支，请先进行 Rebase 操作", args: []string{"+release-create", "--app-id", "app_x", "--as", "user"}, want: []string{"origin/main", "do not force-push"}, forbid: []string{"sprint/default"}},
		{name: "extra unpublished set", command: "+access-scope-set", method: "PUT", path: appPath + "/access-scope", status: 409, code: 3340002, message: unpublishedAccessScopeMessage, args: []string{"+access-scope-set", "--app-id", "app_x", "--scope", "tenant", "--as", "user"}, want: []string{"user confirmation", "+release-create --help"}, forbid: []string{"+access-scope-get"}},
		{name: "extra expert delete", command: "+env-delete", method: "POST", path: appPath + "/delete_env_vars", status: 200, code: 400000072, message: expertModeOnlyMessageZH, args: []string{"+env-delete", "--app-id", "app_x", "--key", "FOO", "--yes", "--as", "user"}, want: []string{"expert mode", "do not retry"}, forbid: []string{"apps +list"}},
		{name: "deleted get", command: "+get", method: "GET", path: appPath, status: 404, code: 2320008, message: "app tenant not found", args: []string{"+get", "--app-id", "app_x", "--as", "user"}, want: []string{"apps +list", "stop"}, forbid: []string{"verify --app-id"}},
		{name: "deleted delete", command: "+delete", method: "GET", path: appPath, status: 404, code: 2320008, message: "app tenant not found", args: []string{"+delete", "--app-id", "app_x", "--confirm-name", "Deleted App", "--yes", "--as", "user"}, want: []string{"apps +list", "stop"}, forbid: []string{"verify --app-id"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { runContextualHintCase(t, tc) })
	}
}

// TestAppsContextualHints_BoundariesAndInvariants pins neutral server-code boundaries and legacy fallbacks.
func TestAppsContextualHints_BoundariesAndInvariants(t *testing.T) {
	tests := []struct {
		name    string
		problem *errs.APIError
		want    string
		forbid  string
	}{
		{name: "lower bound", problem: errs.NewAPIError(errs.SubtypeUnknown, "unknown").WithCode(500000000), want: "no specific recovery guidance", forbid: "internal failure"},
		{name: "upper inside", problem: errs.NewAPIError(errs.SubtypeUnknown, "unknown").WithCode(599999999), want: "no specific recovery guidance", forbid: "internal failure"},
		{name: "below range keeps candidate", problem: errs.NewAPIError(errs.SubtypeUnknown, "unknown").WithCode(499999999), want: "candidate hint"},
		{name: "upper outside keeps candidate", problem: errs.NewAPIError(errs.SubtypeUnknown, "unknown").WithCode(600000000), want: "candidate hint"},
		{name: "50000 keeps candidate", problem: errs.NewAPIError(errs.SubtypeUnknown, "unknown").WithCode(50000), want: "candidate hint"},
		{name: "50001 generic neutral", problem: errs.NewAPIError(errs.SubtypeUnknown, "explicit input problem").WithCode(50001), want: "follow any explicit cause", forbid: "internal failure"},
		{name: "50002 keeps candidate", problem: errs.NewAPIError(errs.SubtypeUnknown, "unknown").WithCode(50002), want: "candidate hint"},
		{name: "50003 keeps candidate", problem: errs.NewAPIError(errs.SubtypeUnknown, "unknown").WithCode(50003), want: "candidate hint"},
		{name: "500000049 chinese generic wording", problem: errs.NewAPIError(errs.SubtypeUnknown, releaseInternalMessageZH).WithCode(500000049), want: "internal failure"},
		// The shared selector cannot tell reads from writes, so the wording must
		// not presume that a write happened.
		{name: "internal wording does not presume a write", problem: errs.NewAPIError(errs.SubtypeUnknown, releaseInternalMessageEN).WithCode(500000049), want: "if this command changes data", forbid: "previous write"},
		{name: "500000049 other wording stays neutral", problem: errs.NewAPIError(errs.SubtypeUnknown, "internal server error").WithCode(500000049), want: "no specific recovery guidance", forbid: "internal failure"},
		{name: "generic wording on another code stays neutral", problem: errs.NewAPIError(errs.SubtypeUnknown, releaseInternalMessageZH).WithCode(500002776), want: "no specific recovery guidance", forbid: "internal failure"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out := withAppsHint(tc.problem, "candidate hint")
			p, _ := errs.ProblemOf(out)
			if !strings.Contains(strings.ToLower(p.Hint), strings.ToLower(tc.want)) {
				t.Fatalf("hint = %q, want %q", p.Hint, tc.want)
			}
			if tc.forbid != "" && strings.Contains(strings.ToLower(p.Hint), strings.ToLower(tc.forbid)) {
				t.Fatalf("hint = %q, forbids %q", p.Hint, tc.forbid)
			}
		})
	}
}

// TestAppsContextualHints_CommonSignaturesAndPrecedence proves exact matching and shared-hint priority.
func TestAppsContextualHints_CommonSignaturesAndPrecedence(t *testing.T) {
	t.Run("existing hint wins", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeUnknown, "app tenant not found").WithCode(2320008).WithHint("upstream hint")
		p, _ := errs.ProblemOf(withAppsHint(in, "candidate hint"))
		if p.Hint != "upstream hint" {
			t.Fatalf("hint = %q", p.Hint)
		}
	})
	// 2320008 is the gateway's generic business-error code: other gateway
	// rejections (ingress denied, quota) carry it too, so only the app-tenant
	// wording identifies an unavailable app.
	t.Run("other gateway rejection under 2320008 keeps candidate", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeUnknown, "You do not have permission to access this tenant dev.").WithCode(2320008)
		p, _ := errs.ProblemOf(withAppsHint(in, "candidate hint"))
		if p.Hint != "candidate hint" {
			t.Fatalf("hint = %q", p.Hint)
		}
	})
	t.Run("rate limit beats banned marker", func(t *testing.T) {
		in := errs.NewAPIError(errs.SubtypeRateLimit, "[GetApp] app is banned for Miaoda").WithCode(500232002).WithRetryable()
		p, _ := errs.ProblemOf(withAppsHint(in, "candidate hint"))
		if p.Hint != appRateLimitHint || p.Subtype != errs.SubtypeRateLimit {
			t.Fatalf("problem = %#v", p)
		}
	})
	for _, message := range []string{"app is banned", "APP IS BANNED FOR MIAODA.", "[GetApp] app is banned for Miaoda"} {
		t.Run(message, func(t *testing.T) {
			in := errs.NewAPIError(errs.SubtypeUnknown, message).WithCode(500232002)
			p, _ := errs.ProblemOf(withAppsHint(in, "candidate hint"))
			if !strings.Contains(p.Hint, "banned") {
				t.Fatalf("hint = %q", p.Hint)
			}
		})
	}
	for _, message := range []string{"invalid query: 'app is banned'", "app is banned for Miaoda because user input said so", "not app is banned"} {
		t.Run("negative "+message, func(t *testing.T) {
			in := errs.NewAPIError(errs.SubtypeUnknown, message).WithCode(500232002)
			p, _ := errs.ProblemOf(withAppsHint(in, "candidate hint"))
			if strings.Contains(p.Hint, "owner or enterprise administrator") {
				t.Fatalf("false banned match: %q", p.Hint)
			}
		})
	}
	for _, message := range []string{
		dbPasswordAuthMessage,
		dbConnectionSlotsMessage,
		dbGetConnectionMessage,
		"k_dl_1600052: Get DB connection failed.",
		"k_dl_000001：code:k_dl_1600052,msg:Get DB connection failed",
		"k_dl_1300002：ERROR: remaining connection slots are reserved for roles with the SUPERUSER attribute (SQLSTATE XX000)",
		"k_dl_000001：ERROR: sorry, too many clients already (SQLSTATE XX000)",
		"k_dl_000002：ERROR: remaining connection slots are reserved for roles with the SUPERUSER attribute (SQLSTATE XX000)",
		"k_dl_000001：ERROR: HikariPool-pool|app_user/app_db-workspace_x|1a2b3c - Connection is not available, request timed out after 30000ms. (SQLSTATE 08003)",
		"k_dl_000001：ERROR: HikariPool-pool|app_user/app_db-workspace_x|1a2b3c - Connection is not available, request timed out after 30000ms. (SQLSTATE 58000)",
		"k_dl_000001：failed to connect to `user=app_user database=app_db`: [2001:db8::1]:5432 (2001:db8::1): dial error: dial tcp [2001:db8::1]:5432: connect: connection refused",
		"k_dl_000001：failed to connect to `user=app_user database=app_db`: [2001:db8::1]:5432 (2001:db8::1): dial error: timeout: dial tcp [2001:db8::1]:5432: i/o timeout",
		"k_dl_000001：failed to connect to `user=app_user database=app_db`: [2001:db8::1]:5432 (2001:db8::1): server error: ERROR: proxy is draining, please retry later (SQLSTATE 57P03)",
		"k_dl_000001：failed to connect to `user=app_user database=app_db`: [2001:db8::1]:5432 (2001:db8::1): tls error: timeout: context deadline exceeded",
		"k_dl_000001：failed to connect to `user=app_user database=app_db`: [2001:db8::1]:5432 (2001:db8::1): failed SASL auth: timeout: context deadline exceeded",
		"k_dl_000001：failed to connect to `user=app_user database=app_db`: [2001:db8::1]:5432 (2001:db8::1): failed to receive message: timeout: context deadline exceeded",
	} {
		t.Run(message, func(t *testing.T) {
			in := errs.NewAPIError(errs.SubtypeUnknown, message).WithCode(500002776)
			p, _ := errs.ProblemOf(withAppsHint(in, "candidate hint"))
			if !strings.Contains(p.Hint, "database connection is unavailable") {
				t.Fatalf("hint = %q", p.Hint)
			}
		})
	}
	for _, message := range []string{
		"k_dl_1600052：Get DB connection failed for database foo",
		"k_dl_000001：ERROR: password authentication failed",
		"k_dl_000001：ERROR: remaining connection slots are reserved for roles with the SUPERUSER attribute (SQLSTATE 42601)",
		"k_dl_000001：ERROR: syntax error at or near \"connection\" (SQLSTATE 42601)",
		"k_dl_000001：failed to connect to `user=app_user database=app_db`: something else",
		// A connection dropped mid-statement: a write may have run, so this is
		// not "never executed" and must stay neutral.
		"k_dl_000001：ERROR: This connection has been closed. (SQLSTATE 08003)",
		"k_dl_9999999：Get DB connection failed",
		"Get DB connection failed",
	} {
		t.Run("connection boundary "+message, func(t *testing.T) {
			in := errs.NewAPIError(errs.SubtypeUnknown, message).WithCode(500002776)
			p, _ := errs.ProblemOf(withAppsHint(in, "candidate hint"))
			if strings.Contains(p.Hint, "database connection is unavailable") || !strings.Contains(p.Hint, "no specific recovery guidance") {
				t.Fatalf("hint = %q", p.Hint)
			}
		})
	}
}

// TestAppsContextualHints_PreserveTypedFieldsAndCause ensures contextual guidance remains Hint-only.
func TestAppsContextualHints_PreserveTypedFieldsAndCause(t *testing.T) {
	cause := errors.New("wire cause")
	in := errs.NewAPIError(errs.SubtypeUnknown, "app tenant not found").WithCode(2320008).WithCause(cause)
	out := withAppsHint(in, "candidate hint")
	p, _ := errs.ProblemOf(out)
	if p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeUnknown || p.Code != 2320008 || p.Message != "app tenant not found" || p.Retryable {
		t.Fatalf("typed fields changed: %#v", p)
	}
	if !errors.Is(out, cause) {
		t.Fatalf("cause chain dropped: %v", out)
	}
	plain := errors.New("plain")
	if withAppsHint(nil, "candidate") != nil || withAppsHint(plain, "candidate") != plain {
		t.Fatal("nil/plain errors must pass through")
	}
}

// TestAppsContextualHints_NonUnknownServerClassKeepsCandidate protects known server classifications from remapping.
func TestAppsContextualHints_NonUnknownServerClassKeepsCandidate(t *testing.T) {
	in := errs.NewAPIError(errs.SubtypeServerError, "internal server error").WithCode(500000049)
	p, _ := errs.ProblemOf(withAppsHint(in, "candidate hint"))
	if p.Hint != "candidate hint" || p.Subtype != errs.SubtypeServerError {
		t.Fatalf("problem = %#v, want known classification and candidate hint unchanged", p)
	}
}

// TestAppsContextualHints_RepeatedApplicationIsIdempotent ensures a second wrapper pass cannot replace guidance.
func TestAppsContextualHints_RepeatedApplicationIsIdempotent(t *testing.T) {
	in := errs.NewAPIError(errs.SubtypeUnknown, "app tenant not found").WithCode(2320008)
	once := withAppsHint(in, "first candidate")
	p1, _ := errs.ProblemOf(once)
	want := p1.Hint
	twice := withAppsHint(once, "second candidate")
	p2, _ := errs.ProblemOf(twice)
	if p2.Hint != want || p2.Code != 2320008 || p2.Subtype != errs.SubtypeUnknown {
		t.Fatalf("second application changed problem: %#v", p2)
	}
}

// TestAppsContextualHints_AccessScopeUnknownMessagesStopSafely prevents state guesses from unmatched messages.
func TestAppsContextualHints_AccessScopeUnknownMessagesStopSafely(t *testing.T) {
	for _, tc := range []contextualHintCase{
		{name: "unknown 3340002", command: "+access-scope-get", method: "GET", path: "/open-apis/spark/v1/apps/app_x/access-scope", status: 409, code: 3340002, message: "Application has not been published yet. Publishing is disabled for this application type.", args: []string{"+access-scope-get", "--app-id", "app_x", "--as", "user"}, want: []string{"cannot determine", "stop"}, forbid: []string{"not been published", "creative", "release-create"}},
		{name: "partial 3340002", command: "+access-scope-get", method: "GET", path: "/open-apis/spark/v1/apps/app_x/access-scope", status: 409, code: 3340002, message: "Application has not been published yet", args: []string{"+access-scope-get", "--app-id", "app_x", "--as", "user"}, want: []string{"cannot determine", "stop"}, forbid: []string{"release-create"}},
		{name: "unknown 40002", command: "+access-scope-set", method: "PUT", path: "/open-apis/spark/v1/apps/app_x/access-scope", status: 200, code: 40002, message: "different state", args: []string{"+access-scope-set", "--app-id", "app_x", "--scope", "tenant", "--as", "user"}, want: []string{"cannot determine", "stop"}, forbid: []string{"not been published", "creative"}},
	} {
		t.Run(tc.name, func(t *testing.T) { runContextualHintCase(t, tc) })
	}
}

// TestAppsContextualHints_EnvMessageBoundaries distinguishes supported size signatures from unrelated validation text.
func TestAppsContextualHints_EnvMessageBoundaries(t *testing.T) {
	for _, tc := range []struct{ message, want, forbid string }{
		{expertModeOnlyMessageZH, "expert mode", "apps +list"},
		{expertModeOnlyMessageEN, "expert mode", "apps +list"},
		{"env var operation is only supported for expert mode apps.", "expert mode", "apps +list"},
		{"请求参数不合法：env var operation is only supported for expert mode apps; see the console", "apps +list", "expert mode"},
		{envValueTooLargeMessage, "shorten", "apps +list"},
		{"请求参数不合法：value size 5000 is greater than max size 4096", "shorten", "apps +list"},
		{"value size 5000 is greater than max size 4096", "shorten", "apps +list"},
		{"Invalid Request: value size 999999999999999999999999 is greater than max size 4096", "shorten", "apps +list"},
		{"Invalid Request: value size 4096 is greater than max size 4096", "apps +list", "shorten"},
		{"Invalid Request: value size -1 is greater than max size 4096", "apps +list", "shorten"},
		{"Invalid Request: value size nope is greater than max size 4096", "apps +list", "shorten"},
		{"Invalid Request: value size 5000 is greater than max size 8192", "apps +list", "shorten"},
		{"Invalid Request: value size 9000 is greater than max size 8192", "8192 bytes", "4096"},
		{"Invalid Request: value size 5000 is greater than max size 0", "apps +list", "shorten"},
		{"Bad Request: value size 5000 is greater than max size 4096", "apps +list", "shorten"},
		{"Invalid Request: source.base_url is required", "apps +list", "shorten"},
	} {
		t.Run(tc.message, func(t *testing.T) {
			p := errs.NewAPIError(errs.SubtypeUnknown, tc.message).WithCode(400000072)
			hint := envVarMutationHint(p)
			if !strings.Contains(hint, tc.want) || (tc.forbid != "" && strings.Contains(hint, tc.forbid)) {
				t.Fatalf("hint = %q", hint)
			}
		})
	}
}

// TestAppsContextualHints_DeleteUnsupportedTypeRegression preserves delete-specific 40002 and prefixed-code handling.
func TestAppsContextualHints_DeleteUnsupportedTypeRegression(t *testing.T) {
	for _, code := range []int{40002, 3340002} {
		in := errs.NewAPIError(errs.SubtypeUnknown, "unsupported app type").WithCode(code)
		p, _ := errs.ProblemOf(annotateAppLookupErr(in))
		if p.Subtype != errs.SubtypeFailedPrecondition || p.Hint != unsupportedAppTypeHint || p.Code != code {
			t.Fatalf("code %d: %#v", code, p)
		}
	}
}
