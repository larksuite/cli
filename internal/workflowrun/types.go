// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package workflowrun implements the local-only workflow log mock protocol.
// All wire names and ID mappings are 契约提案，待 T1–T14. No retention duration
// or production OpenAPI support is implied by this package.
package workflowrun

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/larksuite/cli/errs"
)

const (
	FormatVersion = 2
	Runs          = "runs"
	Details       = "details"
	Buttons       = "button_events"
	Guaranteed    = "guaranteed"
	Expired       = "expired"
	Unverified    = "unverified"
	ResponseField = "response_field"
	StrictLegacy  = "strict_equals+legacy_group"
	LegacyCompat  = "legacy_in_compat"
)

var StatusNames = []string{"running", "success", "fail", "not_executed", "pause", "partial_success", "all"}

type Window struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

func (w Window) Wire() [2]string {
	return [2]string{time.UnixMilli(w.Start).UTC().Format(time.RFC3339Nano), time.UnixMilli(w.End).UTC().Format(time.RFC3339Nano)}
}
func ParseTime(s string) (int64, error) {
	if n, e := strconv.ParseInt(s, 10, 64); e == nil {
		return n, nil
	}
	t, e := time.Parse(time.RFC3339Nano, s)
	if e != nil {
		return 0, errs.NewValidationError(errs.SubtypeInvalidArgument, "time must be epoch milliseconds or RFC3339").WithCause(e)
	}
	return t.UnixMilli(), nil
}
func Windows(w Window) []Window {
	out := []Window{}
	for s := w.Start; s < w.End; {
		e := s + int64(24*time.Hour/time.Millisecond)
		if e < s || e > w.End {
			e = w.End
		}
		out = append(out, Window{s, e})
		s = e
	}
	return out
}

