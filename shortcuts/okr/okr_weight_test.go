// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/spf13/cobra"
)

// getWeightFloat extracts a float64 weight from either float64 or json.Number.
func getWeightFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case json.Number:
		f, _ := val.Float64()
		return f
	default:
		return 0
	}
}

func weightTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	return &core.CliConfig{
		AppID:     "test-okr-weight",
		AppSecret: "secret-okr-weight",
		Brand:     core.BrandFeishu,
	}
}

func runWeightShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "okr"}
	OKRWeight.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

// --- Validate tests ---

func TestWeightValidate_MissingLevel(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, weightTestConfig(t))
	err := runWeightShortcut(t, f, stdout, []string{
		"+weight",
		"--cycle-id", "123",
		"--weights", `[{"id":"1","weight":0.5}]`,
	})
	// cobra Required:true reports flag name without "--" prefix
	if err == nil || !strings.Contains(err.Error(), "level") {
		t.Fatalf("expected --level required error, got: %v", err)
	}
}

func TestWeightValidate_InvalidLevel(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, weightTestConfig(t))
	err := runWeightShortcut(t, f, stdout, []string{
		"+weight",
		"--level", "invalid",
		"--cycle-id", "123",
		"--weights", `[{"id":"1","weight":0.5}]`,
	})
	if err == nil {
		t.Fatal("expected error for invalid --level")
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
	if validationErr.Param != "--level" {
		t.Fatalf("expected param --level, got %q", validationErr.Param)
	}
}

func TestWeightValidate_MissingCycleID(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, weightTestConfig(t))
	err := runWeightShortcut(t, f, stdout, []string{
		"+weight",
		"--level", "objective",
		"--weights", `[{"id":"1","weight":0.5}]`,
	})
	// cobra Required:true reports flag name without "--" prefix
	if err == nil || !strings.Contains(err.Error(), "cycle-id") {
		t.Fatalf("expected --cycle-id required error, got: %v", err)
	}
}

func TestWeightValidate_MissingObjectiveIDForKRLevel(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, weightTestConfig(t))
	err := runWeightShortcut(t, f, stdout, []string{
		"+weight",
		"--level", "key-result",
		"--cycle-id", "123",
		"--weights", `[{"id":"1","weight":0.5}]`,
	})
	if err == nil {
		t.Fatal("expected error for missing --objective-id when --level=key-result")
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
	if validationErr.Param != "--objective-id" {
		t.Fatalf("expected param --objective-id, got %q", validationErr.Param)
	}
}

func TestWeightValidate_MissingWeights(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, weightTestConfig(t))
	err := runWeightShortcut(t, f, stdout, []string{
		"+weight",
		"--level", "objective",
		"--cycle-id", "123",
	})
	// cobra Required:true reports flag name without "--" prefix
	if err == nil || !strings.Contains(err.Error(), "weights") {
		t.Fatalf("expected --weights required error, got: %v", err)
	}
}

func TestWeightValidate_InvalidWeightsJSON(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, weightTestConfig(t))
	err := runWeightShortcut(t, f, stdout, []string{
		"+weight",
		"--level", "objective",
		"--cycle-id", "123",
		"--weights", "not-json",
	})
	if err == nil {
		t.Fatal("expected error for invalid --weights JSON")
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
	if validationErr.Param != "--weights" {
		t.Fatalf("expected param --weights, got %q", validationErr.Param)
	}
}

func TestWeightValidate_EmptyWeightsArray(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, weightTestConfig(t))
	err := runWeightShortcut(t, f, stdout, []string{
		"+weight",
		"--level", "objective",
		"--cycle-id", "123",
		"--weights", "[]",
	})
	if err == nil {
		t.Fatal("expected error for empty --weights array")
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
	if validationErr.Param != "--weights" {
		t.Fatalf("expected param --weights, got %q", validationErr.Param)
	}
}

