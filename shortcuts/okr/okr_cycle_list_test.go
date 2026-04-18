// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"bytes"
	"strings"
	"testing"

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
}

func TestCycleListExecute_WithCycles(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, cycleListTestConfig(t))
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{
						"id":              "cycle-1",
						"start_time":      "1735689600000",
						"end_time":        "1751318400000",
						"cycle_status":    1,
						"owner":           map[string]interface{}{"owner_type": "user", "user_id": "ou-1"},
						"tenant_cycle_id": "tc-1",
						"score":           0.75,
					},
					map[string]interface{}{
						"id":              "cycle-2",
						"start_time":      "1704067200000",
						"end_time":        "1719792000000",
						"cycle_status":    2,
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
