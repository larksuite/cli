// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/imcontract"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

type listPageAllCase struct {
	name        string
	shortcut    common.Shortcut
	path        string
	method      string
	outputKey   string
	outputID    string
	baseFlags   map[string]string
	makeRawItem func(string) interface{}
}

func listPageAllCases() []listPageAllCase {
	messageItem := func(id string) interface{} {
		return map[string]interface{}{
			"message_id":  id,
			"msg_type":    "text",
			"body":        map[string]interface{}{"content": fmt.Sprintf(`{"text":%q}`, id)},
			"create_time": "0",
		}
	}
	chatItem := func(id string) interface{} {
		return map[string]interface{}{"chat_id": id, "name": id, "chat_mode": "group"}
	}
	searchItem := func(id string) interface{} {
		return map[string]interface{}{"meta_data": chatItem(id)}
	}
	return []listPageAllCase{
		{
			name: "chat-messages-list", shortcut: ImChatMessageList,
			path: "/open-apis/im/v1/messages", method: http.MethodGet,
			outputKey: "messages", outputID: "message_id",
			baseFlags:   map[string]string{"chat-id": "oc_test", "no-reactions": "true"},
			makeRawItem: messageItem,
		},
		{
			name: "threads-messages-list", shortcut: ImThreadsMessagesList,
			path: "/open-apis/im/v1/messages", method: http.MethodGet,
			outputKey: "messages", outputID: "message_id",
			baseFlags:   map[string]string{"thread": "omt_test", "no-reactions": "true"},
			makeRawItem: messageItem,
		},
		{
			name: "chat-list", shortcut: ImChatList,
			path: "/open-apis/im/v1/chats", method: http.MethodGet,
			outputKey: "chats", outputID: "chat_id",
			baseFlags:   map[string]string{},
			makeRawItem: chatItem,
		},
		{
			name: "chat-search", shortcut: ImChatSearch,
			path: "/open-apis/im/v2/chats/search", method: http.MethodPost,
			outputKey: "chats", outputID: "chat_id",
			baseFlags:   map[string]string{"query": "team"},
			makeRawItem: searchItem,
		},
	}
}

func newListPageAllCommand(t *testing.T, shortcut common.Shortcut, flags map[string]string) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: shortcut.Command}
	for _, flag := range shortcut.Flags {
		switch flag.Type {
		case "bool":
			cmd.Flags().Bool(flag.Name, flag.Default == "true", flag.Desc)
		case "int":
			defaultValue := 0
			if flag.Default != "" {
				defaultValue, _ = strconv.Atoi(flag.Default)
			}
			cmd.Flags().Int(flag.Name, defaultValue, flag.Desc)
		case "string_slice":
			cmd.Flags().StringSlice(flag.Name, nil, flag.Desc)
		default:
			cmd.Flags().String(flag.Name, flag.Default, flag.Desc)
		}
	}
	if err := cmd.ParseFlags(nil); err != nil {
		t.Fatalf("ParseFlags() error = %v", err)
	}
	for name, value := range flags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s=%s: %v", name, value, err)
		}
	}
	return cmd
}

func mergeListPageAllFlags(base map[string]string, overrides map[string]string) map[string]string {
	flags := make(map[string]string, len(base)+len(overrides))
	for name, value := range base {
		flags[name] = value
	}
	for name, value := range overrides {
		flags[name] = value
	}
	return flags
}

func newListPageAllRuntime(t *testing.T, tc listPageAllCase, flags map[string]string, responder func(*http.Request, int) map[string]interface{}) (*common.RuntimeContext, *int) {
	t.Helper()
	calls := 0
	transport := shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != tc.method || req.URL.Path != tc.path {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
		}
		calls++
		data := responder(req, calls)
		return shortcutJSONResponse(http.StatusOK, map[string]interface{}{"code": 0, "data": data}), nil
	})
	runtime := newUserShortcutRuntime(t, transport)
	allFlags := mergeListPageAllFlags(tc.baseFlags, flags)
	if _, explicitlyTestingDelay := allFlags["page-delay"]; !explicitlyTestingDelay {
		// Unit tests exercise pagination semantics without adding wall-clock
		// latency. The real default remains asserted in the flag-surface test.
		allFlags["page-delay"] = "0"
	}
	runtime.Cmd = newListPageAllCommand(t, tc.shortcut, allFlags)
	runtime.Format = "json"
	return runtime, &calls
}

