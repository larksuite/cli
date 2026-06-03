// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

// newChatListTestRuntimeContext registers flags and returns a user-identity runtime context.
func newChatListTestRuntimeContext(t *testing.T, stringFlags map[string]string, boolFlags map[string]bool) *common.RuntimeContext {
	return newChatListTestRuntimeContextWithIdentity(t, stringFlags, boolFlags, core.AsUser)
}

// newChatListTestRuntimeContextWithIdentity is the identity-aware variant.
func newChatListTestRuntimeContextWithIdentity(t *testing.T, stringFlags map[string]string, boolFlags map[string]bool, as core.Identity) *common.RuntimeContext {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Int("page-size", 20, "")
	for name := range stringFlags {
		if name == "page-size" {
			continue
		}
		if name == "types" {
			cmd.Flags().StringSlice(name, nil, "")
		} else {
			cmd.Flags().String(name, "", "")
		}
	}
	for name := range boolFlags {
		cmd.Flags().Bool(name, false, "")
	}
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	for name, val := range stringFlags {
		if err := cmd.Flags().Set(name, val); err != nil {
			t.Fatalf("Flags().Set(%q) error = %v", name, err)
		}
	}
	for name, val := range boolFlags {
		if err := cmd.Flags().Set(name, strconv.FormatBool(val)); err != nil {
			t.Fatalf("Flags().Set(%q) error = %v", name, err)
		}
	}
	rt := common.TestNewRuntimeContextWithIdentity(cmd, nil, as)
	// Attach a minimal Factory with IOStreams so DryRun / Execute paths that
	// emit stderr warnings (e.g. bot_strip_p2p) don't panic on runtime.IO().
	// Stays pure-logic — no HTTP client, no httpmock; integration tests use
	// newBotShortcutRuntime / newUserShortcutRuntime for that.
	rt.Factory = &cmdutil.Factory{
		IOStreams: &cmdutil.IOStreams{
			Out:    &bytes.Buffer{},
			ErrOut: &bytes.Buffer{},
		},
	}
	return rt
}

func TestBuildChatListParams_Defaults(t *testing.T) {
	rt := newChatListTestRuntimeContext(t, map[string]string{
		"user-id-type": "open_id",
		"sort-type":    "ByCreateTimeAsc",
	}, nil)
	got := buildChatListParams(rt, "")
	if got["user_id_type"] != "open_id" {
		t.Fatalf("user_id_type = %v", got["user_id_type"])
	}
	if got["sort_type"] != "ByCreateTimeAsc" {
		t.Fatalf("sort_type = %v", got["sort_type"])
	}
	if got["page_size"] != 20 {
		t.Fatalf("page_size = %v, want 20", got["page_size"])
	}
	if _, present := got["page_token"]; present {
		t.Fatalf("page_token should be omitted when empty")
	}
	if _, present := got["types"]; present {
		t.Fatalf("types should be omitted when --types is empty")
	}
}

func TestBuildChatListParams_Overrides(t *testing.T) {
	rt := newChatListTestRuntimeContext(t, map[string]string{
		"user-id-type": "user_id",
		"sort-type":    "ByActiveTimeDesc",
		"page-size":    "50",
		"page-token":   "tok_xyz",
	}, nil)
	got := buildChatListParams(rt, "")
	if got["user_id_type"] != "user_id" {
		t.Fatalf("user_id_type = %v", got["user_id_type"])
	}
	if got["sort_type"] != "ByActiveTimeDesc" {
		t.Fatalf("sort_type = %v", got["sort_type"])
	}
	if got["page_size"] != 50 {
		t.Fatalf("page_size = %v, want 50", got["page_size"])
	}
	if got["page_token"] != "tok_xyz" {
		t.Fatalf("page_token = %v", got["page_token"])
	}
}

