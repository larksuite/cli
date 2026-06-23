// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/spf13/cobra"
)

// testReorderItem implements reorderItem for testing.
type testReorderItem struct {
	id string
}

func (t testReorderItem) GetID() string { return t.id }

func reorderTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	return &core.CliConfig{
		AppID:     "test-okr-reorder",
		AppSecret: "secret-okr-reorder",
		Brand:     core.BrandFeishu,
	}
}

func runReorderShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "okr"}
	OKRReorder.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

// --- Validate tests ---

func TestReorderValidate_MissingLevel(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, reorderTestConfig(t))
	err := runReorderShortcut(t, f, stdout, []string{
		"+reorder",
		"--cycle-id", "123",
		"--ops", `[{"id":"1","position":1}]`,
	})
	// cobra Required:true reports flag name without "--" prefix
	if err == nil || !strings.Contains(err.Error(), "level") {
		t.Fatalf("expected --level required error, got: %v", err)
	}
}

func TestReorderValidate_InvalidLevel(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, reorderTestConfig(t))
	err := runReorderShortcut(t, f, stdout, []string{
		"+reorder",
		"--level", "invalid",
		"--cycle-id", "123",
		"--ops", `[{"id":"1","position":1}]`,
	})
	if err == nil {
		t.Fatal("expected error for invalid --level")
	}
	_, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	validationErr, ok := err.(*errs.ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got: %T", err)
	}
	if validationErr.Param != "--level" {
		t.Fatalf("expected param --level, got %q", validationErr.Param)
	}
}

func TestReorderValidate_MissingCycleID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, reorderTestConfig(t))
	err := runReorderShortcut(t, f, stdout, []string{
		"+reorder",
		"--level", "objective",
		"--ops", `[{"id":"1","position":1}]`,
	})
	// cobra Required:true reports flag name without "--" prefix
	if err == nil || !strings.Contains(err.Error(), "cycle-id") {
		t.Fatalf("expected --cycle-id required error, got: %v", err)
	}
}

func TestReorderValidate_MissingObjectiveIDForKRLevel(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, reorderTestConfig(t))
	err := runReorderShortcut(t, f, stdout, []string{
		"+reorder",
		"--level", "key-result",
		"--cycle-id", "123",
		"--ops", `[{"id":"1","position":1}]`,
	})
	if err == nil {
		t.Fatal("expected error for missing --objective-id when --level=key-result")
	}
	_, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	validationErr, ok := err.(*errs.ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got: %T", err)
	}
	if validationErr.Param != "--objective-id" {
		t.Fatalf("expected param --objective-id, got %q", validationErr.Param)
	}
}

func TestReorderValidate_MissingOps(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, reorderTestConfig(t))
	err := runReorderShortcut(t, f, stdout, []string{
		"+reorder",
		"--level", "objective",
		"--cycle-id", "123",
	})
	// cobra Required:true reports flag name without "--" prefix
	if err == nil || !strings.Contains(err.Error(), "ops") {
		t.Fatalf("expected --ops required error, got: %v", err)
	}
}

func TestReorderValidate_InvalidOpsJSON(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, reorderTestConfig(t))
	err := runReorderShortcut(t, f, stdout, []string{
		"+reorder",
		"--level", "objective",
		"--cycle-id", "123",
		"--ops", "not-json",
	})
	if err == nil {
		t.Fatal("expected error for invalid --ops JSON")
	}
	prob, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if prob.Category != errs.CategoryValidation {
		t.Fatalf("expected CategoryValidation, got %q", prob.Category)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected error to be *errs.ValidationError, got: %T", err)
	}
	if !errors.Is(err, validationErr) {
		t.Fatal("errors.Is should find the ValidationError in the chain")
	}
	if validationErr.Param != "--ops" {
		t.Fatalf("expected param --ops, got %q", validationErr.Param)
	}
}

func TestReorderValidate_EmptyOpsArray(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, reorderTestConfig(t))
	err := runReorderShortcut(t, f, stdout, []string{
		"+reorder",
		"--level", "objective",
		"--cycle-id", "123",
		"--ops", "[]",
	})
	if err == nil {
		t.Fatal("expected error for empty --ops array")
	}
	_, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	validationErr, ok := err.(*errs.ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got: %T", err)
	}
	if validationErr.Param != "--ops" {
		t.Fatalf("expected param --ops, got %q", validationErr.Param)
	}
}

