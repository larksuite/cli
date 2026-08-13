// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	iagents "github.com/larksuite/cli/internal/agents"
)

const adapterTaskSchemaVersion = 1

func taskID(in adapterTask) string {
	if in.TaskID != "" {
		return in.TaskID
	}
	return in.ID
}

func contextID(in adapterContext) string {
	if in.ContextID != "" {
		return in.ContextID
	}
	return in.ID
}

func mapState(raw string, allowEmpty bool) (iagents.TaskState, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "pending", "submitted":
		return iagents.StateSubmitted, nil
	case "running", "working":
		return iagents.StateWorking, nil
	case "waiting_for_input", "input_required":
		return iagents.StateInputRequired, nil
	case "done", "finish", "finished", "turn_finished", "completed":
		return iagents.StateCompleted, nil
	case "failed":
		return iagents.StateFailed, nil
	case "cancel", "canceled", "cancelled":
		return iagents.StateCanceled, nil
	case "":
		if allowEmpty {
			return iagents.StateSubmitted, nil
		}
	}
	return "", errs.NewInternalError(errs.SubtypeInvalidResponse,
		"Base Adapter returned unsupported task state %q", raw)
}

func mapTask(in adapterTask, allowEmptyState bool) (*iagents.AgentTask, error) {
	if in.SchemaVersion != 0 {
		return mapVersionedTask(in)
	}
	return mapLegacyTask(in, allowEmptyState)
}

func mapVersionedTask(in adapterTask) (*iagents.AgentTask, error) {
	if in.SchemaVersion != adapterTaskSchemaVersion {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"Base Adapter returned unsupported task schema_version %d", in.SchemaVersion).
			WithHint("update lark-cli to a version that supports this Base Agent task schema")
	}
	state, err := mapState(in.Status, false)
	if err != nil {
		return nil, err
	}
	messages, artifacts, err := mapOutputs(in.Outputs)
	if err != nil {
		return nil, err
	}
	createdAt, err := mapTime(in.CreatedAt)
	if err != nil {
		return nil, err
	}
	updatedAt, err := mapTime(in.UpdatedAt)
	if err != nil {
		return nil, err
	}
	pending := latestPendingClarification(in.Outputs)
	var inputRequired *iagents.InputRequired
	switch {
	case state == iagents.StateInputRequired && pending == nil:
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"Base Adapter returned waiting_for_input without an unresolved required clarification")
	case state == iagents.StateInputRequired:
		inputRequired, err = mapInputRequired(*pending.Clarification)
		if err != nil {
			return nil, err
		}
	default:
		// Job status is authoritative. Clarification outputs are persisted on a
		// separate path and can briefly remain unresolved after an answer moves
		// the task back to running. Treat that card as historical so --watch keeps
		// polling instead of surfacing an invalid-response error.
	}
	return &iagents.AgentTask{
		TaskID:        taskID(in),
		ContextID:     in.ContextID,
		State:         state,
		IsTerminal:    state.IsTerminal(),
		CreatedAt:     createdAt,
		UpdatedAt:     updatedAt,
		BizError:      mapBizError(in.adapterBizErr),
		Messages:      messages,
		Artifacts:     artifacts,
		InputRequired: inputRequired,
	}, nil
}