func TestImChatList_Validate_PageSizeBounds(t *testing.T) {
	cases := []struct {
		name     string
		pageSize string
		wantErr  bool
	}{
		{"zero rejected", "0", true},
		{"negative rejected", "-1", true},
		{"one ok", "1", false},
		{"hundred ok", "100", false},
		{"oneoone rejected", "101", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := newChatListTestRuntimeContext(t, map[string]string{"page-size": c.pageSize}, nil)
			err := ImChatList.Validate(context.Background(), rt)
			if (err != nil) != c.wantErr {
				t.Fatalf("Validate() err = %v, wantErr=%v", err, c.wantErr)
			}
		})
	}
}

func TestImChatList_DryRun_IncludesEndpoint(t *testing.T) {
	rt := newChatListTestRuntimeContext(t, map[string]string{
		"user-id-type": "open_id",
		"sort-type":    "ByActiveTimeDesc",
		"page-size":    "30",
	}, nil)
	got := mustMarshalDryRun(t, ImChatList.DryRun(context.Background(), rt))
	if !strings.Contains(got, `"/open-apis/im/v1/chats"`) {
		t.Fatalf("DryRun missing endpoint: %s", got)
	}
	if !strings.Contains(got, `"sort_type":"ByActiveTimeDesc"`) {
		t.Fatalf("DryRun missing sort_type: %s", got)
	}
	if !strings.Contains(got, `"page_size":30`) {
		t.Fatalf("DryRun missing page_size: %s", got)
	}
}

func TestNormalizeTypes(t *testing.T) {
	cases := []struct {
		name    string
		raw     []string
		want    []string
		wantErr string // substring match
	}{
		{"empty returns nil no error", nil, nil, ""},
		{"single p2p", []string{"p2p"}, []string{"p2p"}, ""},
		{"single group", []string{"group"}, []string{"group"}, ""},
		{"p2p,group preserves order", []string{"p2p", "group"}, []string{"p2p", "group"}, ""},
		{"group,p2p preserves order", []string{"group", "p2p"}, []string{"group", "p2p"}, ""},
		{"trim whitespace", []string{" p2p ", " group "}, []string{"p2p", "group"}, ""},
		{"lowercase", []string{"P2P", "GROUP"}, []string{"p2p", "group"}, ""},
		{"dedupe", []string{"p2p", "p2p"}, []string{"p2p"}, ""},
		{"empty element rejected", []string{""}, nil, "must contain at least one of p2p, group"},
		{"invalid element rejected", []string{"private"}, nil, `expected one of p2p, group`},
		{"mixed invalid rejected", []string{"p2p", "private"}, nil, `expected one of p2p, group`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := normalizeTypes(c.raw)
			if c.wantErr != "" {
				if err == nil {
					t.Fatalf("normalizeTypes(%v) err = nil; want substring %q", c.raw, c.wantErr)
				}
				if !strings.Contains(err.Error(), c.wantErr) {
					t.Fatalf("normalizeTypes(%v) err = %v; want substring %q", c.raw, err, c.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeTypes(%v) unexpected err = %v", c.raw, err)
			}
			if len(got) != len(c.want) {
				t.Fatalf("normalizeTypes(%v) = %v; want %v", c.raw, got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("normalizeTypes(%v)[%d] = %q; want %q", c.raw, i, got[i], c.want[i])
				}
			}
		})
	}
}

func TestResolveTypes(t *testing.T) {
	cases := []struct {
		name          string
		raw           string
		as            core.Identity
		wantEffective string
		wantStripped  bool
	}{
		{"user empty", "", core.AsUser, "", false},
		{"user p2p", "p2p", core.AsUser, "p2p", false},
		{"user p2p,group", "p2p,group", core.AsUser, "p2p,group", false},
		{"user group,p2p preserves order", "group,p2p", core.AsUser, "group,p2p", false},
		{"user normalized casing", "P2P,GROUP", core.AsUser, "p2p,group", false},
		{"bot empty", "", core.AsBot, "", false},
		{"bot group only", "group", core.AsBot, "group", false},
		{"bot p2p,group strips p2p", "p2p,group", core.AsBot, "group", true},
		{"bot group,p2p strips p2p", "group,p2p", core.AsBot, "group", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := newChatListTestRuntimeContextWithIdentity(t, map[string]string{"types": c.raw}, nil, c.as)
			effective, stripped, err := resolveTypes(rt)
			if err != nil {
				t.Fatalf("resolveTypes() unexpected err = %v", err)
			}
			if effective != c.wantEffective {
				t.Fatalf("effective = %q; want %q", effective, c.wantEffective)
			}
			if stripped != c.wantStripped {
				t.Fatalf("stripped = %v; want %v", stripped, c.wantStripped)
			}
		})
	}
}

