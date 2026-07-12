// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package attendance

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const userTasksQueryPath = "/open-apis/attendance/v1/user_tasks/query"

type userTasksQueryReq struct {
	UserIDs            []string `json:"user_ids"` // empty (non-nil) array = self-query
	CheckDateFrom      int      `json:"check_date_from"`
	CheckDateTo        int      `json:"check_date_to"`
	NeedOvertimeResult bool     `json:"need_overtime_result"` // always true: surface overtime segments via task_shift_type
}

// userFlow is the punch flow nested under check_in_record / check_out_record.
// The device-fingerprint fields (ssid/bssid/device_id/photo_urls/...) are left
// undeclared on purpose so they never reach output — zero agent value, and a
// surveillance/privacy surface.
type userFlow struct {
	CheckTime    string `json:"check_time"`
	LocationName string `json:"location_name"`
}

type shiftRecord struct {
	CheckInResult            string    `json:"check_in_result"`
	CheckInResultSupplement  string    `json:"check_in_result_supplement"`
	CheckInShiftTime         string    `json:"check_in_shift_time"`
	CheckInRecord            *userFlow `json:"check_in_record"`
	CheckInRecordID          string    `json:"check_in_record_id"`
	CheckOutResult           string    `json:"check_out_result"`
	CheckOutResultSupplement string    `json:"check_out_result_supplement"`
	CheckOutShiftTime        string    `json:"check_out_shift_time"`
	CheckOutRecord           *userFlow `json:"check_out_record"`
	CheckOutRecordID         string    `json:"check_out_record_id"`
	TaskShiftType            int       `json:"task_shift_type"`
}

type userTaskResult struct {
	EmployeeName string        `json:"employee_name"`
	UserID       string        `json:"user_id"`
	GroupID      string        `json:"group_id"`
	ShiftID      string        `json:"shift_id"`
	Day          int           `json:"day"`
	Records      []shiftRecord `json:"records"`
}

type userTasksQueryData struct {
	UserTaskResults []userTaskResult `json:"user_task_results"`
}

type userTasksQueryResp struct {
	Data *userTasksQueryData `json:"data"`
}

type punchRow struct {
	Date         string `json:"date"`
	ShiftType    string `json:"shift_type"`
	CheckIn      string `json:"check_in"`
	CheckInSupp  string `json:"check_in_supplement,omitempty"`
	CheckOut     string `json:"check_out"`
	CheckOutSupp string `json:"check_out_supplement,omitempty"`
}

// punchDetail is the single projection target; the default view derives from it
// via compact(). Time fields are raw second-level unix timestamps — no
// rendering, no timezone arithmetic.
type punchDetail struct {
	Date         string `json:"date"`
	ShiftType    string `json:"shift_type"`
	EmployeeName string `json:"employee_name,omitempty"`
	UserID       string `json:"user_id,omitempty"`
	GroupID      string `json:"group_id,omitempty"`
	ShiftID      string `json:"shift_id,omitempty"`

	CheckIn          string `json:"check_in"`
	CheckInSupp      string `json:"check_in_supplement,omitempty"`
	CheckInScheduled int64  `json:"check_in_scheduled_at,omitempty"`
	CheckInPunchAt   int64  `json:"check_in_punch_at,omitempty"`
	CheckInLocation  string `json:"check_in_location,omitempty"`
	CheckInRecordID  string `json:"check_in_record_id,omitempty"`

	CheckOut          string `json:"check_out"`
	CheckOutSupp      string `json:"check_out_supplement,omitempty"`
	CheckOutScheduled int64  `json:"check_out_scheduled_at,omitempty"`
	CheckOutPunchAt   int64  `json:"check_out_punch_at,omitempty"`
	CheckOutLocation  string `json:"check_out_location,omitempty"`
	CheckOutRecordID  string `json:"check_out_record_id,omitempty"`
}

func (d punchDetail) compact() punchRow {
	return punchRow{
		Date:         d.Date,
		ShiftType:    d.ShiftType,
		CheckIn:      d.CheckIn,
		CheckInSupp:  d.CheckInSupp,
		CheckOut:     d.CheckOut,
		CheckOutSupp: d.CheckOutSupp,
	}
}

// parseWorkday converts a strict YYYY-MM-DD date into a yyyyMMdd integer. The
// round-trip guard rejects non-canonical widths (e.g. "2026-6-1") that
// time.Parse would otherwise accept, so the caller and the API mean the same day.
func parseWorkday(s string) (int, error) {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return 0, fmt.Errorf("expected a calendar date in YYYY-MM-DD form, got %q", s)
	}
	if t.Format("2006-01-02") != s {
		return 0, fmt.Errorf("expected a calendar date in YYYY-MM-DD form, got %q", s)
	}
	return t.Year()*10000 + int(t.Month())*100 + t.Day(), nil
}

func formatWorkday(d int) string {
	return fmt.Sprintf("%04d-%02d-%02d", d/10000, (d/100)%100, d%100)
}

func shiftTypeLabel(t int) string {
	if t == 1 {
		return "overtime"
	}
	return "normal"
}

// omitNone collapses the API's "None" sentinel to "" so omitempty drops it.
func omitNone(s string) string {
	if s == "None" {
		return ""
	}
	return s
}

