// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
)

func TestBaseFormExecuteList(t *testing.T) {
	t.Run("single page", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"has_more": false,
					"total":    2,
					"forms": []interface{}{
						map[string]interface{}{"id": "vew_form1", "name": "用户调研问卷", "description": "2024年调研"},
						map[string]interface{}{"id": "vew_form2", "name": "产品反馈表", "description": ""},
					},
				},
			},
		})
		if err := runShortcut(t, BaseFormsList, []string{"+form-list", "--base-token", "app_x", "--table-id", "tbl_x"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"vew_form1"`) || !strings.Contains(got, `"total": 2`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("auto pagination", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		// First page: has_more=true
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"has_more":   true,
					"page_token": "tok_p2",
					"total":      2,
					"forms": []interface{}{
						map[string]interface{}{"id": "vew_form1", "name": "Page1 Form", "description": ""},
					},
				},
			},
		})
		// Second page: has_more=false
		reg.Register(&httpmock.Stub{
			Method: "GET",
			URL:    "page_token=tok_p2",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"has_more": false,
					"total":    2,
					"forms": []interface{}{
						map[string]interface{}{"id": "vew_form2", "name": "Page2 Form", "description": ""},
					},
				},
			},
		})
		if err := runShortcut(t, BaseFormsList, []string{"+form-list", "--base-token", "app_x", "--table-id", "tbl_x"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		got := stdout.String()
		if !strings.Contains(got, `"vew_form1"`) || !strings.Contains(got, `"vew_form2"`) {
			t.Fatalf("stdout=%s", got)
		}
		if !strings.Contains(got, `"total": 2`) {
			t.Fatalf("expected total=2 in stdout=%s", got)
		}
	})
}

func TestBaseFormExecuteGet(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms/vew_form1",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"id":          "vew_form1",
				"name":        "用户调研问卷",
				"description": "2024年度用户满意度调研",
			},
		},
	})
	if err := runShortcut(t, BaseFormGet, []string{"+form-get", "--base-token", "app_x", "--table-id", "tbl_x", "--form-id", "vew_form1"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"vew_form1"`) || !strings.Contains(got, `"用户调研问卷"`) {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseFormExecuteCreate(t *testing.T) {
	t.Run("name only", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"id":          "vew_form_new",
					"name":        "新建表单",
					"description": "",
				},
			},
		})
		if err := runShortcut(t, BaseFormCreate, []string{"+form-create", "--base-token", "app_x", "--table-id", "tbl_x", "--name", "新建表单"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"vew_form_new"`) || !strings.Contains(got, `"新建表单"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("with description", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"id":          "vew_form_desc",
					"name":        "含描述表单",
					"description": "这是表单说明",
				},
			},
		})
		args := []string{"+form-create", "--base-token", "app_x", "--table-id", "tbl_x", "--name", "含描述表单",
			"--description", "这是表单说明"}
		if err := runShortcut(t, BaseFormCreate, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"vew_form_desc"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("with description link", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"id":          "vew_form_link",
					"name":        "含链接表单",
					"description": "更多信息请查看[这里](https://example.com)",
				},
			},
		})
		args := []string{"+form-create", "--base-token", "app_x", "--table-id", "tbl_x", "--name", "含链接表单",
			"--description", "更多信息请查看[这里](https://example.com)"}
		if err := runShortcut(t, BaseFormCreate, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"vew_form_link"`) {
			t.Fatalf("stdout=%s", got)
		}
	})
}

func TestBaseFormExecuteUpdate(t *testing.T) {
	t.Run("update name", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "PATCH",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms/vew_form1",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"id":          "vew_form1",
					"name":        "更新后的表单",
					"description": "",
				},
			},
		})
		if err := runShortcut(t, BaseFormUpdate, []string{"+form-update", "--base-token", "app_x", "--table-id", "tbl_x", "--form-id", "vew_form1", "--name", "更新后的表单"}, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"vew_form1"`) || !strings.Contains(got, `"更新后的表单"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("update with description", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "PATCH",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms/vew_form1",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"id":          "vew_form1",
					"name":        "Form",
					"description": "更新的描述内容",
				},
			},
		})
		args := []string{"+form-update", "--base-token", "app_x", "--table-id", "tbl_x", "--form-id", "vew_form1",
			"--description", "更新的描述内容"}
		if err := runShortcut(t, BaseFormUpdate, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"vew_form1"`) {
			t.Fatalf("stdout=%s", got)
		}
	})
}

