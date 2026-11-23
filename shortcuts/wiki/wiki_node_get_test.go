// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/spf13/cobra"
)

const (
	testWikiNodeToken = "Abcdw_EXAMPLE_WIKI_TOKEN_27"
	testDocxObjToken  = "Abcdd_EXAMPLE_DOCX_TOKEN_27"
	testBaseObjToken  = "Abcdb_EXAMPLE_BASE_TOKEN_27"
	testSheetObjToken = "Abcds_EXAMPLE_SHT_TOKEN_027"
)

func TestParseWikiNodeGetSpecRawNodeToken(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeGetSpec(testWikiNodeToken, "")
	if err != nil {
		t.Fatalf("parseWikiNodeGetSpec() error = %v", err)
	}
	if spec.Token != testWikiNodeToken {
		t.Fatalf("spec = %+v, want token %s", spec, testWikiNodeToken)
	}
	if got := spec.RequestParams(); !reflect.DeepEqual(got, map[string]interface{}{"token": testWikiNodeToken}) {
		t.Fatalf("RequestParams() = %v, want {token: %s}", got, testWikiNodeToken)
	}
}

func TestParseWikiNodeGetSpecOpaqueRawNodeToken(t *testing.T) {
	t.Parallel()

	// Opaque tokens must not require a known resource-type prefix.
	const opaqueNodeToken = "Sm78_EXAMPLE_OPAQUE_TOKEN_X"
	spec, err := parseWikiNodeGetSpec(opaqueNodeToken, "")
	if err != nil {
		t.Fatalf("parseWikiNodeGetSpec() error = %v", err)
	}
	if spec.Token != opaqueNodeToken {
		t.Fatalf("spec = %+v, want token %s", spec, opaqueNodeToken)
	}
	if got := spec.RequestParams(); !reflect.DeepEqual(got, map[string]interface{}{"token": opaqueNodeToken}) {
		t.Fatalf("RequestParams() = %v, want {token: %s}", got, opaqueNodeToken)
	}
}

func TestParseWikiNodeGetSpecRawObjTokenWithoutObjType(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeGetSpec(testBaseObjToken, "")
	if err != nil {
		t.Fatalf("parseWikiNodeGetSpec() error = %v", err)
	}
	if spec.Token != testBaseObjToken {
		t.Fatalf("spec = %+v, want token %s", spec, testBaseObjToken)
	}
}

func TestParseWikiNodeGetSpecExtractsTokenFromWikiURL(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeGetSpec("https://feishu.cn/wiki/"+testWikiNodeToken+"?foo=bar", "")
	if err != nil {
		t.Fatalf("parseWikiNodeGetSpec() error = %v", err)
	}
	if spec.Token != testWikiNodeToken {
		t.Fatalf("spec = %+v, want url-wiki %s", spec, testWikiNodeToken)
	}
}

func TestParseWikiNodeGetSpecExtractsTokenFromDocxURL(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeGetSpec("https://feishu.cn/docx/"+testDocxObjToken, "")
	if err != nil {
		t.Fatalf("parseWikiNodeGetSpec() error = %v", err)
	}
	if spec.Token != testDocxObjToken {
		t.Fatalf("spec = %+v, want url-obj %s", spec, testDocxObjToken)
	}
}

func TestParseWikiNodeGetSpecRejectsUnsupportedURLPath(t *testing.T) {
	t.Parallel()

	_, err := parseWikiNodeGetSpec("https://feishu.cn/im/chat/oc_123", "")
	if err == nil || !strings.Contains(err.Error(), "unsupported --node-token URL path") {
		t.Fatalf("expected unsupported URL path error, got %v", err)
	}
}

func TestParseWikiNodeGetSpecRejectsPartialPath(t *testing.T) {
	t.Parallel()

	_, err := parseWikiNodeGetSpec("/wiki/wikcnABC", "")
	if err == nil || !strings.Contains(err.Error(), "partial paths are not accepted") {
		t.Fatalf("expected partial-path rejection, got %v", err)
	}
}