// atoiTS returns 0 for empty/"None"/unparseable input so omitempty drops the
// field rather than emitting a misleading 0.
func atoiTS(s string) int64 {
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// flowTime / flowLoc tolerate a nil record (the API omits it for shifts that
// never required a punch, e.g. NoNeedCheck).
func flowTime(f *userFlow) int64 {
	if f == nil {
		return 0
	}
	return atoiTS(f.CheckTime)
}

func flowLoc(f *userFlow) string {
	if f == nil {
		return ""
	}
	return f.LocationName
}

// projectDetails is the single typed boundary: it flattens the nested results
// into one punchDetail per shift segment so nothing downstream touches the raw maps.
func projectDetails(d *userTasksQueryData) []punchDetail {
	if d == nil {
		return nil
	}
	var rows []punchDetail
	for _, t := range d.UserTaskResults {
		date := formatWorkday(t.Day)
		for _, r := range t.Records {
			rows = append(rows, punchDetail{
				Date: date, ShiftType: shiftTypeLabel(r.TaskShiftType),
				EmployeeName: t.EmployeeName, UserID: t.UserID, GroupID: t.GroupID, ShiftID: t.ShiftID,
				CheckIn: r.CheckInResult, CheckInSupp: omitNone(r.CheckInResultSupplement),
				CheckInScheduled: atoiTS(r.CheckInShiftTime), CheckInPunchAt: flowTime(r.CheckInRecord),
				CheckInLocation: flowLoc(r.CheckInRecord), CheckInRecordID: r.CheckInRecordID,
				CheckOut: r.CheckOutResult, CheckOutSupp: omitNone(r.CheckOutResultSupplement),
				CheckOutScheduled: atoiTS(r.CheckOutShiftTime), CheckOutPunchAt: flowTime(r.CheckOutRecord),
				CheckOutLocation: flowLoc(r.CheckOutRecord), CheckOutRecordID: r.CheckOutRecordID,
			})
		}
	}
	return rows
}

// parseRange is the single source of truth for the date contract, shared by
// Validate, DryRun and Execute so the rules can't drift. --from is required;
// --to defaults to --from (single-day query).
func parseRange(runtime *common.RuntimeContext) (from, to int, err error) {
	fromStr := runtime.Str("from")
	if fromStr == "" {
		return 0, 0, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--from is required, e.g. --from 2026-06-01 (a YYYY-MM-DD workday)").WithParam("--from")
	}
	from, perr := parseWorkday(fromStr)
	if perr != nil {
		return 0, 0, errs.NewValidationError(errs.SubtypeInvalidArgument, "--from: %v", perr).WithParam("--from")
	}

	toStr := runtime.Str("to")
	if toStr == "" {
		return from, from, nil
	}
	to, perr = parseWorkday(toStr)
	if perr != nil {
		return 0, 0, errs.NewValidationError(errs.SubtypeInvalidArgument, "--to: %v", perr).WithParam("--to")
	}
	if to < from {
		return 0, 0, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--to (%s) must not be earlier than --from (%s)", toStr, fromStr).WithParam("--to")
	}
	return from, to, nil
}

// queryBody fixes the boilerplate the meta API made callers supply by hand —
// employee_type (a query param) and an always-empty user_ids — so a self query
// never asks for it.
func queryBody(from, to int) *userTasksQueryReq {
	return &userTasksQueryReq{
		UserIDs:            []string{},
		CheckDateFrom:      from,
		CheckDateTo:        to,
		NeedOvertimeResult: true,
	}
}

var AttendanceRecords = common.Shortcut{
	Service:     "attendance",
	Command:     "+records",
	Description: "Query your own attendance punch results over a workday range",
	Risk:        "read",
	Scopes:      []string{"attendance:task:readonly"},
	AuthTypes:   []string{"user"}, // self-query needs a user token; a bot has no "self"
	Flags: []common.Flag{
		{Name: "from", Desc: "start workday YYYY-MM-DD (required)"},
		{Name: "to", Desc: "end workday YYYY-MM-DD (defaults to --from, i.e. a single day)"},
		{Name: "detail", Type: "bool", Desc: "return full detail (punch time, location, scheduled time, group/shift/employee); default returns result enums only"},
	},
	Tips: []string{
		"Single day: lark-cli attendance +records --from 2026-06-01",
		"Date range: lark-cli attendance +records --from 2026-06-01 --to 2026-06-08",
		"For punch time / location / group detail: add --detail",
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		from, to, err := parseRange(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			POST(userTasksQueryPath).
			Params(map[string]interface{}{"employee_type": "employee_no"}).
			Body(queryBody(from, to))
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, _, err := parseRange(runtime)
		return err
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		from, to, err := parseRange(runtime)
		if err != nil {
			return err
		}

		apiResp, err := runtime.DoAPI(&larkcore.ApiReq{
			HttpMethod:  "POST",
			ApiPath:     userTasksQueryPath,
			QueryParams: larkcore.QueryParams{"employee_type": []string{"employee_no"}},
			Body:        queryBody(from, to),
		})
		if err != nil {
			if _, ok := errs.ProblemOf(err); ok {
				return err
			}
			return errs.WrapInternal(err)
		}
		if _, err := runtime.ClassifyAPIResponse(apiResp); err != nil {
			return err
		}

		var resp userTasksQueryResp
		if err := json.Unmarshal(apiResp.RawBody, &resp); err != nil {
			return errs.NewInternalError(errs.SubtypeInvalidResponse, "unmarshal attendance response failed").WithCause(err)
		}

		// Default derives the compact view; --detail keeps the full rows. items[]
		// and meta.count stay consistent across both.
		details := projectDetails(resp.Data)
		var items interface{}
		if runtime.Bool("detail") {
			items = details
		} else {
			rows := make([]punchRow, 0, len(details))
			for _, d := range details {
				rows = append(rows, d.compact())
			}
			items = rows
		}

		// prettyFn=nil → --format pretty falls back to JSON; table/csv/ndjson still render the flattened rows.
		runtime.OutFormat(map[string]interface{}{"items": items}, &output.Meta{Count: len(details)}, nil)
		return nil
	},
}
