// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/shortcuts/common"
)

func TestErrorDetailHelpers(t *testing.T) {
	detail := map[string]interface{}{"message": "boom", "hint": "retry later"}
	if got := extractErrorHint(map[string]interface{}{"data": map[string]interface{}{"error": detail}}); got != "retry later" {
		t.Fatalf("hint=%q", got)
	}
	if got := extractDataErrorMessage(map[string]interface{}{"data": map[string]interface{}{"error": detail}}); got != "boom" {
		t.Fatalf("message=%q", got)
	}
	if got := extractDataErrorMessage(map[string]interface{}{"data": map[string]interface{}{}}); got != "" {
		t.Fatalf("message=%q", got)
	}
}

func TestHandleBaseAPIResultErrorPaths(t *testing.T) {
	if _, err := handleBaseAPIResultAny(nil, nil, assertErr{}, "list fields"); err == nil || !strings.Contains(err.Error(), "list fields") {
		t.Fatalf("err=%v", err)
	}
	result := map[string]interface{}{
		"code": 190001,
		"msg":  "bad request",
		"data": map[string]interface{}{
			"error": map[string]interface{}{"message": "invalid filter", "hint": "check field name"},
		},
	}
	if _, err := handleBaseAPIResultAny(nil, result, nil, "set filter"); err == nil || !strings.Contains(err.Error(), "invalid filter") {
		t.Fatalf("err=%v", err)
	} else {
		p, ok := errs.ProblemOf(err)
		if !ok || p.Code != 190001 {
			t.Fatalf("expected typed code 190001, got %T %v", err, err)
		}
		if p.Hint != "check field name" {
			t.Fatalf("hint=%q", p.Hint)
		}
	}
	if _, err := handleBaseAPIResult(nil, result, nil, "set filter"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestHandleBaseAPIResultPromotesBaseErrorFields(t *testing.T) {
	result := map[string]interface{}{
		"code": 800010407,
		"msg":  "cell value invalid",
		"data": map[string]interface{}{
			"error": map[string]interface{}{
				"docs_url":       nil,
				"hint":           "Provide a number value.",
				"level":          "error",
				"logid":          "20260508160000000000000000000000",
				"message":        "The cell value does not match the expected input shape.",
				"path":           "Amount",
				"retry_after_ms": nil,
				"retryable":      false,
				"extra_context":  "future detail field",
				"table":          map[string]interface{}{"id": "tbl_1", "name": "Orders"},
				"type":           "invalid_request",
				"upstream_code":  nil,
				"value":          "abc",
			},
		},
	}

	_, err := handleBaseAPIResultAny(nil, result, nil, "API call failed")
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T %v", err, err)
	}
	if p.Code != 800010407 {
		t.Fatalf("code=%d", p.Code)
	}
	if p.Message != "The cell value does not match the expected input shape." {
		t.Fatalf("message=%q", p.Message)
	}
	if p.Hint != "Provide a number value." {
		t.Fatalf("hint=%q", p.Hint)
	}
	if p.LogID != "20260508160000000000000000000000" {
		t.Fatalf("logID=%q", p.LogID)
	}
}

func TestHandleBaseAPIResultClassifiesKnownPermissionCode(t *testing.T) {
	result := map[string]interface{}{
		"code": 99991676,
		"msg":  "permission denied",
		"data": map[string]interface{}{
			"error": map[string]interface{}{
				"hint":    "Grant base:record:read to the app.",
				"message": "Missing required scope base:record:read.",
			},
		},
	}

	_, err := handleBaseAPIResultAny(nil, result, nil, "API call failed")
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T %v", err, err)
	}
	if p.Code != 99991676 {
		t.Fatalf("code=%d", p.Code)
	}
	if p.Category != errs.CategoryAuthorization || p.Subtype != errs.SubtypeTokenScopeInsufficient {
		t.Fatalf("category/subtype=%s/%s", p.Category, p.Subtype)
	}
}

func TestAttachBaseResponseLogIDFromHeader(t *testing.T) {
	result := map[string]interface{}{
		"code": 91402,
		"msg":  "NOTEXIST",
		"data": map[string]interface{}{},
	}
	attachBaseErrorLogID(result, "20260508170000000000000000000000")

	_, err := handleBaseAPIResultAny(nil, result, nil, "API call failed")
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T %v", err, err)
	}
	if p.LogID != "20260508170000000000000000000000" {
		t.Fatalf("logID=%q", p.LogID)
	}
}