func TestReorderValidate_DuplicateID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, reorderTestConfig(t))
	err := runReorderShortcut(t, f, stdout, []string{
		"+reorder",
		"--level", "objective",
		"--cycle-id", "123",
		"--ops", `[{"id":"1","position":1},{"id":"1","position":2}]`,
	})
	if err == nil {
		t.Fatal("expected error for duplicate id in --ops")
	}
	prob, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if prob.Category != errs.CategoryValidation {
		t.Fatalf("expected CategoryValidation, got %q", prob.Category)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected error to be *errs.ValidationError, got: %T", err)
	}
	if !errors.Is(err, validationErr) {
		t.Fatal("errors.Is should find the ValidationError in the chain")
	}
	if validationErr.Param != "--ops" {
		t.Fatalf("expected param --ops, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "duplicate id") {
		t.Fatalf("expected error to mention duplicate id, got: %v", err)
	}
}

func TestReorderValidate_DuplicatePosition(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, reorderTestConfig(t))
	err := runReorderShortcut(t, f, stdout, []string{
		"+reorder",
		"--level", "objective",
		"--cycle-id", "123",
		"--ops", `[{"id":"1","position":1},{"id":"2","position":1}]`,
	})
	if err == nil {
		t.Fatal("expected error for duplicate position in --ops")
	}
	prob, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if prob.Category != errs.CategoryValidation {
		t.Fatalf("expected CategoryValidation, got %q", prob.Category)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected error to be *errs.ValidationError, got: %T", err)
	}
	if !errors.Is(err, validationErr) {
		t.Fatal("errors.Is should find the ValidationError in the chain")
	}
	if validationErr.Param != "--ops" {
		t.Fatalf("expected param --ops, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "duplicate position") {
		t.Fatalf("expected error to mention duplicate position, got: %v", err)
	}
}

func TestReorderValidate_NegativePosition(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, reorderTestConfig(t))
	err := runReorderShortcut(t, f, stdout, []string{
		"+reorder",
		"--level", "objective",
		"--cycle-id", "123",
		"--ops", `[{"id":"1","position":0}]`,
	})
	if err == nil {
		t.Fatal("expected error for position <= 0")
	}
	_, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	validationErr, ok := err.(*errs.ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got: %T", err)
	}
	if validationErr.Param != "--ops" {
		t.Fatalf("expected param --ops, got %q", validationErr.Param)
	}
}

// --- DryRun tests ---

func TestReorderDryRun_Objectives(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, reorderTestConfig(t))
	err := runReorderShortcut(t, f, stdout, []string{
		"+reorder",
		"--level", "objective",
		"--cycle-id", "123",
		"--ops", `[{"id":"1","position":2},{"id":"2","position":1}]`,
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "/open-apis/okr/v2/cycles/123/objectives") {
		t.Fatalf("dry-run output should contain objectives list API path, got: %s", output)
	}
	if !strings.Contains(output, "GET") {
		t.Fatalf("dry-run output should contain GET method for list, got: %s", output)
	}
	if !strings.Contains(output, "/open-apis/okr/v2/cycles/123/objectives_position") {
		t.Fatalf("dry-run output should contain position update API path, got: %s", output)
	}
	if !strings.Contains(output, "PUT") {
		t.Fatalf("dry-run output should contain PUT method for update, got: %s", output)
	}
	if !strings.Contains(output, "objective_ids") {
		t.Fatalf("dry-run output should contain objective_ids in body, got: %s", output)
	}
}

func TestReorderDryRun_KeyResults(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, reorderTestConfig(t))
	err := runReorderShortcut(t, f, stdout, []string{
		"+reorder",
		"--level", "key-result",
		"--cycle-id", "123",
		"--objective-id", "456",
		"--ops", `[{"id":"1","position":2},{"id":"2","position":1}]`,
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "/open-apis/okr/v2/objectives/456/key_results") {
		t.Fatalf("dry-run output should contain key_results list API path, got: %s", output)
	}
	if !strings.Contains(output, "/open-apis/okr/v2/objectives/456/key_results_position") {
		t.Fatalf("dry-run output should contain key_results position update API path, got: %s", output)
	}
	if !strings.Contains(output, "key_result_ids") {
		t.Fatalf("dry-run output should contain key_result_ids in body, got: %s", output)
	}
}

// --- Execute tests ---