func TestParseWikiNodeGetSpecRejectsEmptyToken(t *testing.T) {
	t.Parallel()

	if _, err := parseWikiNodeGetSpec("   ", ""); err == nil || !strings.Contains(err.Error(), "--node-token is required") {
		t.Fatalf("expected required-token error, got %v", err)
	}
}

func TestParseWikiNodeGetSpecLeavesTokenLengthToServer(t *testing.T) {
	t.Parallel()

	for _, length := range []int{1, 21, 22, 26, 27, 128} {
		token := strings.Repeat("a", length)
		for _, input := range []string{token, "https://feishu.cn/wiki/" + token} {
			spec, err := parseWikiNodeGetSpec(input, "")
			if err != nil {
				t.Fatalf("parse(%q): %v", input, err)
			}
			if spec.Token != token {
				t.Fatalf("token = %q, want %q", spec.Token, token)
			}
		}
	}
}

func TestWikiNodeGetShortTokenReachesAPI(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	factory, stdout, stderr, reg := cmdutil.TestFactory(t, wikiTestConfig())
	requested := false
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/node_by_token",
		Status: http.StatusOK,
		Body:   map[string]interface{}{"code": 131016, "msg": "invalid token length"},
		OnMatch: func(req *http.Request) {
			requested = true
			if got := req.URL.Query(); len(got) != 1 || got.Get("token") != "short" {
				t.Errorf("query = %v, want only token=short", got)
			}
		},
	})
	parent := mountWikiNodeGetWithFlagOut(t, factory, stderr)
	parent.SetArgs([]string{
		"+node-get", "--node-token", "short", "--obj-type=unknown", "--as", "bot",
	})
	err := parent.Execute()
	if !requested {
		t.Fatal("short token did not reach the API")
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeInvalidParameters || p.Code != 131016 || p.Retryable {
		t.Fatalf("problem = %#v, want non-retryable api/invalid_parameters/131016", p)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want no success output", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no warning before the error envelope", stderr.String())
	}
}

func TestResolveWikiNodeGetRawTokenPrefersNodeToken(t *testing.T) {
	t.Parallel()

	got, err := resolveWikiNodeGetRawToken("wikcnNEW", "")
	if err != nil || got != "wikcnNEW" {
		t.Fatalf("resolve(node-token only) = (%q, %v), want (wikcnNEW, nil)", got, err)
	}
}

func TestResolveWikiNodeGetRawTokenAcceptsLegacyToken(t *testing.T) {
	t.Parallel()

	got, err := resolveWikiNodeGetRawToken("", "wikcnLEGACY")
	if err != nil || got != "wikcnLEGACY" {
		t.Fatalf("resolve(legacy only) = (%q, %v), want (wikcnLEGACY, nil)", got, err)
	}
}

func TestResolveWikiNodeGetRawTokenAcceptsBothWhenEqual(t *testing.T) {
	t.Parallel()

	// Same value on both flags is harmless (e.g. a script doubled the input
	// while migrating to --node-token) — prefer the canonical one and don't
	// surface a conflict error.
	got, err := resolveWikiNodeGetRawToken("wikcnSAME", "wikcnSAME")
	if err != nil || got != "wikcnSAME" {
		t.Fatalf("resolve(both same) = (%q, %v), want (wikcnSAME, nil)", got, err)
	}
}

func TestResolveWikiNodeGetRawTokenRejectsConflict(t *testing.T) {
	t.Parallel()

	_, err := resolveWikiNodeGetRawToken("wikcnNEW", "wikcnOLD")
	if err == nil || !strings.Contains(err.Error(), "both set with different values") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestResolveWikiNodeGetRawTokenEmptyDefersToParser(t *testing.T) {
	t.Parallel()

	// Both empty is not an error here — the caller (parseWikiNodeGetSpec) is
	// where the required-flag check lives and produces the user-facing message.
	got, err := resolveWikiNodeGetRawToken("", "")
	if err != nil || got != "" {
		t.Fatalf("resolve(empty) = (%q, %v), want ('', nil)", got, err)
	}
}

func TestBuildWikiNodeGetDryRunSendsOnlyToken(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeGetSpec("https://feishu.cn/docx/"+testDocxObjToken, "")
	if err != nil {
		t.Fatalf("parseWikiNodeGetSpec() error = %v", err)
	}

	dry := buildWikiNodeGetDryRun(spec)
	data, err := json.Marshal(dry)
	if err != nil {
		t.Fatalf("marshal dry run: %v", err)
	}
	var got struct {
		API []struct {
			Method string                 `json:"method"`
			URL    string                 `json:"url"`
			Params map[string]interface{} `json:"params"`
		} `json:"api"`
	}
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal dry run: %v", err)
	}
	if len(got.API) != 1 || got.API[0].URL != "/open-apis/wiki/v2/spaces/node_by_token" {
		t.Fatalf("dry-run api = %#v, want single node_by_token call", got.API)
	}
	if got.API[0].Params["token"] != testDocxObjToken || len(got.API[0].Params) != 1 {
		t.Fatalf("dry-run params = %#v", got.API[0].Params)
	}
}

