// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"strings"
	"testing"
)

// questionsWithLimit 把一段 input_range_limit JSON 包成 create 能接受的最小题目。
func questionsWithLimit(limit string) string {
	return `[{"type":"text","title":"Q","input_range_limit":` + limit + `}]`
}

func TestFormQuestionsCreateInputRangeLimitValidate(t *testing.T) {
	ctx := context.Background()

	valid := []struct {
		name  string
		limit string
	}{
		{"text_length both bounds", `{"type":"text_length","min_length":10,"max_length":200}`},
		{"text_length min only", `{"type":"text_length","min_length":1}`},
		{"number_range negative and decimal", `{"type":"number_range","min_value":-1.5,"max_value":100}`},
		{"number_range max only", `{"type":"number_range","max_value":0}`},
		{"date_range fixed", `{"type":"date_range","earliest":{"fixed_ms":1790784000000}}`},
		{"date_range dynamic", `{"type":"date_range","latest":{"dynamic":{"anchor":"today","offset_days":7}}}`},
		{"date_range negative offset", `{"type":"date_range","earliest":{"dynamic":{"anchor":"monday","offset_days":-7}}}`},
		{"date_range dynamic without offset", `{"type":"date_range","earliest":{"dynamic":{"anchor":"month_end"}}}`},
		{"null clears the limit", `null`},
		// 下游对枚举值做别名归一化，CLI 校验不能比它严
		{"PascalCase type alias", `{"type":"TextLength","min_length":1}`},
		{"camelCase type alias", `{"type":"numberRange","min_value":0}`},
		{"UPPER_SNAKE type alias", `{"type":"NUMBER_RANGE","min_value":0}`},
		{"kebab type alias", `{"type":"number-range","min_value":0}`},
		{"spaced type alias", `{"type":"number range","min_value":0}`},
		{"PascalCase anchor alias", `{"type":"date_range","earliest":{"dynamic":{"anchor":"MonthStart"}}}`},
		{"kebab anchor alias", `{"type":"date_range","earliest":{"dynamic":{"anchor":"month-start"}}}`},
		{"max_length at upper bound", `{"type":"text_length","max_length":100000}`},
	}
	for _, tc := range valid {
		t.Run("valid/"+tc.name, func(t *testing.T) {
			rt := newBaseTestRuntime(map[string]string{
				"base-token": "app_x", "table-id": "tbl_x", "form-id": "vew_x",
				"questions": questionsWithLimit(tc.limit),
			}, nil, nil)
			if err := BaseFormQuestionsCreate.Validate(ctx, rt); err != nil {
				t.Fatalf("err=%v", err)
			}
		})
	}

	invalid := []struct {
		name  string
		limit string
		want  string
	}{
		// 驼峰别名是模型最容易犯的错，此前会被原样透传
		{"camelCase field", `{"type":"text_length","minLength":10}`, "minLength is not a valid field"},
		{"number as string", `{"type":"text_length","min_length":"10"}`, "must be a JSON number, not a string"},
		{"unknown type", `{"type":"text_len","min_length":1}`, "type must be one of"},
		{"missing type", `{"min_length":1}`, "type must be a non-empty string"},
		{"cross-type field", `{"type":"text_length","min_value":1}`, "min_value is not a valid field"},
		{"no bound at all", `{"type":"text_length"}`, "must set min_length or max_length"},
		{"min exceeds max", `{"type":"text_length","min_length":200,"max_length":10}`, "must not exceed max_length"},
		{"zero length", `{"type":"text_length","min_length":0}`, "must be a positive integer"},
		{"negative length", `{"type":"text_length","min_length":-1}`, "must be a positive integer"},
		{"fractional length", `{"type":"text_length","max_length":1.5}`, "must be a positive integer"},
		{"number_range min exceeds max", `{"type":"number_range","min_value":10,"max_value":1}`, "must not exceed max_value"},
		{"date_range no bound", `{"type":"date_range"}`, "must set earliest or latest"},
		{"date bound both forms", `{"type":"date_range","earliest":{"fixed_ms":1,"dynamic":{"anchor":"today"}}}`, "not both"},
		{"date bound neither form", `{"type":"date_range","earliest":{}}`, "must set fixed_ms or dynamic"},
		{"unknown anchor", `{"type":"date_range","earliest":{"dynamic":{"anchor":"next_week"}}}`, "anchor must be one of"},
		{"fractional offset", `{"type":"date_range","earliest":{"dynamic":{"anchor":"today","offset_days":1.5}}}`, "must be an integer number of days"},
		{"unknown dynamic field", `{"type":"date_range","earliest":{"dynamic":{"anchor":"today","offsetDays":1}}}`, "offsetDays is not a valid field"},
		{"limit not an object", `"text_length"`, "must be an object or null"},
		{"fixed earliest after latest", `{"type":"date_range","earliest":{"fixed_ms":2000},"latest":{"fixed_ms":1000}}`, "earliest must not exceed latest"},
		{"max_length over upper bound", `{"type":"text_length","max_length":100001}`, "must not exceed 100000"},
		// 不在 CLI 支持清单里的 type 必须在本地拒掉，放行只会让请求走到服务端才失败。
		{"unsupported type is rejected", `{"type":"geo_range","enabled":true,"center":{"address":"x"},"radius_meters":500}`, "type must be one of"},
	}
	for _, tc := range invalid {
		t.Run("invalid/"+tc.name, func(t *testing.T) {
			rt := newBaseTestRuntime(map[string]string{
				"base-token": "app_x", "table-id": "tbl_x", "form-id": "vew_x",
				"questions": questionsWithLimit(tc.limit),
			}, nil, nil)
			err := BaseFormQuestionsCreate.Validate(ctx, rt)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, err=%v", tc.want, err)
			}
			// 报错必须带题目序号，模型才知道是哪一条写错了
			if !strings.Contains(err.Error(), "--questions item 1") {
				t.Fatalf("error should carry the item index, err=%v", err)
			}
		})
	}
}

