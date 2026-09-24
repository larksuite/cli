// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// Dashboard views own their configuration inside the block. They share filter
// and grouping syntax with charts, but have no measures or chart dimension cap.
type dashboardViewConfig struct {
	TableName     string               `json:"table_name"`
	ViewType      string               `json:"view_type"`
	Filter        json.RawMessage      `json:"filter"`
	GroupBy       []dashboardViewGroup `json:"group_by"`
	SortBy        []dashboardViewSort  `json:"sort_by"`
	VisibleFields []string             `json:"visible_fields"`
}

type dashboardViewGroup struct {
	FieldName string                  `json:"field_name"`
	Mode      *string                 `json:"mode"`
	Sort      *dashboardViewGroupSort `json:"sort"`
}

type dashboardViewGroupSort struct {
	Type  string `json:"type"`
	Order string `json:"order"`
}

type dashboardViewSort struct {
	FieldName string `json:"field_name"`
	Order     string `json:"order"`
}

// Validate only a complete create request. Updates have no type flag and keep
// the existing server-owned partial-update contract. The wire body is still
// built from the parsed input, not this projection, to preserve omitted fields.
func validateDashboardViewConfig(cfg map[string]interface{}) []string {
	encoded, err := json.Marshal(cfg)
	if err != nil {
		return []string{fmt.Sprintf("view 配置编码失败: %v", err)}
	}
	var view dashboardViewConfig
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&view); err != nil {
		return []string{fmt.Sprintf("view 配置结构无效: %v", err)}
	}
	var problems []string
	requireName := func(name, value string) {
		if strings.TrimSpace(value) == "" {
			problems = append(problems, name+" 必须是非空字符串")
		}
	}
	checkOrder := func(name, value string) {
		if value != "asc" && value != "desc" {
			problems = append(problems, name+" 仅支持 asc|desc")
		}
	}
	requireName("table_name", view.TableName)
	requireName("view_type", view.ViewType)
	// The component API owns the supported view types and per-type capabilities;
	// do not infer an enum or limits from the separate table-view endpoints.
	for _, key := range []string{"group_by", "sort_by", "visible_fields"} {
		if value, exists := cfg[key]; exists && value == nil {
			problems = append(problems, key+" 不能为 null")
		}
	}
	for i, group := range view.GroupBy {
		path := fmt.Sprintf("group_by[%d]", i)
		requireName(path+".field_name", group.FieldName)
		if group.Mode != nil {
			requireName(path+".mode", *group.Mode)
		}
		if group.Sort != nil {
			requireName(path+".sort.type", group.Sort.Type)
			checkOrder(path+".sort.order", group.Sort.Order)
		}
	}
	for i, sort := range view.SortBy {
		requireName(fmt.Sprintf("sort_by[%d].field_name", i), sort.FieldName)
		checkOrder(fmt.Sprintf("sort_by[%d].order", i), sort.Order)
	}
	for i, field := range view.VisibleFields {
		requireName(fmt.Sprintf("visible_fields[%d]", i), field)
	}
	return append(problems, validateProtocolFilter(cfg, "filter")...)
}
