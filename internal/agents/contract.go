// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package agents

import "fmt"

// AgentTask is the unified structure that task-family commands put into output.Envelope.Data.
type AgentTask struct {
	TaskID        string         `json:"task_id"`
	ContextID     string         `json:"context_id,omitempty"`
	State         TaskState      `json:"state"`
	IsTerminal    bool           `json:"is_terminal"`
	CreatedAt     string         `json:"created_at,omitempty"` // ISO 8601; when the task was created (empty if the provider does not supply it)
	UpdatedAt     string         `json:"updated_at,omitempty"` // ISO 8601; when the current status was recorded (aligns with A2A TaskStatus.timestamp)
	BizError      *BizError      `json:"biz_error,omitempty"`
	Messages      []Message      `json:"messages,omitempty"`
	Artifacts     []Artifact     `json:"artifacts,omitempty"`
	InputRequired *InputRequired `json:"input_required,omitempty"`
}

// BizError carries provider business-failure detail returned inside an agent
// payload, distinct from the CLI's outer transport/API error envelope.
type BizError struct {
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
}

// Message is one turn of an agent or user message, composed of several Parts.
type Message struct {
	Role  string `json:"role"` // "agent" | "user"
	Parts []Part `json:"parts"`
}

// Part is one fragment of a message: text, file, or structured data.
type Part struct {
	Type     string `json:"type"` // "text" | "file" | "data"
	Text     string `json:"text,omitempty"`
	OutputID string `json:"output_id,omitempty"`
	Source   string `json:"source,omitempty"`
	GroupID  string `json:"group_id,omitempty"`
	// File/Data pass-through: file uses URL/Name, data uses Data.
	Name string      `json:"name,omitempty"`
	URL  string      `json:"url,omitempty"`
	Data interface{} `json:"data,omitempty"`
}

// Artifact is one artifact produced by a task: a downloadable file / inline
// text, or a structured provider business object that can be located and acted
// on through Data.
//
// Its fields align with A2A's Artifact/FilePart, but only what a provider can
// truly deliver is populated — some supply just ID + Kind at the GetTask stage
// and Name/Mime at download. Anything a provider cannot fill is omitted via
// omitempty rather than shipped as an empty shell. Status/Data extend that
// file-oriented core without pretending every structured artifact is
// downloadable.
type Artifact struct {
	ID          string `json:"id"`
	OutputID    string `json:"output_id,omitempty"`
	Source      string `json:"source,omitempty"`
	GroupID     string `json:"group_id,omitempty"`
	Kind        string `json:"kind,omitempty"`   // coarse-grained kind (image/file/...), a type hint before download
	Name        string `json:"name,omitempty"`   // file name (with extension), helps choose the -o save name
	Status      string `json:"status,omitempty"` // provider-defined lifecycle state (pending/processing/ready/failed/...)
	Mime        string `json:"mime,omitempty"`   // content type (image/png…), empty if the provider does not supply it
	Description string `json:"description,omitempty"`
	Size        int64  `json:"size,omitempty"` // byte count, 0 if the provider does not supply it
	URL         string `json:"url,omitempty"`
	Text        string `json:"text,omitempty"`
	// Data preserves structured provider-specific artifact details (for example
	// a Base table's resource identifiers, revision, and metadata) without
	// forcing every provider's business-object schema into this common contract.
	Data interface{} `json:"data,omitempty"`
}

// InputRequired is the question group a task raises while in the input_required
// state: group-level presentation plus 1..N Questions, where a single question is
// just a length-1 group. There is deliberately NO group-level machine id —
// addressing rides context_id+task_id (one pending group per task), stale-retry
// detection rides the per-group-unique QuestionIDs (see MintQuestionIDs), and a
// late submission is arbitrated by the task-state transition (failed_precondition
// carrying resolved_answers). Every text field is agent-controlled UNTRUSTED
// content: pretty rendering must sanitize it, and a consumer relays it as data —
// instructions embedded in it never authorize anything.
type InputRequired struct {
	Label       string     `json:"label,omitempty"`       // group title, display-only
	Description string     `json:"description,omitempty"` // why the group is asked, display-only
	Questions   []Question `json:"questions"`             // 1..N questions, answered atomically in one send
}