func TestBuildChatListParams_TypesPassthrough(t *testing.T) {
	rt := newChatListTestRuntimeContext(t, map[string]string{
		"user-id-type": "open_id",
		"sort-type":    "ByCreateTimeAsc",
	}, nil)
	got := buildChatListParams(rt, "p2p,group")
	if got["types"] != "p2p,group" {
		t.Fatalf("types = %v; want \"p2p,group\"", got["types"])
	}
}

func TestImChatList_Validate_Types(t *testing.T) {
	cases := []struct {
		name     string
		typesRaw string
		as       core.Identity
		wantErr  string // substring; "" means no error
	}{
		{"user empty ok", "", core.AsUser, ""},
		{"user p2p ok", "p2p", core.AsUser, ""},
		{"user group ok", "group", core.AsUser, ""},
		{"user p2p,group ok", "p2p,group", core.AsUser, ""},
		{"user invalid element rejected", "private", core.AsUser, "expected one of p2p, group"},
		{"user comma-only rejected", ",", core.AsUser, "must contain at least one of p2p, group"},
		{"bot empty ok", "", core.AsBot, ""},
		{"bot group ok", "group", core.AsBot, ""},
		{"bot p2p,group ok (degraded at Execute)", "p2p,group", core.AsBot, ""},
		{"bot single p2p rejected", "p2p", core.AsBot, "only supported with user identity"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := newChatListTestRuntimeContextWithIdentity(t,
				map[string]string{"types": c.typesRaw, "page-size": "20"},
				nil, c.as)
			err := ImChatList.Validate(context.Background(), rt)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() unexpected err = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("Validate() err = nil; want substring %q", c.wantErr)
			}
			if !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("Validate() err = %v; want substring %q", err, c.wantErr)
			}
		})
	}
}

// attachChatListCmd builds a cobra.Command pre-loaded with all flags ImChatList
// reads, applies stringFlags / boolFlags, and assigns it to runtime.Cmd. Format
// is forced to "json" so Execute output lands in a parseable form on
// runtime.Factory.IOStreams.Out.
func attachChatListCmd(t *testing.T, runtime *common.RuntimeContext, stringFlags map[string]string, boolFlags map[string]bool) {
	t.Helper()
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().Int("page-size", 20, "")
	cmd.Flags().String("user-id-type", "open_id", "")
	cmd.Flags().String("sort-type", "ByCreateTimeAsc", "")
	cmd.Flags().StringSlice("types", nil, "")
	cmd.Flags().String("page-token", "", "")
	cmd.Flags().Bool("exclude-muted", false, "")
	cmd.Flags().Bool("dry-run", false, "")
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	for name, val := range stringFlags {
		if err := cmd.Flags().Set(name, val); err != nil {
			t.Fatalf("Flags().Set(%q) error = %v", name, err)
		}
	}
	for name, val := range boolFlags {
		if err := cmd.Flags().Set(name, strconv.FormatBool(val)); err != nil {
			t.Fatalf("Flags().Set(%q) error = %v", name, err)
		}
	}
	runtime.Cmd = cmd
	runtime.Format = "json"
}

// chatListOutBuf retrieves the captured stdout buffer for assertions.
func chatListOutBuf(t *testing.T, runtime *common.RuntimeContext) *bytes.Buffer {
	t.Helper()
	buf, ok := runtime.Factory.IOStreams.Out.(*bytes.Buffer)
	if !ok {
		t.Fatalf("expected IOStreams.Out to be *bytes.Buffer")
	}
	return buf
}