func TestHandleBaseAPIResultRejectsNonNumericCode(t *testing.T) {
	for _, code := range []interface{}{"oops", map[string]interface{}{}, nil} {
		result := map[string]interface{}{"code": code, "msg": "weird envelope"}
		_, err := handleBaseAPIResultAny(nil, result, nil, "list tables")
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("code=%#v: expected typed error, got %T %v", code, err, err)
		}
		if p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeInvalidResponse {
			t.Fatalf("code=%#v: category/subtype=%s/%s", code, p.Category, p.Subtype)
		}
		if !strings.Contains(p.Message, "list tables") {
			t.Fatalf("code=%#v: message=%q", code, p.Message)
		}
	}
}

func TestEnrichBaseAPIErrorFromBodyLogIDMerge(t *testing.T) {
	t.Run("body without log_id keeps header-derived LogID", func(t *testing.T) {
		outer := errs.NewAPIError(errs.SubtypeUnknown, "outer failure").WithCode(190001).WithLogID("header-log-id")
		err := enrichBaseAPIErrorFromBody(outer, []byte(`{"code":190001,"msg":"boom"}`), errclass.ClassifyContext{})
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("expected typed error, got %T %v", err, err)
		}
		if p.Message != "boom" {
			t.Fatalf("message=%q", p.Message)
		}
		if p.LogID != "header-log-id" {
			t.Fatalf("logID=%q, want header-log-id", p.LogID)
		}
	})

	t.Run("body log_id overrides header-derived LogID", func(t *testing.T) {
		outer := errs.NewAPIError(errs.SubtypeUnknown, "outer failure").WithCode(190001).WithLogID("header-log-id")
		body := `{"code":190001,"msg":"boom","data":{"error":{"logid":"body-log-id"}}}`
		err := enrichBaseAPIErrorFromBody(outer, []byte(body), errclass.ClassifyContext{})
		p, ok := errs.ProblemOf(err)
		if !ok {
			t.Fatalf("expected typed error, got %T %v", err, err)
		}
		if p.LogID != "body-log-id" {
			t.Fatalf("logID=%q, want body-log-id", p.LogID)
		}
	})
}

func TestBaseMissingFileIOErrorIsInternal(t *testing.T) {
	p, ok := errs.ProblemOf(baseMissingFileIOError("file operations require a FileIO provider"))
	if !ok {
		t.Fatal("expected typed error")
	}
	if p.Category != errs.CategoryInternal || p.Subtype != errs.SubtypeFileIO {
		t.Fatalf("category/subtype=%s/%s", p.Category, p.Subtype)
	}
}

type assertErr struct{}

func (assertErr) Error() string { return "network timeout" }

func assertProblemCode(t *testing.T, err error, code int, messageParts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with code %d", code)
	}
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed problem, got %T %v", err, err)
	}
	if p.Code != code {
		t.Fatalf("code=%d, want %d; err=%v", p.Code, code, err)
	}
	for _, part := range messageParts {
		if !strings.Contains(p.Message, part) {
			t.Fatalf("message=%q missing %q", p.Message, part)
		}
	}
}

// Every Base v3 shortcut is available to an Agent Employee, so an authorization
// failure there has to route recovery to the Agent platform: an Agent Employee
// has nobody to complete a device login, and `auth login` refuses its profile
// outright. The human recovery must stay untouched for everyone else.
func TestHandleBaseAPIResultRoutesRecoveryByCredentialKind(t *testing.T) {
	envelope := func() map[string]interface{} {
		return map[string]interface{}{"code": 99991679, "msg": "user access token invalid"}
	}
	runtimeFor := func(kind core.CredentialKind) *common.RuntimeContext {
		return &common.RuntimeContext{Config: &core.CliConfig{
			AppID:          "cli_base_agent",
			Brand:          core.BrandFeishu,
			CredentialKind: kind,
		}}
	}

	_, err := handleBaseAPIResultAny(runtimeFor(core.CredentialAAT), envelope(), nil, "API call failed")
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T %v", err, err)
	}
	if strings.Contains(p.Hint, "auth login") {
		t.Fatalf("Agent Employee was sent to the human device login: %q", p.Hint)
	}
	if !strings.Contains(p.Hint, "Agent Employee") {
		t.Fatalf("hint does not name the Agent platform: %q", p.Hint)
	}

	_, err = handleBaseAPIResultAny(runtimeFor(""), envelope(), nil, "API call failed")
	p, ok = errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed error, got %T %v", err, err)
	}
	if !strings.Contains(p.Hint, "auth login") {
		t.Fatalf("a human profile must keep the device-login recovery: %q", p.Hint)
	}
}

