// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

func TestParseWikiNodeGetSpecRawNodeToken(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeGetSpec("wikcnABC", "", "")
	if err != nil {
		t.Fatalf("parseWikiNodeGetSpec() error = %v", err)
	}
	if spec.Token != "wikcnABC" || spec.ObjType != "" || spec.SourceKind != "raw-node" {
		t.Fatalf("spec = %+v, want raw-node wikcnABC with no obj_type", spec)
	}
	if got := spec.RequestParams(); !reflect.DeepEqual(got, map[string]interface{}{"token": "wikcnABC"}) {
		t.Fatalf("RequestParams() = %v, want {token: wikcnABC}", got)
	}
}

func TestParseWikiNodeGetSpecRawObjTokenWithExplicitObjType(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeGetSpec("docxXYZ", "docx", "")
	if err != nil {
		t.Fatalf("parseWikiNodeGetSpec() error = %v", err)
	}
	if spec.Token != "docxXYZ" || spec.ObjType != "docx" || spec.SourceKind != "raw-obj" {
		t.Fatalf("spec = %+v, want raw-obj docxXYZ obj_type=docx", spec)
	}
}

func TestParseWikiNodeGetSpecRejectsRawObjTokenWithoutObjType(t *testing.T) {
	t.Parallel()

	// Mirrors +node-delete: a raw obj_token with no --obj-type must fail
	// upfront instead of defaulting to "doc" and hitting an opaque API error.
	_, err := parseWikiNodeGetSpec("bascnXYZ", "", "")
	if err == nil || !strings.Contains(err.Error(), "--obj-type is required for a raw obj_token") {
		t.Fatalf("expected raw obj_token obj-type-required error, got %v", err)
	}
}

func TestParseWikiNodeGetSpecRejectsObjTypeOnNodeToken(t *testing.T) {
	t.Parallel()

	_, err := parseWikiNodeGetSpec("wikcnABC", "docx", "")
	if err == nil || !strings.Contains(err.Error(), "only valid for obj_tokens") {
		t.Fatalf("expected node_token + obj_type rejection, got %v", err)
	}
}

func TestParseWikiNodeGetSpecExtractsTokenFromWikiURL(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeGetSpec("https://feishu.cn/wiki/wikcnABC?foo=bar", "", "")
	if err != nil {
		t.Fatalf("parseWikiNodeGetSpec() error = %v", err)
	}
	if spec.Token != "wikcnABC" || spec.ObjType != "" || spec.SourceKind != "url-wiki" {
		t.Fatalf("spec = %+v, want url-wiki wikcnABC", spec)
	}
}

func TestParseWikiNodeGetSpecExtractsTokenAndObjTypeFromDocxURL(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeGetSpec("https://feishu.cn/docx/docxXYZ", "", "")
	if err != nil {
		t.Fatalf("parseWikiNodeGetSpec() error = %v", err)
	}
	if spec.Token != "docxXYZ" || spec.ObjType != "docx" || spec.SourceKind != "url-obj" {
		t.Fatalf("spec = %+v, want url-obj docxXYZ", spec)
	}
}

func TestParseWikiNodeGetSpecRejectsURLObjTypeMismatch(t *testing.T) {
	t.Parallel()

	_, err := parseWikiNodeGetSpec("https://feishu.cn/sheets/shtXYZ", "docx", "")
	if err == nil || !strings.Contains(err.Error(), "does not match the obj_type") {
		t.Fatalf("expected URL/obj-type mismatch error, got %v", err)
	}
}

func TestParseWikiNodeGetSpecRejectsUnsupportedURLPath(t *testing.T) {
	t.Parallel()

	_, err := parseWikiNodeGetSpec("https://feishu.cn/im/chat/oc_123", "", "")
	if err == nil || !strings.Contains(err.Error(), "unsupported --token URL path") {
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

	if _, err := parseWikiNodeGetSpec("   ", "", ""); err == nil || !strings.Contains(err.Error(), "--token is required") {
		t.Fatalf("expected required-token error, got %v", err)
	}
}

func TestBuildWikiNodeGetDryRunSendsObjType(t *testing.T) {
	t.Parallel()

	spec, err := parseWikiNodeGetSpec("https://feishu.cn/docx/docxXYZ", "", "")
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
	if got.API[0].Params["token"] != "docxXYZ" || got.API[0].Params["obj_type"] != "docx" {
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
		"--token", "https://feishu.cn/docx/docxXYZ",
		"--as", "bot",
	}, factory, stdout)
	if err != nil {
		t.Fatalf("mountAndRunWiki() error = %v", err)
	}

	if !strings.Contains(capturedQuery, "token=docxXYZ") || !strings.Contains(capturedQuery, "obj_type=docx") {
		t.Fatalf("captured query = %q, want token=docxXYZ and obj_type=docx", capturedQuery)
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
		"--token", "wikcnABC",
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
		"--token", "wikcnABC",
		"--space-id", "space_expected",
		"--as", "bot",
	}, factory, stdout)
	if err == nil || !strings.Contains(err.Error(), "does not match the resolved node space") {
		t.Fatalf("expected space mismatch error, got %v", err)
	}
}