func TestFormatWikiTimestamp(t *testing.T) {
	t.Parallel()

	if got := formatWikiTimestamp(""); got != "" {
		t.Fatalf("formatWikiTimestamp(empty) = %q, want empty", got)
	}
	if got := formatWikiTimestamp("not-a-number"); got != "" {
		t.Fatalf("formatWikiTimestamp(non-numeric) = %q, want empty", got)
	}
	// Output is UTC, so it is deterministic regardless of host timezone.
	if got := formatWikiTimestamp("1700000000"); got != "2023-11-14T22:13:20Z" {
		t.Fatalf("formatWikiTimestamp(1700000000) = %q, want 2023-11-14T22:13:20Z (UTC)", got)
	}
}

func TestWikiNodeGetSilentlyIgnoresLegacyObjectType(t *testing.T) {
	for _, tt := range []struct {
		name       string
		input      string
		token      string
		legacyArgs []string
		actual     string
	}{
		{name: "raw obj_token needs no type", input: testDocxObjToken, token: testDocxObjToken, actual: "docx"},
		{name: "legacy matching type", input: testWikiNodeToken, token: testWikiNodeToken, legacyArgs: []string{"--obj-type", "docx"}, actual: "docx"},
		{name: "legacy mismatching type", input: testWikiNodeToken, token: testWikiNodeToken, legacyArgs: []string{"--obj-type", "sheet"}, actual: "docx"},
		{name: "unknown type", input: testWikiNodeToken, token: testWikiNodeToken, legacyArgs: []string{"--obj-type=unknown"}, actual: "docx"},
		{name: "empty type", input: testWikiNodeToken, token: testWikiNodeToken, legacyArgs: []string{"--obj-type="}, actual: "docx"},
		{name: "URL does not assert returned type", input: "https://feishu.cn/sheets/" + testSheetObjToken, token: testSheetObjToken, actual: "docx"},
		{name: "legacy type contradicts URL", input: "https://feishu.cn/sheets/" + testSheetObjToken, token: testSheetObjToken, legacyArgs: []string{"--obj-type", "docx"}, actual: "docx"},
		{name: "missing returned type", input: testWikiNodeToken, token: testWikiNodeToken, legacyArgs: []string{"--obj-type", "docx"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
			factory, stdout, stderr, reg := cmdutil.TestFactory(t, wikiTestConfig())
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/wiki/v2/spaces/node_by_token",
				Body: map[string]interface{}{
					"code": 0,
					"data": map[string]interface{}{"node": map[string]interface{}{
						"node_token": testWikiNodeToken, "obj_token": testDocxObjToken,
						"obj_type": tt.actual, "space_id": "space_123",
					}},
				},
				OnMatch: func(req *http.Request) {
					query := req.URL.Query()
					if len(query) != 1 || query.Get("token") != tt.token {
						t.Errorf("query = %v, want only token=%s", query, tt.token)
					}
				},
			})
			args := []string{"+node-get", "--node-token", tt.input, "--space-id", "space_123", "--as", "bot"}
			args = append(args, tt.legacyArgs...)
			parent := mountWikiNodeGetWithFlagOut(t, factory, stderr)
			parent.SetArgs(args)
			err := parent.Execute()
			if err != nil {
				t.Fatal(err)
			}
			if got := decodeWikiEnvelope(t, stdout)["obj_type"]; got != tt.actual {
				t.Fatalf("obj_type = %v, want server value %q", got, tt.actual)
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want silent compatibility", stderr.String())
			}
		})
	}
	if !reflect.DeepEqual(WikiNodeGet.Scopes, []string{"wiki:node:retrieve"}) {
		t.Fatalf("scopes changed: %v", WikiNodeGet.Scopes)
	}
}