// chatListErrBuf retrieves the captured stderr buffer for assertions
// (used to verify request-level warnings like `bot_strip_p2p`).
func chatListErrBuf(t *testing.T, runtime *common.RuntimeContext) *bytes.Buffer {
	t.Helper()
	buf, ok := runtime.Factory.IOStreams.ErrOut.(*bytes.Buffer)
	if !ok {
		t.Fatalf("expected IOStreams.ErrOut to be *bytes.Buffer")
	}
	return buf
}

func TestImChatList_Execute_BotStripsP2p(t *testing.T) {
	var capturedURL string
	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		capturedURL = req.URL.String()
		body := `{"code":0,"msg":"ok","data":{"items":[{"chat_id":"oc_g","name":"G","chat_mode":"group"}],"has_more":false,"page_token":""}}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}))
	attachChatListCmd(t, rt, map[string]string{"types": "p2p,group"}, nil)

	if err := ImChatList.Execute(context.Background(), rt); err != nil {
		t.Fatalf("Execute() err = %v", err)
	}

	if !strings.Contains(capturedURL, "types=group") {
		t.Fatalf("request URL = %s; want types=group (bot strips p2p)", capturedURL)
	}
	if strings.Contains(capturedURL, "p2p") {
		t.Fatalf("request URL = %s; must NOT contain p2p (bot stripped it)", capturedURL)
	}

	// Structured notice: outData["notices"] contains a {code, message} entry
	// for bot_strip_p2p (request-level adjustment, not a row-level filter).
	out := chatListOutBuf(t, rt).String()
	for _, want := range []string{`"notices"`, `"code": "bot_strip_p2p"`, `"message"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout JSON missing notice field %q:\n%s", want, out)
		}
	}
	// filter slot must remain mute-scoped: bot_strip_p2p must not leak into
	// outData["filter"].applied (no priority conflict by design).
	if strings.Contains(out, `"applied": "bot_strip_p2p"`) {
		t.Fatalf("bot_strip_p2p should not appear in filter.applied (separate slot):\n%s", out)
	}

	// Stderr: matches repo `warning: <code>: <message>` convention (cf.
	// shortcuts/common/runner.go unknown-format fallback).
	errOut := chatListErrBuf(t, rt).String()
	if !strings.Contains(errOut, "warning: bot_strip_p2p:") {
		t.Fatalf("stderr missing `warning: bot_strip_p2p:` prefix:\n%s", errOut)
	}
}

// TestImChatList_DryRun_BotStripsP2pStderrNotice verifies the DryRun branch
// also emits the bot_strip_p2p warning to stderr so a previewed request
// truthfully reflects what Execute would send (drive_search.go DryRun parity).
func TestImChatList_DryRun_BotStripsP2pStderrNotice(t *testing.T) {
	rt := newBotShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("DryRun should not make HTTP calls; got: %s", req.URL.String())
		return nil, nil
	}))
	attachChatListCmd(t, rt, map[string]string{"types": "p2p,group"}, nil)

	dr := ImChatList.DryRun(context.Background(), rt)
	if dr == nil {
		t.Fatalf("DryRun returned nil")
	}

	errOut := chatListErrBuf(t, rt).String()
	if !strings.Contains(errOut, "warning: bot_strip_p2p:") {
		t.Fatalf("DryRun stderr missing `warning: bot_strip_p2p:` prefix:\n%s", errOut)
	}
}

