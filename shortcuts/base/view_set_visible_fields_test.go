// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"
	"testing"
)

func TestViewSetVisibleFieldsValidate(t *testing.T) {
	ctx := context.Background()

	run := func(jsonArg string) error {
		rt := newBaseTestRuntime(map[string]string{
			"base-token": "app_x", "table-id": "tbl_x", "view-id": "vew_x",
			"json": jsonArg,
		}, nil, nil)
		return BaseViewSetVisibleFields.Validate(ctx, rt)
	}

	t.Run("valid", func(t *testing.T) {
		if err := run(`{"visible_fields":["Name","Status"]}`); err != nil {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("empty array clears visibility", func(t *testing.T) {
		if err := run(`{"visible_fields":[]}`); err != nil {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("missing key", func(t *testing.T) {
		err := run(`{"fields":["Name"]}`)
		if err == nil || !strings.Contains(err.Error(), `must contain "visible_fields"`) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("bare array is rejected", func(t *testing.T) {
		if err := run(`["Name","Status"]`); err == nil {
			t.Fatal("bare array should be rejected")
		}
	})

	t.Run("non-string item", func(t *testing.T) {
		err := run(`{"visible_fields":["Name",123]}`)
		if err == nil || !strings.Contains(err.Error(), "visible_fields[1] must be a field name or ID string") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("empty item", func(t *testing.T) {
		err := run(`{"visible_fields":["Name","  "]}`)
		if err == nil || !strings.Contains(err.Error(), "visible_fields[1] must not be empty") {
			t.Fatalf("err=%v", err)
		}
	})

	// 拒绝首尾空格：值原样发给服务端，带空格的名字会被当成不存在的
	// 字段，等同漏传而把题目隐藏。
	t.Run("leading whitespace", func(t *testing.T) {
		err := run(`{"visible_fields":[" Name"]}`)
		if err == nil || !strings.Contains(err.Error(), "must not have leading or trailing whitespace") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("trailing whitespace", func(t *testing.T) {
		err := run(`{"visible_fields":["Name "]}`)
		if err == nil || !strings.Contains(err.Error(), "must not have leading or trailing whitespace") {
			t.Fatalf("err=%v", err)
		}
	})

	// 字面空串与纯空白都必须报「空」而不是「带空格」——判断顺序反了，提示会变成
	// 「写成 ""」这种废话。
	t.Run("literal empty string reports empty", func(t *testing.T) {
		err := run(`{"visible_fields":["Name",""]}`)
		if err == nil || !strings.Contains(err.Error(), "must not be empty") {
			t.Fatalf("err=%v", err)
		}
	})

	// 重复 ID 时服务端的排序结果未定义，必须在本地挡下
	t.Run("duplicate item", func(t *testing.T) {
		err := run(`{"visible_fields":["Name","Status","Name"]}`)
		if err == nil || !strings.Contains(err.Error(), "duplicates item 0") {
			t.Fatalf("err=%v", err)
		}
	})
}
