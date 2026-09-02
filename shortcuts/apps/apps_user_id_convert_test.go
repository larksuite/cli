// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/httpmock"
)

// requireConvertValidation asserts the error carries the typed validation
// metadata (category + subtype via errs.ProblemOf) and returns the
// *errs.ValidationError so its Param / Hint can be asserted (Param lives on
// ValidationError, not the Problem projection).
func requireConvertValidation(t *testing.T, err error) *errs.ValidationError {
	t.Helper()
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("err = %T %v, want a typed problem", err, err)
	}
	if p.Category != errs.CategoryValidation {
		t.Fatalf("category = %s, want %s", p.Category, errs.CategoryValidation)
	}
	if p.Subtype != errs.SubtypeInvalidArgument {
		t.Fatalf("subtype = %s, want %s", p.Subtype, errs.SubtypeInvalidArgument)
	}
	var ve *errs.ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("err = %T %v, want *errs.ValidationError", err, err)
	}
	return ve
}

// idConvertEnvelope mirrors the success/partial JSON envelope for assertions.
type idConvertEnvelope struct {
	OK       bool   `json:"ok"`
	Identity string `json:"identity"`
	DryRun   bool   `json:"dry_run"`
	Data     struct {
		ConvertType string `json:"convert_type"`
		Items       []struct {
			Index    int    `json:"index"`
			SourceID string `json:"source_id"`
			TargetID string `json:"target_id"`
		} `json:"items"`
		Missed []struct {
			Index    int    `json:"index"`
			SourceID string `json:"source_id"`
			Reason   string `json:"reason"`
		} `json:"missed"`
	} `json:"data"`
	Meta struct {
		Total       *int `json:"total"`
		HitCount    *int `json:"hit_count"`
		MissedCount *int `json:"missed_count"`
	} `json:"meta"`
}

func decodeIDConvert(t *testing.T, out string) idConvertEnvelope {
	t.Helper()
	var env idConvertEnvelope
	if err := json.Unmarshal([]byte(out), &env); err != nil {
		t.Fatalf("decode envelope: %v\n%s", err, out)
	}
	return env
}

