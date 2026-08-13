// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
	"github.com/larksuite/cli/internal/agents/agenttest"
	"github.com/larksuite/cli/internal/core"
)

func init() { iagents.Register(Provider()) }

type apiCall struct {
	method string
	path   string
	query  map[string]string
	body   any
}

type fakeRuntime struct {
	agentID   string
	bot       bool
	params    map[string]string
	responses []json.RawMessage
	errs      []error
	calls     []apiCall
}

func (f *fakeRuntime) AgentID() string           { return f.agentID }
func (f *fakeRuntime) IsBot() bool               { return f.bot }
func (f *fakeRuntime) Params() map[string]string { return f.params }
func (f *fakeRuntime) CallMultipart(context.Context, string, string, map[string]string, []iagents.FilePart) (json.RawMessage, error) {
	panic("base provider must not upload files")
}
func (f *fakeRuntime) CallAPI(_ context.Context, method, path string, query map[string]string, body any) (json.RawMessage, error) {
	q := make(map[string]string, len(query))
	for k, v := range query {
		q[k] = v
	}
	f.calls = append(f.calls, apiCall{method: method, path: path, query: q, body: body})
	if len(f.errs) > 0 {
		err := f.errs[0]
		f.errs = f.errs[1:]
		if err != nil {
			return nil, err
		}
	}
	if len(f.responses) == 0 {
		return nil, nil
	}
	raw := f.responses[0]
	f.responses = f.responses[1:]
	return raw, nil
}

// dataResponse mirrors Runtime.CallAPI: the OpenAPI envelope has already been
// checked and its data field is returned as raw JSON. Base's api.map="data"
// response is an object/array here, not a JSON-encoded string.
func dataResponse(t *testing.T, data string) json.RawMessage {
	t.Helper()
	return json.RawMessage(data)
}

func bodyMap(t *testing.T, body any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func problem(t *testing.T, err error, category errs.Category, subtype errs.Subtype) {
	t.Helper()
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("want typed error, got %T: %v", err, err)
	}
	if p.Category != category || p.Subtype != subtype {
		t.Fatalf("problem=(%s,%s), want (%s,%s)", p.Category, p.Subtype, category, subtype)
	}
}

func TestProviderConformance(t *testing.T) {
	agenttest.RunConformance(t, "base", "assistant")
	p := Provider()
	wantScopes := []string{"base:agent:execute"}
	if !reflect.DeepEqual(p.ScopesForIdentity(iagents.IdentityUser), wantScopes) {
		t.Fatalf("user scopes=%v", p.ScopesForIdentity(iagents.IdentityUser))
	}
	if got := p.ScopesForIdentity(iagents.IdentityBot); got != nil {
		t.Fatalf("bot scopes should be nil (user-only provider), got %v", got)
	}
	if len(p.Catalog) != 1 || p.Catalog[0].ID != "assistant" {
		t.Fatalf("catalog=%+v", p.Catalog)
	}
	spec := p.Catalog[0]
	wantDescription := "Handles multi-component Base construction and restructuring, plus user-facing data retrieval and analysis. Use Base CLI shortcuts for a single atomic edit or record create, update, or delete."
	if spec.Name != "Base Assistant" || spec.Description != wantDescription {
		t.Fatalf("public card metadata name=%q description=%q", spec.Name, spec.Description)
	}
	if len(spec.Skills) != 1 || spec.Skills[0].ID != "base_assistant" ||
		spec.Skills[0].Name != "Build and analyze a Base" || len(spec.Skills[0].Examples) != 3 {
		t.Fatalf("public card skills=%+v", spec.Skills)
	}
	for _, forbidden := range []string{"base_build", "base_analyze", "Building Agent", "Analysis Agent"} {
		if strings.Contains(spec.Description, forbidden) ||
			strings.Contains(spec.Skills[0].ID, forbidden) ||
			strings.Contains(spec.Skills[0].Name, forbidden) ||
			strings.Contains(strings.Join(spec.Skills[0].Examples, "\n"), forbidden) {
			t.Fatalf("public card exposes internal capability name %q: %+v", forbidden, spec)
		}
	}
	for _, op := range spec.Ops() {
		if !op.Wired {
			continue
		}
		foundBaseToken := false
		for _, param := range op.Params {
			if param.Name != "base_token" {
				continue
			}
			foundBaseToken = true
			if param.Desc != baseTokenParamDescription {
				t.Fatalf("operation %s base_token description=%q, want %q", op.Verb, param.Desc, baseTokenParamDescription)
			}
		}
		if !foundBaseToken {
			t.Fatalf("operation %s must declare base_token guidance", op.Verb)
		}
	}
	for _, brand := range []core.LarkBrand{core.BrandFeishu, core.BrandLark} {
		caps := iagents.DeriveCapabilities(&spec, brand)
		if !caps.TaskGet || !caps.TaskList || !caps.TaskCancel || !caps.ContextList || !caps.ContextGet || !caps.ContextDelete {
			t.Fatalf("brand=%s missing required capability: %+v", brand, caps)
		}
		if !caps.InputRequired {
			t.Fatalf("brand=%s must advertise input_required: %+v", brand, caps)
		}
		if caps.FileInput || caps.ArtifactDownload {
			t.Fatalf("brand=%s unsupported capability advertised: %+v", brand, caps)
		}
	}
	agenttest.CheckParamsBinding[sendParams](t, &spec, iagents.VerbSend)
	agenttest.CheckParamsBinding[getTaskParams](t, &spec, iagents.VerbTaskGet)
	agenttest.CheckParamsBinding[listTasksParams](t, &spec, iagents.VerbTaskList)
	agenttest.CheckParamsBinding[listContextsParams](t, &spec, iagents.VerbContextList)
}

func TestAgentRootUsesPublicBaseV3Prefix(t *testing.T) {
	got := agentRoot("basc/token")
	want := "/open-apis/base/v3/bases/basc%2Ftoken/ai/agents/assistant"
	if got != want {
		t.Fatalf("agentRoot()=%q want %q", got, want)
	}
}

