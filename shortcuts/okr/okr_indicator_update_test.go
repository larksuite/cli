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

func indicatorUpdateTestConfig(t *testing.T) *core.CliConfig {
	t.Helper()
	return &core.CliConfig{
		AppID:     "test-okr-indicator-update",
		AppSecret: "secret-okr-indicator-update",
		Brand:     core.BrandFeishu,
	}
}

func runIndicatorUpdateShortcut(t *testing.T, f *cmdutil.Factory, stdout *bytes.Buffer, args []string) error {
	t.Helper()
	parent := &cobra.Command{Use: "okr"}
	OKRIndicatorUpdate.Mount(parent, f)
	parent.SetArgs(args)
	parent.SilenceErrors = true
	parent.SilenceUsage = true
	if stdout != nil {
		stdout.Reset()
	}
	return parent.Execute()
}

// --- Validate tests ---

func TestIndicatorUpdateValidate_MissingLevel(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, indicatorUpdateTestConfig(t))
	err := runIndicatorUpdateShortcut(t, f, stdout, []string{
		"+indicator-update",
		"--id", "123",
		"--value", "50",
	})
	// cobra Required:true reports flag name without "--" prefix
	if err == nil || !strings.Contains(err.Error(), "level") {
		t.Fatalf("expected --level required error, got: %v", err)
	}
}

func TestIndicatorUpdateValidate_InvalidLevel(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, indicatorUpdateTestConfig(t))
	err := runIndicatorUpdateShortcut(t, f, stdout, []string{
		"+indicator-update",
		"--level", "invalid",
		"--id", "123",
		"--value", "50",
	})
	if err == nil {
		t.Fatal("expected error for invalid level")
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

func TestIndicatorUpdateValidate_MissingID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, indicatorUpdateTestConfig(t))
	err := runIndicatorUpdateShortcut(t, f, stdout, []string{
		"+indicator-update",
		"--level", "objective",
		"--value", "50",
	})
	if err == nil || !strings.Contains(err.Error(), "id") {
		t.Fatalf("expected --id required error, got: %v", err)
	}
}

func TestIndicatorUpdateValidate_InvalidID(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, indicatorUpdateTestConfig(t))
	err := runIndicatorUpdateShortcut(t, f, stdout, []string{
		"+indicator-update",
		"--level", "objective",
		"--id", "not-a-number",
		"--value", "50",
	})
	if err == nil {
		t.Fatal("expected error for invalid id")
	}
	_, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	validationErr, ok := err.(*errs.ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got: %T", err)
	}
	if validationErr.Param != "--id" {
		t.Fatalf("expected param --id, got %q", validationErr.Param)
	}
}

func TestIndicatorUpdateValidate_MissingValue(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, indicatorUpdateTestConfig(t))
	err := runIndicatorUpdateShortcut(t, f, stdout, []string{
		"+indicator-update",
		"--level", "objective",
		"--id", "123",
	})
	if err == nil || !strings.Contains(err.Error(), "value") {
		t.Fatalf("expected --value required error, got: %v", err)
	}
}

func TestIndicatorUpdateValidate_InvalidValue(t *testing.T) {
	t.Parallel()
	f, stdout, _, _ := cmdutil.TestFactory(t, indicatorUpdateTestConfig(t))
	err := runIndicatorUpdateShortcut(t, f, stdout, []string{
		"+indicator-update",
		"--level", "objective",
		"--id", "123",
		"--value", "not-a-number",
	})
	if err == nil {
		t.Fatal("expected error for invalid value")
	}
	_, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	validationErr, ok := err.(*errs.ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got: %T", err)
	}
	if validationErr.Param != "--value" {
		t.Fatalf("expected param --value, got %q", validationErr.Param)
	}
}

