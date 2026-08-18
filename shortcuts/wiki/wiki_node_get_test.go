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

	spec, err := parseWikiNodeGetSpec(testWikiNodeToken, "", "")
	if err != nil {
		t.Fatalf("parseWikiNodeGetSpec() error = %v", err)
	}
	if spec.Token != testWikiNodeToken || spec.ObjType != "" || spec.SourceKind != "raw-node" {
		t.Fatalf("spec = %+v, want raw-node %s with no obj_type", spec, testWikiNodeToken)
	}
	if got := spec.RequestParams(); !reflect.DeepEqual(got, map[string]interface{}{"token": testWikiNodeToken}) {
		t.Fatalf("RequestParams() = %v, want {token: %s}", got, testWikiNodeToken)
	}
}

func TestParseWikiNodeGetSpecOpaqueRawNodeToken(t *testing.T) {
	t.Parallel()

	// Opaque tokens must not require a known resource-type prefix. Use a
	// complete-length fixture so this test remains independent of the
	// truncated-token validation contract.
	const opaqueNodeToken = "Sm78_EXAMPLE_OPAQUE_TOKEN_X"
	if len(opaqueNodeToken) != minWikiResourceTokenLength {
		t.Fatalf("opaqueNodeToken length = %d, want %d", len(opaqueNodeToken), minWikiResourceTokenLength)
	}
	spec, err := parseWikiNodeGetSpec(opaqueNodeToken, "", "")
	if err != nil {
		t.Fatalf("parseWikiNodeGetSpec() error = %v", err)
	}
	if spec.Token != opaqueNodeToken || spec.ObjType != "" || spec.SourceKind != "raw-node" {
		t.Fatalf("spec = %+v, want raw-node %s with no obj_type", spec, opaqueNodeToken)
	}
	if got := spec.RequestParams(); !reflect.DeepEqual(got, map[string]interface{}{"token": opaqueNodeToken}) {
		t.Fatalf("RequestParams() = %v, want {token: %s}", got, opaqueNodeToken)
	}
}

func TestParseWikiNodeGetSpecRawObjTokenWithExplicitObjType(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeGetSpec(testDocxObjToken, "docx", "")
	if err != nil {
		t.Fatalf("parseWikiNodeGetSpec() error = %v", err)
	}
	if spec.Token != testDocxObjToken || spec.ObjType != "docx" || spec.SourceKind != "raw-obj" {
		t.Fatalf("spec = %+v, want raw-obj %s obj_type=docx", spec, testDocxObjToken)
	}
}

func TestParseWikiNodeGetSpecRawTokenWithoutObjTypeDefaultsToNodeToken(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeGetSpec(testBaseObjToken, "", "")
	if err != nil {
		t.Fatalf("parseWikiNodeGetSpec() error = %v", err)
	}
	if spec.Token != testBaseObjToken || spec.ObjType != "" || spec.SourceKind != "raw-node" {
		t.Fatalf("spec = %+v, want raw-node %s with no obj_type", spec, testBaseObjToken)
	}
}

func TestParseWikiNodeGetSpecRawTokenWithObjTypeUsesObjTokenLookup(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeGetSpec(testWikiNodeToken, "docx", "")
	if err != nil {
		t.Fatalf("parseWikiNodeGetSpec() error = %v", err)
	}
	if spec.Token != testWikiNodeToken || spec.ObjType != "docx" || spec.SourceKind != "raw-obj" {
		t.Fatalf("spec = %+v, want raw-obj %s with obj_type docx", spec, testWikiNodeToken)
	}
	if got := spec.RequestParams(); !reflect.DeepEqual(got, map[string]interface{}{"token": testWikiNodeToken, "obj_type": "docx"}) {
		t.Fatalf("RequestParams() = %v, want {token: %s, obj_type: docx}", got, testWikiNodeToken)
	}
}

func TestParseWikiNodeGetSpecExtractsTokenFromWikiURL(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeGetSpec("https://feishu.cn/wiki/"+testWikiNodeToken+"?foo=bar", "", "")
	if err != nil {
		t.Fatalf("parseWikiNodeGetSpec() error = %v", err)
	}
	if spec.Token != testWikiNodeToken || spec.ObjType != "" || spec.SourceKind != "url-wiki" {
		t.Fatalf("spec = %+v, want url-wiki %s", spec, testWikiNodeToken)
	}
}

func TestParseWikiNodeGetSpecExtractsTokenAndObjTypeFromDocxURL(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeGetSpec("https://feishu.cn/docx/"+testDocxObjToken, "", "")
	if err != nil {
		t.Fatalf("parseWikiNodeGetSpec() error = %v", err)
	}
	if spec.Token != testDocxObjToken || spec.ObjType != "docx" || spec.SourceKind != "url-obj" {
		t.Fatalf("spec = %+v, want url-obj %s", spec, testDocxObjToken)
	}
}

func TestParseWikiNodeGetSpecRejectsURLObjTypeMismatch(t *testing.T) {
	t.Parallel()

	_, err := parseWikiNodeGetSpec("https://feishu.cn/sheets/"+testSheetObjToken, "docx", "")
	if err == nil || !strings.Contains(err.Error(), "does not match the obj_type") {
		t.Fatalf("expected URL/obj-type mismatch error, got %v", err)
	}
}

func TestParseWikiNodeGetSpecRejectsUnsupportedURLPath(t *testing.T) {
	t.Parallel()

	_, err := parseWikiNodeGetSpec("https://feishu.cn/im/chat/oc_123", "", "")
	if err == nil || !strings.Contains(err.Error(), "unsupported --node-token URL path") {
		t.Fatalf("expected unsupported URL path error, got %v", err)
	}
}

