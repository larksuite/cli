// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package citation

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// 命令侧构造 Citation 时只准写 citation.SourceXxx 符号常量。字面量 int 绕过
// 值域追踪：值域变更时 grep 常量表就能找到全部引用，字面量找不到。
func TestNoSourceTypeIntLiteralsInShortcuts(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", "shortcuts"))
	if err != nil {
		t.Fatal(err)
	}
	literal := regexp.MustCompile(`SourceType:\s*[0-9]|SourceType\(\s*[0-9]`)
	err = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for i, line := range strings.Split(string(content), "\n") {
			if literal.MatchString(line) {
				t.Errorf("%s:%d: SourceType must use citation.SourceXxx constants, not int literals: %s",
					path, i+1, strings.TrimSpace(line))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
