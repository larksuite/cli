// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
)

// +form-update 只有 --name / --description。Tips 一旦把主题色、背景引到它那里，
// Agent 就会编造不存在的 flag，或在什么都没改的情况下报告「已改好」。
func TestBaseFormDisplaySettingsUpdateTipsDoNotRouteThemeToFormUpdate(t *testing.T) {
	tips := strings.Join(BaseFormDisplaySettingsUpdate.Tips, "\n")
	if !strings.Contains(tips, "Theme color and background are not supported by the CLI") {
		t.Fatalf("tips should state theme color/background are unsupported:\n%s", tips)
	}
	if !strings.Contains(tips, "title or description with +form-update") {
		t.Fatalf("tips should route title/description to +form-update:\n%s", tips)
	}
}

// revision 只在显式传入时才进请求体：默认零值被当成「期望版本 0」提交，
// 就会对任何已有表单必然 conflict。显式 --revision 0 则是调用方的真实意图，照发。
func TestBuildFormDisplaySettingsUpdateBody(t *testing.T) {
	for _, tc := range []struct {
		name     string
		revision map[string]int
		want     map[string]interface{}
	}{
		{"revision omitted", nil, map[string]interface{}{"display_mode": "one_question_per_page"}},
		{"explicit revision zero", map[string]int{"revision": 0}, map[string]interface{}{"display_mode": "one_question_per_page", "revision": 0}},
		{"explicit revision", map[string]int{"revision": 12}, map[string]interface{}{"display_mode": "one_question_per_page", "revision": 12}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt := newBaseTestRuntime(map[string]string{"display-mode": "one_question_per_page"}, nil, tc.revision)
			if got := buildFormDisplaySettingsUpdateBody(rt); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("body = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestBaseFormDisplaySettingsUpdateValidateRevision(t *testing.T) {
	for _, revision := range []int{0, 7} {
		rt := newBaseTestRuntime(map[string]string{"display-mode": "traditional"}, nil, map[string]int{"revision": revision})
		if err := BaseFormDisplaySettingsUpdate.Validate(context.Background(), rt); err != nil {
			t.Fatalf("--revision %d should be accepted: %v", revision, err)
		}
	}

	rt := newBaseTestRuntime(map[string]string{"display-mode": "traditional"}, nil, map[string]int{"revision": -1})
	err := BaseFormDisplaySettingsUpdate.Validate(context.Background(), rt)
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("negative --revision should be a validation error, got %T %v", err, err)
	}
	if verr.Param != "--revision" {
		t.Fatalf("param = %q, want --revision", verr.Param)
	}
}