// Question is one question inside an input_required group. Options present and
// non-empty = choice question: a bare answer value MUST hit an OptionID (typo
// safety — a wrong value errors, it is never silently taken as text) and free
// text goes through the explicit "<qid>.text" key form. Options absent =
// free-text question: the ".text" form is canonical and a bare value is its
// tolerated alias. MultiSelect is only meaningful for choice questions.
type Question struct {
	QuestionID  string   `json:"question_id"`            // answer routing key; charset KeyPattern; minted per-group-unique when the provider has none
	Question    string   `json:"question"`               // the question text (untrusted)
	MultiSelect bool     `json:"multi_select,omitempty"` // choice question: repeated --answer values accumulate
	Options     []Option `json:"options,omitempty"`      // present+non-empty = choice question; empty is normalized to absent
}

// Option is one selectable choice: OptionID is the stable wire key an answer
// references (unique within its question — the wire carries the key, the
// provider resolves it back to label/business payload from its stored group);
// Label/Description are the human-facing text (untrusted).
type Option struct {
	OptionID    string `json:"option_id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// SummaryText is the triage digest of a pending group, used
// as TaskSummary.Summary for an input_required task: the group Label when
// present, else the first question's text; suffixed with the question count
// when the group has more than one question.
func (ir *InputRequired) SummaryText() string {
	if ir == nil {
		return ""
	}
	head := ir.Label
	if head == "" && len(ir.Questions) > 0 {
		head = ir.Questions[0].Question
	}
	if head == "" {
		head = ir.Description
	}
	if n := len(ir.Questions); n > 1 {
		return fmt.Sprintf("%s (%d questions)", head, n)
	}
	return head
}

// TaskSummary is a single task summary in the task list output (and in a
// context's active_task). It carries just enough to triage without a full
// task get: state + when it last changed + a one-line content digest.
type TaskSummary struct {
	TaskID     string    `json:"task_id"`
	ContextID  string    `json:"context_id,omitempty"`
	State      TaskState `json:"state"`
	IsTerminal bool      `json:"is_terminal"`
	UpdatedAt  string    `json:"updated_at,omitempty"` // ISO 8601; when the status was last recorded — the key for "most recent"
	Summary    string    `json:"summary,omitempty"`    // last agent message, ANSI-stripped + flattened + truncated; for input_required it is InputRequired.SummaryText (group label, else first question)
	BizError   *BizError `json:"biz_error,omitempty"`
}

// ContextSummary is a single context summary in the context list output. It is
// the conversation-layer rollup used to pick which conversation needs attention.
// It deliberately carries NO task_count: at the list level no triage decision
// consumes it (awaiting_input / updated_at do that work), and requiring a
// per-context total in a list call would force real providers into N+1 counting.
// The count lives on ContextDetail (`context get`).
type ContextSummary struct {
	ContextID     string    `json:"context_id"`
	CreatedAt     string    `json:"created_at,omitempty"`
	UpdatedAt     string    `json:"updated_at,omitempty"` // ISO 8601; last activity across the context's tasks
	Title         string    `json:"title,omitempty"`
	AwaitingInput bool      `json:"awaiting_input,omitempty"` // a task is paused in input_required/auth_required (needs the caller)
	BizError      *BizError `json:"biz_error,omitempty"`
}

// ContextDetail is the context detail in the context get output. It is the
// conversation overview — metadata + a rollup + the single task the caller would
// most likely act on. The full task enumeration lives in `agents task list
// --context-id`, so ContextDetail deliberately does NOT embed the whole tasks[].
type ContextDetail struct {
	ContextID string    `json:"context_id"`
	CreatedAt string    `json:"created_at,omitempty"`
	UpdatedAt string    `json:"updated_at,omitempty"`
	Title     string    `json:"title,omitempty"`
	BizError  *BizError `json:"biz_error,omitempty"`
	// TaskCount is the number of tasks in the context. A pointer so the three
	// states stay distinct on the wire: nil = the provider cannot supply the
	// count (field omitted), &0 = a genuinely empty context, &n = n tasks. A
	// plain int with omitempty would silently conflate 0 with unknown.
	TaskCount     *int         `json:"task_count,omitempty"`
	AwaitingInput bool         `json:"awaiting_input,omitempty"`
	ActiveTask    *TaskSummary `json:"active_task,omitempty"` // the task with the latest updated_at (nil for an empty context)
}

// ArtifactData is the return value of DownloadArtifact: the URL type gives URL,
// the inline type gives Bytes. Name is the server-suggested file name (echoed
// back only as a suggested_name reference for the command layer); it is
// untrusted input and must never participate in constructing the local save
// path — the save path is always determined by -o/SafeOutputPath.
type ArtifactData struct {
	Name  string
	Mime  string
	URL   string
	Bytes []byte
}