func mapLegacyTask(in adapterTask, allowEmptyState bool) (*iagents.AgentTask, error) {
	stateRaw := in.State
	if stateRaw == "" {
		stateRaw = in.Status
	}
	state, err := mapState(stateRaw, allowEmptyState)
	if err != nil {
		return nil, err
	}
	messages, err := mapMessages(in.Messages)
	if err != nil {
		return nil, err
	}
	artifacts, err := mapArtifacts(in.Artifacts)
	if err != nil {
		return nil, err
	}
	createdAt, err := mapTime(in.CreatedAt)
	if err != nil {
		return nil, err
	}
	updatedAt, err := mapTime(in.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &iagents.AgentTask{
		TaskID:     taskID(in),
		ContextID:  in.ContextID,
		State:      state,
		IsTerminal: state.IsTerminal(),
		CreatedAt:  createdAt,
		UpdatedAt:  updatedAt,
		BizError:   mapBizError(in.adapterBizErr),
		Messages:   messages,
		Artifacts:  artifacts,
	}, nil
}

func mapOutputs(in []adapterOutput) ([]iagents.Message, []iagents.Artifact, error) {
	parts := make([]iagents.Part, 0, len(in))
	artifacts := make([]iagents.Artifact, 0)
	for _, output := range in {
		partMetadata := iagents.Part{OutputID: output.ID, Source: output.Source, GroupID: output.GroupID}
		switch strings.ToLower(strings.TrimSpace(output.Type)) {
		case "text":
			if output.Text == "" {
				return nil, nil, invalidOutput(output, "text output is empty")
			}
			partMetadata.Type = "text"
			partMetadata.Text = output.Text
			parts = append(parts, partMetadata)
		case "data":
			if output.Data == nil {
				return nil, nil, invalidOutput(output, "data output is missing data")
			}
			if len(output.Data.Payload) == 0 || !json.Valid(output.Data.Payload) {
				return nil, nil, invalidOutput(output, "data payload is invalid JSON")
			}
			partMetadata.Type = "data"
			partMetadata.Data = map[string]interface{}{
				"kind":           output.Data.Kind,
				"schema_version": output.Data.SchemaVersion,
				"payload":        append(json.RawMessage(nil), output.Data.Payload...),
			}
			parts = append(parts, partMetadata)
		case "clarification":
			if output.Clarification == nil {
				return nil, nil, invalidOutput(output, "clarification output is missing clarification")
			}
		case "artifact":
			if output.Artifact == nil {
				return nil, nil, invalidOutput(output, "artifact output is missing artifact")
			}
			artifacts = append(artifacts, mapOutputArtifact(output))
		default:
			raw := output.Raw
			if len(raw) == 0 {
				var err error
				raw, err = json.Marshal(output)
				if err != nil {
					return nil, nil, invalidOutputWithCause(output, "unknown output cannot be preserved", err)
				}
			}
			partMetadata.Type = "data"
			partMetadata.Data = append(json.RawMessage(nil), raw...)
			parts = append(parts, partMetadata)
		}
	}
	var messages []iagents.Message
	if len(parts) > 0 {
		messages = []iagents.Message{{Role: "agent", Parts: parts}}
	}
	return messages, artifacts, nil
}

func mapOutputArtifact(output adapterOutput) iagents.Artifact {
	in := *output.Artifact
	data := make(map[string]interface{}, 3)
	if len(in.Resource) > 0 {
		data["resource"] = in.Resource
	}
	if in.Revision != nil {
		data["revision"] = *in.Revision
	}
	if len(in.Metadata) > 0 && string(in.Metadata) != "null" {
		data["metadata"] = append(json.RawMessage(nil), in.Metadata...)
	}
	var details interface{}
	if len(data) > 0 {
		details = data
	}
	return iagents.Artifact{
		ID:       in.ID,
		OutputID: output.ID,
		Source:   output.Source,
		GroupID:  output.GroupID,
		Kind:     in.Type,
		Name:     in.Title,
		Status:   in.Status,
		Data:     details,
	}
}

func latestPendingClarification(outputs []adapterOutput) *adapterOutput {
	for i := len(outputs) - 1; i >= 0; i-- {
		clarification := outputs[i].Clarification
		if strings.EqualFold(outputs[i].Type, "clarification") && clarification != nil &&
			clarification.Required && !clarification.Submitted {
			return &outputs[i]
		}
	}
	return nil
}

// mapInputRequired expands a pending clarification into ONE question group: the
// unified contract answers the whole group atomically, so every still-open
// question is surfaced together — top-level questions, each form's questions
// (prompt prefixed with the form title), and each button set as a synthetic
// action question (the clarification/form id is the action question id, each
// button id an option). Questions with preselected values remain visible while
// the card is pending: answered=true can describe a default selection, not a
// submitted answer. IDs pass through verbatim — the backend mints CLI-legal
// public ids and resolves them back; the CLI never rewrites them. Conditional
// sub-questions are NOT answerable through the flat CLI model (answering a
// hidden branch would be wrong), so their presence is a typed
// failed_precondition here rather than a silent drop or a late backend rejection.
func mapInputRequired(in adapterClarification) (*iagents.InputRequired, error) {
	questions := make([]iagents.Question, 0)
	actions := make([]iagents.Question, 0)

	topQuestions, err := expandClarificationQuestions(in.Questions, "")
	if err != nil {
		return nil, err
	}
	questions = append(questions, topQuestions...)
	if len(in.Buttons) > 0 {
		actions = append(actions, buttonActionQuestion(in.ID, clarificationActionPrompt(in), in.Buttons))
	}
	for _, form := range in.Forms {
		formQuestions, err := expandClarificationQuestions(form.Questions, form.Title)
		if err != nil {
			return nil, err
		}
		questions = append(questions, formQuestions...)
		if len(form.Buttons) > 0 {
			actions = append(actions, buttonActionQuestion(form.ID, form.Title, form.Buttons))
		}
	}
	// Content questions first, then synthetic action buttons — a submit/skip
	// action reads naturally after the questions it applies to.
	questions = append(questions, actions...)

	label := strings.TrimSpace(in.Title)
	if len(questions) == 0 {
		// A bare clarification with neither questions nor buttons: present the
		// title (or a default) as one free-text question. The empty-title case is
		// also covered centrally by NormalizeInputRequired.
		prompt := label
		if prompt == "" {
			prompt = "Please provide more information"
		}
		questions = append(questions, iagents.Question{QuestionID: in.ID, Question: prompt})
	}

	return &iagents.InputRequired{Label: label, Questions: questions}, nil
}

// expandClarificationQuestions maps a pending question list into contract
// Questions and rejects any question that carries conditional sub-questions
// (unsupported through the flat CLI model this phase). Do not skip Answered
// questions here: for an unsubmitted card that flag may only mean the backend
// supplied a default or recommended value that the user can still change.
func expandClarificationQuestions(questions []adapterClarificationQuestion, formTitle string) ([]iagents.Question, error) {
	out := make([]iagents.Question, 0, len(questions))
	for _, question := range questions {
		if len(question.SubQuestions) > 0 {
			return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"base:assistant cannot answer a clarification with conditional sub-questions from the CLI").
				WithHint("open this Base in the Feishu/Lark client to answer the nested question group")
		}
		out = append(out, questionFromClarification(question, formTitle))
	}
	return out, nil
}

