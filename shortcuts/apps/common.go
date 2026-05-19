// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

// appsService 是 CLI 命令的 service 前缀（lark-cli apps ...）。
const appsService = "apps"

// apiBasePath 是服务端 OAPI URL 前缀，待服务端最终注册名 finalize 时改一处。
const apiBasePath = "/open-apis/spark/v1" // BOE 端 OAPI 当前以 /open-apis/spark/v1 注册；待后端 /miaoda/v1 注册稳定后再切回
