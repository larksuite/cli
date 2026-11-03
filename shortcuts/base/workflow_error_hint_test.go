// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// Exercise the actual shortcut/HTTP classification boundary, rather than only
// constructing the CLI's former model-specific error fields in a test.
func TestWorkflowConfigurationErrorRetainsServerDetail(t *testing.T) {
	commands := []struct {
		name     string
		shortcut common.Shortcut
		method   string
		path     string
		args     []string
	}{
		{"create", BaseWorkflowCreate, "POST", "/open-apis/base/v3/bases/app_x/workflows", []string{"+workflow-create", "--base-token", "app_x", "--json", `{"title":"draft","steps":[{"id":"agent","type":"AIAgentAction","data":{"model":{"name":"future-model","thinking_mode":"auto"}}}]}`}},
		{"update", BaseWorkflowUpdate, "PUT", "/open-apis/base/v3/bases/app_x/workflows/wkf_1", []string{"+workflow-update", "--base-token", "app_x", "--workflow-id", "wkf_1", "--json", `{"title":"draft","steps":[{"id":"agent","type":"AIAgentAction","data":{"model":{"name":"future-model","thinking_mode":"auto"}}}]}`}},
	}
	details := []string{
		`{"type":"validation_error","code":800004006,"message":"model deepseek_chat_model does not support auto; supported modes: deep, quick","hint":"请检查请求参数","retryable":false}`,
		`{"message":"unsupported model configuration","hint":"choose a supported mode","model_id":"model-1","requested_thinking_mode":"auto","supported_thinking_modes":[],"future_detail":{"stage":"spec_to_draft","value":null}}`,
		`{"message":"invalid configuration","supported_thinking_modes":null,"upstream_code":800004006}`,
	}
	for _, command := range commands {
		for _, status := range []int{http.StatusOK, http.StatusBadRequest} {
			for _, placement := range []string{"error", "data.error"} {
				for i, raw := range details {
					t.Run(fmt.Sprintf("%s/http-%d/%s/%d", command.name, status, placement, i), func(t *testing.T) {
						factory, stdout, registry := newExecuteFactory(t)
						var detail map[string]interface{}
						if err := json.Unmarshal([]byte(raw), &detail); err != nil {
							t.Fatal(err)
						}
						response := map[string]interface{}{"code": 800004006, "msg": "invalid parameter", "log_id": "workflow-config-log-id"}
						if placement == "error" {
							response["error"] = detail
						} else {
							response["data"] = map[string]interface{}{"error": detail}
						}
						stub := &httpmock.Stub{Method: command.method, URL: command.path, Status: status, Body: response}
						registry.Register(stub)
						err := runShortcut(t, command.shortcut, command.args, factory, stdout)
						registry.Verify(t)
						var submitted map[string]interface{}
						if decodeErr := json.Unmarshal(stub.CapturedBody, &submitted); decodeErr != nil {
							t.Fatal(decodeErr)
						}
						if !reflect.DeepEqual(submitted, mustWorkflowJSON(t, command.args[len(command.args)-1])) {
							t.Fatalf("request changed before server rejection: %s", stub.CapturedBody)
						}
						problem, ok := errs.ProblemOf(err)
						if !ok || problem.Code != 800004006 || problem.LogID != "workflow-config-log-id" || problem.Category != errs.CategoryAPI {
							t.Fatalf("error metadata changed: %T %v", err, err)
						}
						encoded, _ := json.Marshal(detail)
						if strings.Count(problem.Hint, "error="+string(encoded)) != 1 {
							t.Fatalf("server detail missing, changed or duplicated: hint=%s want=%s", problem.Hint, encoded)
						}
						if !strings.Contains(problem.Message, detail["message"].(string)) {
							t.Fatalf("message lost: %s", problem.Message)
						}
						var rendered bytes.Buffer
						if !output.WriteTypedErrorEnvelope(&rendered, err, "user") {
							t.Fatal("typed error not rendered")
						}
						var envelope struct {
							Error map[string]json.RawMessage `json:"error"`
						}
						if err := json.Unmarshal(rendered.Bytes(), &envelope); err != nil {
							t.Fatal(err)
						}
						for _, field := range []string{"model_id", "requested_thinking_mode", "supported_thinking_modes"} {
							if _, exists := envelope.Error[field]; exists {
								t.Errorf("Workflow-specific field %s leaked into common error envelope", field)
							}
						}
						var hint string
						if err := json.Unmarshal(envelope.Error["hint"], &hint); err != nil || hint != problem.Hint {
							t.Fatalf("wire hint changed: %s (%v)", hint, err)
						}
					})
				}
			}
		}
	}
}

