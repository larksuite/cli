// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestNormalizeRecordSortValue(t *testing.T) {
	t.Run("array", func(t *testing.T) {
		sortConfig, err := normalizeRecordSortValue([]interface{}{
			map[string]interface{}{"field": "Updated", "desc": true},
		}, "--sort-json")
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		if len(sortConfig) != 1 {
			t.Fatalf("sortConfig=%#v", sortConfig)
		}
	})

	t.Run("wrapped sort_config", func(t *testing.T) {
		sortConfig, err := normalizeRecordSortValue(map[string]interface{}{
			"sort_config": []interface{}{
				map[string]interface{}{"field": "Updated", "desc": false},
			},
		}, "--json.sort")
		if err != nil {
			t.Fatalf("err=%v", err)
		}
		first := sortConfig[0].(map[string]interface{})
		if first["field"] != "Updated" || first["desc"] != false {
			t.Fatalf("sortConfig=%#v", sortConfig)
		}
	})

	t.Run("invalid wrapper", func(t *testing.T) {
		_, err := normalizeRecordSortValue(map[string]interface{}{"sort": []interface{}{}}, "--sort-json")
		if err == nil || !strings.Contains(err.Error(), "sort_config array") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("invalid sort_config type", func(t *testing.T) {
		_, err := normalizeRecordSortValue(map[string]interface{}{"sort_config": "Updated"}, "--sort-json")
		if err == nil || !strings.Contains(err.Error(), "--sort-json.sort_config must be a JSON array") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("invalid scalar", func(t *testing.T) {
		_, err := normalizeRecordSortValue("Updated", "--sort-json")
		if err == nil || !strings.Contains(err.Error(), "must be a JSON array") {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestApplyRecordQueryToParams(t *testing.T) {
	runtime := newBaseTestRuntime(
		map[string]string{
			"filter-json": `{"logic":"and","conditions":[["Status","==","Todo"]]}`,
			"sort-json":   `{"sort_config":[{"field":"Updated","desc":true}]}`,
		},
		nil,
		nil,
	)
	params := map[string]interface{}{"view_id": "viw_1"}
	if err := applyRecordQueryToParams(runtime, params); err != nil {
		t.Fatalf("err=%v", err)
	}
	if params["view_id"] != "viw_1" {
		t.Fatalf("params=%#v", params)
	}
	var filter map[string]interface{}
	if err := json.Unmarshal([]byte(params["filter"].(string)), &filter); err != nil {
		t.Fatalf("filter err=%v", err)
	}
	if filter["logic"] != "and" {
		t.Fatalf("filter=%#v", filter)
	}
	var sortConfig []interface{}
	if err := json.Unmarshal([]byte(params["sort"].(string)), &sortConfig); err != nil {
		t.Fatalf("sort err=%v", err)
	}
	firstSort := sortConfig[0].(map[string]interface{})
	if firstSort["field"] != "Updated" || firstSort["desc"] != true {
		t.Fatalf("sort=%#v", sortConfig)
	}
}

func TestApplyRecordQueryToURLValues(t *testing.T) {
	runtime := newBaseTestRuntime(
		map[string]string{
			"filter-json": `{"logic":"or","conditions":[["Score",">",90]]}`,
			"sort-json":   `[{"field":"Score","desc":false}]`,
		},
		nil,
		nil,
	)
	params := url.Values{"view_id": {"viw_1"}}
	if err := applyRecordQueryToURLValues(runtime, params); err != nil {
		t.Fatalf("err=%v", err)
	}
	if got := params.Get("view_id"); got != "viw_1" {
		t.Fatalf("view_id=%q", got)
	}
	if !strings.Contains(params.Get("filter"), `"logic":"or"`) || !strings.Contains(params.Get("sort"), `"field":"Score"`) {
		t.Fatalf("params=%#v", params)
	}
}

func TestRecordSearchJSONBodyAppliesQueryFlagOverrides(t *testing.T) {
	runtime := newBaseTestRuntime(
		map[string]string{
			"json":        `{"keyword":"urgent","search_fields":["Title"],"filter":{"logic":"and","conditions":[["Status","==","Done"]]},"sort":{"sort_config":[{"field":"Updated","desc":false}]}}`,
			"filter-json": `{"logic":"and","conditions":[["Status","==","Todo"]]}`,
			"sort-json":   `[{"field":"Score","desc":true}]`,
		},
		nil,
		nil,
	)
	body, err := recordSearchJSONBody(runtime)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	filter := body["filter"].(map[string]interface{})
	conditions := filter["conditions"].([]interface{})
	statusCondition := conditions[0].([]interface{})
	if statusCondition[2] != "Todo" {
		t.Fatalf("filter=%#v", filter)
	}
	sortConfig := body["sort"].([]interface{})
	firstSort := sortConfig[0].(map[string]interface{})
	if firstSort["field"] != "Score" || firstSort["desc"] != true {
		t.Fatalf("sort=%#v", sortConfig)
	}
}

func TestRecordSearchJSONBodyNormalizesWrappedSort(t *testing.T) {
	runtime := newBaseTestRuntime(
		map[string]string{
			"json": `{"keyword":"urgent","search_fields":["Title"],"sort":{"sort_config":[{"field":"Updated","desc":false}]}}`,
		},
		nil,
		nil,
	)
	body, err := recordSearchJSONBody(runtime)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	sortConfig := body["sort"].([]interface{})
	firstSort := sortConfig[0].(map[string]interface{})
	if firstSort["field"] != "Updated" || firstSort["desc"] != false {
		t.Fatalf("sort=%#v", sortConfig)
	}
}