func listPageAllOutputData(t *testing.T, runtime *common.RuntimeContext) map[string]interface{} {
	t.Helper()
	envelope := listPageAllOutputEnvelope(t, runtime)
	data, ok := envelope["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("stdout data has unexpected shape: %#v", envelope["data"])
	}
	return data
}

func listPageAllOutputEnvelope(t *testing.T, runtime *common.RuntimeContext) map[string]interface{} {
	t.Helper()
	out, ok := runtime.IO().Out.(*bytes.Buffer)
	if !ok {
		t.Fatal("stdout is not a bytes.Buffer")
	}
	var envelope map[string]interface{}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, out.String())
	}
	return envelope
}

func attachFullIMReadSession(t *testing.T, runtime *common.RuntimeContext, command string) {
	t.Helper()
	contract, ok := imcontract.Lookup(imcontract.ContractKey("im " + command))
	if !ok {
		t.Fatalf("missing IM contract for %s", command)
	}
	session, err := imcontract.NewReadSession(contract, imcontract.ReadOptions{FullRead: true})
	if err != nil {
		t.Fatalf("NewReadSession() error = %v", err)
	}
	setRuntimeField(t, runtime, "readSession", session)
}

func assertListPaginationMeta(t *testing.T, runtime *common.RuntimeContext, complete bool, pages, items int, nextToken string) {
	t.Helper()
	envelope := listPageAllOutputEnvelope(t, runtime)
	meta, ok := envelope["meta"].(map[string]interface{})
	if !ok {
		t.Fatalf("stdout meta has unexpected shape: %#v", envelope["meta"])
	}
	pagination, ok := meta["pagination"].(map[string]interface{})
	if !ok {
		t.Fatalf("stdout pagination meta has unexpected shape: %#v", meta["pagination"])
	}
	if got, _ := pagination["complete"].(bool); got != complete {
		t.Fatalf("pagination.complete = %v, want %v", got, complete)
	}
	if got := int(pagination["pages"].(float64)); got != pages {
		t.Fatalf("pagination.pages = %d, want %d", got, pages)
	}
	if got := int(pagination["items"].(float64)); got != items {
		t.Fatalf("pagination.items = %d, want %d", got, items)
	}
	if got, _ := pagination["next_token"].(string); got != nextToken {
		t.Fatalf("pagination.next_token = %q, want %q", got, nextToken)
	}
}

func assertListPageAllOrder(t *testing.T, data map[string]interface{}, tc listPageAllCase, want ...string) {
	t.Helper()
	items, ok := data[tc.outputKey].([]interface{})
	if !ok {
		t.Fatalf("%s has unexpected shape: %#v", tc.outputKey, data[tc.outputKey])
	}
	if len(items) != len(want) {
		t.Fatalf("%s length = %d, want %d: %#v", tc.outputKey, len(items), len(want), items)
	}
	for i, item := range items {
		row, _ := item.(map[string]interface{})
		if got, _ := row[tc.outputID].(string); got != want[i] {
			t.Fatalf("%s[%d].%s = %q, want %q", tc.outputKey, i, tc.outputID, got, want[i])
		}
	}
}