type Group struct {
	Ref       string `json:"button_ref"`
	NodeID    string `json:"node_id"`
	Key       string `json:"button_key"`
	Text      string `json:"text,omitempty"`
	FlowID    *int64 `json:"flow_inst_id,omitempty"`
	StepIndex int    `json:"step_index"`
	Legacy    bool   `json:"legacy,omitempty"`
}
type Item struct {
	DataSource      string          `json:"data_source"`
	RunID           string          `json:"run_id"`
	WorkflowID      string          `json:"workflow_id,omitempty"`
	StartTime       int64           `json:"start_time,omitempty"`
	Duration        int64           `json:"duration_ms,omitempty"`
	EditorID        string          `json:"editor_id,omitempty"`
	Status          string          `json:"status,omitempty"`
	RawStatus       json.RawMessage `json:"raw_status,omitempty"`
	NodeID          string          `json:"node_id,omitempty"`
	ParentID        string          `json:"parent_node_id,omitempty"`
	FlowID          *int64          `json:"flow_inst_id,omitempty"`
	StepIndex       int             `json:"step_index,omitempty"`
	EventID         string          `json:"event_id,omitempty"`
	CreatedTime     int64           `json:"created_time,omitempty"`
	Clicker         string          `json:"clicker_id,omitempty"`
	Iteration       string          `json:"iteration,omitempty"`
	ButtonKey       string          `json:"button_key,omitempty"`
	ButtonRef       string          `json:"button_ref,omitempty"`
	Groups          []Group         `json:"buttons,omitempty"`
	Children        []Item          `json:"sub_steps,omitempty"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	SendResults     string          `json:"send_results_availability,omitempty"`
	EmptyRecipients bool            `json:"empty_recipients,omitempty"`
	FetchedAt       int64           `json:"fetched_at,omitempty"`
}

func (i *Item) UnmarshalJSON(b []byte) error {
	type alias Item
	var wire struct {
		*alias
		Status json.RawMessage `json:"status"`
	}
	wire.alias = (*alias)(i)
	if e := json.Unmarshal(b, &wire); e != nil {
		return e
	}
	if len(wire.Status) == 0 {
		return nil
	}
	var s string
	if json.Unmarshal(wire.Status, &s) == nil && s == "unknown" && len(i.RawStatus) > 0 {
		i.Status = s
		return nil
	}
	if json.Unmarshal(wire.Status, &s) == nil {
		for _, v := range StatusNames[:6] {
			if s == v {
				i.Status = s
				return nil
			}
		}
	}
	i.Status = "unknown"
	i.RawStatus = append(json.RawMessage(nil), wire.Status...)
	return nil
}
func Key(plane string, i Item) string {
	switch plane {
	case Runs:
		return i.RunID
	case Buttons:
		return i.EventID
	default:
		f := "unknown"
		if i.FlowID != nil {
			f = strconv.FormatInt(*i.FlowID, 10)
		}
		b, _ := json.Marshal([]any{i.RunID, i.NodeID, f, i.StepIndex})
		return string(b)
	}
}
func Flatten(items []Item) []Item {
	out := []Item{}
	for _, i := range items {
		c := i.Children
		i.Children = nil
		out = append(out, i)
		out = append(out, Flatten(c)...)
	}
	return out
}

type Capabilities struct {
	BaseAggregate   bool   `json:"base_aggregate"`
	CompositeCursor bool   `json:"composite_cursor_ok"`
	Attribution     string `json:"attribution_mode"`
}
type Page struct {
	DataSource      string       `json:"data_source"`
	RunStartedAt    *int64       `json:"run_started_at,omitempty"`
	Plane           string       `json:"plane"`
	Window          [2]string    `json:"window"`
	HasMore         bool         `json:"has_more"`
	Token           string       `json:"page_token"`
	Coverage        string       `json:"coverage"`
	RetainedSince   *string      `json:"retained_since"`
	Uncovered       [][2]string  `json:"uncovered_intervals"`
	Result          string       `json:"result"`
	TruncatedReason string       `json:"truncated_reason,omitempty"`
	MissingScope    string       `json:"permission_scope_missing,omitempty"`
	Items           []Item       `json:"items"`
	Caps            Capabilities `json:"capabilities"`
}
type Query struct {
	Base           string   `json:"base_token"`
	Workflow       string   `json:"workflow_id,omitempty"`
	Run            string   `json:"run_id,omitempty"`
	Window         Window   `json:"window"`
	Status         string   `json:"status"`
	MinDuration    int64    `json:"min_duration_ms,omitempty"`
	MaxDuration    int64    `json:"max_duration_ms,omitempty"`
	Editors        []string `json:"editor_ids,omitempty"`
	IncludePayload bool     `json:"include_payload"`
	Token          string   `json:"page_token,omitempty"`
	PageSize       int      `json:"page_size,omitempty"`
	Group          *Group   `json:"group,omitempty"`
	CollectedAt    int64    `json:"collected_at"`
	Direction      string   `json:"direction,omitempty"`
}
type Params struct {
	Query         Query    `json:"query"`
	Planes        []string `json:"planes"`
	FormatVersion int      `json:"format_version"`
}
type Job struct {
	ID        string      `json:"id"`
	Plane     string      `json:"plane"`
	Query     Query       `json:"query"`
	Done      bool        `json:"done"`
	Seq       int         `json:"checkpoint_seq"`
	Coverage  string      `json:"coverage"`
	Reasons   []string    `json:"reasons"`
	Uncovered [][2]string `json:"uncovered_intervals,omitempty"`
}
type PlaneState struct {
	State      string            `json:"state"`
	QueryRange map[string]Window `json:"query_range,omitempty"`
}
type Selected struct {
	RunID       string `json:"run_id"`
	WorkflowID  string `json:"workflow_id"`
	StartTime   int64  `json:"start_time"`
	CollectedAt int64  `json:"collected_at"`
}
type Manifest struct {
	DataSource      string                `json:"data_source"`
	Params          Params                `json:"parameters"`
	SelectionWindow Window                `json:"run_selection_window"`
	Selected        []Selected            `json:"selected_runs"`
	Jobs            []Job                 `json:"jobs"`
	Planes          map[string]PlaneState `json:"planes"`
	Completeness    string                `json:"completeness"`
	Reasons         []string              `json:"reason_codes"`
	Pending         []string              `json:"pending"`
	Uncovered       [][2]string           `json:"uncovered_intervals,omitempty"`
	Scope           string                `json:"scope"`
}
type Batch struct {
	JobID     string `json:"job_id"`
	Seq       int    `json:"seq"`
	Previous  string `json:"previous_page_token"`
	Query     Query  `json:"query"`
	Page      Page   `json:"page"`
	FetchedAt int64  `json:"fetched_at"`
}
type Checkpoint struct {
	Batch  Batch  `json:"batch"`
	Shard  string `json:"shard"`
	SHA256 string `json:"sha256"`
}
