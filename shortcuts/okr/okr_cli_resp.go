// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

// RespAlignment 对齐关系
type RespAlignment struct {
	ID             string    `json:"id"`
	CreateTime     string    `json:"create_time"`
	UpdateTime     string    `json:"update_time"`
	FromOwner      RespOwner `json:"from_owner"`
	ToOwner        RespOwner `json:"to_owner"`
	FromEntityType string    `json:"from_entity_type"`
	FromEntityID   string    `json:"from_entity_id"`
	ToEntityType   string    `json:"to_entity_type"`
	ToEntityID     string    `json:"to_entity_id"`
}

// RespCategory 分类
type RespCategory struct {
	ID           string       `json:"id"`
	CreateTime   string       `json:"create_time"`
	UpdateTime   string       `json:"update_time"`
	CategoryType string       `json:"category_type"`
	Enabled      *bool        `json:"enabled,omitempty"`
	Color        *string      `json:"color,omitempty"`
	Name         CategoryName `json:"name"`
}

// RespCycle 周期
type RespCycle struct {
	ID            string    `json:"id"`
	CreateTime    string    `json:"create_time"`
	UpdateTime    string    `json:"update_time"`
	TenantCycleID string    `json:"tenant_cycle_id"`
	Owner         RespOwner `json:"owner"`
	StartTime     string    `json:"start_time"`
	EndTime       string    `json:"end_time"`
	CycleStatus   *string   `json:"cycle_status,omitempty"`
	Score         *float64  `json:"score,omitempty"`
}

// RespIndicator 指标
type RespIndicator struct {
	ID                        string             `json:"id"`
	CreateTime                string             `json:"create_time"`
	UpdateTime                string             `json:"update_time"`
	Owner                     RespOwner          `json:"owner"`
	EntityType                *string            `json:"entity_type,omitempty"`
	EntityID                  *string            `json:"entity_id,omitempty"`
	IndicatorStatus           *string            `json:"indicator_status,omitempty"`
	StatusCalculateType       *string            `json:"status_calculate_type,omitempty"`
	StartValue                *float64           `json:"start_value,omitempty"`
	TargetValue               *float64           `json:"target_value,omitempty"`
	CurrentValue              *float64           `json:"current_value,omitempty"`
	CurrentValueCalculateType *string            `json:"current_value_calculate_type,omitempty"`
	Unit                      *RespIndicatorUnit `json:"unit,omitempty"`
}

// RespIndicatorUnit 指标单位
type RespIndicatorUnit struct {
	UnitType  *string `json:"unit_type,omitempty"`
	UnitValue *string `json:"unit_value,omitempty"`
}

// RespKeyResult 关键结果
type RespKeyResult struct {
	ID          string    `json:"id"`
	CreateTime  string    `json:"create_time"`
	UpdateTime  string    `json:"update_time"`
	Owner       RespOwner `json:"owner"`
	ObjectiveID string    `json:"objective_id"`
	Position    *int32    `json:"position,omitempty"`
	Content     *string   `json:"content,omitempty"`
	Score       *float64  `json:"score,omitempty"`
	Weight      *float64  `json:"weight,omitempty"`
	Deadline    *string   `json:"deadline,omitempty"`
}

// RespObjective 目标
type RespObjective struct {
	ID         string          `json:"id"`
	CreateTime string          `json:"create_time"`
	UpdateTime string          `json:"update_time"`
	Owner      RespOwner       `json:"owner"`
	CycleID    string          `json:"cycle_id"`
	Position   *int32          `json:"position,omitempty"`
	Content    *string         `json:"content,omitempty"`
	Score      *float64        `json:"score,omitempty"`
	Notes      *string         `json:"notes,omitempty"`
	Weight     *float64        `json:"weight,omitempty"`
	Deadline   *string         `json:"deadline,omitempty"`
	CategoryID *string         `json:"category_id,omitempty"`
	KeyResults []RespKeyResult `json:"key_results,omitempty"`
}

// RespOwner OKR 所有者
type RespOwner struct {
	OwnerType string  `json:"owner_type"`
	UserID    *string `json:"user_id,omitempty"`
}

// ProgressStatus 进展状态
type ProgressStatus int32

const (
	ProgressStatusNormal  ProgressStatus = 0 // 正常
	ProgressStatusOverdue ProgressStatus = 1 // 逾期
	ProgressStatusDone    ProgressStatus = 2 // 已完成
)

// ParseProgressStatus parses a progress status string into ProgressStatus.
// Accepts "normal", "overdue", "done" or their numeric values "0", "1", "2".
func ParseProgressStatus(s string) (ProgressStatus, bool) {
	switch s {
	case "normal", "0":
		return ProgressStatusNormal, true
	case "overdue", "1":
		return ProgressStatusOverdue, true
	case "done", "2":
		return ProgressStatusDone, true
	default:
		return 0, false
	}
}

// String returns a human-readable name for ProgressStatus.
func (s ProgressStatus) String() string {
	switch s {
	case ProgressStatusNormal:
		return "normal"
	case ProgressStatusOverdue:
		return "overdue"
	case ProgressStatusDone:
		return "done"
	default:
		return ""
	}
}

// RespProgressRate 进度率（面向用户的响应格式，Status 为可读字符串）
type RespProgressRate struct {
	Percent *float64 `json:"percent,omitempty"`
	Status  *string  `json:"status,omitempty"`
}

// RespProgress 进展记录
type RespProgress struct {
	ID           string            `json:"progress_id"`
	ModifyTime   string            `json:"modify_time"`
	CreateTime   *string           `json:"create_time,omitempty"`
	Content      *string           `json:"content,omitempty"`
	ProgressRate *RespProgressRate `json:"progress_rate,omitempty"`
}