func questionFromClarification(in adapterClarificationQuestion, formTitle string) iagents.Question {
	options := make([]iagents.Option, 0, len(in.Options))
	for _, option := range in.Options {
		options = append(options, iagents.Option{
			OptionID:    option.ID,
			Label:       option.Label,
			Description: option.Description,
		})
	}
	return iagents.Question{
		QuestionID:  in.ID,
		Question:    joinPrompt(formTitle, in.Prompt),
		MultiSelect: strings.EqualFold(in.Type, "multi_select") && len(options) > 0,
		Options:     options,
	}
}

// buttonActionQuestion turns a button set into one synthetic action question:
// the clarification/form id is the question id, each button id an option. It has
// no free-text fallback (a button set is a pure choice), and multi-select is off
// (a button click is a single action).
func buttonActionQuestion(id, prompt string, buttons []adapterClarificationButton) iagents.Question {
	options := make([]iagents.Option, 0, len(buttons))
	for _, button := range buttons {
		options = append(options, iagents.Option{OptionID: button.ID, Label: button.Label})
	}
	if strings.TrimSpace(prompt) == "" {
		prompt = "Select an action"
	}
	return iagents.Question{QuestionID: id, Question: prompt, Options: options}
}

func clarificationActionPrompt(in adapterClarification) string {
	if in.DefaultAction != nil && in.DefaultAction.ButtonText != "" {
		return joinPrompt(in.Title, in.DefaultAction.ButtonText)
	}
	return in.Title
}