func TestWorkflowErrorHintLeavesOtherCommandsUnchanged(t *testing.T) {
	for _, command := range []string{"base +record-create", "base +form-submit", "base +workflow-get", "base +workflow-enable", "other +workflow-create", "base +workflow-create", "lark-cli base +workflow-update", ""} {
		for _, placement := range []string{"error", "data.error"} {
			t.Run(command+"/"+placement, func(t *testing.T) {
				detail := map[string]interface{}{
					"message": "invalid input", "hint": "original hint", "value": "synthetic-private-value",
					"private_detail": map[string]interface{}{"submitted": "synthetic-private-detail"},
				}
				response := map[string]interface{}{"code": 800004006, "msg": "invalid value", "log_id": "original-log-id"}
				if placement == "error" {
					response["error"] = detail
				} else {
					response["data"] = map[string]interface{}{"error": detail}
				}
				// The legacy entry point uses the default policy even if its display
				// command resembles a Workflow configuration command.
				problem, ok := errs.ProblemOf(baseAPIErrorFromResult(response, errclass.ClassifyContext{LarkCmd: command}))
				if !ok || problem.Hint != "original hint" || problem.Code != 800004006 || problem.LogID != "original-log-id" {
					t.Fatalf("default error projection changed: %+v", problem)
				}
				for _, leaked := range []string{"error=", "synthetic-private-value", "synthetic-private-detail", "submitted"} {
					if strings.Contains(problem.Hint, leaked) {
						t.Errorf("default error hint leaked %q: %s", leaked, problem.Hint)
					}
				}
			})
		}
	}
}

func TestWorkflowErrorHintDoesNotInventMissingDetails(t *testing.T) {
	for _, raw := range []string{`{"code":800004006,"msg":"rejected"}`, `{"code":800004006,"msg":"rejected","error":null}`, `{"code":800004006,"msg":"rejected","data":{"error":null}}`} {
		t.Run(raw, func(t *testing.T) {
			var response map[string]interface{}
			if err := json.Unmarshal([]byte(raw), &response); err != nil {
				t.Fatal(err)
			}
			problem, _ := errs.ProblemOf(baseAPIErrorFromResultWithPolicy(response, errclass.ClassifyContext{}, baseAPIErrorPolicy{preserveOpaqueServerError: true}))
			if problem == nil || strings.Contains(problem.Hint, "error=") {
				t.Fatalf("invented an error detail with the policy enabled: %+v", problem)
			}
		})
	}
}