func TestIMListPageAllMergesPagesAndUsesFinalPaginationMeta(t *testing.T) {
	for _, tc := range listPageAllCases() {
		t.Run(tc.name, func(t *testing.T) {
			var requestTokens []string
			runtime, calls := newListPageAllRuntime(t, tc, map[string]string{"page-all": "true"}, func(req *http.Request, call int) map[string]interface{} {
				requestTokens = append(requestTokens, req.URL.Query().Get("page_token"))
				if call == 1 {
					return map[string]interface{}{"items": []interface{}{tc.makeRawItem("first")}, "has_more": true, "page_token": "next", "total": 2}
				}
				return map[string]interface{}{"items": []interface{}{tc.makeRawItem("second")}, "has_more": false, "page_token": "final", "total": 2}
			})
			if err := tc.shortcut.Validate(context.Background(), runtime); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if err := tc.shortcut.Execute(context.Background(), runtime); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if *calls != 2 {
				t.Fatalf("API calls = %d, want 2", *calls)
			}
			if len(requestTokens) != 2 || requestTokens[0] != "" || requestTokens[1] != "next" {
				t.Fatalf("request page tokens = %v, want [\"\" \"next\"]", requestTokens)
			}
			data := listPageAllOutputData(t, runtime)
			assertListPageAllOrder(t, data, tc, "first", "second")
			if hasMore, _ := data["has_more"].(bool); hasMore {
				t.Fatalf("has_more = true, want final page value false")
			}
			if token, _ := data["page_token"].(string); token != "final" {
				t.Fatalf("page_token = %q, want final", token)
			}
			assertListPaginationMeta(t, runtime, true, 2, 2, "")
		})
	}
}

func TestIMListPageLimitZeroReadsToExhaustion(t *testing.T) {
	for _, tc := range listPageAllCases() {
		t.Run(tc.name, func(t *testing.T) {
			runtime, calls := newListPageAllRuntime(t, tc, map[string]string{
				"page-all":   "true",
				"page-limit": "0",
			}, func(_ *http.Request, call int) map[string]interface{} {
				if call == 1 {
					return map[string]interface{}{
						"items": []interface{}{tc.makeRawItem("first")}, "has_more": true, "page_token": "next",
					}
				}
				return map[string]interface{}{
					"items": []interface{}{tc.makeRawItem("second")}, "has_more": false,
				}
			})
			if err := tc.shortcut.Validate(context.Background(), runtime); err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if err := tc.shortcut.Execute(context.Background(), runtime); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if *calls != 2 {
				t.Fatalf("API calls = %d, want 2", *calls)
			}
			assertListPageAllOrder(t, listPageAllOutputData(t, runtime), tc, "first", "second")
			assertListPaginationMeta(t, runtime, true, 2, 2, "")
		})
	}
}

func TestIMListPageAllRejectsRepeatedToken(t *testing.T) {
	for _, tc := range listPageAllCases() {
		t.Run(tc.name, func(t *testing.T) {
			runtime, calls := newListPageAllRuntime(t, tc, map[string]string{"page-all": "true"}, func(_ *http.Request, call int) map[string]interface{} {
				return map[string]interface{}{"items": []interface{}{tc.makeRawItem(fmt.Sprintf("item-%d", call))}, "has_more": true, "page_token": "same", "total": 10}
			})
			attachFullIMReadSession(t, runtime, tc.shortcut.Command)
			if err := tc.shortcut.Execute(context.Background(), runtime); err != nil {
				t.Fatalf("Execute() error = %v, want partial result emitted through contract", err)
			}
			envelope := listPageAllOutputEnvelope(t, runtime)
			if ok, _ := envelope["ok"].(bool); ok {
				t.Fatalf("ok = true, want repeated-token partial failure: %#v", envelope)
			}
			notice, _ := envelope["_notice"].(map[string]interface{})
			readNotice, _ := notice["im_read"].(map[string]interface{})
			if readNotice["stop_reason"] != "repeated_token" {
				t.Fatalf("stop_reason = %#v, want repeated_token", readNotice["stop_reason"])
			}
			problem, _ := envelope["error"].(map[string]interface{})
			if problem["type"] != string(errs.CategoryInternal) || problem["subtype"] != string(errs.SubtypeInvalidResponse) {
				t.Fatalf("error = %#v, want internal/invalid_response", problem)
			}
			if message, _ := problem["message"].(string); !strings.Contains(message, "repeated page token") {
				t.Fatalf("error message = %q, want repeated-token diagnosis", message)
			}
			if *calls != 2 {
				t.Fatalf("API calls = %d, want 2", *calls)
			}
			if stderr := runtime.IO().ErrOut.(*bytes.Buffer).String(); strings.Contains(stderr, "reached page limit") {
				t.Fatalf("repeated token must not report a page-limit stop: %q", stderr)
			}
		})
	}
}

