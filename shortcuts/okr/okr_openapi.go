// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"encoding/json"
	"strconv"
	"time"
)

// CycleStatus 周期状态
type CycleStatus int32

const (
	CycleStatusDefault CycleStatus = 0
	CycleStatusNormal  CycleStatus = 1
	CycleStatusInvalid CycleStatus = 2
	CycleStatusHidden  CycleStatus = 3
)

func (t CycleStatus) Ptr() *CycleStatus { return &t }

// StatusCalculateType 状态计算类型
type StatusCalculateType int32

const (
	StatusCalculateTypeManualUpdate                                      StatusCalculateType = 0
	StatusCalculateTypeAutomaticallyUpdatesBasedOnProgressAndCurrentTime StatusCalculateType = 1
	StatusCalculateTypeStatusUpdatesBasedOnTheHighestRiskKeyResults      StatusCalculateType = 2
)

// BlockElementType 块元素类型
type BlockElementType string

const (
	BlockElementTypeGallery   BlockElementType = "gallery"
	BlockElementTypeParagraph BlockElementType = "paragraph"
)

func (t BlockElementType) Ptr() *BlockElementType { return &t }

// CategoryName 分类名称
type CategoryName struct {
	Zh *string `json:"zh,omitempty"`
	En *string `json:"en,omitempty"`
	Ja *string `json:"ja,omitempty"`
}

// ListType 列表类型
type ListType string

const (
	ListTypeBullet     ListType = "bullet"
	ListTypeCheckBox   ListType = "checkBox"
	ListTypeCheckedBox ListType = "checkedBox"
	ListTypeIndent     ListType = "indent"
	ListTypeNumber     ListType = "number"
)

// OwnerType 所有者类型
type OwnerType string

const (
	OwnerTypeDepartment OwnerType = "department"
	OwnerTypeUser       OwnerType = "user"
)

// ParagraphElementType 段落元素类型
type ParagraphElementType string

const (
	ParagraphElementTypeDocsLink ParagraphElementType = "docsLink"
	ParagraphElementTypeMention  ParagraphElementType = "mention"
	ParagraphElementTypeTextRun  ParagraphElementType = "textRun"
)

func (t ParagraphElementType) Ptr() *ParagraphElementType { return &t }

// ContentBlock 内容块
type ContentBlock struct {
	Blocks []ContentBlockElement `json:"blocks,omitempty"`
}

// ContentBlockElement 内容块元素
type ContentBlockElement struct {
	BlockElementType *BlockElementType `json:"block_element_type,omitempty"`
	Paragraph        *ContentParagraph `json:"paragraph,omitempty"`
	Gallery          *ContentGallery   `json:"gallery,omitempty"`
}

// ContentColor 颜色
type ContentColor struct {
	Red   *int32   `json:"red,omitempty"`
	Green *int32   `json:"green,omitempty"`
	Blue  *int32   `json:"blue,omitempty"`
	Alpha *float64 `json:"alpha,omitempty"`
}

// ContentDocsLink 文档链接
type ContentDocsLink struct {
	URL   *string `json:"url,omitempty"`
	Title *string `json:"title,omitempty"`
}

// ContentGallery 图库
type ContentGallery struct {
	Images []ContentImageItem `json:"images,omitempty"`
}

// ContentImageItem 图片项
type ContentImageItem struct {
	FileToken *string  `json:"file_token,omitempty"`
	Src       *string  `json:"src,omitempty"`
	Width     *float64 `json:"width,omitempty"`
	Height    *float64 `json:"height,omitempty"`
}

// ContentLink 链接
type ContentLink struct {
	URL *string `json:"url,omitempty"`
}

// ContentList 列表
type ContentList struct {
	ListType    *ListType `json:"list_type,omitempty"`
	IndentLevel *int32    `json:"indent_level,omitempty"`
	Number      *int32    `json:"number,omitempty"`
}

// ContentMention 提及
type ContentMention struct {
	UserID *string `json:"user_id,omitempty"`
}

// ContentParagraph 段落
type ContentParagraph struct {
	Style    *ContentParagraphStyle    `json:"style,omitempty"`
	Elements []ContentParagraphElement `json:"elements,omitempty"`
}

// ContentParagraphElement 段落元素
type ContentParagraphElement struct {
	ParagraphElementType *ParagraphElementType `json:"paragraph_element_type,omitempty"`
	TextRun              *ContentTextRun       `json:"text_run,omitempty"`
	DocsLink             *ContentDocsLink      `json:"docs_link,omitempty"`
	Mention              *ContentMention       `json:"mention,omitempty"`
}

// ContentParagraphStyle 段落样式
type ContentParagraphStyle struct {
	List *ContentList `json:"list,omitempty"`
}

// ContentTextRun 文本块
type ContentTextRun struct {
	Text  *string           `json:"text,omitempty"`
	Style *ContentTextStyle `json:"style,omitempty"`
}

// ContentTextStyle 文本样式
type ContentTextStyle struct {
	Bold          *bool         `json:"bold,omitempty"`
	StrikeThrough *bool         `json:"strike_through,omitempty"`
	BackColor     *ContentColor `json:"back_color,omitempty"`
	TextColor     *ContentColor `json:"text_color,omitempty"`
	Link          *ContentLink  `json:"link,omitempty"`
}

// Cycle 周期
type Cycle struct {
	ID            string       `json:"id"`
	CreateTime    string       `json:"create_time"`
	UpdateTime    string       `json:"update_time"`
	TenantCycleID string       `json:"tenant_cycle_id"`
	Owner         Owner        `json:"owner"`
	StartTime     string       `json:"start_time"`
	EndTime       string       `json:"end_time"`
	CycleStatus   *CycleStatus `json:"cycle_status,omitempty"`
	Score         *float64     `json:"score,omitempty"`
}

