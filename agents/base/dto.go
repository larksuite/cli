// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"encoding/json"
	"fmt"
)

type adapterSendRequest struct {
	ContextID      string            `json:"context_id,omitempty"`
	TaskID         string            `json:"task_id,omitempty"`
	Message        adapterMessage    `json:"message"`
	Params         map[string]string `json:"params,omitempty"`
	IdempotencyKey string            `json:"idempotency_key"`
	Metadata       map[string]string `json:"metadata"`
}

type adapterMessage struct {
	MessageID string        `json:"message_id,omitempty"`
	Role      string        `json:"role"`
	Parts     []adapterPart `json:"parts,omitempty"`
	Text      string        `json:"text,omitempty"`
}

// answersDataKind marks the DataPart that carries an input_required reply.
const answersDataKind = "answers"

// answersDataSchemaVersion is the wire schema version of the answers payload.
// It is included in the canonical encoding so a schema change forces a new
// deterministic id (an old-schema retry never dedupes against a new one).
const answersDataSchemaVersion = 1

// answersDataPart is the typed body of a kind=answers DataPart. The bridge
// serializes it into adapterPart.Data; the backend decodes the same shape and
// restores the pending clarification card from payload.answers (public
// question_id → option_id/text values). Keys/values ride argv order from the
// command layer; the deterministic id derived alongside canonicalizes them.
type answersDataPart struct {
	Kind          string             `json:"kind"`
	SchemaVersion int                `json:"schema_version"`
	Payload       answersDataPayload `json:"payload"`
}

type answersDataPayload struct {
	Answers map[string][]string `json:"answers"`
}

type adapterPart struct {
	Type string          `json:"type"`
	Text string          `json:"text,omitempty"`
	Name string          `json:"name,omitempty"`
	URL  string          `json:"url,omitempty"`
	Data json.RawMessage `json:"data,omitempty"`
}

type adapterArtifact struct {
	ID   string `json:"id"`
	Kind string `json:"kind,omitempty"`
	Name string `json:"name,omitempty"`
	URL  string `json:"url,omitempty"`
	Text string `json:"text,omitempty"`
}

type adapterTask struct {
	adapterBizErr

	// SchemaVersion/Status/Outputs are the v1 detail contract returned by
	// SendMessage and GetTask. The remaining fields are retained while task-list
	// and context endpoints still return the legacy summary shape.
	SchemaVersion int             `json:"schema_version,omitempty"`
	ID            string          `json:"id,omitempty"`
	TaskID        string          `json:"task_id,omitempty"`
	ContextID     string          `json:"context_id,omitempty"`
	State         string          `json:"state,omitempty"`
	Status        string          `json:"status,omitempty"`
	CreatedAt     json.RawMessage `json:"created_at,omitempty"`
	UpdatedAt     json.RawMessage `json:"updated_at,omitempty"`
	Summary       string          `json:"summary,omitempty"`
	Outputs       []adapterOutput `json:"outputs,omitempty"`

	// Legacy detail fields are accepted only for the schema_version=0 rollout
	// compatibility path. New v1 responses must use Outputs.
	Messages  []adapterMessage  `json:"messages,omitempty"`
	Artifacts []adapterArtifact `json:"artifacts,omitempty"`
}