func TestIMListPageAllReportsIncompleteResultOnPageLimit(t *testing.T) {
	for _, tc := range listPageAllCases() {
		t.Run(tc.name, func(t *testing.T) {
			runtime, calls := newListPageAllRuntime(t, tc, map[string]string{"page-all": "true", "page-limit": "2"}, func(_ *http.Request, call int) map[string]interface{} {
				return map[string]interface{}{"items": []interface{}{tc.makeRawItem(fmt.Sprintf("item-%d", call))}, "has_more": true, "page_token": fmt.Sprintf("token-%d", call), "total": 10}
			})
			if err := tc.shortcut.Execute(context.Background(), runtime); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if *calls != 2 {
				t.Fatalf("API calls = %d, want 2", *calls)
			}
			data := listPageAllOutputData(t, runtime)
			assertListPageAllOrder(t, data, tc, "item-1", "item-2")
			if hasMore, _ := data["has_more"].(bool); !hasMore {
				t.Fatal("has_more = false, want true for incomplete result")
			}
			if token, _ := data["page_token"].(string); token != "token-2" {
				t.Fatalf("page_token = %q, want token-2", token)
			}
			if _, exists := data["pages"]; exists {
				t.Fatalf("output shape changed: unexpected pages field in %#v", data)
			}
			assertListPaginationMeta(t, runtime, false, 2, 2, "token-2")
			stderr := runtime.IO().ErrOut.(*bytes.Buffer).String()
			for _, forbidden := range []string{"[page", "reached page limit", "result is incomplete", "Increase --page-limit"} {
				if strings.Contains(stderr, forbidden) {
					t.Fatalf("stderr contains business pagination warning %q: %s", forbidden, stderr)
				}
			}
			stdout := runtime.IO().Out.(*bytes.Buffer).String()
			for _, forbidden := range []string{"[pagination]", "result is incomplete", "Increase --page-limit"} {
				if strings.Contains(stdout, forbidden) {
					t.Fatalf("stdout contains pagination notice %q: %s", forbidden, stdout)
				}
			}
		})
	}
}

func TestIMListPageAllLateFailurePreservesPartialData(t *testing.T) {
	for _, tc := range listPageAllCases() {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			transport := shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.Method != tc.method || req.URL.Path != tc.path {
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
				}
				calls++
				if calls == 1 {
					return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
						"code": 0,
						"data": map[string]interface{}{
							"items":      []interface{}{tc.makeRawItem("first")},
							"has_more":   true,
							"page_token": "resume-next",
						},
					}), nil
				}
				return nil, errors.New("late page transport failure")
			})

			flags := mergeListPageAllFlags(tc.baseFlags, map[string]string{
				"page-all":   "true",
				"page-delay": "0",
			})
			if tc.name == "chat-list" || tc.name == "chat-search" {
				flags["exclude-muted"] = "true"
			}
			runtime := newUserShortcutRuntime(t, transport)
			runtime.Cmd = newListPageAllCommand(t, tc.shortcut, flags)
			runtime.Format = "json"
			attachFullIMReadSession(t, runtime, tc.shortcut.Command)

			if err := tc.shortcut.Execute(context.Background(), runtime); err != nil {
				t.Fatalf("Execute() error = %v, want partial result emitted through contract", err)
			}
			if calls != 2 {
				t.Fatalf("API calls = %d, want 2", calls)
			}

			envelope := listPageAllOutputEnvelope(t, runtime)
			if ok, _ := envelope["ok"].(bool); ok {
				t.Fatalf("ok = true, want partial failure: %#v", envelope)
			}
			data, _ := envelope["data"].(map[string]interface{})
			assertListPageAllOrder(t, data, tc, "first")
			if tc.name == "chat-list" || tc.name == "chat-search" {
				filter, _ := data["filter"].(map[string]interface{})
				if hint, _ := filter["hint"].(string); !strings.Contains(hint, "partial results are unfiltered") {
					t.Fatalf("filter = %#v, want explicit incomplete-read skip hint", filter)
				}
			}
			assertListPaginationMeta(t, runtime, false, 1, 1, "resume-next")
			notice, _ := envelope["_notice"].(map[string]interface{})
			readNotice, _ := notice["im_read"].(map[string]interface{})
			if readNotice["stop_reason"] != "transport_error" {
				t.Fatalf("stop_reason = %#v, want transport_error", readNotice["stop_reason"])
			}
			problem, _ := envelope["error"].(map[string]interface{})
			if problem["type"] != string(errs.CategoryNetwork) {
				t.Fatalf("error = %#v, want network problem", problem)
			}
		})
	}
}