func TestWikiNodeGetMountedExecuteParsesURLAndFormatsOutput(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, stderr, reg := cmdutil.TestFactory(t, wikiTestConfig())

	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/node_by_token",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":          "space_123",
					"node_token":        "wikcnABC",
					"obj_token":         "docxXYZ",
					"obj_type":          "docx",
					"parent_node_token": "wikcnPARENT",
					"node_type":         "origin",
					"title":             "Design Spec",
					"has_child":         true,
					"url":               "https://example.com/wiki/node",
					"creator":           "ou_document_creator",
					"origin_node_token": "wikcnORIGIN",
					"node_creator":      "ou_creator",
					"owner":             "ou_owner",
					"obj_edit_time":     "1700000000",
					"obj_create_time":   "1690000000",
					"node_create_time":  "1690000001",
				},
			},
			"msg": "success",
		},
	}
	var capturedQuery string
	stub.OnMatch = func(req *http.Request) {
		capturedQuery = req.URL.RawQuery
	}
	reg.Register(stub)

	err := mountAndRunWiki(t, WikiNodeGet, []string{
		"+node-get",
		"--node-token", "https://feishu.cn/docx/" + testDocxObjToken,
		"--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	if capturedQuery != "token="+testDocxObjToken {
		t.Fatalf("captured query = %q, want only token=%s", capturedQuery, testDocxObjToken)
	}

	data := decodeWikiEnvelope(t, stdout)
	want := map[string]interface{}{
		"space_id": "space_123", "node_token": "wikcnABC", "obj_token": "docxXYZ",
		"obj_type": "docx", "node_type": "origin", "parent_node_token": "wikcnPARENT",
		"origin_node_token": "wikcnORIGIN", "title": "Design Spec", "has_child": true,
		"creator": "ou_creator", "owner": "ou_owner", "obj_edit_time": "1700000000",
		"obj_create_time": "1690000000", "node_create_time": "1690000001", "updated_at": "2023-11-14T22:13:20Z",
	}
	if !reflect.DeepEqual(data, want) {
		t.Fatalf("output = %#v, want %#v", data, want)
	}
	if data["title"] != "Design Spec" {
		t.Fatalf("title = %#v, want Design Spec", data["title"])
	}
	if data["obj_type"] != "docx" || data["obj_token"] != "docxXYZ" {
		t.Fatalf("obj_type/obj_token = %#v / %#v", data["obj_type"], data["obj_token"])
	}
	if data["parent_node_token"] != "wikcnPARENT" {
		t.Fatalf("parent_node_token = %#v", data["parent_node_token"])
	}
	if data["creator"] != "ou_creator" {
		t.Fatalf("creator = %#v, want ou_creator", data["creator"])
	}
	if data["owner"] != "ou_owner" {
		t.Fatalf("owner = %#v, want ou_owner", data["owner"])
	}
	if got, _ := data["updated_at"].(string); got != "2023-11-14T22:13:20Z" {
		t.Fatalf("updated_at = %#v, want 2023-11-14T22:13:20Z (UTC)", data["updated_at"])
	}
	// Preserve the established output even when the API supplies a URL.
	if _, ok := data["url"]; ok {
		t.Fatalf("did not expect a url field in +node-get output, got %#v", data["url"])
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no progress output", stderr.String())
	}
}

