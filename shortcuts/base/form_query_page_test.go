// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/larksuite/cli/errs"
)

// 请求体是「新建」与「恢复关联」的唯一区分：不传 --query-view-id 必须是空对象，
// 传了必须带上同一个 ID（去首尾空白）。两者混淆，要么恢复变成重建留下孤儿视图，
// 要么新建请求带上一个无意义的 ID。
func TestBuildFormQueryPageCreateBody(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags map[string]string
		want  map[string]interface{}
	}{
		{"create without id", map[string]string{"base-token": "app_x"}, map[string]interface{}{}},
		{"restore with id", map[string]string{"query-view-id": " vewQuery "}, map[string]interface{}{"query_view_id": "vewQuery"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := buildFormQueryPageCreateBody(newBaseTestRuntime(tc.flags, nil, nil)); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("body = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestBaseFormQueryPageCreateValidateRejectsBlankID(t *testing.T) {
	for _, id := range []string{"", "   "} {
		rt := newBaseTestRuntime(map[string]string{"query-view-id": id}, nil, nil)
		err := BaseFormQueryPageCreate.Validate(context.Background(), rt)
		var verr *errs.ValidationError
		if !errors.As(err, &verr) {
			t.Fatalf("blank --query-view-id %q should be a validation error, got %T %v", id, err, err)
		}
		if verr.Param != "--query-view-id" {
			t.Fatalf("param = %q, want --query-view-id", verr.Param)
		}
	}
	if err := BaseFormQueryPageCreate.Validate(context.Background(), newBaseTestRuntime(map[string]string{"base-token": "app_x"}, nil, nil)); err != nil {
		t.Fatalf("omitting --query-view-id must be accepted: %v", err)
	}
}