func TestIMListPageAllContinuesFromExplicitPageToken(t *testing.T) {
	for _, tc := range listPageAllCases() {
		t.Run(tc.name, func(t *testing.T) {
			var requestTokens []string
			runtime, calls := newListPageAllRuntime(t, tc, map[string]string{"page-all": "true", "page-token": "resume"}, func(req *http.Request, call int) map[string]interface{} {
				requestTokens = append(requestTokens, req.URL.Query().Get("page_token"))
				switch call {
				case 1:
					return map[string]interface{}{"items": []interface{}{tc.makeRawItem("first")}, "has_more": true, "page_token": "next_1", "total": 3}
				case 2:
					return map[string]interface{}{"items": []interface{}{tc.makeRawItem("second")}, "has_more": true, "page_token": "next_2", "total": 3}
				default:
					return map[string]interface{}{"items": []interface{}{tc.makeRawItem("third")}, "has_more": false, "page_token": "final", "total": 3}
				}
			})
			if err := tc.shortcut.Execute(context.Background(), runtime); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if *calls != 3 || !reflect.DeepEqual(requestTokens, []string{"resume", "next_1", "next_2"}) {
				t.Fatalf("calls=%d page tokens=%v, want [resume next_1 next_2]", *calls, requestTokens)
			}
			data := listPageAllOutputData(t, runtime)
			assertListPageAllOrder(t, data, tc, "first", "second", "third")
			assertListPaginationMeta(t, runtime, true, 3, 3, "")
		})
	}
}

func TestIMListSinglePageUsesUnifiedPaginationMeta(t *testing.T) {
	for _, tc := range listPageAllCases() {
		t.Run(tc.name, func(t *testing.T) {
			runtime, calls := newListPageAllRuntime(t, tc, nil, func(_ *http.Request, _ int) map[string]interface{} {
				return map[string]interface{}{
					"items":      []interface{}{tc.makeRawItem("only")},
					"has_more":   true,
					"page_token": "next",
					"total":      1,
				}
			})
			if err := tc.shortcut.Execute(context.Background(), runtime); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if *calls != 1 {
				t.Fatalf("API calls = %d, want 1", *calls)
			}
			assertListPaginationMeta(t, runtime, false, 1, 1, "next")
			if stderr := runtime.IO().ErrOut.(*bytes.Buffer).String(); stderr != "" {
				t.Fatalf("single-page call wrote pagination warning/progress: %q", stderr)
			}
		})
	}
}