func TestWikiNodeGetMountedClassifiesTerminalBusinessErrors(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		message  string
		subtype  errs.Subtype
		hintText string
	}{
		{
			name: "missing node", code: 131005, message: "node not found",
			subtype: errs.SubtypeNotFound,
		},
		{
			name: "short token", code: 131016, message: "invalid token length",
			subtype: errs.SubtypeInvalidParameters, hintText: "Do not retry the same token",
		},
		{
			name:     "deleted node",
			code:     131012,
			message:  "node has been deleted",
			subtype:  errs.SubtypeNotFound,
			hintText: "Do not retry the same node token",
		},
		{
			name:     "invalid resource token",
			code:     131013,
			message:  "token is invalid",
			subtype:  errs.SubtypeInvalidParameters,
			hintText: "Do not retry the same token",
		},
		{
			name:     "document not in wiki",
			code:     131014,
			message:  "document is not in wiki",
			subtype:  errs.SubtypeFailedPrecondition,
			hintText: "Do not retry wiki +node-get with the same document",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

			factory, stdout, stderr, reg := cmdutil.TestFactory(t, wikiTestConfig())
			reg.Register(&httpmock.Stub{
				Method: "GET",
				Status: http.StatusOK,
				URL:    "/open-apis/wiki/v2/spaces/node_by_token",
				Body: map[string]interface{}{
					"code":   tt.code,
					"msg":    tt.message,
					"log_id": "log-node-get-terminal",
				},
			})

			err := mountAndRunWiki(t, WikiNodeGet, []string{
				"+node-get",
				"--node-token", testWikiNodeToken,
				"--as", "bot",
			}, factory, stdout)
			if err == nil {
				t.Fatal("expected a terminal business error")
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected typed error, got %T: %v", err, err)
			}
			if p.Category != errs.CategoryAPI || p.Code != tt.code || p.Subtype != tt.subtype {
				t.Fatalf("problem category/code/subtype = %s/%d/%s, want %s/%d/%s",
					p.Category, p.Code, p.Subtype, errs.CategoryAPI, tt.code, tt.subtype)
			}
			if p.Retryable {
				t.Fatalf("problem retryable = true, want false: %#v", p)
			}
			if !strings.Contains(p.Hint, tt.hintText) {
				t.Fatalf("hint = %q, want %q", p.Hint, tt.hintText)
			}
			if tt.code == 131013 || tt.code == 131016 {
				if !strings.Contains(p.Hint, "complete raw obj_token") {
					t.Fatalf("hint = %q, want recovery guidance for a complete raw obj_token", p.Hint)
				}
			}
			if p.LogID != "log-node-get-terminal" {
				t.Fatalf("log_id = %q, want log-node-get-terminal", p.LogID)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want no success envelope", stdout.String())
			}
			if stderr.Len() != 0 {
				t.Fatalf("stderr = %q, want no output before the root error envelope", stderr.String())
			}
		})
	}
}

func TestWikiNodeGetMountedExplainsResourcePermissionDenied(t *testing.T) {
	for _, identity := range []string{"user", "bot"} {
		t.Run(identity, func(t *testing.T) {
			t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

			factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/wiki/v2/spaces/node_by_token",
				Body: map[string]interface{}{
					"code":   131006,
					"msg":    "permission denied: node permission denied, user needs read permission.",
					"log_id": "log-node-get-permission",
				},
			})

			err := mountAndRunWiki(t, WikiNodeGet, []string{
				"+node-get",
				"--node-token", testWikiNodeToken,
				"--as", identity,
			}, factory, stdout)
			if err == nil {
				t.Fatal("expected permission error")
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected typed error, got %T: %v", err, err)
			}
			if p.Category != errs.CategoryAuthorization || p.Subtype != errs.SubtypePermissionDenied || p.Code != 131006 {
				t.Fatalf("problem = %#v, want authorization/permission_denied/131006", p)
			}
			if p.Retryable {
				t.Fatalf("problem retryable = true, want false: %#v", p)
			}
			if !strings.Contains(p.Hint, "resource access, not app scope authorization") || !strings.Contains(p.Hint, "Do not retry the same request") {
				t.Fatalf("hint = %q, want non-retryable resource-access guidance", p.Hint)
			}
		})
	}
}

