// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"archive/zip"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// --timezone 的大小写规则假设「IANA 名字的每一段都以大写字母开头」。用 Go 自带的
// 时区库（与内嵌 time/tzdata 同源）核对这个前提，时区库哪天引入小写开头的名字，
// 这里会先失败，而不是让合法时区在本地被拒。
func TestIANASegmentsCapitalizedHoldsForGoZoneDatabase(t *testing.T) {
	path := filepath.Join(runtime.GOROOT(), "lib", "time", "zoneinfo.zip")
	reader, err := zip.OpenReader(path)
	if err != nil {
		t.Skipf("Go zone database not available at %s: %v", path, err)
	}
	defer reader.Close()

	checked := 0
	for _, file := range reader.File {
		if strings.HasSuffix(file.Name, "/") {
			continue
		}
		checked++
		if !ianaSegmentsCapitalized(file.Name) {
			t.Errorf("zone %q breaks the capitalized-segment rule used by --timezone validation", file.Name)
		}
	}
	if checked == 0 {
		t.Fatalf("no zone names found in %s", path)
	}
}