func TestReorderExecute_Objectives_Success(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, reorderTestConfig(t))
	// Mock fetch objectives
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles/123/objectives",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": "1", "position": 1, "cycle_id": "123", "owner": map[string]interface{}{"owner_type": "user"}},
					map[string]interface{}{"id": "2", "position": 2, "cycle_id": "123", "owner": map[string]interface{}{"owner_type": "user"}},
					map[string]interface{}{"id": "3", "position": 3, "cycle_id": "123", "owner": map[string]interface{}{"owner_type": "user"}},
				},
				"has_more": false,
			},
		},
	})
	// Mock reorder
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/okr/v2/cycles/123/objectives_position",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": "3", "position": 1},
					map[string]interface{}{"id": "1", "position": 2},
					map[string]interface{}{"id": "2", "position": 3},
				},
			},
		},
	})
	err := runReorderShortcut(t, f, stdout, []string{
		"+reorder",
		"--level", "objective",
		"--cycle-id", "123",
		"--ops", `[{"id":"3","position":1}]`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify output
	data := decodeEnvelope(t, stdout)
	if data["level"] != "objective" {
		t.Fatalf("expected level=objective, got %v", data["level"])
	}
	if data["cycle_id"] != "123" {
		t.Fatalf("expected cycle_id=123, got %v", data["cycle_id"])
	}
	ordered, _ := data["ordered"].([]interface{})
	if len(ordered) != 3 {
		t.Fatalf("expected 3 items in ordered list, got %d", len(ordered))
	}
	// First should be 3, then 1, then 2
	if ordered[0] != "3" || ordered[1] != "1" || ordered[2] != "2" {
		t.Fatalf("expected ordered [3,1,2], got %v", ordered)
	}
}

func TestReorderExecute_KeyResults_Success(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, reorderTestConfig(t))
	// Mock fetch key results
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/objectives/456/key_results",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": "kr1", "position": 1, "objective_id": "456", "owner": map[string]interface{}{"owner_type": "user"}},
					map[string]interface{}{"id": "kr2", "position": 2, "objective_id": "456", "owner": map[string]interface{}{"owner_type": "user"}},
				},
				"has_more": false,
			},
		},
	})
	// Mock reorder
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/okr/v2/objectives/456/key_results_position",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": "kr2", "position": 1},
					map[string]interface{}{"id": "kr1", "position": 2},
				},
			},
		},
	})
	err := runReorderShortcut(t, f, stdout, []string{
		"+reorder",
		"--level", "key-result",
		"--cycle-id", "123",
		"--objective-id", "456",
		"--ops", `[{"id":"kr2","position":1}]`,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify output
	data := decodeEnvelope(t, stdout)
	if data["level"] != "key-result" {
		t.Fatalf("expected level=key-result, got %v", data["level"])
	}
	if data["cycle_id"] != "123" {
		t.Fatalf("expected cycle_id=123, got %v", data["cycle_id"])
	}
	ordered, _ := data["ordered"].([]interface{})
	if len(ordered) != 2 {
		t.Fatalf("expected 2 items in ordered list, got %d", len(ordered))
	}
	if ordered[0] != "kr2" || ordered[1] != "kr1" {
		t.Fatalf("expected ordered [kr2,kr1], got %v", ordered)
	}
}

func TestReorderExecute_PositionOutOfRange(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, reorderTestConfig(t))
	// Mock fetch objectives (only 2 items)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles/123/objectives",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": "1", "position": 1, "cycle_id": "123", "owner": map[string]interface{}{"owner_type": "user"}},
					map[string]interface{}{"id": "2", "position": 2, "cycle_id": "123", "owner": map[string]interface{}{"owner_type": "user"}},
				},
				"has_more": false,
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/okr/v2/cycles/123/objectives_position",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
		BodyFilter: func(body []byte) bool {
			var data map[string]interface{}
			if err := json.Unmarshal(body, &data); err != nil {
				return false
			}
			ids, ok := data["objective_ids"].([]interface{})
			if !ok || len(ids) != 2 {
				return false
			}
			// position 5 should be clamped to position 2 (last), so order is [2, 1]
			return ids[0] == "2" && ids[1] == "1"
		},
	})
	err := runReorderShortcut(t, f, stdout, []string{
		"+reorder",
		"--level", "objective",
		"--cycle-id", "123",
		"--ops", `[{"id":"1","position":5}]`, // position 5 exceeds total of 2, should clamp to last
	})
	if err != nil {
		t.Fatalf("unexpected error for out-of-range position (should clamp): %v", err)
	}
}

func TestReorderExecute_IDNotFound(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, reorderTestConfig(t))
	// Mock fetch objectives
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles/123/objectives",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": "1", "position": 1, "cycle_id": "123", "owner": map[string]interface{}{"owner_type": "user"}},
					map[string]interface{}{"id": "2", "position": 2, "cycle_id": "123", "owner": map[string]interface{}{"owner_type": "user"}},
				},
				"has_more": false,
			},
		},
	})
	err := runReorderShortcut(t, f, stdout, []string{
		"+reorder",
		"--level", "objective",
		"--cycle-id", "123",
		"--ops", `[{"id":"999","position":1}]`, // ID 999 doesn't exist
	})
	if err == nil {
		t.Fatal("expected error for non-existent ID")
	}
	prob, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if prob.Category != errs.CategoryValidation {
		t.Fatalf("expected CategoryValidation, got %q", prob.Category)
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected error to be *errs.ValidationError, got: %T", err)
	}
	if !errors.Is(err, validationErr) {
		t.Fatal("errors.Is should find the ValidationError in the chain")
	}
	if validationErr.Param != "--ops" {
		t.Fatalf("expected param --ops, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected error to mention not found, got: %v", err)
	}
}