func TestWorkflowErrorPolicy(t *testing.T) {
	t.Run("envelope boundary carries the selected policy", func(t *testing.T) {
		for _, preserve := range []bool{false, true} {
			t.Run(fmt.Sprintf("preserve=%t", preserve), func(t *testing.T) {
				detail := map[string]interface{}{"hint": "server hint", "private_detail": "opaque configuration diagnostic"}
				encoded, _ := json.Marshal(detail)
				response := map[string]interface{}{"code": 800004006, "msg": "invalid parameter", "data": map[string]interface{}{"error": detail}}
				var err error
				if preserve {
					_, err = handleBaseAPIResultAnyWithPolicy(nil, response, nil, "save workflow", baseAPIErrorPolicy{preserveOpaqueServerError: true})
				} else {
					_, err = handleBaseAPIResultAny(nil, response, nil, "save configuration")
				}
				problem, ok := errs.ProblemOf(err)
				if !ok || problem.Category != errs.CategoryAPI || problem.Code != 800004006 || !strings.Contains(problem.Hint, "server hint") {
					t.Fatalf("envelope error metadata changed: %T %v", err, err)
				}
				wantCount := 0
				if preserve {
					wantCount = 1
				}
				if count := strings.Count(problem.Hint, "error="+string(encoded)); count != wantCount {
					t.Fatalf("envelope policy was lost: preserve=%t count=%d hint=%s", preserve, count, problem.Hint)
				}
			})
		}
	})

	t.Run("explicit envelope policy is independent of command display", func(t *testing.T) {
		for _, command := range []string{"", "renamed root renamed-base renamed-create"} {
			for _, placement := range []string{"error", "data.error"} {
				t.Run(command+"/"+placement, func(t *testing.T) {
					detail := map[string]interface{}{"message": "rejected configuration", "hint": "server hint", "future_detail": map[string]interface{}{"stage": "compile", "nullable": nil}}
					encoded, _ := json.Marshal(detail)
					response := map[string]interface{}{"code": 800004006, "msg": "invalid parameter", "log_id": "body-log-id"}
					if placement == "error" {
						response["error"] = detail
					} else {
						response["data"] = map[string]interface{}{"error": detail}
					}
					problem, ok := errs.ProblemOf(baseAPIErrorFromResultWithPolicy(response, errclass.ClassifyContext{LarkCmd: command}, baseAPIErrorPolicy{preserveOpaqueServerError: true}))
					if !ok || problem.Code != 800004006 || problem.Category != errs.CategoryAPI || problem.LogID != "body-log-id" {
						t.Fatalf("envelope metadata changed: %T %+v", problem, problem)
					}
					if strings.Count(problem.Hint, "error="+string(encoded)) != 1 || !strings.Contains(problem.Hint, "server hint") {
						t.Fatalf("explicit policy lost or duplicated opaque detail: %s", problem.Hint)
					}
				})
			}
		}
	})

	t.Run("typed errors retain identity cause and classification", func(t *testing.T) {
		for _, bodyLogID := range []string{"", "body-log-id"} {
			for _, placement := range []string{"error", "data.error"} {
				t.Run(bodyLogID+"/"+placement, func(t *testing.T) {
					cause := errors.New("synthetic original cause")
					original := errs.NewAPIError(errs.SubtypeUnknown, "original classified error").WithCode(800004006).WithLogID("header-log-id").WithRetryable().WithCause(cause)
					before := *original.ProblemDetail()
					detail := map[string]interface{}{"message": "rejected configuration", "hint": "server hint", "future_detail": map[string]interface{}{"nullable": nil}}
					encoded, _ := json.Marshal(detail)
					response := map[string]interface{}{"code": 800004006, "msg": "invalid parameter"}
					if bodyLogID != "" {
						response["log_id"] = bodyLogID
					}
					if placement == "error" {
						response["error"] = detail
					} else {
						response["data"] = map[string]interface{}{"error": detail}
					}
					body, _ := json.Marshal(response)
					got := enrichBaseAPIErrorFromBodyWithPolicy(original, body, errclass.ClassifyContext{LarkCmd: "renamed caller"}, baseAPIErrorPolicy{preserveOpaqueServerError: true})
					problem, ok := errs.ProblemOf(got)
					if got != original || !errors.Is(got, cause) || !ok {
						t.Fatalf("typed error identity or cause changed: %T %v", got, got)
					}
					if problem.Category != before.Category || problem.Subtype != before.Subtype || problem.Code != before.Code || problem.Retryable != before.Retryable {
						t.Fatalf("typed metadata changed: before=%+v after=%+v", before, problem)
					}
					wantLogID := bodyLogID
					if wantLogID == "" {
						wantLogID = "header-log-id"
					}
					if problem.LogID != wantLogID || strings.Count(problem.Hint, "error="+string(encoded)) != 1 {
						t.Fatalf("typed enrichment lost LogID or detail: %+v", problem)
					}
				})
			}
		}
	})

	t.Run("default typed enrichment keeps private values private", func(t *testing.T) {
		cause := errors.New("synthetic transport cause")
		original := errs.NewAPIError(errs.SubtypeUnknown, "original").WithCode(800004006).WithLogID("header-log-id").WithCause(cause)
		body := []byte(`{"code":800004006,"msg":"rejected","data":{"error":{"hint":"server hint","value":{"query_view_id":"vew_recover","raw_input":"synthetic-private-value"},"private_detail":"synthetic-private-detail"}}}`)
		got := enrichBaseAPIErrorFromBody(original, body, errclass.ClassifyContext{LarkCmd: "base +workflow-create"})
		problem, ok := errs.ProblemOf(got)
		if got != original || !errors.Is(got, cause) || !ok || problem.LogID != "header-log-id" {
			t.Fatalf("default typed metadata changed: %T %v", got, got)
		}
		if !strings.Contains(problem.Hint, "server hint") || !strings.Contains(problem.Hint, "vew_recover") {
			t.Fatalf("default typed projection lost hint or recovery id: %s", problem.Hint)
		}
		for _, leaked := range []string{"error=", "raw_input", "synthetic-private-value", "synthetic-private-detail"} {
			if strings.Contains(problem.Hint, leaked) {
				t.Errorf("default typed hint leaked %q: %s", leaked, problem.Hint)
			}
		}
	})
}