func joinPrompt(parts ...string) string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || (len(out) > 0 && out[len(out)-1] == part) {
			continue
		}
		out = append(out, part)
	}
	return strings.Join(out, ": ")
}

func invalidOutput(output adapterOutput, reason string) error {
	return errs.NewInternalError(errs.SubtypeInvalidResponse,
		"Base Adapter returned invalid output %q (%s): %s", output.ID, output.Type, reason)
}

func invalidOutputWithCause(output adapterOutput, reason string, cause error) error {
	return errs.NewInternalError(errs.SubtypeInvalidResponse,
		"Base Adapter returned invalid output %q (%s): %s: %v", output.ID, output.Type, reason, cause).WithCause(cause)
}

func mapTaskSummary(in adapterTask) (iagents.TaskSummary, error) {
	stateRaw := in.State
	if stateRaw == "" {
		stateRaw = in.Status
	}
	state, err := mapState(stateRaw, false)
	if err != nil {
		return iagents.TaskSummary{}, err
	}
	updatedAt, err := mapTime(in.UpdatedAt)
	if err != nil {
		return iagents.TaskSummary{}, err
	}
	summary := in.Summary
	if summary == "" {
		messages, mapErr := mapMessages(in.Messages)
		if mapErr != nil {
			return iagents.TaskSummary{}, mapErr
		}
		summary = lastText(messages)
	}
	return iagents.TaskSummary{
		TaskID:     taskID(in),
		ContextID:  in.ContextID,
		State:      state,
		IsTerminal: state.IsTerminal(),
		UpdatedAt:  updatedAt,
		Summary:    summary,
		BizError:   mapBizError(in.adapterBizErr),
	}, nil
}

func mapContextSummary(in adapterContext) (iagents.ContextSummary, error) {
	createdAt, err := mapTime(in.CreatedAt)
	if err != nil {
		return iagents.ContextSummary{}, err
	}
	updatedAt, err := mapTime(in.UpdatedAt)
	if err != nil {
		return iagents.ContextSummary{}, err
	}
	return iagents.ContextSummary{
		ContextID: contextID(in),
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
		Title:     in.Title,
		BizError:  mapBizError(in.adapterBizErr),
	}, nil
}

func mapContextDetail(in adapterContext) (*iagents.ContextDetail, error) {
	summary, err := mapContextSummary(in)
	if err != nil {
		return nil, err
	}
	detail := &iagents.ContextDetail{
		ContextID: summary.ContextID,
		CreatedAt: summary.CreatedAt,
		UpdatedAt: summary.UpdatedAt,
		Title:     summary.Title,
		BizError:  mapBizError(in.adapterBizErr),
		TaskCount: iagents.Int(len(in.Tasks)),
	}
	if len(in.Tasks) > 0 {
		var active iagents.TaskSummary
		for index, task := range in.Tasks {
			candidate, mapErr := mapTaskSummary(task)
			if mapErr != nil {
				return nil, mapErr
			}
			if taskIsAwaitingInput(candidate) {
				detail.AwaitingInput = true
			}
			if index == 0 || taskSummaryIsNewer(candidate, active) {
				active = candidate
			}
		}
		detail.ActiveTask = &active
	}
	return detail, nil
}