// --- Unit tests for helper functions ---

func TestParseReorderOps_Valid(t *testing.T) {
	t.Parallel()
	ops, err := parseReorderOps(`[{"id":"1","position":2},{"id":"2","position":1}]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("expected 2 ops, got %d", len(ops))
	}
	if ops[0].ID != "1" || ops[0].Position != 2 {
		t.Fatalf("expected op[0] = {1,2}, got %+v", ops[0])
	}
	if ops[1].ID != "2" || ops[1].Position != 1 {
		t.Fatalf("expected op[1] = {2,1}, got %+v", ops[1])
	}
}

func TestBuildReorderedIDs(t *testing.T) {
	t.Parallel()
	items := []Objective{
		{ID: "1"},
		{ID: "2"},
		{ID: "3"},
		{ID: "4"},
	}
	ops := []reorderOp{
		{ID: "4", Position: 1},
		{ID: "2", Position: 3},
	}
	result, err := buildReorderedIDs(items, ops, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expected: 4 at pos1, 1 at pos2 (unchanged), 2 at pos3, 3 at pos4
	expected := []string{"4", "1", "2", "3"}
	if len(result) != len(expected) {
		t.Fatalf("expected %d items, got %d", len(expected), len(result))
	}
	for i, id := range expected {
		if result[i] != id {
			t.Fatalf("expected result[%d] = %q, got %q", i, id, result[i])
		}
	}
}

func TestBuildReorderedIDs_SingleClampToEnd(t *testing.T) {
	t.Parallel()
	items := []testReorderItem{
		{id: "1"}, {id: "2"}, {id: "3"}, {id: "4"},
	}
	ops := []reorderOp{
		{ID: "1", Position: 99}, // position 99 exceeds total of 4, should clamp to last
	}
	result, err := buildReorderedIDs(items, ops, 4)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expected: 2 at pos1, 3 at pos2, 4 at pos3, 1 at pos4 (clamped)
	expected := []string{"2", "3", "4", "1"}
	for i, id := range expected {
		if result[i] != id {
			t.Fatalf("expected result[%d] = %q, got %q", i, id, result[i])
		}
	}
}

func TestBuildReorderedIDs_MultipleClampToEnd(t *testing.T) {
	t.Parallel()
	items := []testReorderItem{
		{id: "1"}, {id: "2"}, {id: "3"}, {id: "4"}, {id: "5"},
	}
	ops := []reorderOp{
		{ID: "1", Position: 10}, // position 10 exceeds total of 5
		{ID: "2", Position: 20}, // position 20 exceeds total of 5
	}
	result, err := buildReorderedIDs(items, ops, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expected: 3 at pos1, 4 at pos2, 5 at pos3, 1 at pos4 (clamped, pos10 < pos20), 2 at pos5 (clamped)
	expected := []string{"3", "4", "5", "1", "2"}
	for i, id := range expected {
		if result[i] != id {
			t.Fatalf("expected result[%d] = %q, got %q", i, id, result[i])
		}
	}
}

func TestBuildReorderedIDs_MixedClamp(t *testing.T) {
	t.Parallel()
	items := []testReorderItem{
		{id: "1"}, {id: "2"}, {id: "3"}, {id: "4"}, {id: "5"},
	}
	ops := []reorderOp{
		{ID: "5", Position: 1},  // normal position
		{ID: "1", Position: 99}, // clamped to end
		{ID: "2", Position: 50}, // clamped to end, but position 50 < 99, so comes before 1
	}
	result, err := buildReorderedIDs(items, ops, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expected: 5 at pos1, 3 at pos2, 4 at pos3, 2 at pos4 (clamped pos50), 1 at pos5 (clamped pos99)
	expected := []string{"5", "3", "4", "2", "1"}
	for i, id := range expected {
		if result[i] != id {
			t.Fatalf("expected result[%d] = %q, got %q", i, id, result[i])
		}
	}
}

func TestBuildReorderedIDs_LargePositionSafe(t *testing.T) {
	t.Parallel()
	items := []testReorderItem{
		{id: "1"}, {id: "2"}, {id: "3"},
	}
	// Very large position should not cause memory issues with map-based implementation
	ops := []reorderOp{
		{ID: "1", Position: 100000000}, // 10^8, would be dangerous with slice
	}
	result, err := buildReorderedIDs(items, ops, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expected: 2 at pos1, 3 at pos2, 1 at pos3 (clamped to end)
	expected := []string{"2", "3", "1"}
	for i, id := range expected {
		if result[i] != id {
			t.Fatalf("expected result[%d] = %q, got %q", i, id, result[i])
		}
	}
}