func (t *adapterTask) UnmarshalJSON(data []byte) error {
	type taskAlias adapterTask
	var raw struct {
		adapterBizErr
		SnakeBizErr adapterBizErr `json:"-"`
		Task        *taskAlias    `json:"task,omitempty"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if err := decodeBizErrFields(data, &raw.SnakeBizErr); err != nil {
		return err
	}
	if raw.Task != nil {
		*t = adapterTask(*raw.Task)
		t.adapterBizErr = mergeBizErr(t.adapterBizErr, raw.adapterBizErr)
		t.adapterBizErr = mergeBizErr(t.adapterBizErr, raw.SnakeBizErr)
		return nil
	}
	var decoded taskAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*t = adapterTask(decoded)
	t.adapterBizErr = mergeBizErr(t.adapterBizErr, raw.adapterBizErr)
	t.adapterBizErr = mergeBizErr(t.adapterBizErr, raw.SnakeBizErr)
	return nil
}

type adapterTaskList struct {
	adapterBizErr

	Tasks      []adapterTask `json:"tasks"`
	HasMore    bool          `json:"has_more"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

func (l *adapterTaskList) UnmarshalJSON(data []byte) error {
	tasks, hasMore, nextCursor, bizErr, err := decodeAdapterList[adapterTask](data, "tasks")
	if err != nil {
		return err
	}
	*l = adapterTaskList{adapterBizErr: bizErr, Tasks: tasks, HasMore: hasMore, NextCursor: nextCursor}
	return nil
}

type adapterOutput struct {
	ID            string                 `json:"id"`
	Type          string                 `json:"type"`
	Source        string                 `json:"source,omitempty"`
	GroupID       string                 `json:"group_id,omitempty"`
	Text          string                 `json:"text,omitempty"`
	Data          *adapterStructuredData `json:"data,omitempty"`
	Clarification *adapterClarification  `json:"clarification,omitempty"`
	Artifact      *adapterOutputArtifact `json:"artifact,omitempty"`
	Raw           json.RawMessage        `json:"-"`
}

func (o *adapterOutput) UnmarshalJSON(data []byte) error {
	type outputAlias adapterOutput
	var decoded outputAlias
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*o = adapterOutput(decoded)
	o.Raw = append(json.RawMessage(nil), data...)
	return nil
}

type adapterStructuredData struct {
	Kind          string          `json:"kind"`
	SchemaVersion int             `json:"schema_version"`
	Payload       json.RawMessage `json:"payload"`
}

type adapterClarification struct {
	ID            string                             `json:"id"`
	Title         string                             `json:"title,omitempty"`
	Required      bool                               `json:"required"`
	Submitted     bool                               `json:"submitted"`
	Questions     []adapterClarificationQuestion     `json:"questions,omitempty"`
	Forms         []adapterClarificationForm         `json:"forms,omitempty"`
	Buttons       []adapterClarificationButton       `json:"buttons,omitempty"`
	DefaultAction *adapterClarificationDefaultAction `json:"default_action,omitempty"`
}

type adapterClarificationQuestion struct {
	ID               string                         `json:"id"`
	Type             string                         `json:"type"`
	Prompt           string                         `json:"prompt"`
	Required         bool                           `json:"required"`
	AllowCustomInput bool                           `json:"allow_custom_input,omitempty"`
	Options          []adapterClarificationOption   `json:"options,omitempty"`
	SubQuestions     []adapterClarificationQuestion `json:"sub_questions,omitempty"`
	Answered         bool                           `json:"answered,omitempty"`
	Answer           *adapterClarificationAnswer    `json:"answer,omitempty"`
}

type adapterClarificationOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

type adapterClarificationAnswer struct {
	OptionIDs []string        `json:"option_ids,omitempty"`
	Value     json.RawMessage `json:"value,omitempty"`
}

type adapterClarificationForm struct {
	ID        string                         `json:"id"`
	Title     string                         `json:"title,omitempty"`
	Questions []adapterClarificationQuestion `json:"questions,omitempty"`
	Buttons   []adapterClarificationButton   `json:"buttons,omitempty"`
}

type adapterClarificationButton struct {
	ID           string `json:"id"`
	Kind         string `json:"kind"`
	Style        string `json:"style,omitempty"`
	Label        string `json:"label"`
	Default      bool   `json:"default,omitempty"`
	Message      string `json:"message,omitempty"`
	ConfirmText  string `json:"confirm_text,omitempty"`
	ActionParams string `json:"action_params,omitempty"`
}

type adapterClarificationDefaultAction struct {
	ButtonText   string `json:"button_text,omitempty"`
	ActionParams string `json:"action_params,omitempty"`
}

type adapterOutputArtifact struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Title    string            `json:"title,omitempty"`
	Status   string            `json:"status"`
	Resource map[string]string `json:"resource,omitempty"`
	Revision *int64            `json:"revision,omitempty"`
	Metadata json.RawMessage   `json:"metadata,omitempty"`
}

type adapterContext struct {
	adapterBizErr

	ID        string          `json:"id,omitempty"`
	ContextID string          `json:"context_id,omitempty"`
	Title     string          `json:"title,omitempty"`
	Status    string          `json:"status,omitempty"`
	CreatedAt json.RawMessage `json:"created_at,omitempty"`
	UpdatedAt json.RawMessage `json:"updated_at,omitempty"`
	Tasks     []adapterTask   `json:"tasks,omitempty"`
}

type adapterContextList struct {
	adapterBizErr

	Contexts   []adapterContext `json:"contexts"`
	HasMore    bool             `json:"has_more"`
	NextCursor string           `json:"next_cursor,omitempty"`
}

func (l *adapterContextList) UnmarshalJSON(data []byte) error {
	contexts, hasMore, nextCursor, bizErr, err := decodeAdapterList[adapterContext](data, "contexts")
	if err != nil {
		return err
	}
	*l = adapterContextList{adapterBizErr: bizErr, Contexts: contexts, HasMore: hasMore, NextCursor: nextCursor}
	return nil
}

func decodeAdapterList[T any](data []byte, itemsField string) ([]T, bool, string, adapterBizErr, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, false, "", adapterBizErr{}, fmt.Errorf("Base Agent list response is empty")
	}
	if trimmed[0] == '[' {
		var items []T
		if err := json.Unmarshal(trimmed, &items); err != nil {
			return nil, false, "", adapterBizErr{}, err
		}
		return items, false, "", adapterBizErr{}, nil
	}
	if trimmed[0] != '{' {
		return nil, false, "", adapterBizErr{}, fmt.Errorf("Base Agent list response must be an object")
	}

	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &envelope); err != nil {
		return nil, false, "", adapterBizErr{}, err
	}
	var bizErr adapterBizErr
	if err := decodeBizErrFields(trimmed, &bizErr); err != nil {
		return nil, false, "", adapterBizErr{}, fmt.Errorf("decode Base Agent list response business error: %w", err)
	}
	itemsRaw, ok := envelope[itemsField]
	if !ok || bytes.Equal(bytes.TrimSpace(itemsRaw), []byte("null")) {
		return nil, false, "", adapterBizErr{}, fmt.Errorf("Base Agent list response is missing %q", itemsField)
	}
	var items []T
	if err := json.Unmarshal(itemsRaw, &items); err != nil {
		return nil, false, "", adapterBizErr{}, fmt.Errorf("decode Base Agent list response %q: %w", itemsField, err)
	}

	hasMoreRaw, ok := envelope["has_more"]
	if !ok || bytes.Equal(bytes.TrimSpace(hasMoreRaw), []byte("null")) {
		return nil, false, "", adapterBizErr{}, fmt.Errorf("Base Agent list response is missing %q", "has_more")
	}
	var hasMore bool
	if err := json.Unmarshal(hasMoreRaw, &hasMore); err != nil {
		return nil, false, "", adapterBizErr{}, fmt.Errorf("decode Base Agent list response %q: %w", "has_more", err)
	}

	var nextCursor string
	if nextCursorRaw, ok := envelope["next_cursor"]; ok {
		if bytes.Equal(bytes.TrimSpace(nextCursorRaw), []byte("null")) {
			return nil, false, "", adapterBizErr{}, fmt.Errorf("Base Agent list response %q must be a string", "next_cursor")
		}
		if err := json.Unmarshal(nextCursorRaw, &nextCursor); err != nil {
			return nil, false, "", adapterBizErr{}, fmt.Errorf("decode Base Agent list response %q: %w", "next_cursor", err)
		}
	}
	if hasMore != (nextCursor != "") {
		return nil, false, "", adapterBizErr{}, fmt.Errorf("Base Agent list response has inconsistent pagination fields")
	}
	return items, hasMore, nextCursor, bizErr, nil
}