func TestChatListRecordFormatsKeepStdoutPureAndReportPagination(t *testing.T) {
	var tc listPageAllCase
	for _, candidate := range listPageAllCases() {
		if candidate.name == "chat-list" {
			tc = candidate
			break
		}
	}
	for _, format := range []string{"ndjson", "csv"} {
		t.Run(format, func(t *testing.T) {
			runtime, calls := newListPageAllRuntime(t, tc, map[string]string{"page-all": "true"}, func(_ *http.Request, call int) map[string]interface{} {
				if call == 1 {
					return map[string]interface{}{
						"items":      []interface{}{tc.makeRawItem("first")},
						"has_more":   true,
						"page_token": "next",
					}
				}
				return map[string]interface{}{
					"items":      []interface{}{tc.makeRawItem("second")},
					"has_more":   false,
					"page_token": "final",
				}
			})
			runtime.Format = format
			// Even an interactive stderr must remain a pure JSONL diagnostics
			// stream for record-oriented stdout formats.
			runtime.IO().StderrIsTerminal = true

			if err := tc.shortcut.Execute(context.Background(), runtime); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if *calls != 2 {
				t.Fatalf("API calls = %d, want 2", *calls)
			}
			stdout := runtime.IO().Out.(*bytes.Buffer).String()
			if !strings.Contains(stdout, "first") || !strings.Contains(stdout, "second") {
				t.Fatalf("%s stdout = %q, want both chat records", format, stdout)
			}
			if strings.Contains(stdout, "_diagnostic") || strings.Contains(stdout, "next_token") {
				t.Fatalf("%s stdout contains pagination metadata: %q", format, stdout)
			}
			switch format {
			case "ndjson":
				decoder := json.NewDecoder(strings.NewReader(stdout))
				count := 0
				for {
					var record map[string]interface{}
					if err := decoder.Decode(&record); err != nil {
						if err == io.EOF {
							break
						}
						t.Fatalf("decode NDJSON record: %v", err)
					}
					if _, wrapped := record["chats"]; wrapped {
						t.Fatalf("NDJSON emitted aggregate wrapper: %#v", record)
					}
					count++
				}
				if count != 2 {
					t.Fatalf("NDJSON record count = %d, want 2", count)
				}
			case "csv":
				records, err := csv.NewReader(strings.NewReader(stdout)).ReadAll()
				if err != nil {
					t.Fatalf("decode CSV: %v", err)
				}
				if len(records) != 3 {
					t.Fatalf("CSV rows = %d, want header + 2 records", len(records))
				}
			}

			var diagnostic map[string]interface{}
			stderr := runtime.IO().ErrOut.(*bytes.Buffer).Bytes()
			if err := json.Unmarshal(bytes.TrimSpace(stderr), &diagnostic); err != nil {
				t.Fatalf("decode stderr diagnostic %q: %v", stderr, err)
			}
			if diagnostic["_diagnostic"] != "pagination" || diagnostic["complete"] != true || diagnostic["pages"] != float64(2) || diagnostic["items"] != float64(2) {
				t.Fatalf("pagination diagnostic = %#v", diagnostic)
			}
			if _, exists := diagnostic["next_token"]; exists {
				t.Fatalf("complete pagination diagnostic has next_token: %#v", diagnostic)
			}
		})
	}
}

func TestIMListPageLimitValidation(t *testing.T) {
	for _, tc := range listPageAllCases() {
		for _, limit := range []string{"-1", strconv.Itoa(imReadMaxPageLimit + 1)} {
			t.Run(tc.name+"/"+limit, func(t *testing.T) {
				runtime, _ := newListPageAllRuntime(t, tc, map[string]string{"page-limit": limit}, func(_ *http.Request, _ int) map[string]interface{} {
					t.Fatal("validation must fail before an API request")
					return nil
				})
				err := tc.shortcut.Validate(context.Background(), runtime)
				assertValidationError(t, tc.name, err, "--page-limit")
			})
		}
	}
}

func TestIMListPageDelayValidation(t *testing.T) {
	for _, tc := range listPageAllCases() {
		for _, delay := range []string{"-1", "60001"} {
			t.Run(tc.name+"/"+delay, func(t *testing.T) {
				runtime, _ := newListPageAllRuntime(t, tc, map[string]string{"page-delay": delay}, func(_ *http.Request, _ int) map[string]interface{} {
					t.Fatal("validation must fail before an API request")
					return nil
				})
				err := tc.shortcut.Validate(context.Background(), runtime)
				assertValidationError(t, tc.name, err, "--page-delay")
			})
		}
	}
}

