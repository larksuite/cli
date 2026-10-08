// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package recording lists personal recordings independently of meetings.
package recording

import (
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

const listPath = "/open-apis/vc/v1/recordings"

// Shortcuts returns the recording domain's commands.
func Shortcuts() []common.Shortcut { return []common.Shortcut{List} }

type artifact struct {
	Type   int    `json:"type"`
	ID     string `json:"artifact_id"`
	Status int    `json:"status,omitempty"`
	Token  string `json:"artifact_token,omitempty"`
}

type recording struct {
	ID        string     `json:"recording_id"`
	Start     string     `json:"start_time_ms,omitempty"`
	End       string     `json:"end_time_ms,omitempty"`
	Duration  string     `json:"duration_ms,omitempty"`
	Artifacts []artifact `json:"artifacts,omitempty"`
}

type listResult struct {
	Recordings []recording `json:"recordings"`
	HasMore    bool        `json:"has_more"`
	PageToken  string      `json:"page_token,omitempty"`
}

// timeMillis preserves explicit fractional seconds. Date-only end bounds include
// the last millisecond of the day in the local timezone, matching the API's
// inclusive recording-start-time filter.
func timeMillis(input, flag string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", nil
	}
	var ms int64
	precise, preciseErr := time.Parse(time.RFC3339Nano, input)
	if preciseErr != nil {
		for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02 15:04:05"} {
			if parsed, err := time.ParseInLocation(layout, input, time.Local); err == nil {
				precise, preciseErr = parsed, nil
				break
			}
		}
	}
	if preciseErr == nil {
		ms = precise.UnixMilli()
	} else {
		hint := ""
		if flag == "end" {
			hint = "end"
		}
		parsed, err := common.ParseTime(input, hint)
		if err != nil {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--%s: %v", flag, err).WithParam("--" + flag).WithCause(err)
		}
		sec, err := strconv.ParseInt(parsed, 10, 64)
		if err != nil || sec <= 0 || sec > (math.MaxInt64-999)/1000 {
			return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--%s must be a positive time representable in milliseconds", flag).WithParam("--" + flag)
		}
		ms = sec * 1000
		if _, err := time.Parse("2006-01-02", input); err == nil && flag == "end" {
			ms += 999
		}
	}
	if ms <= 0 {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "--%s must be after the Unix epoch", flag).WithParam("--" + flag)
	}
	return strconv.FormatInt(ms, 10), nil
}

func listParams(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	size, err := common.ValidatePageSizeTyped(runtime, "page-size", 20, 1, 50)
	if err != nil {
		return nil, err
	}
	start, err := timeMillis(runtime.Str("start"), "start")
	if err != nil {
		return nil, err
	}
	end, err := timeMillis(runtime.Str("end"), "end")
	if err != nil {
		return nil, err
	}
	if start != "" && end != "" {
		s, _ := strconv.ParseInt(start, 10, 64)
		e, _ := strconv.ParseInt(end, 10, 64)
		if s > e {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--start must not be after --end").WithParam("--start")
		}
	}
	params := map[string]interface{}{"page_size": strconv.Itoa(size)}
	if token := runtime.Str("page-token"); token != "" {
		params["page_token"] = token
	}
	if start != "" {
		params["start_time_ms"] = start
	}
	if end != "" {
		params["end_time_ms"] = end
	}
	return params, nil
}

// List adapts human time bounds and renders recording artifacts for discovery.
var List = common.Shortcut{
	Service: "recording", Command: "+list",
	Description: "List your recordings by date and discover their Minutes or Note artifacts",
	Risk:        "read", Scopes: []string{"vc:record:readonly"}, AuthTypes: []string{"user"},
	Flags: append([]common.Flag{
		{Name: "start", Desc: "inclusive recording start-time lower bound: ISO 8601, YYYY-MM-DD, or Unix seconds; local timezone if omitted"},
		{Name: "end", Desc: "inclusive recording start-time upper bound: ISO 8601, YYYY-MM-DD (through 23:59:59.999), or Unix seconds; local timezone if omitted"},
		{Name: "page-size", Default: "20", Desc: "records per page (1-50)"},
		{Name: "page-token", Desc: "opaque next-page token; keep the same time filters"},
	}, common.PageAllFlags()...),
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		if _, err := listParams(runtime); err != nil {
			return err
		}
		return common.ValidatePageAllFlags(runtime)
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		params, err := listParams(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().GET(listPath).Params(params)
	},
	Execute: executeList,
}

// AddPage preserves all identifiers and the last page's resumable cursor.
func (result *listResult) AddPage(page listResult) error {
	result.Recordings = append(result.Recordings, page.Recordings...)
	result.HasMore, result.PageToken = page.HasMore, page.PageToken
	return nil
}

func executeList(_ context.Context, runtime *common.RuntimeContext) error {
	params, err := listParams(runtime)
	if err != nil {
		return err
	}
	result := &listResult{Recordings: []recording{}}
	pagination, err := common.PaginateInto(runtime, common.PageRequest{
		Method: http.MethodGet, Path: listPath, Params: params,
	}, result)
	if err != nil {
		return err
	}
	pagination.Items = len(result.Recordings)
	runtime.OutFormat(result, &output.Meta{Count: len(result.Recordings), Pagination: pagination}, func(w io.Writer) { printRecordings(w, result) })
	return nil
}

func printRecordings(w io.Writer, result *listResult) {
	if len(result.Recordings) == 0 {
		fmt.Fprintln(w, "No recordings.")
	}
	rows := make([]map[string]interface{}, 0, len(result.Recordings))
	for _, item := range result.Recordings {
		start := item.Start
		if ms, err := strconv.ParseInt(start, 10, 64); err == nil {
			start = time.UnixMilli(ms).Local().Format(time.RFC3339Nano)
		}
		duration := item.Duration
		if ms, err := strconv.ParseInt(duration, 10, 64); err == nil && ms >= 0 {
			duration = fmt.Sprintf("%dm %02d.%03ds", ms/60000, ms/1000%60, ms%1000)
		}
		labels := make([]string, 0, len(item.Artifacts))
		for _, a := range item.Artifacts {
			kind := enumLabel(a.Type, []string{"unspecified", "Minutes", "Note"})
			status := enumLabel(a.Status, []string{"unspecified", "generating", "ready", "unavailable"})
			id := a.ID
			if a.Type == 1 {
				id = a.Token
				if id == "" {
					id = fmt.Sprintf("minute_token unavailable (artifact_id=%s)", a.ID)
				}
			}
			labels = append(labels, fmt.Sprintf("%s (%s): %s", kind, status, id))
		}
		rows = append(rows, map[string]interface{}{"recording_id": item.ID, "start": start, "duration": duration, "artifacts": strings.Join(labels, "; ")})
	}
	if len(rows) > 0 {
		output.PrintTable(w, rows)
	}
}

func enumLabel(value int, labels []string) string {
	if value >= 0 && value < len(labels) {
		return labels[value]
	}
	return fmt.Sprintf("unknown(%d)", value)
}
