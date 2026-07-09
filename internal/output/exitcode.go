// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

import (
	"errors"

	"github.com/larksuite/cli/errs"
)

// Fine-grained error types (permission, not_found, rate_limit, etc.)
// are communicated via the JSON error envelope's "type" field,
// not via exit codes.
const (
	ExitOK                   = 0  // 成功
	ExitAPI                  = 1  // API / 通用错误（含 permission、not_found、conflict、rate_limit）
	ExitValidation           = 2  // 参数校验失败
	ExitAuth                 = 3  // 认证失败（token 无效 / 过期），或登录成功但请求 scopes 未全部授予
	ExitNetwork              = 4  // 网络错误（连接超时、DNS 解析失败等）
	ExitInternal             = 5  // 内部错误（不应发生）
	ExitContentSafety        = 6  // content safety violation (block mode)
	ExitConfirmationRequired = 10 // 高风险操作需要 --yes 确认（agent 协议信号）
)

// ExitCodeForCategory maps an errs.Category to the shell exit code.
// Multiple categories may share an exit code (Authentication / Authorization /
// Config all map to 3), so the relationship is many-to-one.
func ExitCodeForCategory(cat errs.Category) int {
	switch cat {
	case errs.CategoryValidation:
		return ExitValidation
	case errs.CategoryAuthentication, errs.CategoryAuthorization, errs.CategoryConfig:
		return ExitAuth
	case errs.CategoryNetwork:
		return ExitNetwork
	case errs.CategoryAPI:
		return ExitAPI
	case errs.CategoryPolicy:
		return ExitContentSafety
	case errs.CategoryInternal:
		return ExitInternal
	case errs.CategoryConfirmation:
		return ExitConfirmationRequired
	}
	return ExitInternal
}

// ExitCodeOf returns the shell exit code for any error.
//   - typed errors (*errs.PermissionError, *errs.APIError, *errs.ConfigError,
//     *errs.AuthenticationError, ...) → routed by Category
//   - *PartialFailureError / *BareError signals → their own Code field
//   - untyped → ExitInternal
func ExitCodeOf(err error) int {
	if err == nil {
		return ExitOK
	}
	if _, ok := errs.ProblemOf(err); ok {
		return ExitCodeForCategory(errs.CategoryOf(err))
	}
	var pfErr *PartialFailureError
	if errors.As(err, &pfErr) {
		return pfErr.Code
	}
	var bare *BareError
	if errors.As(err, &bare) {
		return bare.Code
	}
	return ExitInternal
}
