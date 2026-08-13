// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
)

const baseAgentServicePath = "/open-apis/base/v3"

func callPayload[T any](ctx context.Context, rt iagents.Runtime, method, path string, query map[string]string, body any) (T, error) {
	return iagents.Call[T](ctx, rt, method, path, query, body)
}

func segment(v string) string { return url.PathEscape(v) }

func agentRoot(baseToken string) string {
	return baseAgentServicePath + "/bases/" + segment(baseToken) + "/ai/agents/" + segment(adapterAgentID)
}

func randomIdempotencyKey() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", errs.NewInternalError(errs.SubtypeUnknown,
			"generate Base Agent idempotency key: %v", err).WithCause(err)
	}
	return "lark-cli-" + hex.EncodeToString(b[:]), nil
}

// canonicalAnswers is the single, fixed encoding both the deterministic
// message_id and idempotency_key derive from. Determinism matters across
// endpoints and retries: the backend hands the idempotency_key straight to
// CreateJob, so an identical answer submitted twice MUST hash the same, and a
// different answer MUST hash differently. Algorithm (frozen):
//   - question keys sorted ascending;
//   - each key's values: a ".text" free-text key keeps its single value
//     verbatim (order/content preserved, never sorted); a choice key's values
//     are deduplicated then sorted ascending (multi-select order is not
//     semantically meaningful);
//   - encoded as compact JSON with the schema version and task id, so a
//     schema bump or a cross-task collision can never dedupe.
func canonicalAnswers(taskID string, answers map[string][]string) ([]byte, error) {
	keys := make([]string, 0, len(answers))
	for k := range answers {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	normalized := make([][2]interface{}, 0, len(keys))
	for _, k := range keys {
		values := append([]string(nil), answers[k]...)
		if _, isText := iagents.SplitAnswerKey(k); !isText {
			values = dedupeSorted(values)
		}
		normalized = append(normalized, [2]interface{}{k, values})
	}
	canonical := struct {
		SchemaVersion int              `json:"schema_version"`
		TaskID        string           `json:"task_id"`
		Answers       [][2]interface{} `json:"answers"`
	}{
		SchemaVersion: answersDataSchemaVersion,
		TaskID:        taskID,
		Answers:       normalized,
	}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown,
			"encode Base Agent answers: %v", err).WithCause(err)
	}
	return encoded, nil
}

// dedupeSorted removes exact-duplicate values and returns them sorted ascending.
func dedupeSorted(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// deterministicAnswerID derives the shared message_id/idempotency_key for an
// answer submission from the canonical encoding: a
// same-command retry dedupes server-side, a different answer does not.
func deterministicAnswerID(taskID string, answers map[string][]string) (string, error) {
	canonical, err := canonicalAnswers(taskID, answers)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return "answer_" + hex.EncodeToString(sum[:]), nil
}

func sendMessage(ctx context.Context, rt iagents.Runtime, in iagents.SendInput) (*iagents.AgentTask, error) {
	p, err := iagents.BindParams[sendParams](rt)
	if err != nil {
		return nil, err
	}
	params := map[string]string{}
	if p.ActiveTableID != "" {
		params["active_table_id"] = p.ActiveTableID
	}
	msg, key, err := buildSendMessage(in)
	if err != nil {
		return nil, err
	}
	req := adapterSendRequest{
		ContextID:      in.ContextID,
		TaskID:         in.TaskID,
		Message:        msg,
		Params:         params,
		IdempotencyKey: key,
		Metadata:       map[string]string{"channel": "lark_cli"},
	}
	path := agentRoot(p.BaseToken) + "/messages"
	got, err := callPayload[adapterTask](ctx, rt, "POST", path, nil, req)
	if err != nil {
		return nil, err
	}
	return mapTask(got, true)
}

// buildSendMessage assembles the wire message and its idempotency key for a
// send. An answer send (in.Answers non-empty) carries a single kind=answers
// DataPart — never a text part, since Base rejects an answer remark — and uses
// a deterministic id shared by message_id and idempotency_key. Every other send
// carries the free text and a random id (a new task/turn is a new logical
// message each time).
func buildSendMessage(in iagents.SendInput) (adapterMessage, string, error) {
	if len(in.Answers) > 0 {
		id, err := deterministicAnswerID(in.TaskID, in.Answers)
		if err != nil {
			return adapterMessage{}, "", err
		}
		data, err := json.Marshal(answersDataPart{
			Kind:          answersDataKind,
			SchemaVersion: answersDataSchemaVersion,
			Payload:       answersDataPayload{Answers: in.Answers},
		})
		if err != nil {
			return adapterMessage{}, "", errs.NewInternalError(errs.SubtypeUnknown,
				"encode Base Agent answers data part: %v", err).WithCause(err)
		}
		msg := adapterMessage{
			MessageID: id,
			Role:      "user",
			Parts:     []adapterPart{{Type: "data", Data: json.RawMessage(data)}},
		}
		return msg, id, nil
	}
	key, err := randomIdempotencyKey()
	if err != nil {
		return adapterMessage{}, "", err
	}
	msg := adapterMessage{Role: "user", Parts: []adapterPart{{Type: "text", Text: in.Text}}}
	return msg, key, nil
}

func getTask(ctx context.Context, rt iagents.Runtime, taskID string) (*iagents.AgentTask, error) {
	p, err := iagents.BindParams[getTaskParams](rt)
	if err != nil {
		return nil, err
	}
	path := agentRoot(p.BaseToken) + "/tasks/" + segment(taskID)
	query := map[string]string{}
	putQuery(query, "context_id", p.ContextID)
	got, err := callPayload[adapterTask](ctx, rt, "GET", path, query, nil)
	if err != nil {
		return nil, err
	}
	return mapTask(got, false)
}

func listTasks(ctx context.Context, rt iagents.Runtime, contextID string, page iagents.PageParams) ([]iagents.TaskSummary, iagents.PageInfo, error) {
	if strings.TrimSpace(contextID) == "" {
		return nil, iagents.PageInfo{}, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"base:assistant task list requires --context-id").WithParam("--context-id").
			WithHint("pass --context-id <context-id> from a Base Agent task or context response")
	}
	p, err := iagents.BindParams[listTasksParams](rt)
	if err != nil {
		return nil, iagents.PageInfo{}, err
	}
	query := map[string]string{}
	putQuery(query, "context_id", contextID)
	putQuery(query, "cursor", page.Token)
	if page.Size > 0 {
		query["limit"] = strconv.Itoa(page.Size)
	}
	putQuery(query, "state", p.State)
	got, err := callPayload[adapterTaskList](ctx, rt, "GET", agentRoot(p.BaseToken)+"/tasks", query, nil)
	if err != nil {
		return nil, iagents.PageInfo{}, err
	}
	if err := bizErrAsAPIError(got.adapterBizErr, "list tasks"); err != nil {
		return nil, iagents.PageInfo{}, err
	}
	out := make([]iagents.TaskSummary, 0, len(got.Tasks))
	for _, item := range got.Tasks {
		summary, err := mapTaskSummary(item)
		if err != nil {
			return nil, iagents.PageInfo{}, err
		}
		out = append(out, summary)
	}
	return out, iagents.PageInfo{HasMore: got.HasMore, NextToken: got.NextCursor}, nil
}

