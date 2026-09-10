// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

// Internal diagnostic switch, intentionally absent from command/help/schema.
// LARKSUITE_CLI_DOCS_CREATE_DEBUG=1 lark-cli docs +create ...
const docsCreateDebugEnv = "LARKSUITE_CLI_DOCS_CREATE_DEBUG"
const docsCreateDebugPrefix = "[docs-create-debug] "

type docsCreateDebugDetails struct {
	Mode       string  `json:"mode,omitempty"`
	TaskID     string  `json:"task_id,omitempty"`
	Status     string  `json:"status,omitempty"`
	Stage      string  `json:"stage,omitempty"`
	LogID      string  `json:"log_id,omitempty"`
	Poll       int     `json:"poll,omitempty"`
	WaitMS     int64   `json:"wait_ms,omitempty"`
	DurationMS float64 `json:"duration_ms,omitempty"`
	Resources  int     `json:"resources,omitempty"`
	Succeeded  int     `json:"succeeded,omitempty"`
	Failed     int     `json:"failed,omitempty"`
	ErrorType  string  `json:"error_type,omitempty"`
	Subtype    string  `json:"error_subtype,omitempty"`
	Code       int     `json:"error_code,omitempty"`
}

type docsCreateDebugTiming struct {
	Step       string  `json:"step"`
	DurationMS float64 `json:"duration_ms"`
	Percent    float64 `json:"percent"`
}

// Invocation-local and used only on the orchestration goroutine. Detailed
// resource events are observations, not additional buckets in the time total.
type docsCreateTrace struct {
	out       io.Writer
	started   time.Time
	stepStart time.Time
	current   string
	timings   []docsCreateDebugTiming
}

func newDocsCreateTrace(runtime *common.RuntimeContext) *docsCreateTrace {
	if os.Getenv(docsCreateDebugEnv) != "1" || runtime == nil || runtime.Factory == nil || runtime.IO() == nil || runtime.IO().ErrOut == nil {
		return nil
	}
	trace := &docsCreateTrace{out: runtime.IO().ErrOut, started: time.Now()}
	trace.event("start", docsCreateDebugDetails{})
	return trace
}

func (t *docsCreateTrace) event(event string, details docsCreateDebugDetails) {
	if t == nil {
		return
	}
	t.write(struct {
		Event     string  `json:"event"`
		ElapsedMS float64 `json:"elapsed_ms"`
		docsCreateDebugDetails
	}{event, milliseconds(time.Since(t.started)), details})
}

func (t *docsCreateTrace) step(name string) {
	if t == nil {
		return
	}
	t.endStep()
	t.current, t.stepStart = name, time.Now()
	t.event(name+".start", docsCreateDebugDetails{})
}

func (t *docsCreateTrace) endStep() {
	if t == nil || t.current == "" {
		return
	}
	elapsed := milliseconds(time.Since(t.stepStart))
	t.timings = append(t.timings, docsCreateDebugTiming{Step: t.current, DurationMS: elapsed})
	t.event(t.current+".end", docsCreateDebugDetails{DurationMS: elapsed})
	t.current = ""
}

func (t *docsCreateTrace) finish(err error) {
	if t == nil {
		return
	}
	if err != nil {
		t.event("error", docsCreateDebugError(err))
	}
	t.endStep()
	total := milliseconds(time.Since(t.started))
	for i := range t.timings {
		if total > 0 {
			t.timings[i].Percent = math.Round(t.timings[i].DurationMS*10000/total) / 100
		}
	}
	status := "completed"
	if err != nil {
		status = "failed"
	}
	t.write(struct {
		Event   string                  `json:"event"`
		Status  string                  `json:"status"`
		TotalMS float64                 `json:"total_ms"`
		Timings []docsCreateDebugTiming `json:"timings"`
	}{"summary", status, total, t.timings})
}

func (t *docsCreateTrace) write(value interface{}) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return
	}
	// Diagnostics must never change command success or replace its typed error.
	_, _ = fmt.Fprintf(t.out, "%s%s\n", docsCreateDebugPrefix, encoded)
}

func docsCreateDebugError(err error) docsCreateDebugDetails {
	var details docsCreateDebugDetails
	if problem, ok := errs.ProblemOf(err); ok {
		details.ErrorType, details.Subtype = string(problem.Category), string(problem.Subtype)
		details.Code, details.LogID = problem.Code, problem.LogID
	}
	return details
}

func milliseconds(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }
