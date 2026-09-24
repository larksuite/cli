// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/errclass"
	"github.com/larksuite/cli/internal/util"
	"github.com/larksuite/cli/shortcuts/common"
)

func handleBaseAPIResult(runtime *common.RuntimeContext, result interface{}, err error, action string) (map[string]interface{}, error) {
	data, err := handleBaseAPIResultAny(runtime, result, err, action)
	if err != nil {
		return nil, err
	}
	dataMap, _ := data.(map[string]interface{})
	return dataMap, nil
}

// handleBaseAPIResultAny normalizes the Base v3 {code,msg,data} envelope used
// by shortcut APIs. Success returns data as-is; API failures become the CLI's
// structured ErrAPI, with server-provided message/hint promoted to the top level.
//
// The runtime is what the failure is classified against, and it is a parameter
// rather than a per-call-site decision because the credential kind it carries
// is what routes the recovery: every Base v3 shortcut is available to an Agent
// Employee, and an Agent Employee has nobody to complete the human device login
// an unclassified authorization failure would recommend.
func handleBaseAPIResultAny(runtime *common.RuntimeContext, result interface{}, err error, action string) (interface{}, error) {
	if err != nil {
		return nil, baseAPIBoundaryError(err, action)
	}

	resultMap, ok := result.(map[string]interface{})
	if !ok || resultMap == nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "%s: API returned a malformed response envelope", action)
	}
	if _, exists := resultMap["code"]; !exists {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "%s: API response is missing code", action)
	}
	code, numeric := util.ToFloat64(resultMap["code"])
	if !numeric {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse, "%s: API response code is not numeric", action)
	}
	if code == 0 {
		return resultMap["data"], nil
	}

	return nil, baseAPIErrorFromResult(resultMap, baseClassifyContext(runtime))
}

// baseClassifyContext describes the failing call to the error classifier. A
// runtime without config only reaches here from tests that exercise envelope
// normalization on its own; a real shortcut always carries one.
func baseClassifyContext(runtime *common.RuntimeContext) errclass.ClassifyContext {
	if runtime == nil || runtime.Config == nil {
		return errclass.ClassifyContext{}
	}
	return runtime.APIClassifyContext()
}

// baseFlagErrorf marks flag-usage failures; it shares baseValidationErrorf's
// typed envelope and exists so call sites read as flag rejections.
func baseFlagErrorf(format string, args ...any) error {
	return baseValidationErrorf(format, args...)
}

func baseValidationErrorf(format string, args ...any) error {
	msg := fmt.Sprintf(format, args...)
	err := errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", msg)
	if params := flagParams(msg); len(params) > 0 {
		err = err.WithParam(params[0].Name).WithParams(params...)
	}
	if cause := firstErrorArg(args); cause != nil {
		err = err.WithCause(cause)
	}
	return err
}

func flagParams(msg string) []errs.InvalidParam {
	reason := msg
	seen := map[string]bool{}
	params := []errs.InvalidParam{}
	for start := strings.Index(msg, "--"); start >= 0; start = strings.Index(msg, "--") {
		end := start + 2
		for end < len(msg) {
			ch := msg[end]
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '-' {
				end++
				continue
			}
			break
		}
		if end > start+2 {
			name := msg[start:end]
			if !seen[name] {
				seen[name] = true
				params = append(params, errs.InvalidParam{Name: name, Reason: reason})
			}
		}
		msg = msg[end:]
	}
	return params
}

func firstErrorArg(args []any) error {
	for _, arg := range args {
		if err, ok := arg.(error); ok {
			return err
		}
	}
	return nil
}

// baseMissingFileIOError reports a broken runtime wiring: a command that needs
// local file access was constructed without a FileIO provider. The user cannot
// fix this by changing flags, so it classifies as internal, not validation.
func baseMissingFileIOError(format string, args ...any) error {
	return errs.NewInternalError(errs.SubtypeFileIO, format, args...)
}

func baseInputStatError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, fileio.ErrPathValidation) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe file path: %s", err).WithCause(err)
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument, "cannot read file: %s", err).WithCause(err)
}

func baseSaveError(err error) error {
	if err == nil {
		return nil
	}
	var me *fileio.MkdirError
	switch {
	case errors.Is(err, fileio.ErrPathValidation):
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).WithCause(err)
	case errors.As(err, &me):
		return errs.NewInternalError(errs.SubtypeFileIO, "cannot create parent directory: %s", err).WithCause(err)
	default:
		return errs.NewInternalError(errs.SubtypeFileIO, "cannot create file: %s", err).WithCause(err)
	}
}

func baseAPIBoundaryError(err error, action string) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return errs.NewNetworkError(errs.SubtypeNetworkTransport, "%s: %s", action, err).WithCause(err)
}

func baseUploadAttachmentError(filePath string, err error) error {
	if p, ok := errs.ProblemOf(err); ok {
		p.Message = fmt.Sprintf("failed to upload attachment %s: %s", filePath, p.Message)
		return err
	}
	return errs.NewInternalError(errs.SubtypeSDKError, "failed to upload attachment %s: %s", filePath, err).WithCause(err)
}

func baseAPIErrorFromResult(resultMap map[string]interface{}, cc errclass.ClassifyContext) error {
	if resultMap == nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "API returned a malformed response envelope")
	}
	if msg := extractDataErrorMessage(resultMap); msg != "" {
		resultMap["msg"] = msg
	}
	hint := extractErrorHint(resultMap)
	if logID := extractBaseErrorLogID(resultMap); logID != "" {
		resultMap["log_id"] = logID
	}
	err := errclass.BuildAPIError(resultMap, cc)
	if err == nil {
		return nil
	}
	if p, ok := errs.ProblemOf(err); ok {
		// 最终 hint 由两步决定：服务端 hint 覆盖分类器给出的 hint，再追加恢复标识。
		// 在局部量上拼完再写回一次，两步的先后关系集中在这里一眼可见。
		merged := p.Hint
		// 注意服务端 hint 覆盖分类器结果，与 errclass 的 buildPermissionErrorFromFacts
		// 声明的优先级相反——那里写明 PermissionHint 要压过服务端细节，而这里 hint
		// 非空时会把它整段换掉。属已知待收口，此处不改变这一行为。
		if hint != "" {
			merged = hint
		}
		// CLI 的错误信封只有固定字段，data 整块在错误分支被丢弃；但部分端点（如
		// +form-query-page-create）把恢复所需的 ID 放在 data.error.value 里。不透出来，
		// 调用方只能看到「没有 ID」而误判为可以重新创建，把上一次建出的资源变成孤儿。
		//
		// 恢复标识只追加、绝不参与「是否覆盖」的判定：权限类错误的 PermissionHint
		// 是权威恢复指引（要授哪个 scope、开发者控制台链接），classify.go 明确声明
		// 它压过服务端细节。拿 error.value 当覆盖依据会把整段授权步骤挤成一串裸 JSON。
		if recovery := extractDataErrorValue(resultMap); recovery != "" {
			if merged != "" {
				merged += "; "
			}
			merged += "error.value=" + recovery
		}
		p.Hint = merged
	}
	return err
}

func enrichBaseAPIErrorFromBody(err error, body []byte, cc errclass.ClassifyContext) error {
	if _, ok := errs.ProblemOf(err); !ok {
		return err
	}
	result, parseErr := decodeBaseV3Response(body)
	if parseErr != nil {
		return err
	}
	enriched := baseAPIErrorFromResult(result, cc)
	if enriched == nil {
		return err
	}
	src, _ := errs.ProblemOf(enriched)
	dst, _ := errs.ProblemOf(err)
	if src != nil && dst != nil {
		dst.Message = src.Message
		dst.Hint = src.Hint
		// A body without log_id must not erase a header-derived LogID
		// already carried by err.
		if src.LogID != "" {
			dst.LogID = src.LogID
		}
	}
	return err
}

func extractBaseErrorLogID(resultMap map[string]interface{}) string {
	for _, key := range []string{"log_id", "logid"} {
		if logID, _ := resultMap[key].(string); strings.TrimSpace(logID) != "" {
			return strings.TrimSpace(logID)
		}
	}
	if detail, ok := resultMap["error"].(map[string]interface{}); ok {
		for _, key := range []string{"log_id", "logid"} {
			if logID, _ := detail[key].(string); strings.TrimSpace(logID) != "" {
				return strings.TrimSpace(logID)
			}
		}
	}
	data, _ := resultMap["data"].(map[string]interface{})
	if detail, ok := data["error"].(map[string]interface{}); ok {
		for _, key := range []string{"log_id", "logid"} {
			if logID, _ := detail[key].(string); strings.TrimSpace(logID) != "" {
				return strings.TrimSpace(logID)
			}
		}
	}
	return ""
}

func extractErrorHint(resultMap map[string]interface{}) string {
	if detail, ok := resultMap["error"].(map[string]interface{}); ok {
		if hint := consumeStringField(detail, "hint"); hint != "" {
			return hint
		}
	}
	data, _ := resultMap["data"].(map[string]interface{})
	if detail, ok := data["error"].(map[string]interface{}); ok {
		if hint := consumeStringField(detail, "hint"); hint != "" {
			return hint
		}
	}
	return ""
}

func extractDataErrorMessage(resultMap map[string]interface{}) string {
	data, _ := resultMap["data"].(map[string]interface{})
	if detail, ok := data["error"].(map[string]interface{}); ok {
		if message := consumeStringField(detail, "message"); message != "" {
			return message
		}
	}
	return ""
}

// dataErrorValueRecoveryKeys 是允许从 data.error.value 透出到 hint 的键。
// 用白名单而不是整块序列化：这条路径是 base 包所有 API 错误的公共出口
// （helpers.go 的 baseV3Call 等），+form-submit / +record-batch-* 的失败也走它，
// 服务端在 value 里回带用户填写的原值时，整块透出会把这些内容复制进 hint，
// 而 hint 会进终端输出和日志。
var dataErrorValueRecoveryKeys = []string{"query_view_id", "completed_steps", "failed_step"}

// dataErrorValueMaxLen 给并进 hint 的恢复标识封顶。这三个键正常只有一个 ID、
// 一个短枚举和一个短数组，超出就说明服务端回带了别的东西。
// 超限时退回到只带 query_view_id（恢复真正需要的那个），而不是截断字符串——
// 文档教调用方把 error.value= 后面那段按 JSON 解析，截断出来的半个 JSON 解不动。
const dataErrorValueMaxLen = 512

// extractDataErrorValue 从 data.error.value 里挑出恢复所需的标识并序列化成紧凑 JSON，
// 供调用方从 hint 读回（如 +form-query-page-create 失败后要带回的 query_view_id）。
// 非对象、无白名单命中或空值一律返回空串，不污染 hint。
func extractDataErrorValue(resultMap map[string]interface{}) string {
	data, _ := resultMap["data"].(map[string]interface{})
	detail, ok := data["error"].(map[string]interface{})
	if !ok {
		return ""
	}
	// 恢复字段的落点见过两种形状：在 data.error.value 里，或与 value 平级挂在
	// data.error 上。两处都取、value 优先——少取一个 failed_step，调用方就没有
	// 分支依据（associate/readback 必须带 ID 重试，只有 create 才可以重建），
	// 只能猜，猜成「可以重建」正好留下这个流程要防的孤儿资源。
	value, _ := detail["value"].(map[string]interface{})
	recovery := map[string]interface{}{}
	for _, key := range dataErrorValueRecoveryKeys {
		if raw, ok := firstNonNullValue(key, value, detail); ok {
			recovery[key] = raw
		}
	}
	if len(recovery) == 0 {
		return ""
	}
	encoded, err := json.Marshal(recovery)
	if err != nil {
		return ""
	}
	if len(encoded) > dataErrorValueMaxLen {
		id, ok := recovery["query_view_id"]
		if !ok {
			return ""
		}
		encoded, err = json.Marshal(map[string]interface{}{"query_view_id": id})
		if err != nil || len(encoded) > dataErrorValueMaxLen {
			return ""
		}
	}
	return string(encoded)
}

// firstNonNullValue 按 sources 的顺序返回 key 对应的第一个非 null 值。
// 键缺失与 JSON 显式 null 解码后都是 nil interface，由 case nil 一并跳过。
func firstNonNullValue(key string, sources ...map[string]interface{}) (interface{}, bool) {
	for _, src := range sources {
		switch raw := src[key].(type) {
		case nil:
			continue
		default:
			return raw, true
		}
	}
	return nil, false
}

func consumeStringField(src map[string]interface{}, key string) string {
	value, _ := src[key].(string)
	if _, exists := src[key]; exists {
		delete(src, key)
	}
	return strings.TrimSpace(value)
}