// TestDataErrorValuePromotedToHintWhenStructured 覆盖恢复标识的透出路径：
// +form-query-page-create 在 associate/readback 步失败时把 query_view_id 放在
// data.error.value 里，skill 的恢复分支依赖它。CLI 的错误信封没有自由结构位，
// 不并进 hint 调用方就只能看到「没有 ID」而误判为可以重建，把上一次建出的资源变成孤儿。
func TestDataErrorValuePromotedToHintWhenStructured(t *testing.T) {
	result := map[string]interface{}{
		"code": 1254999,
		"msg":  "associate query view failed",
		"data": map[string]interface{}{
			"error": map[string]interface{}{
				"type":        "internal_error",
				"hint":        "Retry with the same query view id.",
				"failed_step": "associate",
				"value":       map[string]interface{}{"query_view_id": "vewABC123"},
			},
		},
	}
	err := baseAPIErrorFromResult(result, errclass.ClassifyContext{})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a problem error, got %T", err)
	}
	if !strings.Contains(p.Hint, `"query_view_id":"vewABC123"`) {
		t.Fatalf("hint should carry the recovery id, got %q", p.Hint)
	}
	if !strings.Contains(p.Hint, "Retry with the same query view id.") {
		t.Fatalf("hint should keep the server hint, got %q", p.Hint)
	}
}

// TestDataErrorValueOnlyPromotesRecoveryKeys 这条路径是 base 包所有 API 错误的公共
// 出口，+form-submit / +record-batch-* 的失败也走它。服务端在 value 里回带用户填写
// 的原值时，整块透出会把这些内容复制进 hint，而 hint 会进终端输出和日志。
func TestDataErrorValueOnlyPromotesRecoveryKeys(t *testing.T) {
	result := map[string]interface{}{
		"code": 1254999,
		"msg":  "submit failed",
		"data": map[string]interface{}{
			"error": map[string]interface{}{
				"type": "internal_error",
				"value": map[string]interface{}{
					"query_view_id": "vewABC123",
					"failed_step":   "associate",
					"submitted":     map[string]interface{}{"联系电话": "13800138000"},
					"raw_input":     "13800138000",
				},
			},
		},
	}
	err := baseAPIErrorFromResult(result, errclass.ClassifyContext{})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a problem error, got %T", err)
	}
	if !strings.Contains(p.Hint, `"query_view_id":"vewABC123"`) {
		t.Fatalf("hint dropped the recovery id: %q", p.Hint)
	}
	if !strings.Contains(p.Hint, `"failed_step":"associate"`) {
		t.Fatalf("hint dropped failed_step: %q", p.Hint)
	}
	for _, leaked := range []string{"13800138000", "submitted", "raw_input"} {
		if strings.Contains(p.Hint, leaked) {
			t.Fatalf("hint leaked non-recovery content %q: %q", leaked, p.Hint)
		}
	}
}

// TestDataErrorValueScalarNotPromoted 标量 value 是出错字段的原值，message 里已说清，
// 再塞进 hint 只是噪音，因此不透出。
func TestDataErrorValueScalarNotPromoted(t *testing.T) {
	result := map[string]interface{}{
		"code": 800010407,
		"msg":  "cell value invalid",
		"data": map[string]interface{}{
			"error": map[string]interface{}{
				"type":  "invalid_request",
				"hint":  "Provide a number value.",
				"value": "abc",
			},
		},
	}
	err := baseAPIErrorFromResult(result, errclass.ClassifyContext{})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a problem error, got %T", err)
	}
	if strings.Contains(p.Hint, "error.value") {
		t.Fatalf("scalar value must not be promoted, got %q", p.Hint)
	}
}

