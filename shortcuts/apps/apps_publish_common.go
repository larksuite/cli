// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

// Gateway paths for the spark app.release OpenAPI methods.
// Prefix reuses apiBasePath = "/open-apis/spark/v1" (same package).
// Each path contains %s placeholders; use fmt.Sprintf to build the final URL.
const (
	publishCreatePath   = apiBasePath + "/apps/%s/releases"
	publishGetPath      = apiBasePath + "/apps/%s/releases/%s"
	publishErrorLogPath = apiBasePath + "/apps/%s/releases/%s/error_logs"
	publishListPath     = apiBasePath + "/apps/%s/releases"
)