func TestSendBuildsAdapterRequest(t *testing.T) {
	rt := &fakeRuntime{
		agentID:   "assistant",
		params:    map[string]string{"base_token": "basc/token", "active_table_id": "tbl1"},
		responses: []json.RawMessage{dataResponse(t, `{"schema_version":1,"task_id":"task-1","context_id":"ctx-1","status":"pending","created_at":"1710000000","updated_at":"1710000060","outputs":[]}`)},
	}
	task, err := assistantSpec.Send.Handler(context.Background(), rt, iagents.SendInput{Text: "build a dashboard"})
	if err != nil {
		t.Fatal(err)
	}
	if task.TaskID != "task-1" || task.ContextID != "ctx-1" || task.State != iagents.StateSubmitted || task.IsTerminal {
		t.Fatalf("task=%+v", task)
	}
	if task.CreatedAt != "2024-03-09T16:00:00Z" || task.UpdatedAt != "2024-03-09T16:01:00Z" {
		t.Fatalf("task timestamps=%+v", task)
	}
	if len(rt.calls) != 1 {
		t.Fatalf("calls=%d", len(rt.calls))
	}
	call := rt.calls[0]
	if call.method != "POST" || call.path != "/open-apis/base/v3/bases/basc%2Ftoken/ai/agents/assistant/messages" {
		t.Fatalf("call=%s %s", call.method, call.path)
	}
	body := bodyMap(t, call.body)
	if body["context_id"] != nil || body["task_id"] != nil {
		t.Fatalf("fresh send must omit context/task: %#v", body)
	}
	if body["idempotency_key"] == "" || body["idempotency_key"] == nil {
		t.Fatalf("missing idempotency key: %#v", body)
	}
	params := body["params"].(map[string]any)
	if params["active_table_id"] != "tbl1" {
		t.Fatalf("params=%#v", params)
	}
	metadata := body["metadata"].(map[string]any)
	if metadata["channel"] != "lark_cli" {
		t.Fatalf("metadata=%#v", metadata)
	}
	encoded, _ := json.Marshal(body)
	for _, forbidden := range []string{"preJobId", "skill_id", "action_code", "scene_code", "sidebar_tools", "memberId", "UserID", "TenantID", "AppID"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("request leaked forbidden field %q: %s", forbidden, encoded)
		}
	}
}

func TestAgentBizErrPayloads(t *testing.T) {
	t.Run("send and get task carry biz error", func(t *testing.T) {
		rt := &fakeRuntime{
			agentID: "assistant",
			params:  map[string]string{"base_token": "b1"},
			responses: []json.RawMessage{
				dataResponse(t, `{"schema_version":1,"task_id":"1001","context_id":"c1","status":"failed","BizErrCode":800004907,"BizErrMessage":"[MOCK] create job rate limit","outputs":[]}`),
				dataResponse(t, `{"schema_version":1,"task_id":"1001","context_id":"c1","status":"failed","BizErrCode":"800004907","BizErrMessage":"[MOCK] create job rate limit","outputs":[]}`),
			},
		}
		task, err := assistantSpec.Send.Handler(context.Background(), rt, iagents.SendInput{Text: "x"})
		if err != nil {
			t.Fatal(err)
		}
		if task.BizError == nil || task.BizError.Code != "800004907" || task.BizError.Message != "[MOCK] create job rate limit" {
			t.Fatalf("send task biz_error=%+v", task.BizError)
		}
		task, err = assistantSpec.GetTask.Handler(context.Background(), rt, "1001")
		if err != nil {
			t.Fatal(err)
		}
		if task.BizError == nil || task.BizError.Code != "800004907" || task.BizError.Message != "[MOCK] create job rate limit" {
			t.Fatalf("get task biz_error=%+v", task.BizError)
		}
	})

	t.Run("get task accepts adapter task envelope with snake biz error", func(t *testing.T) {
		rt := &fakeRuntime{
			params: map[string]string{"base_token": "b1"},
			responses: []json.RawMessage{
				dataResponse(t, `{"biz_err_code":5000,"biz_err_message":"[MOCK] quota limit exceeded","task":{"schema_version":1,"task_id":"1001","context_id":"c1","status":"completed","created_at":"1786347806","updated_at":"1786347832","outputs":[]}}`),
			},
		}
		task, err := assistantSpec.GetTask.Handler(context.Background(), rt, "1001")
		if err != nil {
			t.Fatal(err)
		}
		if task.State != iagents.StateCompleted || !task.IsTerminal {
			t.Fatalf("task state=%s terminal=%t", task.State, task.IsTerminal)
		}
		if task.BizError == nil || task.BizError.Code != "5000" || task.BizError.Message != "[MOCK] quota limit exceeded" {
			t.Fatalf("task biz_error=%+v", task.BizError)
		}
	})

	t.Run("list task item carries biz error", func(t *testing.T) {
		rt := &fakeRuntime{
			params: map[string]string{"base_token": "b1"},
			responses: []json.RawMessage{
				dataResponse(t, `{"tasks":[{"task_id":"1001","context_id":"c1","state":"failed","BizErrCode":800004907,"BizErrMessage":"rate limited"}],"has_more":false}`),
			},
		}
		tasks, _, err := assistantSpec.ListTasks.Handler(context.Background(), rt, "c1", iagents.PageParams{})
		if err != nil {
			t.Fatal(err)
		}
		if len(tasks) != 1 || tasks[0].BizError == nil || tasks[0].BizError.Code != "800004907" || tasks[0].BizError.Message != "rate limited" {
			t.Fatalf("tasks=%+v", tasks)
		}
	})

	t.Run("context list and get carry biz error", func(t *testing.T) {
		rt := &fakeRuntime{
			params: map[string]string{"base_token": "b1"},
			responses: []json.RawMessage{
				dataResponse(t, `{"contexts":[{"context_id":"c1","title":"ctx","BizErrCode":800004907,"BizErrMessage":"context rate limited"}],"has_more":false}`),
				dataResponse(t, `{"context_id":"c1","BizErrCode":800004907,"BizErrMessage":"context get rate limited","tasks":[]}`),
			},
		}
		contexts, _, err := assistantSpec.ListContexts.Handler(context.Background(), rt, iagents.PageParams{})
		if err != nil {
			t.Fatal(err)
		}
		if len(contexts) != 1 || contexts[0].BizError == nil || contexts[0].BizError.Code != "800004907" || contexts[0].BizError.Message != "context rate limited" {
			t.Fatalf("contexts=%+v", contexts)
		}
		detail, err := assistantSpec.GetContext.Handler(context.Background(), rt, "c1")
		if err != nil {
			t.Fatal(err)
		}
		if detail.BizError == nil || detail.BizError.Code != "800004907" || detail.BizError.Message != "context get rate limited" {
			t.Fatalf("detail=%+v", detail)
		}
	})
}