func TestParseWikiNodeGetSpecRejectsPartialPath(t *testing.T) {
	t.Parallel()

	_, err := parseWikiNodeGetSpec("/wiki/wikcnABC", "", "")
	if err == nil || !strings.Contains(err.Error(), "partial paths are not accepted") {
		t.Fatalf("expected partial-path rejection, got %v", err)
	}
}

func TestParseWikiNodeGetSpecRejectsEmptyToken(t *testing.T) {
	t.Parallel()

	if _, err := parseWikiNodeGetSpec("   ", "", ""); err == nil || !strings.Contains(err.Error(), "--node-token is required") {
		t.Fatalf("expected required-token error, got %v", err)
	}
}

func TestParseWikiNodeGetSpecRejectsTruncatedToken(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		"PImXw",
		"https://feishu.cn/wiki/PImXw",
	} {
		t.Run(input, func(t *testing.T) {
			t.Parallel()

			_, err := parseWikiNodeGetSpec(input, "", "")
			if err == nil {
				t.Fatal("expected truncated token validation error")
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("ProblemOf() ok=false for %T: %v", err, err)
			}
			var validationErr *errs.ValidationError
			if !errors.As(err, &validationErr) {
				t.Fatalf("expected ValidationError, got %T: %v", err, err)
			}
			if p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeInvalidArgument || validationErr.Param != "--node-token" {
				t.Fatalf("problem = %#v param=%q, want validation/invalid_argument/--node-token", p, validationErr.Param)
			}
			if !strings.Contains(p.Hint, "complete token") || !strings.Contains(p.Hint, "full Lark URL") {
				t.Fatalf("hint = %q, want actionable complete-token guidance", p.Hint)
			}
		})
	}
}

func TestValidateWikiResourceTokenLengthBoundary(t *testing.T) {
	t.Parallel()

	if err := validateWikiResourceTokenLength(strings.Repeat("a", minWikiResourceTokenLength-1), "--node-token"); err == nil {
		t.Fatal("expected a 26-character token to be rejected")
	}
	if err := validateWikiResourceTokenLength(strings.Repeat("a", minWikiResourceTokenLength), "--node-token"); err != nil {
		t.Fatalf("expected a 27-character token to be accepted, got %v", err)
	}
}

func TestWikiNodeGetRejectsTruncatedTokenBeforeHTTPRequest(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())
	requested := false
	reg.Register(&httpmock.Stub{
		Method:   "GET",
		URL:      "/open-apis/wiki/v2/spaces/get_node",
		Optional: true,
		OnMatch: func(*http.Request) {
			requested = true
		},
	})

	err := mountAndRunWiki(t, WikiNodeGet, []string{
		"+node-get",
		"--node-token", "PImXw",
		"--as", "bot",
	}, factory, stdout)
	if err == nil {
		t.Fatal("expected truncated token validation error")
	}
	if requested {
		t.Fatal("truncated token reached the OpenAPI transport")
	}
	p, ok := errs.ProblemOf(err)
	if !ok || p.Category != errs.CategoryValidation || p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("problem = %#v ok=%v, want validation/invalid_argument", p, ok)
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

func TestBuildWikiNodeGetDryRunSendsObjType(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeGetSpec("https://feishu.cn/docx/"+testDocxObjToken, "", "")
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
	if len(got.API) != 1 || got.API[0].URL != "/open-apis/wiki/v2/spaces/get_node" {
		t.Fatalf("dry-run api = %#v, want single get_node call", got.API)
	}
	if got.API[0].Params["token"] != testDocxObjToken || got.API[0].Params["obj_type"] != "docx" {
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

func TestWikiNodeGetMountedExecuteParsesURLAndFormatsOutput(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())

	factory, stdout, stderr, reg := cmdutil.TestFactory(t, wikiTestConfig())

	stub := &httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/wiki/v2/spaces/get_node",
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

	if !strings.Contains(capturedQuery, "token="+testDocxObjToken) || !strings.Contains(capturedQuery, "obj_type=docx") {
		t.Fatalf("captured query = %q, want token=%s and obj_type=docx", capturedQuery, testDocxObjToken)
	}

	data := decodeWikiEnvelope(t, stdout)
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
	// +node-get deliberately does not synthesize a url (get_node returns none;
	// a BuildResourceURL fallback would be a non-canonical, misleading link in
	// a read/confirm command).
	if _, ok := data["url"]; ok {
		t.Fatalf("did not expect a url field in +node-get output, got %#v", data["url"])
	}
	if got := stderr.String(); !strings.Contains(got, "Fetching wiki node") {
		t.Fatalf("stderr = %q, want fetching message", got)
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

			factory, stdout, _, reg := cmdutil.TestFactory(t, wikiTestConfig())
			reg.Register(&httpmock.Stub{
				Method: "GET",
				URL:    "/open-apis/wiki/v2/spaces/get_node",
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
			if p.LogID != "log-node-get-terminal" {
				t.Fatalf("log_id = %q, want log-node-get-terminal", p.LogID)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want no success envelope", stdout.String())
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
				URL:    "/open-apis/wiki/v2/spaces/get_node",
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
		URL:    "/open-apis/wiki/v2/spaces/get_node",
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

	if !strings.Contains(capturedQuery, "token="+testDocxObjToken) || !strings.Contains(capturedQuery, "obj_type=docx") {
		t.Fatalf("captured query = %q, want token=%s and obj_type=docx", capturedQuery, testDocxObjToken)
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
		URL:    "/open-apis/wiki/v2/spaces/get_node",
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
		URL:    "/open-apis/wiki/v2/spaces/get_node",
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
		URL:    "/open-apis/wiki/v2/spaces/get_node",
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
