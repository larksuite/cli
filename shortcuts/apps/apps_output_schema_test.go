// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"strings"
	"testing"
	"time"
)

func TestAppsOutputSchemaProjectsAndFormats(t *testing.T) {
	row := map[string]interface{}{
		"timestamp_ns": "1782209472123456789",
		"level":        "ERROR",
		"extra":        "ignored",
		"attributes": map[string]interface{}{
			"module":      "frontend",
			"duration_ms": "1234.5",
		},
	}
	schema := appsOutputSchema{
		Columns: []appsOutputColumn{
			{Key: "timestamp_ns", Label: "time", Format: appsFormatNS("2006-01-02 15:04:05.000")},
			{Key: "module", Value: appsAttrValue("module")},
			{Key: "duration_ms", Value: appsAttrValue("duration_ms"), Format: appsFormatDurationMS},
			{Key: "level"},
		},
		Strict: true,
	}

	projected := appsProjectRow(row, schema)
	if len(projected) != 4 {
		t.Fatalf("projected field count = %d, want 4: %#v", len(projected), projected)
	}
	if projected["module"] != "frontend" || projected["duration_ms"] != "1234.5" {
		t.Fatalf("projected derived fields = %#v", projected)
	}
	if _, ok := projected["extra"]; ok {
		t.Fatalf("strict projection should drop extra field: %#v", projected)
	}

	var b strings.Builder
	appsPrintSchemaTable(&b, []map[string]interface{}{projected}, schema)
	out := b.String()
	wantTime := time.Unix(0, 1782209472123456789).Local().Format("2006-01-02 15:04:05.000")
	if !strings.HasPrefix(out, "time") {
		t.Fatalf("pretty output should start with schema label time, got:\n%s", out)
	}
	if !strings.Contains(out, wantTime) {
		t.Fatalf("pretty output missing formatted time %q:\n%s", wantTime, out)
	}
	if strings.Contains(out, "1782209472123456789") {
		t.Fatalf("pretty output should not contain raw timestamp:\n%s", out)
	}
}