func TestImChatList_RowRendering_P2pFields(t *testing.T) {
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"code":0,"msg":"ok","data":{"items":[
			{"chat_id":"oc_g","name":"Group","chat_mode":"group","owner_id":"ou_owner"},
			{"chat_id":"oc_p","name":"Peer","chat_mode":"p2p","p2p_target_type":"user","p2p_target_id":"ou_peer"}
		],"has_more":false,"page_token":""}}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}))
	attachChatListCmd(t, rt, map[string]string{"types": "p2p,group"}, nil)

	if err := ImChatList.Execute(context.Background(), rt); err != nil {
		t.Fatalf("Execute() err = %v", err)
	}

	out := chatListOutBuf(t, rt).String()
	for _, want := range []string{"oc_g", "oc_p", "Group", "Peer", `"chat_mode": "p2p"`, `"p2p_target_id": "ou_peer"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q; got: %s", want, out)
		}
	}
}

// TestImChatList_Execute_PrettyOutputRendersP2pRow exercises the pretty-format
// rendering closure in Execute, including the new chat_mode=="p2p" branch that
// surfaces p2p_target_type / p2p_target_id, and the has_more footer that
// echoes back the page_token.
func TestImChatList_Execute_PrettyOutputRendersP2pRow(t *testing.T) {
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"code":0,"msg":"ok","data":{"items":[
			{"chat_id":"oc_g","name":"Group","chat_mode":"group","owner_id":"ou_owner","description":"a group","external":false,"chat_status":"normal"},
			{"chat_id":"oc_p","name":"Peer","chat_mode":"p2p","p2p_target_type":"user","p2p_target_id":"ou_peer"}
		],"has_more":true,"page_token":"next_tok"}}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}))
	attachChatListCmd(t, rt, map[string]string{"types": "p2p,group"}, nil)
	rt.Format = "pretty"

	if err := ImChatList.Execute(context.Background(), rt); err != nil {
		t.Fatalf("Execute() err = %v", err)
	}

	out := chatListOutBuf(t, rt).String()
	for _, want := range []string{"oc_g", "Group", "a group", "ou_owner", "normal"} {
		if !strings.Contains(out, want) {
			t.Fatalf("pretty output missing group-row field %q:\n%s", want, out)
		}
	}
	for _, want := range []string{"oc_p", "Peer", "p2p", "ou_peer"} {
		if !strings.Contains(out, want) {
			t.Fatalf("pretty output missing p2p-row field %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "2 chat(s) listed") {
		t.Fatalf("pretty output missing footer count:\n%s", out)
	}
	if !strings.Contains(out, "next_tok") {
		t.Fatalf("pretty output missing page_token in has_more footer:\n%s", out)
	}
}

func TestImChatList_DryRun_TypesPassthrough(t *testing.T) {
	cases := []struct {
		name     string
		as       core.Identity
		typesRaw string
		wantSub  string // substring expected in dry-run JSON
		wantErr  bool   // whether Validate should reject before DryRun runs
	}{
		{"user p2p", core.AsUser, "p2p", `"types":"p2p"`, false},
		{"user p2p,group", core.AsUser, "p2p,group", `"types":"p2p,group"`, false},
		{"bot p2p,group strips to group", core.AsBot, "p2p,group", `"types":"group"`, false},
		{"bot group passes", core.AsBot, "group", `"types":"group"`, false},
		{"bot single p2p rejected at Validate", core.AsBot, "p2p", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rt := newChatListTestRuntimeContextWithIdentity(t, map[string]string{
				"user-id-type": "open_id",
				"sort-type":    "ByCreateTimeAsc",
				"page-size":    "20",
				"types":        c.typesRaw,
			}, nil, c.as)
			if err := ImChatList.Validate(context.Background(), rt); err != nil {
				if !c.wantErr {
					t.Fatalf("Validate() unexpected err = %v", err)
				}
				return
			}
			if c.wantErr {
				t.Fatalf("Validate() err = nil; want rejection")
			}
			got := mustMarshalDryRun(t, ImChatList.DryRun(context.Background(), rt))
			if !strings.Contains(got, c.wantSub) {
				t.Fatalf("DryRun = %s; want substring %q", got, c.wantSub)
			}
		})
	}
}

func TestImChatList_RowRendering_ChatModeAbsent(t *testing.T) {
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		// Response items deliberately omit chat_mode / p2p_target_* (legacy/defensive case).
		body := `{"code":0,"msg":"ok","data":{"items":[
			{"chat_id":"oc_g1","name":"Group1","owner_id":"ou_owner"},
			{"chat_id":"oc_g2","name":"Group2","external":true}
		],"has_more":false,"page_token":""}}`
		return &http.Response{
			StatusCode: 200,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}))
	attachChatListCmd(t, rt, nil, nil) // no --types; default behavior

	if err := ImChatList.Execute(context.Background(), rt); err != nil {
		t.Fatalf("Execute() err = %v", err)
	}

	out := chatListOutBuf(t, rt).String()
	// chat_mode / p2p_target_* must NOT appear since the API didn't return them.
	for _, forbidden := range []string{`"chat_mode"`, `"p2p_target_type"`, `"p2p_target_id"`} {
		// "chats[].chat_mode" is the row-level field — JSON envelope might include it as null or omit it;
		// asserting the rendered table fields are missing is the goal.
		// The JSON pass-through preserves whatever API returned (omitted here),
		// so neither path should produce these strings.
		if strings.Contains(out, forbidden) {
			t.Fatalf("output unexpectedly contains %q (should not appear when API omitted these fields); got: %s", forbidden, out)
		}
	}
	// Sanity: the two chat IDs must still be present (renderer didn't crash).
	for _, want := range []string{"oc_g1", "oc_g2", "Group1", "Group2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q; got: %s", want, out)
		}
	}
}

func TestImChatList_Execute_UserMuteFiltersP2p(t *testing.T) {
	rt := newUserShortcutRuntime(t, shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		path := req.URL.Path
		switch {
		case strings.HasSuffix(path, "/im/v1/chats"):
			body := `{"code":0,"msg":"ok","data":{"items":[
				{"chat_id":"oc_g","name":"Group","chat_mode":"group","owner_id":"ou_owner"},
				{"chat_id":"oc_p","name":"Peer","chat_mode":"p2p","p2p_target_type":"user","p2p_target_id":"ou_peer"}
			],"has_more":false,"page_token":""}}`
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		case strings.HasSuffix(path, "/chat_user_setting/batch_get_mute_status"):
			// Mark oc_p (the p2p) as muted; oc_g not muted.
			body := `{"code":0,"msg":"ok","data":{"items":[
				{"chat_id":"oc_g","is_muted":false},
				{"chat_id":"oc_p","is_muted":true}
			]}}`
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
		}
		t.Fatalf("unexpected request path: %s", path)
		return nil, nil
	}))
	attachChatListCmd(t, rt,
		map[string]string{"types": "p2p,group"},
		map[string]bool{"exclude-muted": true})

	if err := ImChatList.Execute(context.Background(), rt); err != nil {
		t.Fatalf("Execute() err = %v", err)
	}

	out := chatListOutBuf(t, rt).String()

	var parsed struct {
		Data struct {
			Chats  []map[string]interface{} `json:"chats"`
			Filter struct {
				Applied       string `json:"applied"`
				FilteredCount int    `json:"filtered_count"`
			} `json:"filter"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("Unmarshal output failed: %v; raw: %s", err, out)
	}
	if parsed.Data.Filter.Applied != "exclude_muted" {
		t.Fatalf("filter.applied = %q; want exclude_muted (no bot_strip_p2p under user). Raw: %s",
			parsed.Data.Filter.Applied, out)
	}
	if parsed.Data.Filter.FilteredCount != 1 {
		t.Fatalf("filter.filtered_count = %d; want 1 (the muted p2p row). Raw: %s",
			parsed.Data.Filter.FilteredCount, out)
	}
	// The muted p2p row should be gone from chats; only oc_g remains.
	if len(parsed.Data.Chats) != 1 {
		t.Fatalf("expected 1 chat after muting; got %d. Raw: %s", len(parsed.Data.Chats), out)
	}
	if parsed.Data.Chats[0]["chat_id"] != "oc_g" {
		t.Fatalf("remaining chat = %v; want oc_g", parsed.Data.Chats[0]["chat_id"])
	}
}