func TestSendRejectsJSONStringData(t *testing.T) {
	encoded, err := json.Marshal(`{"schema_version":1,"task_id":"task-1","context_id":"ctx-1","status":"pending","outputs":[]}`)
	if err != nil {
		t.Fatal(err)
	}
	rt := &fakeRuntime{
		agentID:   "assistant",
		params:    map[string]string{"base_token": "b1"},
		responses: []json.RawMessage{encoded},
	}
	_, err = assistantSpec.Send.Handler(context.Background(), rt, iagents.SendInput{Text: "hello"})
	problem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse)
}

func TestSendContinuationAndIdempotency(t *testing.T) {
	rt := &fakeRuntime{
		agentID: "assistant",
		params:  map[string]string{"base_token": "b1"},
		responses: []json.RawMessage{
			dataResponse(t, `{"schema_version":1,"task_id":"t1","context_id":"c1","status":"running","outputs":[]}`),
			dataResponse(t, `{"schema_version":1,"task_id":"t1","context_id":"c1","status":"running","outputs":[]}`),
		},
	}
	in := iagents.SendInput{Text: "more", ContextID: "c1", TaskID: "t1"}
	if _, err := assistantSpec.Send.Handler(context.Background(), rt, in); err != nil {
		t.Fatal(err)
	}
	if _, err := assistantSpec.Send.Handler(context.Background(), rt, in); err != nil {
		t.Fatal(err)
	}
	b1, b2 := bodyMap(t, rt.calls[0].body), bodyMap(t, rt.calls[1].body)
	if b1["context_id"] != "c1" || b1["task_id"] != "t1" {
		t.Fatalf("continuation body=%#v", b1)
	}
	if b1["idempotency_key"] == b2["idempotency_key"] {
		t.Fatalf("logical sends reused key %q", b1["idempotency_key"])
	}
}

// TestSendAnswerBuildsDataPart is the adapter call-capture test the plan places
// here instead of dry-run E2E (dry-run returns before the handler): answering an
// input_required group must POST a single kind=answers DataPart, no text part, a
// deterministic message_id == idempotency_key, and preserve context/task ids.
func TestSendAnswerBuildsDataPart(t *testing.T) {
	rt := &fakeRuntime{
		agentID:   "assistant",
		params:    map[string]string{"base_token": "b1"},
		responses: []json.RawMessage{dataResponse(t, `{"schema_version":1,"task_id":"t1","context_id":"c1","status":"running","outputs":[]}`)},
	}
	in := iagents.SendInput{
		ContextID: "c1",
		TaskID:    "t1",
		Answers: map[string][]string{
			"q_scene":     {"opt_1"},
			"q_metric":    {"opt_a", "opt_b"},
			"q_note.text": {"group by calendar month"},
		},
	}
	if _, err := assistantSpec.Send.Handler(context.Background(), rt, in); err != nil {
		t.Fatal(err)
	}
	body := bodyMap(t, rt.calls[0].body)
	if body["context_id"] != "c1" || body["task_id"] != "t1" {
		t.Fatalf("answer body=%#v", body)
	}
	key, _ := body["idempotency_key"].(string)
	if !strings.HasPrefix(key, "answer_") {
		t.Fatalf("idempotency key=%q", key)
	}
	message, ok := body["message"].(map[string]any)
	if !ok {
		t.Fatalf("message=%#v", body["message"])
	}
	if message["message_id"] != key {
		t.Fatalf("message_id %v != idempotency_key %v", message["message_id"], key)
	}
	if message["role"] != "user" {
		t.Fatalf("role=%v", message["role"])
	}
	parts, ok := message["parts"].([]any)
	if !ok || len(parts) != 1 {
		t.Fatalf("parts=%#v", message["parts"])
	}
	part := parts[0].(map[string]any)
	if part["type"] != "data" || part["text"] != nil {
		t.Fatalf("answer must be a lone data part: %#v", part)
	}
	data := part["data"].(map[string]any)
	if data["kind"] != "answers" || data["schema_version"] != float64(1) {
		t.Fatalf("data=%#v", data)
	}
	payload := data["payload"].(map[string]any)
	answers := payload["answers"].(map[string]any)
	if got := answers["q_metric"].([]any); len(got) != 2 || got[0] != "opt_a" || got[1] != "opt_b" {
		t.Fatalf("multi-select answers=%#v", got)
	}
	if got := answers["q_note.text"].([]any); len(got) != 1 || got[0] != "group by calendar month" {
		t.Fatalf("text answer=%#v", got)
	}
}

// TestDeterministicAnswerIDCanonicalization freezes the id contract: the same
// logical answer hashes the same regardless of choice-value order or argv order,
// a different answer hashes differently, and the id is task-scoped.
func TestDeterministicAnswerIDCanonicalization(t *testing.T) {
	base := map[string][]string{"q1": {"opt_a", "opt_b"}, "q2.text": {"note"}}
	reordered := map[string][]string{"q2.text": {"note"}, "q1": {"opt_b", "opt_a"}}
	id1, err := deterministicAnswerID("t1", base)
	if err != nil {
		t.Fatal(err)
	}
	id2, err := deterministicAnswerID("t1", reordered)
	if err != nil {
		t.Fatal(err)
	}
	if id1 != id2 {
		t.Fatalf("same logical answer produced %q != %q", id1, id2)
	}
	different, _ := deterministicAnswerID("t1", map[string][]string{"q1": {"opt_a"}})
	if different == id1 {
		t.Fatal("different answer collided")
	}
	otherTask, _ := deterministicAnswerID("t2", base)
	if otherTask == id1 {
		t.Fatal("id must be task-scoped")
	}
}

func TestSendRejectsAnswerWithText(t *testing.T) {
	rt := &fakeRuntime{params: map[string]string{"base_token": "b1"}}
	in := iagents.SendInput{
		ContextID: "c1",
		TaskID:    "t1",
		Text:      "additional context",
		Answers:   map[string][]string{"q1": {"opt_1"}},
	}
	_, err := assistantSpec.Send.Handler(context.Background(), rt, in)
	problem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument)
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) || validationErr.Param != "--text" {
		t.Fatalf("error=%+v", err)
	}
	if len(rt.calls) != 0 {
		t.Fatal("answers+text must be rejected before any API call")
	}
}

