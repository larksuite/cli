// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"io"

	"github.com/larksuite/cli/internal/output"
)

// Gateway paths for the spark app.release OpenAPI methods.
// Prefix reuses apiBasePath = "/open-apis/spark/v1" (same package).
// Each path contains %s placeholders; use fmt.Sprintf to build the final URL.
const (
	releaseCreatePath = apiBasePath + "/apps/%s/releases"
	releaseGetPath    = apiBasePath + "/apps/%s/releases/%s"
	releaseListPath   = apiBasePath + "/apps/%s/releases"
)

// writeReleaseErrorLogTable renders a release's error_logs (a slice of
// {step, error_log} maps from the gateway) as a two-column step/error_log
// table via output.PrintTable. Used by +release-get to render a failed
// release's error_logs. A nil/non-slice or
// empty value yields an empty table (PrintTable prints "(no data)").
func writeReleaseErrorLogTable(w io.Writer, raw interface{}) {
	logs, _ := raw.([]interface{})
	rows := make([]map[string]interface{}, 0, len(logs))
	for _, l := range logs {
		m, ok := l.(map[string]interface{})
		if !ok {
			continue
		}
		rows = append(rows, map[string]interface{}{
			"step":      m["step"],
			"error_log": m["error_log"],
		})
	}
	output.PrintTable(w, rows)
}
