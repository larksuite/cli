// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

// Fine-grained error types (permission, not_found, rate_limit, etc.)
// are communicated via the JSON error envelope's "type" field,
// not via exit codes.
const (
	ExitOK                   = 0  // 成功
	ExitAPI                  = 1  // API / 通用错误（含 permission、not_found、conflict、rate_limit）
	ExitValidation           = 2  // 参数校验失败
	ExitAuth                 = 3  // 认证失败（token 无效 / 过期）
	ExitNetwork              = 4  // 网络错误（连接超时、DNS 解析失败等）
	ExitInternal             = 5  // 内部错误（不应发生）
	ExitContentSafety        = 6  // content safety violation (block mode)
	ExitConfirmationRequired = 10 // 高风险操作需要 --yes 确认（agent 协议信号）
)