func cancelTask(ctx context.Context, rt iagents.Runtime, taskID string) error {
	p, err := iagents.BindParams[baseTokenParams](rt)
	if err != nil {
		return err
	}
	path := agentRoot(p.BaseToken) + "/tasks/" + segment(taskID) + "/cancel"
	result, err := callPayload[adapterResult](ctx, rt, "POST", path, nil, map[string]any{
		"metadata": map[string]string{"channel": "lark_cli"},
	})
	if err != nil {
		return err
	}
	return mapResult(result, "cancel task")
}

func listContexts(ctx context.Context, rt iagents.Runtime, page iagents.PageParams) ([]iagents.ContextSummary, iagents.PageInfo, error) {
	p, err := iagents.BindParams[listContextsParams](rt)
	if err != nil {
		return nil, iagents.PageInfo{}, err
	}
	query := map[string]string{}
	putQuery(query, "cursor", page.Token)
	if page.Size > 0 {
		query["limit"] = strconv.Itoa(page.Size)
	}
	putQuery(query, "status", p.Status)
	got, err := callPayload[adapterContextList](ctx, rt, "GET", agentRoot(p.BaseToken)+"/contexts", query, nil)
	if err != nil {
		return nil, iagents.PageInfo{}, err
	}
	if err := bizErrAsAPIError(got.adapterBizErr, "list contexts"); err != nil {
		return nil, iagents.PageInfo{}, err
	}
	out := make([]iagents.ContextSummary, 0, len(got.Contexts))
	for _, item := range got.Contexts {
		mapped, err := mapContextSummary(item)
		if err != nil {
			return nil, iagents.PageInfo{}, err
		}
		out = append(out, mapped)
	}
	return out, iagents.PageInfo{HasMore: got.HasMore, NextToken: got.NextCursor}, nil
}

func getContext(ctx context.Context, rt iagents.Runtime, contextID string) (*iagents.ContextDetail, error) {
	p, err := iagents.BindParams[baseTokenParams](rt)
	if err != nil {
		return nil, err
	}
	path := agentRoot(p.BaseToken) + "/contexts/" + segment(contextID)
	got, err := callPayload[adapterContext](ctx, rt, "GET", path, nil, nil)
	if err != nil {
		return nil, err
	}
	return mapContextDetail(got)
}

func deleteContext(ctx context.Context, rt iagents.Runtime, contextID string) error {
	p, err := iagents.BindParams[baseTokenParams](rt)
	if err != nil {
		return err
	}
	path := agentRoot(p.BaseToken) + "/contexts/" + segment(contextID)
	result, err := callPayload[adapterResult](ctx, rt, "DELETE", path, nil, nil)
	if err != nil {
		return err
	}
	return mapResult(result, "delete context")
}

func putQuery(query map[string]string, key, value string) {
	if value != "" {
		query[key] = value
	}
}
