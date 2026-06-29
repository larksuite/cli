// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
)

// --- helpers ---

func cycleListTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	replacer := strings.NewReplacer("/", "-", " ", "-")
	suffix := replacer.Replace(strings.ToLower(t.Name()))
	return &core.CliConfig{
		AppID:     "test-okr-list-" + suffix,
		AppSecret: "secret-okr-list-" + suffix,
		Brand:     core.BrandFeishu,
	}
}

func runCycleListShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "okr"}
	OKRListCycles.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

// --- Validate tests ---

func TestCycleListValidate_InvalidUserIDType(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, cycleListTestConfig(t))
	err := runCycleListShortcut(t, f, stdout, []string{
		"+cycle-list",
		"--user-id", "ou-123",
		"--user-id-type", "invalid_type",
	})
	if err == nil {
		t.Fatal("expected error for invalid --user-id-type")
	}
	if !strings.Contains(err.Error(), "--user-id-type must be one of") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCycleListValidate_ControlCharsInUserID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, cycleListTestConfig(t))
	err := runCycleListShortcut(t, f, stdout, []string{
		"+cycle-list",
		"--user-id", "ou-\t123",
		"--user-id-type", "open_id",
	})
	if err == nil {
		t.Fatal("expected error for control chars in --user-id")
	}
}

func TestCycleListValidate_ControlCharsInTimeRange(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, cycleListTestConfig(t))
	err := runCycleListShortcut(t, f, stdout, []string{
		"+cycle-list",
		"--user-id", "ou-123",
		"--user-id-type", "open_id",
		"--time-range", "2025-01\t--2025-06",
	})
	if err == nil {
		t.Fatal("expected error for control chars in --time-range")
	}
}

func TestCycleListValidate_InvalidTimeRangeFormat(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, cycleListTestConfig(t))
	err := runCycleListShortcut(t, f, stdout, []string{
		"+cycle-list",
		"--user-id", "ou-123",
		"--time-range", "2025-01-2025-06",
	})
	if err == nil {
		t.Fatal("expected error for invalid --time-range format")
	}
	if !strings.Contains(err.Error(), "--time-range") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCycleListValidate_StartAfterEndTimeRange(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, cycleListTestConfig(t))
	err := runCycleListShortcut(t, f, stdout, []string{
		"+cycle-list",
		"--user-id", "ou-123",
		"--time-range", "2025-06--2025-01",
	})
	if err == nil {
		t.Fatal("expected error for start after end in --time-range")
	}
	if !strings.Contains(err.Error(), "--time-range") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCycleListValidate_ValidNoTimeRange(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, cycleListTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{},
			},
		},
	})
	err := runCycleListShortcut(t, f, stdout, []string{
		"+cycle-list",
		"--user-id", "ou-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCycleListValidate_ValidWithTimeRange(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, cycleListTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{},
			},
		},
	})
	err := runCycleListShortcut(t, f, stdout, []string{
		"+cycle-list",
		"--user-id", "ou-123",
		"--time-range", "2025-01--2025-06",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCycleListValidate_AllUserIDTypes(t *testing.T) {
	t.Parallel()
	for _, idType := range []string{"open_id", "union_id", "user_id"} {
		f, stdout, _, reg := cmdutil.TestFactory(t, cycleListTestConfig(t))
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/okr/v2/cycles",
			Body: map[string]interface{}{
				"code": 0,
				"msg":  "ok",
				"data": map[string]interface{}{
					"items": []interface{}{},
				},
			},
		})
		err := runCycleListShortcut(t, f, stdout, []string{
			"+cycle-list",
			"--user-id", "test-id",
			"--user-id-type", idType,
		})
		if err != nil {
			t.Fatalf("user-id-type=%q: unexpected error: %v", idType, err)
		}
	}
}

// --- DryRun tests ---

func TestCycleListDryRun(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, cycleListTestConfig(t))
	err := runCycleListShortcut(t, f, stdout, []string{
		"+cycle-list",
		"--user-id", "ou-456",
		"--user-id-type", "open_id",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "ou-456") {
		t.Fatalf("dry-run output should contain user-id ou-456, got: %s", output)
	}
	if !strings.Contains(output, "/open-apis/okr/v2/cycles") {
		t.Fatalf("dry-run output should contain API path, got: %s", output)
	}
}

func TestCycleListDryRun_WithTimeRange(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, cycleListTestConfig(t))
	err := runCycleListShortcut(t, f, stdout, []string{
		"+cycle-list",
		"--user-id", "ou-789",
		"--time-range", "2025-01--2025-06",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "/open-apis/okr/v2/cycles") {
		t.Fatalf("dry-run output should contain API path, got: %s", output)
	}
}

// --- Execute tests ---