func TestIndicatorUpdateValidate_Valid(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, indicatorUpdateTestConfig(t))
	// Mock fetch indicators
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/objectives/123/indicators",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"indicator": map[string]interface{}{
					"id": "ind-456",
				},
			},
		},
	})
	// Mock patch indicator
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/okr/v2/indicators/ind-456",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	})
	err := runIndicatorUpdateShortcut(t, f, stdout, []string{
		"+indicator-update",
		"--level", "objective",
		"--id", "123",
		"--value", "75.5",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- Execute tests ---

func TestIndicatorUpdateExecute_Objectives_Success(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, indicatorUpdateTestConfig(t))
	// Mock fetch indicators
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/objectives/123/indicators",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"indicator": map[string]interface{}{
					"id": "ind-456",
				},
			},
		},
	})
	// Mock patch indicator
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/okr/v2/indicators/ind-456",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
		BodyFilter: func(body []byte) bool {
			var data map[string]interface{}
			if err := json.Unmarshal(body, &data); err != nil {
				return false
			}
			val, ok := data["current_value"].(float64)
			return ok && val == 75.5
		},
	})
	err := runIndicatorUpdateShortcut(t, f, stdout, []string{
		"+indicator-update",
		"--level", "objective",
		"--id", "123",
		"--value", "75.5",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIndicatorUpdateExecute_KeyResults_Success(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, indicatorUpdateTestConfig(t))
	// Mock fetch indicators
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/key_results/456/indicators",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"indicator": map[string]interface{}{
					"id": "ind-789",
				},
			},
		},
	})
	// Mock patch indicator
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/okr/v2/indicators/ind-789",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
		},
	})
	err := runIndicatorUpdateShortcut(t, f, stdout, []string{
		"+indicator-update",
		"--level", "key-result",
		"--id", "456",
		"--value", "100",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIndicatorUpdateExecute_FetchAPIError(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, indicatorUpdateTestConfig(t))
	// Mock fetch indicators - API error
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/objectives/123/indicators",
		Body: map[string]interface{}{
			"code": 9999,
			"msg":  "fetch error",
		},
	})
	err := runIndicatorUpdateShortcut(t, f, stdout, []string{
		"+indicator-update",
		"--level", "objective",
		"--id", "123",
		"--value", "50",
	})
	if err == nil {
		t.Fatal("expected error for fetch API failure")
	}
	prob, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if prob.Category != errs.CategoryAPI {
		t.Fatalf("expected CategoryAPI, got %q", prob.Category)
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected error to be *errs.APIError, got: %T", err)
	}
	if !errors.Is(err, apiErr) {
		t.Fatal("errors.Is should find the APIError in the chain")
	}
}

func TestIndicatorUpdateExecute_PatchAPIError(t *testing.T) {
	t.Parallel()
	f, stdout, _, reg := cmdutil.TestFactory(t, indicatorUpdateTestConfig(t))
	// Mock fetch indicators
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/okr/v2/objectives/123/indicators",
		Body: map[string]interface{}{
			"code": 0,
			"msg":  "ok",
			"data": map[string]interface{}{
				"indicator": map[string]interface{}{
					"id": "ind-456",
				},
			},
		},
	})
	// Mock patch indicator - API error
	reg.Register(&httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/okr/v2/indicators/ind-456",
		Body: map[string]interface{}{
			"code": 9999,
			"msg":  "patch error",
		},
	})
	err := runIndicatorUpdateShortcut(t, f, stdout, []string{
		"+indicator-update",
		"--level", "objective",
		"--id", "123",
		"--value", "50",
	})
	if err == nil {
		t.Fatal("expected error for patch API failure")
	}
	prob, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got: %v", err)
	}
	if prob.Category != errs.CategoryAPI {
		t.Fatalf("expected CategoryAPI, got %q", prob.Category)
	}
	var apiErr *errs.APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected error to be *errs.APIError, got: %T", err)
	}
	if !errors.Is(err, apiErr) {
		t.Fatal("errors.Is should find the APIError in the chain")
	}
}

// --- parseIndicatorValue tests ---

func TestParseIndicatorValue_Valid(t *testing.T) {
	t.Parallel()
	tests := []string{"0", "100", "75.5", "-10", "0.001", "99999999999"}
	for _, v := range tests {
		result, err := parseIndicatorValue(v)
		if err != nil {
			t.Fatalf("expected no error for %q, got: %v", v, err)
		}
		_ = result
	}
}

func TestParseIndicatorValue_Invalid(t *testing.T) {
	t.Parallel()
	tests := []string{"", "abc", "1e100000", "100000000000"}
	for _, v := range tests {
		_, err := parseIndicatorValue(v)
		if err == nil {
			t.Fatalf("expected error for %q", v)
		}
	}
}