func TestWikiNodeGetProblemBoundsRateLimitRetries(t *testing.T) {
	t.Parallel()

	cause := errors.New("opaque upstream cause")
	const upstreamHint = "upstream pacing hint"
	err := errs.NewAPIError(errs.SubtypeRateLimit, "opaque upstream message").
		WithCode(99991400).
		WithRetryable().
		WithRetryAfterSeconds(8).
		WithHint(upstreamHint).
		WithCause(cause)

	got := wikiNodeGetProblem(err)
	p, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("ProblemOf() ok=false")
	}
	if p.Category != errs.CategoryAPI || p.Subtype != errs.SubtypeRateLimit || p.Code != 99991400 || !p.Retryable {
		t.Fatalf("problem = %#v, want retryable api/rate_limit/99991400", p)
	}
	var apiErr *errs.APIError
	if !errors.As(got, &apiErr) {
		t.Fatalf("error = %T, want *errs.APIError", got)
	}
	if apiErr.RetryAfterSeconds != 8 {
		t.Fatalf("retry_after_seconds = %d, want 8", apiErr.RetryAfterSeconds)
	}
	wantHint := upstreamHint + "\n" + wikiNodeGetRateLimitHint
	if p.Hint != wantHint {
		t.Fatalf("hint = %q, want %q", p.Hint, wantHint)
	}
	if !errors.Is(got, cause) {
		t.Fatalf("error does not preserve cause %v: %v", cause, got)
	}
}

func TestWikiNodeGetProblemPreservesPermissionErrorContract(t *testing.T) {
	t.Parallel()

	cause := errors.New("opaque upstream cause")
	err := errs.NewPermissionError(errs.SubtypePermissionDenied, "opaque upstream message").
		WithCode(131006).
		WithCause(cause)

	got := wikiNodeGetProblem(err)
	p, ok := errs.ProblemOf(got)
	if !ok {
		t.Fatalf("ProblemOf() ok=false")
	}
	if p.Category != errs.CategoryAuthorization || p.Subtype != errs.SubtypePermissionDenied || p.Code != 131006 {
		t.Fatalf("problem = %#v, want authorization/permission_denied/131006", p)
	}
	if p.Retryable {
		t.Fatalf("problem retryable = true, want false: %#v", p)
	}
	if !errors.Is(got, cause) {
		t.Fatalf("errors.Is(got, cause) = false, want preserved cause")
	}
	if p.Hint != wikiPermissionDeniedHint() {
		t.Fatalf("hint = %q, want %q", p.Hint, wikiPermissionDeniedHint())
	}
}

func TestWikiNodeGetMountedAcceptsNodeTokenFlag(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/node_by_token",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":   "space_123",
					"node_token": "wikcnABC",
					"obj_token":  "docxXYZ",
					"obj_type":   "docx",
					"node_type":  "origin",
					"title":      "Via Node-Token",
				},
			},
			"msg": "success",
		},
	}
	var capturedQuery string
	stub.OnMatch = func(req *http.Request) {
		capturedQuery = req.URL.RawQuery
	}
	reg.Register(stub)

	// Mount inline (rather than using mountAndRunWiki) so we can redirect the
	// subcommand's pflag output and assert that no deprecation warning leaks
	// when the canonical --node-token is used. The deprecation message comes
	// from pflag, not cobra, so SetErr on the cobra root is NOT enough — pflag
	// writes to FlagSet.Output(), which we redirect via Flags().SetOutput.
	var flagOut bytes.Buffer
	parent := mountWikiNodeGetWithFlagOut(t, factory, &flagOut)
	parent.SetArgs([]string{
		"+node-get",
		"--node-token", "https://feishu.cn/docx/" + testDocxObjToken,
		"--as", "bot",
	})
	stdout.Reset()
	if err := parent.Execute(); err != nil {
		t.Fatalf("parent.Execute() error = %v", err)
	}

	if capturedQuery != "token="+testDocxObjToken {
		t.Fatalf("captured query = %q, want only token=%s", capturedQuery, testDocxObjToken)
	}

	data := decodeWikiEnvelope(t, stdout)
	if data["title"] != "Via Node-Token" {
		t.Fatalf("title = %#v, want Via Node-Token", data["title"])
	}
	if got := flagOut.String(); strings.Contains(got, "deprecated") {
		t.Fatalf("pflag output unexpectedly contains deprecation warning when using --node-token: %q", got)
	}
}