func taskSummaryIsNewer(candidate, current iagents.TaskSummary) bool {
	if candidate.UpdatedAt != current.UpdatedAt {
		if candidate.UpdatedAt == "" {
			return false
		}
		if current.UpdatedAt == "" {
			return true
		}
		return candidate.UpdatedAt > current.UpdatedAt
	}
	if taskIsAwaitingInput(candidate) != taskIsAwaitingInput(current) {
		return taskIsAwaitingInput(candidate)
	}
	return taskIDIsNewer(candidate.TaskID, current.TaskID)
}

func taskIsAwaitingInput(task iagents.TaskSummary) bool {
	return task.State == iagents.StateInputRequired || task.State == iagents.StateAuthRequired
}

func taskIDIsNewer(candidate, current string) bool {
	candidateID, candidateErr := strconv.ParseUint(candidate, 10, 64)
	currentID, currentErr := strconv.ParseUint(current, 10, 64)
	if candidateErr == nil && currentErr == nil {
		return candidateID > currentID
	}
	return candidate > current
}

func mapMessages(in []adapterMessage) ([]iagents.Message, error) {
	out := make([]iagents.Message, 0, len(in))
	for _, message := range in {
		parts := make([]iagents.Part, 0, len(message.Parts)+1)
		if message.Text != "" {
			parts = append(parts, iagents.Part{Type: "text", Text: message.Text})
		}
		for _, part := range message.Parts {
			mapped, err := mapPart(part)
			if err != nil {
				return nil, err
			}
			parts = append(parts, mapped)
		}
		role := message.Role
		if role == "assistant" {
			role = "agent"
		}
		out = append(out, iagents.Message{Role: role, Parts: parts})
	}
	return out, nil
}

func mapPart(in adapterPart) (iagents.Part, error) {
	switch strings.ToLower(in.Type) {
	case "text":
		return iagents.Part{Type: "text", Text: in.Text}, nil
	case "file":
		return iagents.Part{Type: "file", Name: in.Name, URL: in.URL}, nil
	case "data":
		if len(in.Data) > 0 {
			var data any
			if err := json.Unmarshal(in.Data, &data); err != nil {
				return iagents.Part{}, invalidMessage(err)
			}
			return iagents.Part{Type: "data", Data: data}, nil
		}
		if in.Text == "" {
			return iagents.Part{Type: "data"}, nil
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(in.Text), &data); err != nil {
			return iagents.Part{}, invalidMessage(err)
		}
		op, _ := data["operation_type"].(string)
		content, contentIsString := data["content"].(string)
		switch strings.ToLower(op) {
		case "text", "answer", "message", "plain_text", "markdown":
			if contentIsString {
				return iagents.Part{Type: "text", Text: content}, nil
			}
		}
		return iagents.Part{Type: "data", Data: data}, nil
	default:
		return iagents.Part{Type: "data", Data: map[string]any{
			"type": in.Type, "text": in.Text, "name": in.Name, "url": in.URL,
		}}, nil
	}
}

func mapArtifacts(in []adapterArtifact) ([]iagents.Artifact, error) {
	out := make([]iagents.Artifact, 0, len(in))
	for _, item := range in {
		text := item.Text
		if strings.HasPrefix(strings.TrimSpace(text), "{") {
			part, err := mapPart(adapterPart{Type: "data", Text: text})
			if err != nil {
				return nil, err
			}
			if part.Type == "text" {
				text = part.Text
			}
		}
		out = append(out, iagents.Artifact{ID: item.ID, Kind: item.Kind, Name: item.Name, URL: item.URL, Text: text})
	}
	return out, nil
}

func mapTime(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var unix int64
	if err := json.Unmarshal(raw, &unix); err == nil {
		return time.Unix(unix, 0).UTC().Format(time.RFC3339), nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse,
			"Base Adapter returned invalid timestamp %s", string(raw)).WithCause(err)
	}
	if value == "" {
		return "", nil
	}
	if n, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.Unix(n, 0).UTC().Format(time.RFC3339), nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return "", errs.NewInternalError(errs.SubtypeInvalidResponse,
			"Base Adapter returned invalid timestamp %q", value).WithCause(err)
	}
	return parsed.UTC().Format(time.RFC3339), nil
}