type adapterBizErr struct {
	BizErrCode    json.RawMessage `json:"BizErrCode,omitempty"`
	BizErrMessage string          `json:"BizErrMessage,omitempty"`
}

func decodeBizErrFields(data []byte, out *adapterBizErr) error {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(data, &envelope); err != nil {
		return err
	}
	for _, field := range []string{"BizErrCode", "biz_err_code"} {
		if raw, ok := envelope[field]; ok {
			out.BizErrCode = append(json.RawMessage(nil), raw...)
			break
		}
	}
	for _, field := range []string{"BizErrMessage", "biz_err_message"} {
		if raw, ok := envelope[field]; ok {
			if err := json.Unmarshal(raw, &out.BizErrMessage); err != nil {
				return fmt.Errorf("%s: %w", field, err)
			}
			break
		}
	}
	return nil
}

func mergeBizErr(base, overlay adapterBizErr) adapterBizErr {
	if len(base.BizErrCode) == 0 && len(overlay.BizErrCode) > 0 {
		base.BizErrCode = append(json.RawMessage(nil), overlay.BizErrCode...)
	}
	if base.BizErrMessage == "" && overlay.BizErrMessage != "" {
		base.BizErrMessage = overlay.BizErrMessage
	}
	return base
}

type adapterBusinessError struct {
	Category string `json:"category,omitempty"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message,omitempty"`
}

type adapterResult struct {
	adapterBizErr

	Result bool                 `json:"result"`
	Reason string               `json:"reason,omitempty"`
	Error  adapterBusinessError `json:"error,omitempty"`
}