func TestIMListPageAllDryRunAndFlagSurface(t *testing.T) {
	for _, tc := range listPageAllCases() {
		t.Run(tc.name, func(t *testing.T) {
			runtime, _ := newListPageAllRuntime(t, tc, map[string]string{"page-all": "true"}, func(_ *http.Request, _ int) map[string]interface{} {
				t.Fatal("dry-run must not make an API request")
				return nil
			})
			dryRun := mustMarshalDryRun(t, tc.shortcut.DryRun(context.Background(), runtime))
			var dryRunData map[string]interface{}
			if err := json.Unmarshal([]byte(dryRun), &dryRunData); err != nil {
				t.Fatalf("decode dry-run: %v", err)
			}
			if description, _ := dryRunData["description"].(string); description != pageAllDryRunDescription {
				t.Fatalf("dry-run missing auto-pagination description: %s", dryRun)
			}

			flags := make(map[string]common.Flag)
			for _, flag := range tc.shortcut.Flags {
				flags[flag.Name] = flag
			}
			if flag := flags[common.PageAllFlagName]; flag.Type != "bool" || flag.Desc != common.PageAllFlags(imPageAllPolicy)[0].Desc {
				t.Fatalf("page-all flag = %#v", flag)
			}
			if flag := flags["page-limit"]; flag.Type != "int" || flag.Default != "10" || !strings.Contains(flag.Desc, "1-1000") {
				t.Fatalf("page-limit flag = %#v", flag)
			}
			if flag := flags["page-delay"]; flag.Type != "int" || flag.Default != "200" || !strings.Contains(flag.Desc, "0-60000") {
				t.Fatalf("page-delay flag = %#v", flag)
			}
		})
	}
}

func TestMessageListPageAllEnrichesMergedMessagesOnce(t *testing.T) {
	messageItem := func(id string) interface{} {
		return map[string]interface{}{
			"message_id":  id,
			"msg_type":    "text",
			"body":        map[string]interface{}{"content": fmt.Sprintf(`{"text":%q}`, id)},
			"create_time": "0",
		}
	}
	tests := []struct {
		name     string
		shortcut common.Shortcut
		flags    map[string]string
	}{
		{name: "chat-messages-list", shortcut: ImChatMessageList, flags: map[string]string{"chat-id": "oc_test", "page-all": "true"}},
		{name: "threads-messages-list", shortcut: ImThreadsMessagesList, flags: map[string]string{"thread": "omt_test", "page-all": "true"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pageCalls := 0
			reactionCalls := 0
			reactionQueries := 0
			transport := shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/open-apis/im/v1/messages":
					pageCalls++
					if pageCalls == 1 {
						return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
							"code": 0,
							"data": map[string]interface{}{"items": []interface{}{messageItem("first")}, "has_more": true, "page_token": "next"},
						}), nil
					}
					return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
						"code": 0,
						"data": map[string]interface{}{"items": []interface{}{messageItem("second")}, "has_more": false, "page_token": "final"},
					}), nil
				case "/open-apis/im/v1/messages/reactions/batch_query":
					reactionCalls++
					var body struct {
						Queries []map[string]interface{} `json:"queries"`
					}
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Fatalf("decode reaction request: %v", err)
					}
					reactionQueries = len(body.Queries)
					return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
						"code": 0,
						"data": map[string]interface{}{
							"success_msg_reaction_counts":  []interface{}{},
							"success_msg_reaction_details": []interface{}{},
						},
					}), nil
				default:
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
					return nil, nil
				}
			})
			runtime := newUserShortcutRuntime(t, transport)
			runtime.Cmd = newListPageAllCommand(t, tc.shortcut, tc.flags)
			runtime.Format = "json"

			if err := tc.shortcut.Execute(context.Background(), runtime); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if pageCalls != 2 {
				t.Fatalf("message page calls = %d, want 2", pageCalls)
			}
			if reactionCalls != 1 {
				t.Fatalf("reaction batch calls = %d, want 1 after page merge", reactionCalls)
			}
			if reactionQueries != 2 {
				t.Fatalf("reaction query count = %d, want both merged messages", reactionQueries)
			}
			assertListPaginationMeta(t, runtime, true, 2, 2, "")
		})
	}
}