func TestSendRejectsDisabledInputs(t *testing.T) {
	tests := []iagents.SendInput{
		{Text: "x", Files: []string{"a.txt"}},
	}
	for _, in := range tests {
		rt := &fakeRuntime{params: map[string]string{"base_token": "b1"}}
		_, err := assistantSpec.Send.Handler(context.Background(), rt, in)
		problem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument)
		if len(rt.calls) != 0 {
			t.Fatalf("disabled input made API call: %+v", in)
		}
	}
}

func TestSendRejectsBotIdentity(t *testing.T) {
	rt := &fakeRuntime{bot: true, params: map[string]string{"base_token": "b1"}}
	_, err := assistantSpec.Send.Handler(context.Background(), rt, iagents.SendInput{Text: "x"})
	problem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument)
	if len(rt.calls) != 0 {
		t.Fatal("bot identity must be rejected before an API call")
	}
}

func TestGetTaskForwardsContextIDAcrossPolls(t *testing.T) {
	rt := &fakeRuntime{
		params: map[string]string{"base_token": "b1", "context_id": "c1"},
		responses: []json.RawMessage{
			dataResponse(t, `{"schema_version":1,"task_id":"t1","context_id":"c1","status":"running","outputs":[]}`),
			dataResponse(t, `{"schema_version":1,"task_id":"t1","context_id":"c1","status":"completed","outputs":[]}`),
		},
	}

	for range 2 {
		if _, err := assistantSpec.GetTask.Handler(context.Background(), rt, "t1"); err != nil {
			t.Fatal(err)
		}
	}
	if len(rt.calls) != 2 {
		t.Fatalf("calls=%d", len(rt.calls))
	}
	wantQuery := map[string]string{"context_id": "c1"}
	for i, call := range rt.calls {
		if !reflect.DeepEqual(call.query, wantQuery) {
			t.Fatalf("calls[%d].query=%v want %v", i, call.query, wantQuery)
		}
	}
}

func TestTaskHooksAndMapping(t *testing.T) {
	rt := &fakeRuntime{
		params: map[string]string{"base_token": "b1", "state": "running"},
		responses: []json.RawMessage{
			dataResponse(t, `{"schema_version":1,"task_id":"t/1","context_id":"c1","status":"running","outputs":[{"id":"1:text:1","type":"text","text":"ready"}]}`),
			dataResponse(t, `[{"task_id":"t1","context_id":"c1","state":"done","updated_at":1710000060,"summary":"done"}]`),
		},
	}
	task, err := assistantSpec.GetTask.Handler(context.Background(), rt, "t/1")
	if err != nil {
		t.Fatal(err)
	}
	if task.State != iagents.StateWorking || task.IsTerminal || task.Messages[0].Parts[0].Text != "ready" {
		t.Fatalf("task=%+v", task)
	}
	if rt.calls[0].path != "/open-apis/base/v3/bases/b1/ai/agents/assistant/tasks/t%2F1" {
		t.Fatalf("path=%s", rt.calls[0].path)
	}
	list, pageInfo, err := assistantSpec.ListTasks.Handler(context.Background(), rt, "c1", iagents.PageParams{Token: "next", Size: 20})
	if err != nil {
		t.Fatal(err)
	}
	if pageInfo != (iagents.PageInfo{}) {
		t.Fatalf("pageInfo=%+v", pageInfo)
	}
	if len(list) != 1 || list[0].State != iagents.StateCompleted || !list[0].IsTerminal {
		t.Fatalf("list=%+v", list)
	}
	wantQuery := map[string]string{"context_id": "c1", "cursor": "next", "limit": "20", "state": "running"}
	if !reflect.DeepEqual(rt.calls[1].query, wantQuery) {
		t.Fatalf("query=%v want %v", rt.calls[1].query, wantQuery)
	}
}

func TestListTasksRequiresContextID(t *testing.T) {
	for _, contextID := range []string{"", " \t\n"} {
		rt := &fakeRuntime{params: map[string]string{"base_token": "b1"}}
		_, _, err := assistantSpec.ListTasks.Handler(context.Background(), rt, contextID, iagents.PageParams{})
		problem(t, err, errs.CategoryValidation, errs.SubtypeInvalidArgument)

		var validationErr *errs.ValidationError
		if !errors.As(err, &validationErr) || validationErr.Param != "--context-id" {
			t.Fatalf("contextID=%q error=%+v", contextID, err)
		}
		if len(rt.calls) != 0 {
			t.Fatalf("contextID=%q made API calls: %+v", contextID, rt.calls)
		}
	}
}