// KeyResult 关键结果
type KeyResult struct {
	ID          string        `json:"id"`
	CreateTime  string        `json:"create_time"`
	UpdateTime  string        `json:"update_time"`
	Owner       Owner         `json:"owner"`
	ObjectiveID string        `json:"objective_id"`
	Position    *int32        `json:"position,omitempty"`
	Content     *ContentBlock `json:"content,omitempty"`
	Score       *float64      `json:"score,omitempty"`
	Weight      *float64      `json:"weight,omitempty"`
	Deadline    *string       `json:"deadline,omitempty"`
}

// Objective 目标
type Objective struct {
	ID         string        `json:"id"`
	CreateTime string        `json:"create_time"`
	UpdateTime string        `json:"update_time"`
	Owner      Owner         `json:"owner"`
	CycleID    string        `json:"cycle_id"`
	Position   *int32        `json:"position,omitempty"`
	Content    *ContentBlock `json:"content,omitempty"`
	Score      *float64      `json:"score,omitempty"`
	Notes      *ContentBlock `json:"notes,omitempty"`
	Weight     *float64      `json:"weight,omitempty"`
	Deadline   *string       `json:"deadline,omitempty"`
	CategoryID *string       `json:"category_id,omitempty"`
}

// Owner OKR 所有者
type Owner struct {
	OwnerType OwnerType `json:"owner_type"`
	UserID    *string   `json:"user_id,omitempty"`
}

// ToString CycleStatus to string
func (t CycleStatus) ToString() string {
	switch t {
	case CycleStatusDefault:
		return "default"
	case CycleStatusNormal:
		return "normal"
	case CycleStatusInvalid:
		return "invalid"
	case CycleStatusHidden:
		return "hidden"
	default:
		return ""
	}
}

// formatTimestamp 格式化毫秒级时间戳为 DateTime 格式
func formatTimestamp(ts string) string {
	if ts == "" {
		return ""
	}
	millis, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return ts
	}
	t := time.UnixMilli(millis)
	return t.Format("2006-01-02 15:04:05")
}

// ToResp converts Cycle to RespCycle
func (c *Cycle) ToResp() *RespCycle {
	if c == nil {
		return nil
	}
	resp := &RespCycle{
		ID:            c.ID,
		CreateTime:    formatTimestamp(c.CreateTime),
		UpdateTime:    formatTimestamp(c.UpdateTime),
		TenantCycleID: c.TenantCycleID,
		Owner:         *c.Owner.ToResp(),
		StartTime:     formatTimestamp(c.StartTime),
		EndTime:       formatTimestamp(c.EndTime),
		Score:         c.Score,
	}
	if c.CycleStatus != nil {
		s := c.CycleStatus.ToString()
		resp.CycleStatus = &s
	}
	return resp
}

// ToResp converts KeyResult to RespKeyResult
func (k *KeyResult) ToResp() *RespKeyResult {
	if k == nil {
		return nil
	}
	result := &RespKeyResult{
		ID:          k.ID,
		CreateTime:  formatTimestamp(k.CreateTime),
		UpdateTime:  formatTimestamp(k.UpdateTime),
		Owner:       *k.Owner.ToResp(),
		ObjectiveID: k.ObjectiveID,
		Position:    k.Position,
		Score:       k.Score,
		Weight:      k.Weight,
	}
	if k.Deadline != nil {
		d := formatTimestamp(*k.Deadline)
		result.Deadline = &d
	}
	// Serialize ContentBlock to JSON string (only if Content is not nil and has blocks)
	if k.Content != nil && len(k.Content.Blocks) > 0 {
		if bytes, err := json.Marshal(k.Content); err == nil {
			s := string(bytes)
			result.Content = &s
		}
	}
	return result
}

// ToResp converts Objective to RespObjective
func (o *Objective) ToResp() *RespObjective {
	if o == nil {
		return nil
	}
	result := &RespObjective{
		ID:         o.ID,
		CreateTime: formatTimestamp(o.CreateTime),
		UpdateTime: formatTimestamp(o.UpdateTime),
		Owner:      *o.Owner.ToResp(),
		CycleID:    o.CycleID,
		Position:   o.Position,
		Score:      o.Score,
		Weight:     o.Weight,
		CategoryID: o.CategoryID,
	}
	if o.Deadline != nil {
		d := formatTimestamp(*o.Deadline)
		result.Deadline = &d
	}
	// Serialize Content to JSON string
	if o.Content != nil && len(o.Content.Blocks) > 0 {
		if bytes, err := json.Marshal(o.Content); err == nil {
			s := string(bytes)
			result.Content = &s
		}
	}
	// Serialize Notes to JSON string
	if o.Notes != nil && len(o.Notes.Blocks) > 0 {
		if bytes, err := json.Marshal(o.Notes); err == nil {
			s := string(bytes)
			result.Notes = &s
		}
	}
	return result
}

// ToResp converts Owner to RespOwner
func (o *Owner) ToResp() *RespOwner {
	if o == nil {
		return nil
	}
	return &RespOwner{
		OwnerType: string(o.OwnerType),
		UserID:    o.UserID,
	}
}

// ptrStr dereferences a string pointer, returning "" for nil.
func ptrStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// ptrFloat64 dereferences a float64 pointer, returning 0 for nil.
func ptrFloat64(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