func TestChatListPageAllFiltersMergedChatsOnce(t *testing.T) {
	tests := []struct {
		name     string
		shortcut common.Shortcut
		path     string
		flags    map[string]string
		makeItem func(string) interface{}
	}{
		{
			name: "chat-list", shortcut: ImChatList, path: "/open-apis/im/v1/chats",
			flags: map[string]string{"page-all": "true", "exclude-muted": "true"},
			makeItem: func(id string) interface{} {
				return map[string]interface{}{"chat_id": id, "name": id, "chat_mode": "group"}
			},
		},
		{
			name: "chat-search", shortcut: ImChatSearch, path: "/open-apis/im/v2/chats/search",
			flags: map[string]string{"query": "team", "page-all": "true", "exclude-muted": "true"},
			makeItem: func(id string) interface{} {
				return map[string]interface{}{"meta_data": map[string]interface{}{"chat_id": id, "name": id, "chat_mode": "group"}}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pageCalls := 0
			muteCalls := 0
			muteChatIDs := 0
			transport := shortcutRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case tc.path:
					pageCalls++
					if pageCalls == 1 {
						return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
							"code": 0,
							"data": map[string]interface{}{"items": []interface{}{tc.makeItem("oc_first")}, "has_more": true, "page_token": "next"},
						}), nil
					}
					return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
						"code": 0,
						"data": map[string]interface{}{"items": []interface{}{tc.makeItem("oc_second")}, "has_more": false, "page_token": "final"},
					}), nil
				case BatchGetMuteStatusPath:
					muteCalls++
					var body struct {
						ChatIDs []string `json:"chat_ids"`
					}
					if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
						t.Fatalf("decode mute-status request: %v", err)
					}
					muteChatIDs = len(body.ChatIDs)
					return shortcutJSONResponse(http.StatusOK, map[string]interface{}{
						"code": 0,
						"data": map[string]interface{}{
							"items": []interface{}{
								map[string]interface{}{"chat_id": "oc_first", "is_muted": false},
								map[string]interface{}{"chat_id": "oc_second", "is_muted": false},
							},
						},
					}), nil
				default:
					t.Fatalf("unexpected request: %s %s", req.Method, req.URL.String())
					return nil, nil
				}
			})
			runtime := newUserShortcutRuntime(t, transport)
			runtime.Cmd = newListPageAllCommand(t, tc.shortcut, tc.flags)
			runtime.Format = "json"

			if err := tc.shortcut.Execute(context.Background(), runtime); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if pageCalls != 2 {
				t.Fatalf("chat page calls = %d, want 2", pageCalls)
			}
			if muteCalls != 1 {
				t.Fatalf("mute-status calls = %d, want 1 after page merge", muteCalls)
			}
			if muteChatIDs != 2 {
				t.Fatalf("mute-status chat ID count = %d, want both merged chats", muteChatIDs)
			}
			assertListPaginationMeta(t, runtime, true, 2, 2, "")
		})
	}
}

func TestChatSearchPageAllRetainsNoticeFromEarlierPage(t *testing.T) {
	const notice = "The query was truncated before search."
	var searchCase listPageAllCase
	for _, tc := range listPageAllCases() {
		if tc.name == "chat-search" {
			searchCase = tc
			break
		}
	}

	runtime, calls := newListPageAllRuntime(t, searchCase, map[string]string{"page-all": "true"}, func(_ *http.Request, call int) map[string]interface{} {
		if call == 1 {
			return map[string]interface{}{
				"items":      []interface{}{searchCase.makeRawItem("oc_first")},
				"notice":     notice,
				"total":      2,
				"has_more":   true,
				"page_token": "next",
			}
		}
		return map[string]interface{}{
			"items":      []interface{}{searchCase.makeRawItem("oc_second")},
			"total":      2,
			"has_more":   false,
			"page_token": "final",
		}
	})
	if err := ImChatSearch.Execute(context.Background(), runtime); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if *calls != 2 {
		t.Fatalf("API calls = %d, want 2", *calls)
	}
	data := listPageAllOutputData(t, runtime)
	if got, _ := data["notice"].(string); got != notice {
		t.Fatalf("notice = %q, want %q", got, notice)
	}
	assertListPaginationMeta(t, runtime, true, 2, 2, "")
}