func TestListHooksMapPaginationEnvelope(t *testing.T) {
	rt := &fakeRuntime{
		params: map[string]string{"base_token": "b1", "state": "done", "status": "active"},
		responses: []json.RawMessage{
			dataResponse(t, `{"tasks":[{"task_id":"t1","context_id":"c1","state":"done","updated_at":1710000060}],"has_more":true,"next_cursor":"task-next"}`),
			dataResponse(t, `{"contexts":[{"context_id":"c1","title":"Quarterly plan","created_at":1710000000,"updated_at":1710000060}],"has_more":true,"next_cursor":"context-next"}`),
		},
	}

	tasks, taskPage, err := assistantSpec.ListTasks.Handler(context.Background(), rt, "c1", iagents.PageParams{Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 1 || taskPage != (iagents.PageInfo{HasMore: true, NextToken: "task-next"}) {
		t.Fatalf("tasks=%+v page=%+v", tasks, taskPage)
	}

	contexts, contextPage, err := assistantSpec.ListContexts.Handler(context.Background(), rt, iagents.PageParams{Size: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(contexts) != 1 || contextPage != (iagents.PageInfo{HasMore: true, NextToken: "context-next"}) {
		t.Fatalf("contexts=%+v page=%+v", contexts, contextPage)
	}
}

func TestListHooksRejectMalformedPaginationEnvelope(t *testing.T) {
	tests := []struct {
		name     string
		payload  string
		contexts bool
	}{
		{name: "null task response", payload: `null`},
		{name: "missing tasks", payload: `{"has_more":false}`},
		{name: "missing task has_more", payload: `{"tasks":[]}`},
		{name: "task cursor missing", payload: `{"tasks":[],"has_more":true}`},
		{name: "unexpected task cursor", payload: `{"tasks":[],"has_more":false,"next_cursor":"next"}`},
		{name: "null contexts", payload: `{"contexts":null,"has_more":false}`, contexts: true},
		{name: "missing contexts", payload: `{"has_more":false}`, contexts: true},
		{name: "missing context has_more", payload: `{"contexts":[]}`, contexts: true},
		{name: "context cursor missing", payload: `{"contexts":[],"has_more":true}`, contexts: true},
		{name: "null context cursor", payload: `{"contexts":[],"has_more":false,"next_cursor":null}`, contexts: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rt := &fakeRuntime{
				params:    map[string]string{"base_token": "b1"},
				responses: []json.RawMessage{dataResponse(t, test.payload)},
			}
			var err error
			if test.contexts {
				_, _, err = assistantSpec.ListContexts.Handler(context.Background(), rt, iagents.PageParams{})
			} else {
				_, _, err = assistantSpec.ListTasks.Handler(context.Background(), rt, "c1", iagents.PageParams{})
			}
			problem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse)
		})
	}
}

func TestUnknownStateAndInvalidPayloadAreTyped(t *testing.T) {
	for _, payload := range []string{`{"schema_version":1,"task_id":"t1","status":"paused","outputs":[]}`, `{not-json`} {
		rt := &fakeRuntime{params: map[string]string{"base_token": "b1"}, responses: []json.RawMessage{dataResponse(t, payload)}}
		_, err := assistantSpec.GetTask.Handler(context.Background(), rt, "t1")
		problem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse)
	}
}

func TestVersionedTaskMapsOutputsAndClarification(t *testing.T) {
	rt := &fakeRuntime{
		params: map[string]string{"base_token": "b1"},
		responses: []json.RawMessage{dataResponse(t, `{
  "schema_version": 1,
  "task_id": "t1",
  "context_id": "c1",
  "status": "waiting_for_input",
  "outputs": [
    {"id":"100:text:1","type":"text","source":"base_agent","group_id":"grp_1","text":"Conclusion first"},
    {"id":"101:data_qa_chart:1","type":"data","source":"base_agent","group_id":"grp_1","data":{"kind":"qa_chart","schema_version":1,"payload":{"chartId":"chart_1","baseId":9007199254740993,"vchartSpec":{"type":"bar"}}}},
    {"id":"102:question:1","type":"clarification","source":"base_agent","group_id":"grp_1","clarification":{
      "id":"clarify_1","title":"Additional information required","required":true,"submitted":false,
      "questions":[{"id":"q_done","type":"text","prompt":"Already answered","required":true,"answered":true,"answer":{"value":"ok"}}],
      "forms":[{"id":"form_1","title":"Advanced settings","questions":[{"id":"q_scene","type":"single_select","prompt":"Select a scenario","required":true,"options":[{"id":"opt_1","label":"Create","description":"Create a new table"}]}],"buttons":[{"id":"btn_skip","kind":"custom","label":"Skip","action_params":"{\"action\":\"skip\"}"}]}],
      "buttons":[{"id":"btn_submit","kind":"custom","style":"primary","label":"Confirm","action_params":"{\"action\":\"submit\"}"}]
    }},
    {"id":"103:artifact:1","type":"artifact","source":"table_agent","group_id":"grp_2","artifact":{"id":"artifact_1","type":"table","title":"Sales table","status":"ready","resource":{"block_id":"block_1","view_id":"view_1"},"revision":128,"metadata":{"base_id":9007199254740993,"init_type":2}}}
  ]
}`)},
	}

	task, err := assistantSpec.GetTask.Handler(context.Background(), rt, "t1")
	if err != nil {
		t.Fatal(err)
	}
	if task.State != iagents.StateInputRequired || task.IsTerminal {
		t.Fatalf("state=%s terminal=%t", task.State, task.IsTerminal)
	}
	if len(task.Messages) != 1 || len(task.Messages[0].Parts) != 2 {
		t.Fatalf("messages=%+v", task.Messages)
	}
	if got := task.Messages[0].Parts[0]; got.Type != "text" || got.Text != "Conclusion first" ||
		got.OutputID != "100:text:1" || got.Source != "base_agent" || got.GroupID != "grp_1" {
		t.Fatalf("text part=%+v", got)
	}
	data, ok := task.Messages[0].Parts[1].Data.(map[string]interface{})
	if !ok || data["kind"] != "qa_chart" {
		t.Fatalf("data part=%+v", task.Messages[0].Parts[1])
	}
	payload, ok := data["payload"].(json.RawMessage)
	if !ok || !strings.Contains(string(payload), `"chartId":"chart_1"`) ||
		!strings.Contains(string(payload), `"baseId":9007199254740993`) {
		t.Fatalf("payload=%#v", data["payload"])
	}
	if got := task.Messages[0].Parts[1]; got.OutputID != "101:data_qa_chart:1" || got.Source != "base_agent" || got.GroupID != "grp_1" {
		t.Fatalf("data part metadata=%+v", got)
	}
	if task.InputRequired == nil || len(task.InputRequired.Questions) != 4 {
		t.Fatalf("input_required=%+v", task.InputRequired)
	}
	if task.InputRequired.Label != "Additional information required" {
		t.Fatalf("group label=%q", task.InputRequired.Label)
	}
	// Content questions come first (including preselected q_done), then the synthetic
	// action buttons: top-level button set, then the form's button set.
	if got := task.InputRequired.Questions[0]; got.QuestionID != "q_done" || got.Question != "Already answered" {
		t.Fatalf("preselected question=%+v", got)
	}
	question := task.InputRequired.Questions[1]
	if question.QuestionID != "q_scene" || question.MultiSelect || len(question.Options) != 1 {
		t.Fatalf("question=%+v", question)
	}
	if question.Question != "Advanced settings: Select a scenario" {
		t.Fatalf("question=%q", question.Question)
	}
	if question.Options[0].Description != "Create a new table" {
		t.Fatalf("question details=%+v", question)
	}
	topAction := task.InputRequired.Questions[2]
	if topAction.QuestionID != "clarify_1" || len(topAction.Options) != 1 || topAction.Options[0].OptionID != "btn_submit" {
		t.Fatalf("top action question=%+v", topAction)
	}
	formAction := task.InputRequired.Questions[3]
	if formAction.QuestionID != "form_1" || len(formAction.Options) != 1 || formAction.Options[0].OptionID != "btn_skip" {
		t.Fatalf("form action question=%+v", formAction)
	}
	if len(task.Artifacts) != 1 {
		t.Fatalf("artifacts=%+v", task.Artifacts)
	}
	artifact := task.Artifacts[0]
	if artifact.ID != "artifact_1" || artifact.Kind != "table" || artifact.Name != "Sales table" || artifact.Status != "ready" {
		t.Fatalf("artifact=%+v", artifact)
	}
	if artifact.OutputID != "103:artifact:1" || artifact.Source != "table_agent" || artifact.GroupID != "grp_2" {
		t.Fatalf("artifact metadata=%+v", artifact)
	}
	details, ok := artifact.Data.(map[string]interface{})
	if !ok || details["revision"] != int64(128) {
		t.Fatalf("artifact data=%#v", artifact.Data)
	}
	encoded, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	for _, exact := range []string{`"baseId":9007199254740993`, `"base_id":9007199254740993`} {
		if !strings.Contains(string(encoded), exact) {
			t.Fatalf("large integer %s was not preserved: %s", exact, encoded)
		}
	}
}

func TestVersionedTaskUsesLatestPendingClarification(t *testing.T) {
	task, err := mapTask(adapterTask{
		SchemaVersion: 1,
		TaskID:        "t1",
		Status:        "waiting_for_input",
		Outputs: []adapterOutput{
			{Type: "clarification", Clarification: &adapterClarification{ID: "old", Title: "old question", Required: true}},
			{Type: "text", Text: "working"},
			{Type: "clarification", Clarification: &adapterClarification{ID: "new", Title: "new question", Required: true}},
		},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if task.InputRequired == nil || len(task.InputRequired.Questions) != 1 ||
		task.InputRequired.Questions[0].QuestionID != "new" || task.InputRequired.Questions[0].Question != "new question" {
		t.Fatalf("input_required=%+v", task.InputRequired)
	}
}

func TestVersionedTaskPreservesPreselectedQuestionsFromBackendResponse(t *testing.T) {
	rt := &fakeRuntime{
		params: map[string]string{"base_token": "b1"},
		responses: []json.RawMessage{dataResponse(t, `{
  "context_id": "ctx_dashboard",
  "created_at": "1784721611",
  "outputs": [
    {
      "data": {
        "kind": "text_block",
        "payload": {
          "bizType": "spec_doc_think",
          "contentType": "thinking",
          "iconType": "think",
          "sections": [{"content": "Preparing dashboard clarification.", "sectionKey": "section_dashboard"}],
          "title": "Thinking"
        },
        "schema_version": 1
      },
      "type": "data"
    },
    {
      "clarification": {
        "buttons": [
          {
            "action_params": "{\"continueTurn\": true}",
            "confirm_text": "skip",
            "id": "opt_skip",
            "kind": "custom",
            "label": "Skip",
            "message": "Skip",
            "style": "default"
          },
          {
            "action_params": "{\"continueTurn\": true}",
            "confirm_text": "confirm",
            "default": true,
            "id": "opt_confirm",
            "kind": "custom",
            "label": "Confirm",
            "message": "Additional information provided",
            "style": "primary"
          }
        ],
        "default_action": {
          "action_params": "{\"continueTurn\": true}",
          "button_text": "Confirm"
        },
        "id": "action_confirm",
        "questions": [
          {
            "allow_custom_input": true,
            "answer": {"value": "option_0_0"},
            "answered": true,
            "id": "q_business",
            "options": [
              {"id": "opt_progress", "label": "Task progress"},
              {"id": "opt_project", "label": "Project overview"},
              {"id": "opt_summary", "label": "Business summary"},
              {"id": "opt_sales", "label": "Sales dashboard"},
              {"id": "opt_people", "label": "Team workload"}
            ],
            "prompt": "What business area should this dashboard analyze?",
            "required": false,
            "type": "single_select"
          },
          {
            "allow_custom_input": true,
            "answer": {"value": "option_1_0"},
            "answered": true,
            "id": "q_data_source",
            "options": [
              {"id": "opt_existing_table", "label": "Existing table"},
              {"id": "opt_new_table", "label": "New business table"}
            ],
            "prompt": "Which table should provide data for this dashboard?",
            "required": false,
            "type": "single_select"
          }
        ],
        "required": true,
        "submitted": false,
        "title": "Additional information"
      },
      "type": "clarification"
    }
  ],
  "schema_version": 1,
  "status": "waiting_for_input",
  "task_id": "task_dashboard",
  "updated_at": "1784721658"
}`)},
	}

	task, err := assistantSpec.GetTask.Handler(context.Background(), rt, "task_dashboard")
	if err != nil {
		t.Fatal(err)
	}
	if task.State != iagents.StateInputRequired || task.InputRequired == nil {
		t.Fatalf("task=%+v", task)
	}
	if task.CreatedAt != "2026-07-22T12:00:11Z" || task.UpdatedAt != "2026-07-22T12:00:58Z" {
		t.Fatalf("task timestamps=%+v", task)
	}
	if task.InputRequired.Label != "Additional information" || len(task.InputRequired.Questions) != 3 {
		t.Fatalf("input_required=%+v", task.InputRequired)
	}

	businessQuestion := task.InputRequired.Questions[0]
	if businessQuestion.Question != "What business area should this dashboard analyze?" || len(businessQuestion.Options) != 5 {
		t.Fatalf("business question=%+v", businessQuestion)
	}
	dataSourceQuestion := task.InputRequired.Questions[1]
	if dataSourceQuestion.Question != "Which table should provide data for this dashboard?" || len(dataSourceQuestion.Options) != 2 {
		t.Fatalf("data source question=%+v", dataSourceQuestion)
	}
	actionQuestion := task.InputRequired.Questions[2]
	if actionQuestion.Question != "Additional information: Confirm" || len(actionQuestion.Options) != 2 ||
		actionQuestion.Options[0].Label != "Skip" || actionQuestion.Options[1].Label != "Confirm" {
		t.Fatalf("action question=%+v", actionQuestion)
	}
}

func TestVersionedTaskTerminalStateIgnoresHistoricalClarification(t *testing.T) {
	for _, status := range []string{"completed", "failed", "canceled"} {
		t.Run(status, func(t *testing.T) {
			task, err := mapTask(adapterTask{
				SchemaVersion: 1,
				TaskID:        "t1",
				Status:        status,
				CreatedAt:     json.RawMessage(`"1710000000"`),
				UpdatedAt:     json.RawMessage(`"1710000060"`),
				Outputs: []adapterOutput{{
					Type:          "clarification",
					Clarification: &adapterClarification{ID: "old", Required: true},
				}},
			}, false)
			if err != nil {
				t.Fatal(err)
			}
			if !task.IsTerminal || task.InputRequired != nil {
				t.Fatalf("task=%+v", task)
			}
			if task.CreatedAt != "2024-03-09T16:00:00Z" || task.UpdatedAt != "2024-03-09T16:01:00Z" {
				t.Fatalf("task timestamps=%+v", task)
			}
			if status == "canceled" && task.State != iagents.StateCanceled {
				t.Fatalf("canceled state=%s", task.State)
			}
		})
	}
}

func TestVersionedTaskRunningStateIgnoresStaleClarification(t *testing.T) {
	task, err := mapTask(adapterTask{
		SchemaVersion: 1,
		TaskID:        "t1",
		Status:        "running",
		Outputs: []adapterOutput{{
			Type: "clarification",
			Clarification: &adapterClarification{
				ID:       "old",
				Title:    "Stale question",
				Required: true,
			},
		}},
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if task.State != iagents.StateWorking || task.IsTerminal || task.InputRequired != nil {
		t.Fatalf("task=%+v", task)
	}
}

func TestVersionedTaskProtocolInconsistenciesAreTyped(t *testing.T) {
	tests := []adapterTask{
		{SchemaVersion: 2, TaskID: "t1", Status: "running"},
		{SchemaVersion: 1, TaskID: "t1", Status: "waiting_for_input"},
		{SchemaVersion: 1, TaskID: "t1", Status: "running", CreatedAt: json.RawMessage(`"not-a-time"`)},
		{SchemaVersion: 1, TaskID: "t1", Status: "running", Outputs: []adapterOutput{{ID: "x", Type: "data"}}},
	}
	for _, input := range tests {
		_, err := mapTask(input, false)
		problem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse)
	}
}

func TestVersionedTaskUnknownOutputIsPreservedAsData(t *testing.T) {
	var wire adapterTask
	if err := json.Unmarshal([]byte(`{"schema_version":1,"task_id":"t1","status":"running","outputs":[{"id":"x","type":"future_output","source":"future_agent","group_id":"grp_x","future":{"value":9007199254740993}}]}`), &wire); err != nil {
		t.Fatal(err)
	}
	task, err := mapTask(wire, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(task.Messages) != 1 || len(task.Messages[0].Parts) != 1 || task.Messages[0].Parts[0].Type != "data" {
		t.Fatalf("messages=%+v", task.Messages)
	}
	part := task.Messages[0].Parts[0]
	if part.OutputID != "x" || part.Source != "future_agent" || part.GroupID != "grp_x" {
		t.Fatalf("metadata=%+v", part)
	}
	encoded, err := json.Marshal(task)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"future":{"value":9007199254740993}`) {
		t.Fatalf("unknown output was not preserved: %s", encoded)
	}
}

func TestEmbeddedCliMessageMapping(t *testing.T) {
	t.Run("unknown operation stays data", func(t *testing.T) {
		part, err := mapPart(adapterPart{Type: "data", Text: `{"operation_type":"run_shell","content":"rm -rf /"}`})
		if err != nil {
			t.Fatal(err)
		}
		if part.Type != "data" || part.Data == nil || part.Text != "" {
			t.Fatalf("part=%+v", part)
		}
	})
	t.Run("invalid embedded json is typed", func(t *testing.T) {
		_, err := mapPart(adapterPart{Type: "data", Text: `{bad`})
		problem(t, err, errs.CategoryInternal, errs.SubtypeInvalidResponse)
	})
}

func TestContextHooks(t *testing.T) {
	rt := &fakeRuntime{
		params: map[string]string{"base_token": "b1", "status": "active"},
		responses: []json.RawMessage{
			dataResponse(t, `[{"context_id":"c1","title":"Quarterly plan","created_at":1710000000,"updated_at":1710000060}]`),
			dataResponse(t, `{"context_id":"c1","title":"Quarterly plan","tasks":[{"task_id":"new","state":"running","updated_at":1710000060},{"task_id":"old","state":"done","updated_at":1710000000}]}`),
			dataResponse(t, `{"result":true}`),
		},
	}
	contexts, pageInfo, err := assistantSpec.ListContexts.Handler(context.Background(), rt, iagents.PageParams{Token: "next", Size: 10})
	if err != nil {
		t.Fatal(err)
	}
	if pageInfo != (iagents.PageInfo{}) {
		t.Fatalf("pageInfo=%+v", pageInfo)
	}
	if len(contexts) != 1 {
		t.Fatalf("contexts=%+v", contexts)
	}
	wantQuery := map[string]string{"cursor": "next", "limit": "10", "status": "active"}
	if !reflect.DeepEqual(rt.calls[0].query, wantQuery) {
		t.Fatalf("query=%v", rt.calls[0].query)
	}
	detail, err := assistantSpec.GetContext.Handler(context.Background(), rt, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if detail.TaskCount == nil || *detail.TaskCount != 2 || detail.ActiveTask == nil || detail.ActiveTask.TaskID != "new" {
		t.Fatalf("detail=%+v", detail)
	}
	if err := assistantSpec.DeleteContext.Handler(context.Background(), rt, "c1"); err != nil {
		t.Fatal(err)
	}
	if rt.calls[2].method != "DELETE" {
		t.Fatalf("delete call=%+v", rt.calls[2])
	}
}

func TestMapContextDetailRollsUpTasks(t *testing.T) {
	tests := []struct {
		name          string
		tasks         []adapterTask
		activeID      string
		awaitingInput bool
	}{
		{
			name: "latest task is active while any task can await input",
			tasks: []adapterTask{
				{TaskID: "old", State: "input_required", UpdatedAt: json.RawMessage(`1710000000`), Summary: "choose one"},
				{TaskID: "latest", State: "done", UpdatedAt: json.RawMessage(`1710000060`), Summary: "complete"},
			},
			activeID:      "latest",
			awaitingInput: true,
		},
		{
			name: "waiting task wins an updated at tie regardless of order",
			tasks: []adapterTask{
				{TaskID: "20", State: "done", UpdatedAt: json.RawMessage(`1710000060`)},
				{TaskID: "10", State: "input_required", UpdatedAt: json.RawMessage(`1710000060`)},
			},
			activeID:      "10",
			awaitingInput: true,
		},
		{
			name: "numeric task id breaks a complete task tie",
			tasks: []adapterTask{
				{TaskID: "9", State: "done", UpdatedAt: json.RawMessage(`1710000060`)},
				{TaskID: "10", State: "done", UpdatedAt: json.RawMessage(`1710000060`)},
			},
			activeID: "10",
		},
		{
			name: "empty context has a known zero task count",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, reverse := range []bool{false, true} {
				tasks := append([]adapterTask(nil), test.tasks...)
				if reverse {
					for left, right := 0, len(tasks)-1; left < right; left, right = left+1, right-1 {
						tasks[left], tasks[right] = tasks[right], tasks[left]
					}
				}
				detail, err := mapContextDetail(adapterContext{ContextID: "c1", Tasks: tasks})
				if err != nil {
					t.Fatal(err)
				}
				if detail.TaskCount == nil || *detail.TaskCount != len(tasks) {
					t.Fatalf("reverse=%v task_count=%v tasks=%d", reverse, detail.TaskCount, len(tasks))
				}
				if test.activeID == "" {
					if detail.ActiveTask != nil {
						t.Fatalf("reverse=%v detail=%+v", reverse, detail)
					}
					continue
				}
				if detail.ActiveTask == nil || detail.ActiveTask.TaskID != test.activeID || detail.AwaitingInput != test.awaitingInput {
					t.Fatalf("reverse=%v detail=%+v", reverse, detail)
				}
			}
		})
	}
}

func TestResultFalseUsesTypedCategory(t *testing.T) {
	rt := &fakeRuntime{
		params:    map[string]string{"base_token": "b1"},
		responses: []json.RawMessage{dataResponse(t, `{"result":false,"reason":"task is terminal","error":{"category":"task_terminal"}}`)},
	}
	err := assistantSpec.CancelTask.Handler(context.Background(), rt, "t1")
	problem(t, err, errs.CategoryValidation, errs.SubtypeFailedPrecondition)
}

func TestTopLevelBizErrReturnsTypedError(t *testing.T) {
	tests := []struct {
		name string
		run  func(*fakeRuntime) error
	}{
		{
			name: "task list",
			run: func(rt *fakeRuntime) error {
				_, _, err := assistantSpec.ListTasks.Handler(context.Background(), rt, "c1", iagents.PageParams{})
				return err
			},
		},
		{
			name: "context list",
			run: func(rt *fakeRuntime) error {
				_, _, err := assistantSpec.ListContexts.Handler(context.Background(), rt, iagents.PageParams{})
				return err
			},
		},
		{
			name: "cancel task",
			run: func(rt *fakeRuntime) error {
				return assistantSpec.CancelTask.Handler(context.Background(), rt, "t1")
			},
		},
		{
			name: "delete context",
			run: func(rt *fakeRuntime) error {
				return assistantSpec.DeleteContext.Handler(context.Background(), rt, "c1")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rt := &fakeRuntime{
				params:    map[string]string{"base_token": "b1"},
				responses: []json.RawMessage{dataResponse(t, `{"BizErrCode":800004907,"BizErrMessage":"[MOCK] create job rate limit","result":true,"tasks":[],"contexts":[],"has_more":false}`)},
			}
			err := test.run(rt)
			problem(t, err, errs.CategoryAPI, errs.SubtypeRateLimit)
			p, _ := errs.ProblemOf(err)
			if p.Code != 800004907 || !p.Retryable || !strings.Contains(p.Message, "[MOCK] create job rate limit") {
				t.Fatalf("problem=%+v", p)
			}
		})
	}
}

// TestMapInputRequiredExpandsFullGroup covers atomic-group mapping: multiple
// top-level questions (multi-select and preselected questions preserved) plus a
// form's questions with the form title as prompt prefix, all in one group and
// with ids passed through verbatim for the backend to resolve.
func TestMapInputRequiredExpandsFullGroup(t *testing.T) {
	group, err := mapInputRequired(adapterClarification{
		ID:    "clarify_1",
		Title: "Configure report",
		Questions: []adapterClarificationQuestion{
			{ID: "q_region", Type: "single_select", Prompt: "Select a region", Options: []adapterClarificationOption{{ID: "opt_apac", Label: "APAC"}}},
			{ID: "q_metric", Type: "multi_select", Prompt: "Select metrics", Options: []adapterClarificationOption{{ID: "opt_rev", Label: "Revenue"}, {ID: "opt_cost", Label: "Cost"}}},
			{ID: "q_done", Type: "text", Prompt: "Already answered", Answered: true},
		},
		Forms: []adapterClarificationForm{{
			ID:    "form_1",
			Title: "Advanced",
			Questions: []adapterClarificationQuestion{
				{ID: "q_note", Type: "text", Prompt: "Notes"},
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if group.Label != "Configure report" {
		t.Fatalf("label=%q", group.Label)
	}
	var ids []string
	for _, q := range group.Questions {
		ids = append(ids, q.QuestionID)
	}
	want := []string{"q_region", "q_metric", "q_done", "q_note"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("question ids=%v want %v (preselected question must remain visible)", ids, want)
	}
	if !group.Questions[1].MultiSelect {
		t.Fatalf("multi_select question lost flag: %+v", group.Questions[1])
	}
	if group.Questions[3].Question != "Advanced: Notes" {
		t.Fatalf("form question prompt=%q", group.Questions[3].Question)
	}
}

// TestMapInputRequiredRejectsSubQuestions ensures a conditional sub-question is a
// typed failed_precondition surfaced through mapTask, not a silent drop.
func TestMapInputRequiredRejectsSubQuestions(t *testing.T) {
	_, err := mapTask(adapterTask{
		SchemaVersion: 1,
		TaskID:        "t1",
		Status:        "waiting_for_input",
		Outputs: []adapterOutput{{
			Type: "clarification",
			Clarification: &adapterClarification{
				ID: "clarify_1", Title: "T", Required: true,
				Questions: []adapterClarificationQuestion{{
					ID: "q_parent", Type: "single_select", Prompt: "Parent question",
					Options:      []adapterClarificationOption{{ID: "opt_1", Label: "A"}},
					SubQuestions: []adapterClarificationQuestion{{ID: "q_child", Type: "text", Prompt: "Child question"}},
				}},
			},
		}},
	}, false)
	problem(t, err, errs.CategoryValidation, errs.SubtypeFailedPrecondition)
}
