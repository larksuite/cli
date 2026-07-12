// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package attendance

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/spf13/cobra"
)

const queryURL = "/open-apis/attendance/v1/user_tasks/query"

func defaultConfig() *core.CliConfig {
	return &core.CliConfig{
		AppID: "test-app", AppSecret: "test-secret", Brand: core.BrandFeishu,
		UserOpenId: "ou_testuser", DefaultAs: "user",
	}
}

func mountAndRun(t *testing.T, s common.Shortcut, args []string, f *cmdutil.Factory, stdout *bytes.Buffer) error {
	t.Helper()
	parent := &cobra.Command{Use: "test"}
	s.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

// okResponse builds a successful user_tasks/query envelope with the given results.
func okResponse(results []interface{}) map[string]interface{} {
	return map[string]interface{}{
		"code": 0, "msg": "ok",
		"data": map[string]interface{}{"user_task_results": results},
	}
}

func TestAtoiTS(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{"1609722000", 1609722000},
		{"", 0},
		{"None", 0},
		{"abc", 0},
	}
	for _, c := range cases {
		if got := atoiTS(c.in); got != c.want {
			t.Errorf("atoiTS(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseWorkday_Valid(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"2026-06-01", 20260601},
		{"2026-06-08", 20260608},
		{"2019-08-17", 20190817},
		{"2026-12-31", 20261231},
		{"2026-01-01", 20260101},
	}
	for _, tt := range tests {
		got, err := parseWorkday(tt.in)
		if err != nil {
			t.Errorf("parseWorkday(%q) unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseWorkday(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestParseWorkday_Rejects(t *testing.T) {
	bad := []string{
		"",                          // empty
		"2026-6-1",                  // not zero-padded / not canonical
		"20260601",                  // no separators
		"2026/06/01",                // wrong separator
		"2026-13-01",                // impossible month
		"2026-02-30",                // impossible day
		"2026-06-01T00:00:00+08:00", // carries a time + offset (instant, not a calendar day)
		"2026-06-01 ",               // trailing space
		"June 1, 2026",              // free text
	}
	for _, in := range bad {
		if _, err := parseWorkday(in); err == nil {
			t.Errorf("parseWorkday(%q) = nil error, want rejection", in)
		}
	}
}

func TestFormatWorkday(t *testing.T) {
	tests := []struct {
		in   int
		want string
	}{
		{20260601, "2026-06-01"},
		{20190817, "2019-08-17"},
		{20261231, "2026-12-31"},
	}
	for _, tt := range tests {
		if got := formatWorkday(tt.in); got != tt.want {
			t.Errorf("formatWorkday(%d) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestShiftTypeLabel(t *testing.T) {
	if got := shiftTypeLabel(0); got != "normal" {
		t.Errorf("shiftTypeLabel(0) = %q, want normal", got)
	}
	if got := shiftTypeLabel(1); got != "overtime" {
		t.Errorf("shiftTypeLabel(1) = %q, want overtime", got)
	}
}

func TestOmitNone(t *testing.T) {
	if got := omitNone("None"); got != "" {
		t.Errorf("omitNone(None) = %q, want empty", got)
	}
	if got := omitNone("Leave"); got != "Leave" {
		t.Errorf("omitNone(Leave) = %q, want Leave", got)
	}
	if got := omitNone(""); got != "" {
		t.Errorf("omitNone(\"\") = %q, want empty", got)
	}
}

func TestProjectDetails_FlattensAllFields(t *testing.T) {
	data := &userTasksQueryData{
		UserTaskResults: []userTaskResult{
			{
				EmployeeName: "test-employee", UserID: "test-user-id", GroupID: "g1", ShiftID: "s1",
				Day: 20260601,
				Records: []shiftRecord{
					{
						CheckInResult: "Normal", CheckInResultSupplement: "None",
						CheckInShiftTime: "1609722000",
						CheckInRecord:    &userFlow{CheckTime: "1609722123", LocationName: "test-location"},
						CheckInRecordID:  "rid-in",
						CheckOutResult:   "Late", CheckOutResultSupplement: "None",
						CheckOutShiftTime: "1609754400",
						CheckOutRecord:    &userFlow{CheckTime: "1609755000", LocationName: "test-location"},
						CheckOutRecordID:  "rid-out",
						TaskShiftType:     0,
					},
				},
			},
		},
	}

	got := projectDetails(data)
	if len(got) != 1 {
		t.Fatalf("want 1 detail row, got %d", len(got))
	}
	d := got[0]
	want := punchDetail{
		Date: "2026-06-01", ShiftType: "normal",
		EmployeeName: "test-employee", UserID: "test-user-id", GroupID: "g1", ShiftID: "s1",
		CheckIn: "Normal", CheckInScheduled: 1609722000, CheckInPunchAt: 1609722123,
		CheckInLocation: "test-location", CheckInRecordID: "rid-in",
		CheckOut: "Late", CheckOutScheduled: 1609754400, CheckOutPunchAt: 1609755000,
		CheckOutLocation: "test-location", CheckOutRecordID: "rid-out",
	}
	if !reflect.DeepEqual(d, want) {
		t.Errorf("projectDetails mismatch:\n got: %+v\nwant: %+v", d, want)
	}
}

func TestPunchDetail_Compact(t *testing.T) {
	d := punchDetail{
		Date: "2026-06-01", ShiftType: "overtime",
		EmployeeName: "test-employee", GroupID: "g1",
		CheckIn: "Normal", CheckInSupp: "ManagerModification", CheckInPunchAt: 123, CheckInLocation: "test-location",
		CheckOut: "Late", CheckOutPunchAt: 456,
	}
	got := d.compact()
	want := punchRow{
		Date: "2026-06-01", ShiftType: "overtime",
		CheckIn: "Normal", CheckInSupp: "ManagerModification", CheckOut: "Late",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("compact() = %+v, want %+v (must drop punch_at/location/employee/group)", got, want)
	}
}

func TestProjectDetails_EmptyAndNil(t *testing.T) {
	if got := projectDetails(nil); len(got) != 0 {
		t.Errorf("projectDetails(nil) = %+v, want empty", got)
	}
	if got := projectDetails(&userTasksQueryData{}); len(got) != 0 {
		t.Errorf("projectDetails(empty) = %+v, want empty", got)
	}
}

// ---------------------------------------------------------------------------
// +records shortcut behaviour
// ---------------------------------------------------------------------------

func TestRecords_MissingFrom_IsValidationError(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, AttendanceRecords, []string{"+records"}, f, nil)
	if err == nil {
		t.Fatal("expected a validation error when --from is omitted, got nil")
	}
	if !strings.Contains(err.Error(), "--from") {
		t.Errorf("error should name --from, got: %v", err)
	}
}

func TestRecords_InvalidFrom_IsValidationError(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, AttendanceRecords, []string{"+records", "--from", "2026/06/01"}, f, nil)
	if err == nil {
		t.Fatal("expected a validation error for a malformed --from, got nil")
	}
}

func TestRecords_FromAfterTo_IsValidationError(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, AttendanceRecords, []string{"+records", "--from", "2026-06-02", "--to", "2026-06-01"}, f, nil)
	if err == nil {
		t.Fatal("expected a validation error when --from is after --to, got nil")
	}
}

func TestRecords_RejectsBotIdentity(t *testing.T) {
	f, _, _, _ := cmdutil.TestFactory(t, defaultConfig())
	err := mountAndRun(t, AttendanceRecords, []string{"+records", "--from", "2026-06-01", "--as", "bot"}, f, nil)
	if err == nil {
		t.Fatal("expected an error for bot identity on a user-only shortcut, got nil")
	}
}

// TestRecords_SingleDay_CollapsesToFromEqualsTo pins that omitting --to queries a
// single workday, and that the constant fields are auto-injected into the body.
func TestRecords_SingleDay_CollapsesToFromEqualsTo(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	stub := &httpmock.Stub{Method: "POST", URL: queryURL, Body: okResponse(nil)}
	reg.Register(stub)

	if err := mountAndRun(t, AttendanceRecords, []string{"+records", "--from", "2026-06-01"}, f, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var body struct {
		UserIDs            []string `json:"user_ids"`
		CheckDateFrom      int      `json:"check_date_from"`
		CheckDateTo        int      `json:"check_date_to"`
		NeedOvertimeResult bool     `json:"need_overtime_result"`
	}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("captured body not valid JSON: %v (%s)", err, stub.CapturedBody)
	}
	if body.CheckDateFrom != 20260601 || body.CheckDateTo != 20260601 {
		t.Errorf("single-day should set from==to==20260601, got from=%d to=%d", body.CheckDateFrom, body.CheckDateTo)
	}
	if body.UserIDs == nil || len(body.UserIDs) != 0 {
		t.Errorf("user_ids should serialise to an empty array, got %#v", body.UserIDs)
	}
	if !body.NeedOvertimeResult {
		t.Errorf("need_overtime_result should default to true")
	}
}

// detailResult builds one user_task_result with a full nested punch flow,
// including the device-fingerprint fields the projection must drop.
func detailResult() interface{} {
	return map[string]interface{}{
		"employee_name": "test-employee", "user_id": "test-user-id", "group_id": "g1", "shift_id": "s1",
		"day": 20260601,
		"records": []interface{}{
			map[string]interface{}{
				"check_in_result": "Normal", "check_in_result_supplement": "None",
				"check_in_shift_time": "1609722000",
				"check_in_record": map[string]interface{}{
					"check_time": "1609722123", "location_name": "test-location",
					"ssid": "office-wifi", "bssid": "aa:bb:cc", "device_id": "dev-1",
				},
				"check_in_record_id": "rid-in",
				"check_out_result":   "Late", "check_out_result_supplement": "None",
				"check_out_shift_time": "1609754400",
				"check_out_record": map[string]interface{}{
					"check_time": "1609755000", "location_name": "test-location",
				},
				"check_out_record_id": "rid-out",
				"task_shift_type":     0,
			},
		},
	}
}

func TestRecords_Default_ProjectsCompactItems(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{Method: "POST", URL: queryURL, Body: okResponse([]interface{}{detailResult()})})

	if err := mountAndRun(t, AttendanceRecords, []string{"+records", "--from", "2026-06-01"}, f, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "\"check_in\": \"Normal\"") {
		t.Errorf("default should include result enum, got:\n%s", out)
	}
	for _, leaked := range []string{"check_in_punch_at", "check_in_location", "employee_name", "group_id"} {
		if strings.Contains(out, leaked) {
			t.Errorf("default (compact) must NOT include %q, got:\n%s", leaked, out)
		}
	}
}

func TestRecords_Detail_ProjectsFullItems(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{Method: "POST", URL: queryURL, Body: okResponse([]interface{}{detailResult()})})

	if err := mountAndRun(t, AttendanceRecords, []string{"+records", "--from", "2026-06-01", "--detail"}, f, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env struct {
		Data struct {
			Items []punchDetail `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout not JSON envelope: %v\n%s", err, stdout.String())
	}
	if len(env.Data.Items) != 1 {
		t.Fatalf("want 1 detail item, got %d", len(env.Data.Items))
	}
	d := env.Data.Items[0]
	if d.CheckInPunchAt != 1609722123 || d.CheckInLocation != "test-location" || d.EmployeeName != "test-employee" || d.GroupID != "g1" {
		t.Errorf("detail item missing expected fields: %+v", d)
	}
}

func TestRecords_Detail_ExcludesFingerprintFields(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{Method: "POST", URL: queryURL, Body: okResponse([]interface{}{detailResult()})})

	if err := mountAndRun(t, AttendanceRecords, []string{"+records", "--from", "2026-06-01", "--detail"}, f, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	for _, leaked := range []string{"ssid", "bssid", "device_id", "office-wifi", "dev-1"} {
		if strings.Contains(out, leaked) {
			t.Errorf("--detail must NOT leak fingerprint field %q, got:\n%s", leaked, out)
		}
	}
}

func TestRecords_PrettyFallsBackToJSON(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, defaultConfig())
	reg.Register(&httpmock.Stub{Method: "POST", URL: queryURL, Body: okResponse([]interface{}{detailResult()})})

	if err := mountAndRun(t, AttendanceRecords, []string{"+records", "--from", "2026-06-01", "--format", "pretty"}, f, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var env struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("pretty should fall back to JSON envelope, got non-JSON:\n%s", stdout.String())
	}
	if !env.OK {
		t.Errorf("expected ok=true envelope, got:\n%s", stdout.String())
	}
}

func TestRecords_DryRun_ShowsRequestShape(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, defaultConfig())
	if err := mountAndRun(t, AttendanceRecords, []string{"+records", "--from", "2026-06-01", "--dry-run"}, f, stdout); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "user_tasks/query") {
		t.Errorf("dry-run should show the endpoint path, got:\n%s", out)
	}
	if !strings.Contains(out, "employee_no") {
		t.Errorf("dry-run should show the employee_type=employee_no query param, got:\n%s", out)
	}
}