func TestBaseFormExecuteDelete(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms/vew_form1",
		Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
	})
	if err := runShortcut(t, BaseFormDelete, []string{"+form-delete", "--base-token", "app_x", "--table-id", "tbl_x", "--form-id", "vew_form1", "--yes"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"deleted": true`) || !strings.Contains(got, `"form_id": "vew_form1"`) {
		t.Fatalf("stdout=%s", got)
	}
}

func TestBaseFormQuestionsExecuteList(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms/vew_form1/questions",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"total": 2,
				"questions": []interface{}{
					map[string]interface{}{"id": "q_001", "title": "您的姓名", "required": true, "description": nil},
					map[string]interface{}{"id": "q_002", "title": "发票抬头", "required": false, "description": nil,
						"visible_rule": map[string]interface{}{"logic": "and", "conditions": []interface{}{[]interface{}{"q_001", "==", "是"}}}},
				},
			},
		},
	})
	if err := runShortcut(t, BaseFormQuestionsList, []string{"+form-questions-list", "--base-token", "app_x", "--table-id", "tbl_x", "--form-id", "vew_form1"}, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	got := stdout.String()
	if !strings.Contains(got, `"q_001"`) || !strings.Contains(got, `"total": 2`) {
		t.Fatalf("stdout=%s", got)
	}
	// The list output must forward visible_rule verbatim so agents can read existing display conditions.
	if !strings.Contains(got, `"visible_rule"`) {
		t.Fatalf("visible_rule missing from list output: %s", got)
	}
}

func TestBaseFormQuestionsExecuteCreate(t *testing.T) {
	t.Run("create questions", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms/vew_form1/questions",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"questions": []interface{}{
						map[string]interface{}{"id": "q_new1", "title": "您的姓名", "required": true},
					},
				},
			},
		})
		args := []string{"+form-questions-create", "--base-token", "app_x", "--table-id", "tbl_x", "--form-id", "vew_form1",
			"--questions", `[{"type":"text","title":"您的姓名","required":true}]`}
		if err := runShortcut(t, BaseFormQuestionsCreate, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"questions"`) || !strings.Contains(got, `"q_new1"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("invalid questions json", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		args := []string{"+form-questions-create", "--base-token", "app_x", "--table-id", "tbl_x", "--form-id", "vew_form1",
			"--questions", `not-an-array`}
		if err := runShortcut(t, BaseFormQuestionsCreate, args, factory, stdout); err == nil {
			t.Fatalf("expected error for invalid questions JSON")
		}
	})

	t.Run("visible_rule passthrough", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		stub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms/vew_form1/questions",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"questions": []interface{}{
						map[string]interface{}{"id": "q_new1", "title": "发票抬头"},
					},
				},
			},
		}
		reg.Register(stub)
		args := []string{"+form-questions-create", "--base-token", "app_x", "--table-id", "tbl_x", "--form-id", "vew_form1",
			"--questions", `[{"type":"text","title":"发票抬头","visible_rule":{"logic":"and","conditions":[["是否需要发票","==","是"]]}}]`}
		if err := runShortcut(t, BaseFormQuestionsCreate, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		var body struct {
			Questions []map[string]interface{} `json:"questions"`
		}
		if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
			t.Fatalf("captured body json err=%v body=%s", err, string(stub.CapturedBody))
		}
		if len(body.Questions) != 1 {
			t.Fatalf("questions=%#v", body.Questions)
		}
		rule, ok := body.Questions[0]["visible_rule"].(map[string]interface{})
		if !ok {
			t.Fatalf("visible_rule not forwarded verbatim: body=%s", string(stub.CapturedBody))
		}
		if rule["logic"] != "and" {
			t.Fatalf("visible_rule logic not preserved: %#v", rule)
		}
	})

	t.Run("use existing field passthrough", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		stub := &httpmock.Stub{
			Method: "POST",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms/vew_form1/questions",
			Body: map[string]interface{}{
				"code": 0,
				"data": map[string]interface{}{
					"questions": []interface{}{
						map[string]interface{}{"id": "fldEmail", "title": "你的邮箱"},
					},
				},
			},
		}
		reg.Register(stub)
		args := []string{"+form-questions-create", "--base-token", "app_x", "--table-id", "tbl_x", "--form-id", "vew_form1",
			"--questions", `[{"use_existing_field":true,"field_id":"fldEmail","title":"你的邮箱","required":true}]`}
		if err := runShortcut(t, BaseFormQuestionsCreate, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		body := decodeCapturedJSONBody(t, stub)
		questions, _ := body["questions"].([]interface{})
		if len(questions) != 1 {
			t.Fatalf("questions=%#v", body["questions"])
		}
		question, _ := questions[0].(map[string]interface{})
		if question["use_existing_field"] != true || question["field_id"] != "fldEmail" {
			t.Fatalf("existing field question not forwarded: body=%s", string(stub.CapturedBody))
		}
	})
}

func TestBaseFormQuestionsExecuteUpdate(t *testing.T) {
	factory, stdout, reg := newExecuteFactory(t)
	stub := &httpmock.Stub{
		Method: "PATCH",
		URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms/vew_form1/questions",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"questions": []interface{}{
					map[string]interface{}{"id": "q_001", "title": "更新后的问题", "required": true},
				},
			},
		},
	}
	reg.Register(stub)
	args := []string{"+form-questions-update", "--base-token", "app_x", "--table-id", "tbl_x", "--form-id", "vew_form1",
		"--questions", `[{"id":"q_001","title":"更新后的问题","required":true,"visible_rule":{"logic":"and","conditions":[["q_002","==","是"]]}}]`}
	if err := runShortcut(t, BaseFormQuestionsUpdate, args, factory, stdout); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := stdout.String(); !strings.Contains(got, `"questions"`) || !strings.Contains(got, `"q_001"`) {
		t.Fatalf("stdout=%s", got)
	}
	// visible_rule must be forwarded verbatim to the API (transcribe faithfully).
	var body struct {
		Questions []map[string]interface{} `json:"questions"`
	}
	if err := json.Unmarshal(stub.CapturedBody, &body); err != nil {
		t.Fatalf("captured body json err=%v body=%s", err, string(stub.CapturedBody))
	}
	if len(body.Questions) != 1 {
		t.Fatalf("questions=%#v", body.Questions)
	}
	if _, ok := body.Questions[0]["visible_rule"].(map[string]interface{}); !ok {
		t.Fatalf("visible_rule not forwarded verbatim: body=%s", string(stub.CapturedBody))
	}
}

func TestBaseFormQuestionsExecuteDelete(t *testing.T) {
	t.Run("delete questions", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		reg.Register(&httpmock.Stub{
			Method: "DELETE",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms/vew_form1/questions",
			Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
		})
		args := []string{"+form-questions-delete", "--base-token", "app_x", "--table-id", "tbl_x", "--form-id", "vew_form1",
			"--question-ids", `["q_001","q_002"]`, "--yes"}
		if err := runShortcut(t, BaseFormQuestionsDelete, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		if got := stdout.String(); !strings.Contains(got, `"deleted": true`) || !strings.Contains(got, `"q_001"`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("keep field", func(t *testing.T) {
		factory, stdout, reg := newExecuteFactory(t)
		stub := &httpmock.Stub{
			Method: "DELETE",
			URL:    "/open-apis/base/v3/bases/app_x/tables/tbl_x/forms/vew_form1/questions",
			Body:   map[string]interface{}{"code": 0, "data": map[string]interface{}{}},
		}
		reg.Register(stub)
		args := []string{"+form-questions-delete", "--base-token", "app_x", "--table-id", "tbl_x", "--form-id", "vew_form1",
			"--question-ids", `["fldEmail"]`, "--keep-field", "--yes"}
		if err := runShortcut(t, BaseFormQuestionsDelete, args, factory, stdout); err != nil {
			t.Fatalf("err=%v", err)
		}
		body := decodeCapturedJSONBody(t, stub)
		if body["keep_field"] != true {
			t.Fatalf("keep_field not forwarded: body=%s", string(stub.CapturedBody))
		}
		if got := stdout.String(); !strings.Contains(got, `"keep_field": true`) {
			t.Fatalf("stdout=%s", got)
		}
	})

	t.Run("invalid question-ids json", func(t *testing.T) {
		factory, stdout, _ := newExecuteFactory(t)
		args := []string{"+form-questions-delete", "--base-token", "app_x", "--table-id", "tbl_x", "--form-id", "vew_form1",
			"--question-ids", `not-json`}
		if err := runShortcut(t, BaseFormQuestionsDelete, args, factory, stdout); err == nil {
			t.Fatalf("expected error for invalid question-ids JSON")
		}
	})
}