func TestCycleListExecute_NoCycles(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, cycleListTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{},
			},
		},
	})
	err := runCycleListShortcut(t, f, stdout, []string{
		"+cycle-list",
		"--user-id", "ou-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	cycles, _ := data["cycles"].([]interface{})
	if len(cycles) != 0 {
		t.Fatalf("cycles = %v, want empty", cycles)
	}
	// Assert current_active_cycles field exists and is a slice
	rawCurrentActive, ok := data["current_active_cycles"]
	if !ok {
		t.Fatal("current_active_cycles field is missing from response")
	}
	currentActive, ok := rawCurrentActive.([]interface{})
	if !ok {
		t.Fatalf("current_active_cycles is not a slice, got %T", rawCurrentActive)
	}
	if len(currentActive) != 0 {
		t.Fatalf("current_active_cycles = %v, want empty", currentActive)
	}
}

// --- isCurrentActiveCycle unit tests ---

func TestIsCurrentActiveCycle(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		cycle    *Cycle
		expected bool
	}{
		{
			name: "active cycle with normal status",
			cycle: &Cycle{
				ID:          "c1",
				StartTime:   "1767225600000", // 2026-01-01
				EndTime:     "1798761599999", // 2026-12-31 23:59:59
				CycleStatus: CycleStatusNormal.Ptr(),
			},
			expected: true,
		},
		{
			name: "active cycle with default status",
			cycle: &Cycle{
				ID:          "c2",
				StartTime:   "1767225600000", // 2026-01-01
				EndTime:     "1798761599999", // 2026-12-31
				CycleStatus: CycleStatusDefault.Ptr(),
			},
			expected: true,
		},
		{
			name: "cycle with invalid status",
			cycle: &Cycle{
				ID:          "c3",
				StartTime:   "1767225600000", // 2026-01-01
				EndTime:     "1798761599999", // 2026-12-31
				CycleStatus: CycleStatusInvalid.Ptr(),
			},
			expected: false,
		},
		{
			name: "cycle with hidden status",
			cycle: &Cycle{
				ID:          "c4",
				StartTime:   "1767225600000", // 2026-01-01
				EndTime:     "1798761599999", // 2026-12-31
				CycleStatus: CycleStatusHidden.Ptr(),
			},
			expected: false,
		},
		{
			name: "past cycle",
			cycle: &Cycle{
				ID:          "c5",
				StartTime:   "1704067200000", // 2024-01-01
				EndTime:     "1719791999999", // 2024-06-30
				CycleStatus: CycleStatusNormal.Ptr(),
			},
			expected: false,
		},
		{
			name: "future cycle",
			cycle: &Cycle{
				ID:          "c6",
				StartTime:   "1830297600000", // 2028-01-01
				EndTime:     "1861833599999", // 2028-12-31
				CycleStatus: CycleStatusNormal.Ptr(),
			},
			expected: false,
		},
		{
			name: "nil cycle status",
			cycle: &Cycle{
				ID:          "c7",
				StartTime:   "1767225600000", // 2026-01-01
				EndTime:     "1798761599999", // 2026-12-31
				CycleStatus: nil,
			},
			expected: false,
		},
		{
			name: "invalid start time",
			cycle: &Cycle{
				ID:          "c8",
				StartTime:   "invalid",
				EndTime:     "1798761599999", // 2026-12-31
				CycleStatus: CycleStatusNormal.Ptr(),
			},
			expected: false,
		},
		{
			name: "exact start time boundary",
			cycle: &Cycle{
				ID:          "c9",
				StartTime:   "1782734400000", // 2026-06-29 12:00:00 UTC
				EndTime:     "1798761599000", // 2026-12-31 23:59:59 UTC
				CycleStatus: CycleStatusNormal.Ptr(),
			},
			expected: true,
		},
		{
			name: "exact end time boundary",
			cycle: &Cycle{
				ID:          "c10",
				StartTime:   "1767225600000", // 2026-01-01 00:00:00 UTC
				EndTime:     "1782734400000", // 2026-06-29 12:00:00 UTC
				CycleStatus: CycleStatusNormal.Ptr(),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isCurrentActiveCycle(tt.cycle, now)
			if result != tt.expected {
				t.Fatalf("isCurrentActiveCycle() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCycleListExecute_WithCycles(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, cycleListTestConfig(t))

	// Calculate timestamps relative to now to avoid test expiration
	now := time.Now().UTC()
	// Active cycle: 6 months before to 6 months after now
	activeStartMs := now.AddDate(0, -6, 0).UnixMilli()
	activeEndMs := now.AddDate(0, 6, 0).UnixMilli()
	// Past cycle: 2 years before to 1.5 years before now
	pastStartMs := now.AddDate(-2, 0, 0).UnixMilli()
	pastEndMs := now.AddDate(-1, -6, 0).UnixMilli()

	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":              "cycle-active",
						"start_time":      strconv.FormatInt(activeStartMs, 10),
						"end_time":        strconv.FormatInt(activeEndMs, 10),
						"cycle_status":    1, // normal
						"owner":           map[string]interface{}{"owner_type": "user", "user_id": "ou-1"},
						"tenant_cycle_id": "tc-1",
						"score":           0.75,
					},
					map[string]interface{}{
						"id":              "cycle-past",
						"start_time":      strconv.FormatInt(pastStartMs, 10),
						"end_time":        strconv.FormatInt(pastEndMs, 10),
						"cycle_status":    2, // invalid
						"owner":           map[string]interface{}{"owner_type": "user", "user_id": "ou-1"},
						"tenant_cycle_id": "tc-2",
						"score":           0.5,
					},
				},
			},
		},
	})
	err := runCycleListShortcut(t, f, stdout, []string{
		"+cycle-list",
		"--user-id", "ou-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	cycles, _ := data["cycles"].([]interface{})
	if len(cycles) != 2 {
		t.Fatalf("cycles count = %d, want 2", len(cycles))
	}
	total, _ := data["total"].(float64)
	if int(total) != 2 {
		t.Fatalf("total = %v, want 2", total)
	}

	// Check current_active_cycles - should only contain cycle-active
	rawCurrentActive, ok := data["current_active_cycles"]
	if !ok {
		t.Fatal("current_active_cycles field is missing from response")
	}
	currentActive, ok := rawCurrentActive.([]interface{})
	if !ok {
		t.Fatalf("current_active_cycles is not a slice, got %T", rawCurrentActive)
	}
	if len(currentActive) != 1 {
		t.Fatalf("current_active_cycles count = %d, want 1", len(currentActive))
	}
	activeCycle, ok := currentActive[0].(map[string]interface{})
	if !ok {
		t.Fatalf("current_active_cycles[0] is not a map, got %T", currentActive[0])
	}
	if activeCycle["id"] != "cycle-active" {
		t.Fatalf("current_active_cycles[0].id = %v, want cycle-active", activeCycle["id"])
	}

	// Verify removed fields are not present in the response
	for _, c := range cycles {
		cycleMap, _ := c.(map[string]interface{})
		if _, ok := cycleMap["create_time"]; ok {
			t.Fatal("create_time should not be present in response")
		}
		if _, ok := cycleMap["update_time"]; ok {
			t.Fatal("update_time should not be present in response")
		}
		if _, ok := cycleMap["tenant_cycle_id"]; ok {
			t.Fatal("tenant_cycle_id should not be present in response")
		}
		if _, ok := cycleMap["owner"]; ok {
			t.Fatal("owner should not be present in response")
		}
		if _, ok := cycleMap["score"]; ok {
			t.Fatal("score should not be present in response")
		}
	}
}

func TestCycleListExecute_WithTimeRangeFilter(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, cycleListTestConfig(t))

	// Return two cycles: one inside the range, one outside
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":           "cycle-in-range",
						"start_time":   "1735689600000", // 2025-01-01
						"end_time":     "1738368000000", // 2025-02-01
						"cycle_status": 1,
						"owner":        map[string]interface{}{"owner_type": "user", "user_id": "ou-1"},
					},
					map[string]interface{}{
						"id":           "cycle-out-range",
						"start_time":   "1704067200000", // 2024-01-01
						"end_time":     "1706745600000", // 2024-02-01
						"cycle_status": 1,
						"owner":        map[string]interface{}{"owner_type": "user", "user_id": "ou-1"},
					},
				},
			},
		},
	})

	err := runCycleListShortcut(t, f, stdout, []string{
		"+cycle-list",
		"--user-id", "ou-123",
		"--time-range", "2025-01--2025-06",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	cycles, _ := data["cycles"].([]interface{})
	if len(cycles) != 1 {
		t.Fatalf("cycles count = %d, want 1 (only cycle-in-range should pass filter)", len(cycles))
	}
	cycle, _ := cycles[0].(map[string]interface{})
	if cycle["id"] != "cycle-in-range" {
		t.Fatalf("cycle id = %v, want cycle-in-range", cycle["id"])
	}
}

func TestCycleListExecute_Pagination(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, cycleListTestConfig(t))

	// First page
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":           "cycle-p1",
						"start_time":   "1735689600000",
						"end_time":     "1738368000000",
						"cycle_status": 1,
						"owner":        map[string]interface{}{"owner_type": "user", "user_id": "ou-1"},
					},
				},
				"has_more":   true,
				"page_token": "next_page",
			},
		},
	})

	// Second page
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":           "cycle-p2",
						"start_time":   "1738368000000",
						"end_time":     "1743465600000",
						"cycle_status": 1,
						"owner":        map[string]interface{}{"owner_type": "user", "user_id": "ou-1"},
					},
				},
			},
		},
	})

	err := runCycleListShortcut(t, f, stdout, []string{
		"+cycle-list",
		"--user-id", "ou-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, stdout)
	cycles, _ := data["cycles"].([]interface{})
	if len(cycles) != 2 {
		t.Fatalf("cycles count = %d, want 2", len(cycles))
	}
}

func TestCycleListExecute_APIError(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, cycleListTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles",
		Status: 500,
		Body: map[string]interface{}{
			"code": 999,
			"msg":  "internal error",
		},
	})
	err := runCycleListShortcut(t, f, stdout, []string{
		"+cycle-list",
		"--user-id", "ou-123",
	})
	if err == nil {
		t.Fatal("expected error for API failure")
	}
}