func TestFormQuestionsUpdateValidate(t *testing.T) {
	ctx := context.Background()

	run := func(questions string) error {
		rt := newBaseTestRuntime(map[string]string{
			"base-token": "app_x", "table-id": "tbl_x", "form-id": "vew_x",
			"questions": questions,
		}, nil, nil)
		return BaseFormQuestionsUpdate.Validate(ctx, rt)
	}

	t.Run("valid", func(t *testing.T) {
		if err := run(`[{"id":"fld_x","title":"Q","input_range_limit":{"type":"number_range","min_value":0}}]`); err != nil {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("null clears the limit", func(t *testing.T) {
		if err := run(`[{"id":"fld_x","input_range_limit":null}]`); err != nil {
			t.Fatalf("err=%v", err)
		}
	})

	// 参数描述早就承诺了 id 必填和 10 条上限，此前代码里一条都没实现
	t.Run("missing id", func(t *testing.T) {
		err := run(`[{"title":"Q"}]`)
		if err == nil || !strings.Contains(err.Error(), `must include a non-empty string "id"`) {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("too many items", func(t *testing.T) {
		items := make([]string, 11)
		for i := range items {
			items[i] = `{"id":"fld_x"}`
		}
		err := run("[" + strings.Join(items, ",") + "]")
		if err == nil || !strings.Contains(err.Error(), "at most 10 items") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("invalid limit is rejected too", func(t *testing.T) {
		err := run(`[{"id":"fld_x","input_range_limit":{"type":"text_length","minLength":1}}]`)
		if err == nil || !strings.Contains(err.Error(), "minLength is not a valid field") {
			t.Fatalf("err=%v", err)
		}
	})
}

// map 遍历顺序是随机的；同时有多个非法字段时，报出的字段必须固定，
// 否则同一输入两次运行给出不同报错，调用方照着改完一个又冒出另一个。
func TestValidateInputRangeLimitReportsInvalidFieldsDeterministically(t *testing.T) {
	limit := map[string]interface{}{
		"type": "text_length", "zeta": 1.0, "maxLength": 5.0, "alpha": 1.0,
	}
	for i := 0; i < 50; i++ {
		err := validateInputRangeLimit("--questions item 1", limit)
		if err == nil || !strings.Contains(err.Error(), "input_range_limit.alpha is not a valid field") {
			t.Fatalf("run %d: want the alphabetically first invalid field (alpha), got %v", i, err)
		}
	}
}