// mountWikiNodeGetWithFlagOut mounts +node-get on a fresh parent and redirects
// the subcommand's pflag output to w so tests can capture cobra/pflag-level
// deprecation messages (which bypass the runtime IO stderr exposed by
// TestFactory).
func mountWikiNodeGetWithFlagOut(t *testing.T, factory *cmdutil.Factory, w *bytes.Buffer) *cobra.Command {
	t.Helper()
	parent := &cobra.Command{Use: "wiki"}
	WikiNodeGet.Mount(parent, factory)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	parent.SetErr(w)
	for _, child := range parent.Commands() {
		if child.Use == WikiNodeGet.Command {
			child.Flags().SetOutput(w)
			return parent
		}
	}
	t.Fatalf("mountWikiNodeGetWithFlagOut: subcommand %q not registered on parent", WikiNodeGet.Command)
	return nil
}

func TestWikiNodeGetMountedLegacyTokenFlagWarnsButWorks(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/node_by_token",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":   "space_123",
					"node_token": "wikcnABC",
					"obj_token":  "docxXYZ",
					"obj_type":   "docx",
					"node_type":  "origin",
					"title":      "Legacy Token Path",
				},
			},
			"msg": "success",
		},
	})

	var flagOut bytes.Buffer
	parent := mountWikiNodeGetWithFlagOut(t, factory, &flagOut)
	parent.SetArgs([]string{
		"+node-get",
		"--token", testWikiNodeToken,
		"--as", "bot",
	})
	stdout.Reset()
	if err := parent.Execute(); err != nil {
		t.Fatalf("parent.Execute() error = %v", err)
	}

	data := decodeWikiEnvelope(t, stdout)
	if data["title"] != "Legacy Token Path" {
		t.Fatalf("title = %#v, want Legacy Token Path", data["title"])
	}
	// pflag MarkDeprecated prints "Flag --token has been deprecated, use --node-token instead".
	got := flagOut.String()
	if !strings.Contains(got, "deprecated") || !strings.Contains(got, "--node-token") {
		t.Fatalf("pflag output = %q, want a deprecation warning pointing to --node-token", got)
	}
}

func TestWikiNodeGetMountedRejectsConflictingTokenFlags(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	// reg is unused: conflict is caught in Validate before any HTTP call.
	factory, stdout, _, _ := cmdutil.TestFactory(t, wikiTestConfig())

	err := mountAndRunWiki(t, WikiNodeGet, []string{
		"+node-get",
		"--node-token", "wikcnNEW",
		"--token", "wikcnOLD",
		"--as", "bot",
	}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "both set with different values") {
		t.Fatalf("expected conflict error, got %v", err)
	}
}

func TestWikiNodeGetFallsBackToCreatorWhenNodeCreatorMissing(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/node_by_token",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":   "space_123",
					"node_token": "wikcnABC",
					"obj_token":  "docxXYZ",
					"obj_type":   "docx",
					"node_type":  "origin",
					"title":      "Fallback Creator",
					"creator":    "ou_legacy_creator",
				},
			},
			"msg": "success",
		},
	})

	err := mountAndRunWiki(t, WikiNodeGet, []string{
		"+node-get",
		"--node-token", testWikiNodeToken,
		"--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	data := decodeWikiEnvelope(t, stdout)
	if data["creator"] != "ou_legacy_creator" {
		t.Fatalf("creator = %#v, want fallback to creator field", data["creator"])
	}
}

func TestWikiNodeGetRejectsSpaceIDMismatch(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/node_by_token",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"node": map[string]interface{}{
					"space_id":   "space_actual",
					"node_token": "wikcnABC",
					"obj_token":  "docxXYZ",
					"obj_type":   "docx",
					"node_type":  "origin",
					"title":      "Mismatch",
				},
			},
			"msg": "success",
		},
	})

	err := mountAndRunWiki(t, WikiNodeGet, []string{
		"+node-get",
		"--node-token", testWikiNodeToken,
		"--space-id", "space_expected",
		"--as", "bot",
	}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "does not match the resolved node space") {
		t.Fatalf("expected space mismatch error, got %v", err)
	}
}