func TestWeightValidate_NegativeWeight(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, weightTestConfig(t))
	err := runWeightShortcut(t, f, stdout, []string{
		"+weight",
		"--level", "objective",
		"--cycle-id", "123",
		"--weights", `[{"id":"1","weight":-0.1}]`,
	})
	if err == nil {
		t.Fatal("expected error for negative weight")
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
	if validationErr.Param != "--weights" {
		t.Fatalf("expected param --weights, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "non-negative") {
		t.Fatalf("expected error to mention non-negative, got: %v", err)
	}
}

func TestWeightValidate_WeightGreaterThanOne(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, weightTestConfig(t))
	err := runWeightShortcut(t, f, stdout, []string{
		"+weight",
		"--level", "objective",
		"--cycle-id", "123",
		"--weights", `[{"id":"1","weight":1.5}]`,
	})
	if err == nil {
		t.Fatal("expected error for weight > 1")
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
	if validationErr.Param != "--weights" {
		t.Fatalf("expected param --weights, got %q", validationErr.Param)
	}
}

func TestWeightValidate_TooManyDecimalPlaces(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, weightTestConfig(t))
	err := runWeightShortcut(t, f, stdout, []string{
		"+weight",
		"--level", "objective",
		"--cycle-id", "123",
		"--weights", `[{"id":"1","weight":0.1234}]`,
	})
	if err == nil {
		t.Fatal("expected error for weight with more than 3 decimal places")
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
	if validationErr.Param != "--weights" {
		t.Fatalf("expected param --weights, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "3 decimal places") {
		t.Fatalf("expected error to mention 3 decimal places, got: %v", err)
	}
}

func TestWeightValidate_SumGreaterThanOne(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, weightTestConfig(t))
	err := runWeightShortcut(t, f, stdout, []string{
		"+weight",
		"--level", "objective",
		"--cycle-id", "123",
		"--weights", `[{"id":"1","weight":0.6},{"id":"2","weight":0.5}]`,
	})
	if err == nil {
		t.Fatal("expected error for sum of weights > 1")
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
	if validationErr.Param != "--weights" {
		t.Fatalf("expected param --weights, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "sum of weights") {
		t.Fatalf("expected error to mention sum of weights, got: %v", err)
	}
}

func TestWeightValidate_DuplicateID(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, weightTestConfig(t))
	err := runWeightShortcut(t, f, stdout, []string{
		"+weight",
		"--level", "objective",
		"--cycle-id", "123",
		"--weights", `[{"id":"1","weight":0.3},{"id":"1","weight":0.4}]`,
	})
	if err == nil {
		t.Fatal("expected error for duplicate id in --weights")
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
	if validationErr.Param != "--weights" {
		t.Fatalf("expected param --weights, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "duplicate id") {
		t.Fatalf("expected error to mention duplicate id, got: %v", err)
	}
}

// --- DryRun tests ---

func TestWeightDryRun_Objectives(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, weightTestConfig(t))
	err := runWeightShortcut(t, f, stdout, []string{
		"+weight",
		"--level", "objective",
		"--cycle-id", "123",
		"--weights", `[{"id":"1","weight":0.5},{"id":"2","weight":0.5}]`,
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
	if !strings.Contains(output, "/open-apis/okr/v2/cycles/123/objectives_weight") {
		t.Fatalf("dry-run output should contain weight update API path, got: %s", output)
	}
	if !strings.Contains(output, "PUT") {
		t.Fatalf("dry-run output should contain PUT method for update, got: %s", output)
	}
	if !strings.Contains(output, "objective_weights") {
		t.Fatalf("dry-run output should contain objective_weights in body, got: %s", output)
	}
}

func TestWeightDryRun_KeyResults(t *testing.T) {
	f, stdout, _, _ := cmdutil.TestFactory(t, weightTestConfig(t))
	err := runWeightShortcut(t, f, stdout, []string{
		"+weight",
		"--level", "key-result",
		"--cycle-id", "123",
		"--objective-id", "456",
		"--weights", `[{"id":"kr1","weight":0.5},{"id":"kr2","weight":0.5}]`,
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	output := stdout.String()
	if !strings.Contains(output, "/open-apis/okr/v2/objectives/456/key_results") {
		t.Fatalf("dry-run output should contain key_results list API path, got: %s", output)
	}
	if !strings.Contains(output, "/open-apis/okr/v2/objectives/456/key_results_weight") {
		t.Fatalf("dry-run output should contain key_results weight update API path, got: %s", output)
	}
	if !strings.Contains(output, "key_result_weights") {
		t.Fatalf("dry-run output should contain key_result_weights in body, got: %s", output)
	}
}

// --- Execute tests ---

func TestWeightExecute_Objectives_Success(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, weightTestConfig(t))
	// Mock fetch objectives
	w1 := 0.5
	w2 := 0.5
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles/123/objectives",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": "1", "weight": &w1, "cycle_id": "123", "owner": map[string]interface{}{"owner_type": "user"}},
					map[string]interface{}{"id": "2", "weight": &w2, "cycle_id": "123", "owner": map[string]interface{}{"owner_type": "user"}},
				},
				"has_more": false,
			},
		},
	})
	// Mock weight update
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/okr/v2/cycles/123/objectives_weight",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": "1", "weight": 0.7},
					map[string]interface{}{"id": "2", "weight": 0.3},
				},
			},
		},
	})
	err := runWeightShortcut(t, f, stdout, []string{
		"+weight",
		"--level", "objective",
		"--cycle-id", "123",
		"--weights", `[{"id":"1","weight":0.7}]`,
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
	weights, _ := data["weights"].([]interface{})
	if len(weights) != 2 {
		t.Fatalf("expected 2 items in weights list, got %d", len(weights))
	}

	// Verify sum is exactly 1.0
	var sum float64
	for _, w := range weights {
		wm, _ := w.(map[string]interface{})
		weightVal := getWeightFloat(wm["weight"])
		sum += weightVal
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Fatalf("expected sum of weights = 1.0, got %.10f", sum)
	}
}

func TestWeightExecute_KeyResults_Success(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, weightTestConfig(t))
	// Mock fetch key results
	w1 := 0.3
	w2 := 0.3
	w3 := 0.4
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/objectives/456/key_results",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": "kr1", "weight": &w1, "objective_id": "456", "owner": map[string]interface{}{"owner_type": "user"}},
					map[string]interface{}{"id": "kr2", "weight": &w2, "objective_id": "456", "owner": map[string]interface{}{"owner_type": "user"}},
					map[string]interface{}{"id": "kr3", "weight": &w3, "objective_id": "456", "owner": map[string]interface{}{"owner_type": "user"}},
				},
				"has_more": false,
			},
		},
	})
	// Mock weight update
	reg.Register(&httpmock.Stub{
		Method: "PUT",
		URL:    "/open-apis/okr/v2/objectives/456/key_results_weight",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{},
			},
		},
	})
	err := runWeightShortcut(t, f, stdout, []string{
		"+weight",
		"--level", "key-result",
		"--cycle-id", "123",
		"--objective-id", "456",
		"--weights", `[{"id":"kr1","weight":0.5}]`,
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
	weights, _ := data["weights"].([]interface{})

	// Verify sum is exactly 1.0
	var sum float64
	for _, w := range weights {
		wm, _ := w.(map[string]interface{})
		weightVal := getWeightFloat(wm["weight"])
		sum += weightVal
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Fatalf("expected sum of weights = 1.0, got %.10f", sum)
	}
}

func TestWeightExecute_IDNotFound(t *testing.T) {
	f, stdout, _, reg := cmdutil.TestFactory(t, weightTestConfig(t))
	// Mock fetch objectives
	w1 := 0.5
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/cycles/123/objectives",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"items": []interface{}{
					map[string]interface{}{"id": "1", "weight": &w1, "cycle_id": "123", "owner": map[string]interface{}{"owner_type": "user"}},
				},
				"has_more": false,
			},
		},
	})
	err := runWeightShortcut(t, f, stdout, []string{
		"+weight",
		"--level", "objective",
		"--cycle-id", "123",
		"--weights", `[{"id":"999","weight":0.5}]`,
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
	if validationErr.Param != "--weights" {
		t.Fatalf("expected param --weights, got %q", validationErr.Param)
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Fatalf("expected error to mention not found, got: %v", err)
	}
}

// --- Unit tests for helper functions ---

func TestParseWeightOps_Valid(t *testing.T) {
	ops, err := parseWeightOps(`[{"id":"1","weight":0.3},{"id":"2","weight":0.7}]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ops) != 2 {
		t.Fatalf("expected 2 ops, got %d", len(ops))
	}
	if ops[0].ID != "1" || math.Abs(ops[0].Weight-0.3) > 1e-9 {
		t.Fatalf("expected op[0] = {1,0.3}, got %+v", ops[0])
	}
	if ops[1].ID != "2" || math.Abs(ops[1].Weight-0.7) > 1e-9 {
		t.Fatalf("expected op[1] = {2,0.7}, got %+v", ops[1])
	}
}

func TestNormalizeWeights_AllSpecified(t *testing.T) {
	w1 := 0.0
	w2 := 0.0
	items := []Objective{
		{ID: "1", Weight: &w1},
		{ID: "2", Weight: &w2},
	}
	ops := []weightOp{
		{ID: "1", Weight: 0.3},
		{ID: "2", Weight: 0.7},
	}
	result, err := normalizeWeights(items, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	// Sum should be exactly 1.0
	var sum float64
	for _, r := range result {
		sum += getWeightFloat(r["weight"])
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Fatalf("expected sum = 1.0, got %.10f", sum)
	}
}

func TestNormalizeWeights_PartialSpecified_Proportional(t *testing.T) {
	w1 := 0.5
	w2 := 0.3
	w3 := 0.2
	items := []Objective{
		{ID: "1", Weight: &w1},
		{ID: "2", Weight: &w2},
		{ID: "3", Weight: &w3},
	}
	ops := []weightOp{
		{ID: "1", Weight: 0.4}, // Specify 0.4 for item 1
	}
	result, err := normalizeWeights(items, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
	// Sum should be exactly 1.0
	var sum float64
	for _, r := range result {
		sum += getWeightFloat(r["weight"])
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Fatalf("expected sum = 1.0, got %.10f", sum)
	}
	// Item 1 should have weight 0.4
	var item1Weight float64
	for _, r := range result {
		if r["id"] == "1" {
			item1Weight = getWeightFloat(r["weight"])
			break
		}
	}
	if math.Abs(item1Weight-0.4) > 1e-9 {
		t.Fatalf("expected item 1 weight = 0.4, got %.10f", item1Weight)
	}
}

func TestNormalizeWeights_ZeroOriginalWeights(t *testing.T) {
	w1 := 0.0
	w2 := 0.0
	items := []Objective{
		{ID: "1", Weight: &w1},
		{ID: "2", Weight: &w2},
	}
	ops := []weightOp{
		{ID: "1", Weight: 0.5},
	}
	result, err := normalizeWeights(items, ops)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	// Sum should be exactly 1.0
	var sum float64
	for _, r := range result {
		sum += getWeightFloat(r["weight"])
	}
	if math.Abs(sum-1.0) > 1e-9 {
		t.Fatalf("expected sum = 1.0, got %.10f", sum)
	}
	// When original weights are zero, remaining should be distributed evenly
	var item2Weight float64
	for _, r := range result {
		if r["id"] == "2" {
			item2Weight = getWeightFloat(r["weight"])
			break
		}
	}
	if math.Abs(item2Weight-0.5) > 1e-9 {
		t.Fatalf("expected item 2 weight = 0.5 (even distribution), got %.10f", item2Weight)
	}
}