// Acceptance 1: batch, partial hit. One of two IDs resolves; the other is
// reconstructed into missed with reason not_found, keeping its input index.
func TestAppsUserIDConvert_PartialHit(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/directory/user/id_convert",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"source_id": "ou_abc123", "target_id": "1234567890123456"},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsUserIDConvert,
		[]string{"+user-id-convert", "--convert-type", "open-id-to-miaoda", "--ids", "ou_abc123,ou_def456", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	env := decodeIDConvert(t, stdout.String())
	if !env.OK {
		t.Fatalf("ok=false: %s", stdout.String())
	}
	if env.Data.ConvertType != "open-id-to-miaoda" {
		t.Fatalf("convert_type=%q", env.Data.ConvertType)
	}
	if len(env.Data.Items) != 1 || env.Data.Items[0].Index != 0 || env.Data.Items[0].SourceID != "ou_abc123" || env.Data.Items[0].TargetID != "1234567890123456" {
		t.Fatalf("items wrong: %+v", env.Data.Items)
	}
	if len(env.Data.Missed) != 1 || env.Data.Missed[0].Index != 1 || env.Data.Missed[0].SourceID != "ou_def456" || env.Data.Missed[0].Reason != "not_found" {
		t.Fatalf("missed wrong: %+v", env.Data.Missed)
	}
	if env.Meta.Total == nil || *env.Meta.Total != 2 || env.Meta.HitCount == nil || *env.Meta.HitCount != 1 || env.Meta.MissedCount == nil || *env.Meta.MissedCount != 1 {
		t.Fatalf("meta wrong: %+v", env.Meta)
	}
}

// Full hit must still emit missed_count: 0 (explicit zero), proving the pointer
// meta fields are not dropped by omitempty when the value is a real zero.
func TestAppsUserIDConvert_FullHitEmitsZeroMissed(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/directory/user/id_convert",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"source_id": "1234567890123456", "target_id": "700123456789"},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsUserIDConvert,
		[]string{"+user-id-convert", "--convert-type", "miaoda-to-feishu-user-id", "--ids", "1234567890123456", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	if !strings.Contains(stdout.String(), `"missed_count": 0`) {
		t.Fatalf("expected explicit missed_count: 0 in meta: %s", stdout.String())
	}
	env := decodeIDConvert(t, stdout.String())
	if len(env.Data.Missed) != 0 {
		t.Fatalf("missed should be empty: %+v", env.Data.Missed)
	}
	if env.Meta.MissedCount == nil || *env.Meta.MissedCount != 0 {
		t.Fatalf("missed_count should be present zero: %+v", env.Meta.MissedCount)
	}
}

// Duplicate input IDs are not de-duped and are matched to distinct output rows
// by position (first-match consumption of returned targets).
func TestAppsUserIDConvert_DuplicateIDsByPosition(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/directory/user/id_convert",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"source_id": "ou_dup", "target_id": "111"},
				},
			},
		},
	})

	if err := runAppsShortcut(t, AppsUserIDConvert,
		[]string{"+user-id-convert", "--convert-type", "open-id-to-miaoda", "--ids", "ou_dup,ou_dup", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	env := decodeIDConvert(t, stdout.String())
	if *env.Meta.Total != 2 {
		t.Fatalf("total should count duplicates: %+v", env.Meta)
	}
	// First occurrence hits (index 0), the second has no target left → missed (index 1).
	if len(env.Data.Items) != 1 || env.Data.Items[0].Index != 0 {
		t.Fatalf("items wrong: %+v", env.Data.Items)
	}
	if len(env.Data.Missed) != 1 || env.Data.Missed[0].Index != 1 {
		t.Fatalf("missed wrong: %+v", env.Data.Missed)
	}
}

// Acceptance 2: --dry-run prints the assembled body without calling.
func TestAppsUserIDConvert_DryRun(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	if err := runAppsShortcut(t, AppsUserIDConvert,
		[]string{"+user-id-convert", "--convert-type", "miaoda-to-open-id", "--ids", "111,222", "--dry-run", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"id_convert_type": 10`) {
		t.Fatalf("dry-run body missing id_convert_type 10: %s", got)
	}
	if !strings.Contains(got, `"111"`) || !strings.Contains(got, `"222"`) {
		t.Fatalf("dry-run body missing ids: %s", got)
	}
	if !strings.Contains(got, "id_convert") {
		t.Fatalf("dry-run should reference the id_convert endpoint: %s", got)
	}
}

// requestIDs decodes the ids array the CLI actually sent, so @file / stdin tests
// prove the input was split into discrete IDs rather than forwarded as one blob.
func requestIDs(t *testing.T, body []byte) []string {
	t.Helper()
	var req struct {
		IDConvertType int      `json:"id_convert_type"`
		IDs           []string `json:"ids"`
	}
	if err := json.Unmarshal(body, &req); err != nil {
		t.Fatalf("decode request body: %v\n%s", err, body)
	}
	return req.IDs
}

// A stdin list is newline-delimited (one ID per line), not comma-delimited. The
// framework hands the whole block to parseConvertIDs, which must split it into
// discrete IDs — not send the block as a single request ID. A trailing newline
// (every well-formed text file has one) must not create an empty trailing ID.
func TestAppsUserIDConvert_StdinNewlineList(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	factory.IOStreams.In = strings.NewReader("ou_a\nou_b\nou_c\n")
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/directory/user/id_convert",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"items": []interface{}{}},
		},
	}
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsUserIDConvert,
		[]string{"+user-id-convert", "--convert-type", "open-id-to-miaoda", "--ids", "-", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := requestIDs(t, stub.CapturedBody)
	want := []string{"ou_a", "ou_b", "ou_c"}
	if len(got) != len(want) {
		t.Fatalf("request ids = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("request ids = %v, want %v", got, want)
		}
	}
}

// @file input goes through the same path: a one-ID-per-line file must be split
// into discrete IDs. Mixed comma+newline separators within a file are accepted
// too, since a newline is treated as equivalent to a comma.
func TestAppsUserIDConvert_FileNewlineList(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	dir := t.TempDir()
	cmdutil.TestChdir(t, dir)
	if err := os.WriteFile("ids.txt", []byte("ou_a\nou_b,ou_c\nou_d\n"), 0o600); err != nil {
		t.Fatalf("write ids file: %v", err)
	}
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/directory/user/id_convert",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{"items": []interface{}{}},
		},
	}
	reg.Register(stub)

	if err := runAppsShortcut(t, AppsUserIDConvert,
		[]string{"+user-id-convert", "--convert-type", "open-id-to-miaoda", "--ids", "@ids.txt", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	got := requestIDs(t, stub.CapturedBody)
	want := []string{"ou_a", "ou_b", "ou_c", "ou_d"}
	if len(got) != len(want) {
		t.Fatalf("request ids = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("request ids = %v, want %v", got, want)
		}
	}
}

// Acceptance 3: invalid --convert-type → typed validation error on
// --convert-type listing the allowed directions. The flag's Enum makes the
// framework reject a bad value before Execute; subtype is invalid_argument
// (the taxonomy's validation subtype) with param --convert-type.
func TestAppsUserIDConvert_InvalidConvertType(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsUserIDConvert,
		[]string{"+user-id-convert", "--convert-type", "bogus", "--ids", "111", "--as", "user"},
		factory, stdout)
	ve := requireConvertValidation(t, err)
	if ve.Param != "--convert-type" {
		t.Fatalf("param=%q, want --convert-type", ve.Param)
	}
	if !strings.Contains(ve.Message, "miaoda-to-open-id") || !strings.Contains(ve.Message, "miaoda-to-feishu-user-id") {
		t.Fatalf("message should list allowed directions: %q", ve.Message)
	}
}

// resolveConvertType is the internal mapping used by Execute/DryRun. Table-drive
// every direction so each --convert-type → id_convert_type mapping (10/11/20/21/40)
// is directly protected; the HTTP tests do not assert the request body. Also cover
// both error branches: the empty-input hint wording (pipe-joined allowed list) and
// the non-empty "not a valid direction" message the runner's enum gate normally
// preempts.
func TestResolveConvertType(t *testing.T) {
	cases := []struct {
		flag string
		want int
	}{
		{"miaoda-to-open-id", 10},
		{"miaoda-to-union-id", 11},
		{"open-id-to-miaoda", 20},
		{"union-id-to-miaoda", 21},
		{"miaoda-to-feishu-user-id", 40},
	}
	if len(cases) != len(idConvertDirections) {
		t.Fatalf("cases cover %d directions, idConvertDirections has %d — keep them in sync", len(cases), len(idConvertDirections))
	}
	for _, c := range cases {
		st, err := resolveConvertType(c.flag)
		if err != nil || st != c.want {
			t.Fatalf("resolveConvertType(%q) = %d, %v; want %d, nil", c.flag, st, err, c.want)
		}
	}
	_, err := resolveConvertType("")
	ve := requireConvertValidation(t, err)
	if ve.Param != "--convert-type" || !strings.Contains(ve.Hint, "|") {
		t.Fatalf("empty convert-type: param=%q hint=%q", ve.Param, ve.Hint)
	}

	// A non-empty bad value takes resolveConvertType's other branch. The runner's
	// enum gate normally rejects this first, so this branch is only reachable by a
	// direct caller (DryRun/Execute call resolveConvertType themselves) — exercise
	// it so the distinct "not a valid direction" wording stays covered, not dead.
	_, err = resolveConvertType("bogus")
	ve = requireConvertValidation(t, err)
	if ve.Param != "--convert-type" {
		t.Fatalf("bad convert-type: param=%q, want --convert-type", ve.Param)
	}
	if !strings.Contains(ve.Message, "bogus") || !strings.Contains(ve.Message, "not a valid direction") {
		t.Fatalf("bad convert-type message should name the value and reason: %q", ve.Message)
	}
}

// Acceptance 4a: empty --ids → validation / invalid_argument on --ids.
func TestAppsUserIDConvert_EmptyIDs(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsUserIDConvert,
		[]string{"+user-id-convert", "--convert-type", "open-id-to-miaoda", "--ids", "  ,  ,", "--as", "user"},
		factory, stdout)
	ve := requireConvertValidation(t, err)
	if ve.Param != "--ids" {
		t.Fatalf("param=%q, want --ids", ve.Param)
	}
	if !strings.Contains(ve.Hint, "1–100") {
		t.Fatalf("hint should mention 1–100: %q", ve.Hint)
	}
}

// An interior empty element (e.g. "id-a,,id-b") must be rejected, not silently
// dropped — dropping it would shift later result indices and break the
// position-keyed items/missed contract.
func TestAppsUserIDConvert_InteriorEmptyID(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	err := runAppsShortcut(t, AppsUserIDConvert,
		[]string{"+user-id-convert", "--convert-type", "open-id-to-miaoda", "--ids", "ou_a,,ou_b", "--as", "user"},
		factory, stdout)
	ve := requireConvertValidation(t, err)
	if ve.Param != "--ids" {
		t.Fatalf("param=%q, want --ids", ve.Param)
	}
	if !strings.Contains(ve.Message, "empty entry") {
		t.Fatalf("message should flag the empty entry: %q", ve.Message)
	}
}

// Acceptance 4b: more than 100 IDs → validation / invalid_argument on --ids.
func TestAppsUserIDConvert_TooManyIDs(t *testing.T) {
	factory, stdout, _ := newAppsExecuteFactory(t)
	ids := make([]string, 101)
	for i := range ids {
		ids[i] = "ou_" + strings.Repeat("a", 3) + string(rune('0'+i%10))
	}
	err := runAppsShortcut(t, AppsUserIDConvert,
		[]string{"+user-id-convert", "--convert-type", "open-id-to-miaoda", "--ids", strings.Join(ids, ","), "--as", "user"},
		factory, stdout)
	ve := requireConvertValidation(t, err)
	if ve.Param != "--ids" {
		t.Fatalf("param=%q, want --ids", ve.Param)
	}
}

// Whole-batch rejection (OpenAPI code != 0) surfaces as an api error with the
// passthrough code and log_id, and does not retry.
func TestAppsUserIDConvert_WholeBatchRejected(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/directory/user/id_convert",
		Body: map[string]interface{}{
			"code":   1254045,
			"msg":    "invalid request",
			"log_id": "logid-xyz",
			"data":   map[string]interface{}{},
		},
	})

	err := runAppsShortcut(t, AppsUserIDConvert,
		[]string{"+user-id-convert", "--convert-type", "open-id-to-miaoda", "--ids", "ou_x", "--as", "user"},
		factory, stdout)
	p := requireAppsProblem(t, err, errs.CategoryAPI)
	if p.Code != 1254045 {
		t.Fatalf("code=%d, want passthrough 1254045", p.Code)
	}
	if p.LogID != "logid-xyz" {
		t.Fatalf("log_id=%q, want logid-xyz", p.LogID)
	}
}

// Numeric JSON IDs must be stringified, not silently coerced to "". Responses
// decode with json.Number (client.ParseJSONResponse uses dec.UseNumber()), so a
// server that emits source_id/target_id as bare numbers — plausible for the
// numeric Miaoda user_id form — would, under a strict string assertion, drop the
// source_id (→ false not_found) and blank the target_id (→ false success). This
// asserts the loose reader keeps such rows intact with their literal digits.
func TestAppsUserIDConvert_NumericJSONIDs(t *testing.T) {
	factory, stdout, reg := newAppsExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "POST",
		URL:    "/open-apis/spark/v1/directory/user/id_convert",
		// RawBody so the numbers stay bare in the wire JSON and decode as
		// json.Number, exactly as the live endpoint would deliver them.
		RawBody: []byte(`{"code":0,"data":{"items":[{"source_id":1234567890123456,"target_id":700123456789}]}}`),
	})

	if err := runAppsShortcut(t, AppsUserIDConvert,
		[]string{"+user-id-convert", "--convert-type", "miaoda-to-feishu-user-id", "--ids", "1234567890123456", "--as", "user"},
		factory, stdout); err != nil {
		t.Fatalf("execute err=%v", err)
	}
	env := decodeIDConvert(t, stdout.String())
	if len(env.Data.Missed) != 0 {
		t.Fatalf("numeric source_id was dropped → false not_found: %+v", env.Data.Missed)
	}
	if len(env.Data.Items) != 1 {
		t.Fatalf("want one resolved item, got: %+v", env.Data.Items)
	}
	if env.Data.Items[0].SourceID != "1234567890123456" {
		t.Fatalf("source_id = %q, want the literal digits (no float rounding)", env.Data.Items[0].SourceID)
	}
	if env.Data.Items[0].TargetID != "700123456789" {
		t.Fatalf("target_id = %q, want literal digits, not blank/rounded", env.Data.Items[0].TargetID)
	}
}

func TestAppsUserIDConvert_Registered(t *testing.T) {
	found := false
	for _, sc := range Shortcuts() {
		if sc.Command == "+user-id-convert" {
			found = true
			if sc.Risk != "read" {
				t.Fatalf("risk=%q, want read", sc.Risk)
			}
		}
	}
	if !found {
		t.Fatalf("+user-id-convert not registered in Shortcuts()")
	}
}