func invalidMessage(cause error) error {
	return errs.NewInternalError(errs.SubtypeInvalidResponse,
		"Base Adapter returned an invalid embedded CliMessage: %v", cause).WithCause(cause)
}

func lastText(messages []iagents.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		for j := len(messages[i].Parts) - 1; j >= 0; j-- {
			if messages[i].Parts[j].Type == "text" {
				return messages[i].Parts[j].Text
			}
		}
	}
	return ""
}

func mapResult(result adapterResult, action string) error {
	if err := bizErrAsAPIError(result.adapterBizErr, action); err != nil {
		return err
	}
	if result.Result {
		return nil
	}
	message := result.Reason
	if message == "" {
		message = result.Error.Message
	}
	if message == "" {
		message = action + " failed"
	}
	switch strings.ToLower(result.Error.Category) {
	case "not_found":
		return errs.NewAPIError(errs.SubtypeNotFound, "%s: %s", action, message)
	case "permission_denied", "forbidden":
		return errs.NewPermissionError(errs.SubtypePermissionDenied, "%s: %s", action, message)
	case "task_terminal", "failed_precondition":
		return errs.NewValidationError(errs.SubtypeFailedPrecondition, "%s: %s", action, message)
	case "conflict", "idempotency_conflict":
		return errs.NewAPIError(errs.SubtypeConflict, "%s: %s", action, message)
	case "rate_limit":
		return errs.NewAPIError(errs.SubtypeRateLimit, "%s: %s", action, message).WithRetryable()
	case "internal_route", "server_error":
		return errs.NewAPIError(errs.SubtypeServerError, "%s: %s", action, message).WithRetryable()
	default:
		return errs.NewInternalError(errs.SubtypeInvalidResponse,
			"Base Adapter returned an unknown business error category %q for %s", result.Error.Category, action)
	}
}

func bizErrAsAPIError(in adapterBizErr, action string) error {
	biz := mapBizError(in)
	if biz == nil {
		return nil
	}
	message := biz.Message
	if message == "" {
		message = action + " failed"
	}
	err := errs.NewAPIError(errs.SubtypeUnknown, "%s: %s", action, message)
	if code, ok := bizErrCodeInt(biz.Code); ok {
		err.WithCode(code)
	}
	if isBizRateLimit(biz.Code, biz.Message) {
		err.Subtype = errs.SubtypeRateLimit
		err.WithRetryable()
	}
	return err
}

func mapBizError(in adapterBizErr) *iagents.BizError {
	code := bizErrCodeString(in.BizErrCode)
	message := strings.TrimSpace(in.BizErrMessage)
	if isZeroBizErr(code, message) {
		return nil
	}
	return &iagents.BizError{Code: code, Message: message}
}

func bizErrCodeString(raw json.RawMessage) string {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return ""
	}
	var s string
	if err := json.Unmarshal(trimmed, &s); err == nil {
		return strings.TrimSpace(s)
	}
	var n json.Number
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(&n); err == nil {
		return n.String()
	}
	return strings.TrimSpace(string(trimmed))
}

func isZeroBizErr(code, message string) bool {
	code = strings.TrimSpace(code)
	message = strings.TrimSpace(message)
	return (code == "" || code == "0") && message == ""
}

func bizErrCodeInt(code string) (int, bool) {
	if code == "" {
		return 0, false
	}
	n, err := strconv.Atoi(code)
	if err != nil {
		return 0, false
	}
	return n, true
}

func isBizRateLimit(code, message string) bool {
	if code == "800004907" {
		return true
	}
	lower := strings.ToLower(message)
	return strings.Contains(lower, "rate limit") || strings.Contains(lower, "rate_limit")
}