// 权限类错误的 PermissionHint 是 classify.go 明确声明的权威恢复指引（要授哪个
// scope、开发者控制台链接）。恢复标识必须只追加：一旦拿它当「是否覆盖 hint」的
// 依据，鉴权失败时整段授权步骤会被一串裸 JSON 挤掉，用户看不到该怎么授权。
func TestDataErrorValueDoesNotClobberPermissionHint(t *testing.T) {
	result := map[string]interface{}{
		"code": 99991679,
		"msg":  "user access token invalid",
		"data": map[string]interface{}{
			"error": map[string]interface{}{
				"value": map[string]interface{}{"query_view_id": "vewABC123"},
			},
		},
	}
	err := baseAPIErrorFromResult(result, errclass.ClassifyContext{})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a problem error, got %T", err)
	}
	if p.Hint == "" {
		t.Fatal("permission recovery hint was dropped entirely")
	}
	if strings.HasPrefix(p.Hint, "error.value=") {
		t.Fatalf("recovery id replaced the permission hint instead of being appended: %q", p.Hint)
	}
	if !strings.Contains(p.Hint, "error.value=") {
		t.Fatalf("recovery id was not appended at all: %q", p.Hint)
	}
}

// 文档教调用方把 hint 里 error.value= 后面那段按 JSON 解析，所以无论服务端回带
// 多长的内容，输出都必须是能解析的 JSON——截断出半个 JSON 等于把恢复路径堵死。
func TestDataErrorValueStaysParseableWhenOversized(t *testing.T) {
	steps := make([]interface{}, 0, 60)
	for i := 0; i < 60; i++ {
		steps = append(steps, fmt.Sprintf("step-%02d-with-a-deliberately-long-name", i))
	}
	result := map[string]interface{}{
		"code": 1254999,
		"msg":  "associate failed",
		"data": map[string]interface{}{
			"error": map[string]interface{}{
				"value": map[string]interface{}{
					"query_view_id":   "vewABC123",
					"failed_step":     "associate",
					"completed_steps": steps,
				},
			},
		},
	}
	err := baseAPIErrorFromResult(result, errclass.ClassifyContext{})
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a problem error, got %T", err)
	}
	_, payload, found := strings.Cut(p.Hint, "error.value=")
	if !found {
		t.Fatalf("recovery id was dropped entirely for an oversized value: %q", p.Hint)
	}
	if len(payload) > 512 {
		t.Fatalf("recovery hint exceeded the public size limit: %d bytes", len(payload))
	}
	var decoded map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &decoded); err != nil {
		t.Fatalf("hint payload is not valid JSON (%v): %q", err, payload)
	}
	if decoded["query_view_id"] != "vewABC123" {
		t.Fatalf("oversized fallback lost the recovery id: %v", decoded)
	}
}

// 恢复字段的两种落点都要取到，且 value 优先。没有这条，把 detail 兜底折叠掉
// 或把优先级调反，CI 都不会红；而 detail 形状下取不到 failed_step，调用方就没有
// 「必须带 ID 重试」还是「可以重建」的分支依据。
func TestDataErrorValueReadsBothShapesWithValueWinning(t *testing.T) {
	t.Run("only alongside value", func(t *testing.T) {
		result := map[string]interface{}{
			"code": 1254999, "msg": "associate failed",
			"data": map[string]interface{}{
				"error": map[string]interface{}{
					"query_view_id": "vewFromDetail",
					"failed_step":   "associate",
				},
			},
		}
		err := baseAPIErrorFromResult(result, errclass.ClassifyContext{})
		p, _ := errs.ProblemOf(err)
		if !strings.Contains(p.Hint, `"query_view_id":"vewFromDetail"`) {
			t.Fatalf("recovery keys sitting next to value were not read: %q", p.Hint)
		}
		if !strings.Contains(p.Hint, `"failed_step":"associate"`) {
			t.Fatalf("failed_step sitting next to value was not read: %q", p.Hint)
		}
	})

	t.Run("value wins over detail", func(t *testing.T) {
		result := map[string]interface{}{
			"code": 1254999, "msg": "associate failed",
			"data": map[string]interface{}{
				"error": map[string]interface{}{
					"query_view_id": "vewFromDetail",
					"value":         map[string]interface{}{"query_view_id": "vewFromValue"},
				},
			},
		}
		err := baseAPIErrorFromResult(result, errclass.ClassifyContext{})
		p, _ := errs.ProblemOf(err)
		if !strings.Contains(p.Hint, "vewFromValue") {
			t.Fatalf("value should win over the sibling key: %q", p.Hint)
		}
		if strings.Contains(p.Hint, "vewFromDetail") {
			t.Fatalf("sibling key leaked while value was present: %q", p.Hint)
		}
	})
}
