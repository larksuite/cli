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

type ParagraphElementTypeV1 string

const (
	ParagraphElementTypeV1DocsLink ParagraphElementTypeV1 = "docsLink"
	ParagraphElementTypeV1Mention  ParagraphElementTypeV1 = "person"
	ParagraphElementTypeV1TextRun  ParagraphElementTypeV1 = "textRun"
)

func (t ParagraphElementTypeV1) Ptr() *ParagraphElementTypeV1 { return &t }

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

// ========== ContentBlockV1 (for OKR v1 API ContentBlock) ==========

// ContentBlockV1 是 OKR v1 API 使用的内容块
type ContentBlockV1 struct {
	Blocks []ContentBlockElementV1 `json:"blocks,omitempty"`
}

// ContentBlockElementV1 内容块元素
type ContentBlockElementV1 struct {
	Type      *BlockElementType   `json:"type,omitempty"`
	Paragraph *ContentParagraphV1 `json:"paragraph,omitempty"`
	Gallery   *ContentGalleryV1   `json:"gallery,omitempty"`
}

// ContentGalleryV1 图库
type ContentGalleryV1 struct {
	ImageList []ContentImageItemV1 `json:"imageList,omitempty"`
}

// ContentImageItemV1 图片项
type ContentImageItemV1 struct {
	FileToken *string  `json:"fileToken,omitempty"`
	Src       *string  `json:"src,omitempty"`
	Width     *float64 `json:"width,omitempty"`
	Height    *float64 `json:"height,omitempty"`
}

// ContentParagraphV1 段落
type ContentParagraphV1 struct {
	Style    *ContentParagraphStyleV1    `json:"style,omitempty"`
	Elements []ContentParagraphElementV1 `json:"elements,omitempty"`
}

// ContentParagraphElementV1 段落元素
type ContentParagraphElementV1 struct {
	Type     *ParagraphElementTypeV1 `json:"type,omitempty"`
	TextRun  *ContentTextRunV1       `json:"textRun,omitempty"`
	DocsLink *ContentDocsLink        `json:"docsLink,omitempty"`
	Person   *ContentPersonV1        `json:"person,omitempty"`
}

// ContentParagraphStyleV1 段落样式
type ContentParagraphStyleV1 struct {
	List *ContentListV1 `json:"list,omitempty"`
}

// ContentListV1 列表
type ContentListV1 struct {
	Type        *ListType `json:"type,omitempty"`
	IndentLevel *int32    `json:"indentLevel,omitempty"`
	Number      *int32    `json:"number,omitempty"`
}

// ContentPersonV1 提及的人
type ContentPersonV1 struct {
	OpenID *string `json:"openId,omitempty"`
}

// ContentTextRunV1 文本块
type ContentTextRunV1 struct {
	Text  *string             `json:"text,omitempty"`
	Style *ContentTextStyleV1 `json:"style,omitempty"`
}

// ContentTextStyleV1 文本样式
type ContentTextStyleV1 struct {
	Bold          *bool         `json:"bold,omitempty"`
	StrikeThrough *bool         `json:"strikeThrough,omitempty"`
	BackColor     *ContentColor `json:"backColor,omitempty"`
	TextColor     *ContentColor `json:"textColor,omitempty"`
	Link          *ContentLink  `json:"link,omitempty"`
}

// ToV1 将 ContentBlock 转换为 ContentBlockV1
func (c *ContentBlock) ToV1() *ContentBlockV1 {
	if c == nil {
		return nil
	}
	result := &ContentBlockV1{}
	for _, block := range c.Blocks {
		result.Blocks = append(result.Blocks, block.ToV1())
	}
	return result
}

// ToV1 将 ContentBlockElement 转换为 ContentBlockElementV1
func (e *ContentBlockElement) ToV1() ContentBlockElementV1 {
	return ContentBlockElementV1{
		Type:      e.BlockElementType,
		Paragraph: e.Paragraph.ToV1(),
		Gallery:   e.Gallery.ToV1(),
	}
}

// ToV1 将 ContentGallery 转换为 ContentGalleryV1
func (g *ContentGallery) ToV1() *ContentGalleryV1 {
	if g == nil {
		return nil
	}
	imageList := make([]ContentImageItemV1, 0, len(g.Images))
	for _, img := range g.Images {
		imageList = append(imageList, img.ToV1())
	}
	return &ContentGalleryV1{
		ImageList: imageList,
	}
}

// ToV1 将 ContentImageItem 转换为 ContentImageItemV1
func (i *ContentImageItem) ToV1() ContentImageItemV1 {
	return ContentImageItemV1{
		FileToken: i.FileToken,
		Src:       i.Src,
		Width:     i.Width,
		Height:    i.Height,
	}
}

// ToV1 将 ContentParagraph 转换为 ContentParagraphV1
func (p *ContentParagraph) ToV1() *ContentParagraphV1 {
	if p == nil {
		return nil
	}
	result := &ContentParagraphV1{
		Style: p.Style.ToV1(),
	}
	for _, elem := range p.Elements {
		result.Elements = append(result.Elements, elem.ToV1())
	}
	return result
}

// ToV1 将 ParagraphElementType 转换为 ParagraphElementTypeV1
func (t ParagraphElementType) ToV1() ParagraphElementTypeV1 {
	switch t {
	case ParagraphElementTypeDocsLink:
		return ParagraphElementTypeV1DocsLink
	case ParagraphElementTypeMention:
		return ParagraphElementTypeV1Mention // "person"
	case ParagraphElementTypeTextRun:
		return ParagraphElementTypeV1TextRun
	default:
		return ParagraphElementTypeV1(t)
	}
}

// ToV2 将 ParagraphElementTypeV1 转换为 ParagraphElementType
func (t ParagraphElementTypeV1) ToV2() ParagraphElementType {
	switch t {
	case ParagraphElementTypeV1DocsLink:
		return ParagraphElementTypeDocsLink
	case ParagraphElementTypeV1Mention: // "person"
		return ParagraphElementTypeMention
	case ParagraphElementTypeV1TextRun:
		return ParagraphElementTypeTextRun
	default:
		return ParagraphElementType(t)
	}
}

// ToV1 将 ContentParagraphElement 转换为 ContentParagraphElementV1
func (e *ContentParagraphElement) ToV1() ContentParagraphElementV1 {
	t := ParagraphElementTypeV1TextRun
	if e.ParagraphElementType != nil {
		t = e.ParagraphElementType.ToV1()
	}
	return ContentParagraphElementV1{
		Type:     t.Ptr(),
		TextRun:  e.TextRun.ToV1(),
		DocsLink: e.DocsLink,
		Person:   e.Mention.ToV1(),
	}
}

// ToV1 将 ContentParagraphStyle 转换为 ContentParagraphStyleV1
func (s *ContentParagraphStyle) ToV1() *ContentParagraphStyleV1 {
	if s == nil {
		return nil
	}
	return &ContentParagraphStyleV1{
		List: s.List.ToV1(),
	}
}

// ToV1 将 ContentList 转换为 ContentListV1
func (l *ContentList) ToV1() *ContentListV1 {
	if l == nil {
		return nil
	}
	return &ContentListV1{
		Type:        l.ListType,
		IndentLevel: l.IndentLevel,
		Number:      l.Number,
	}
}

// ToV1 将 ContentTextStyle 转换为 ContentTextStyleV1
func (s *ContentTextStyle) ToV1() *ContentTextStyleV1 {
	if s == nil {
		return nil
	}
	return &ContentTextStyleV1{
		Bold:          s.Bold,
		StrikeThrough: s.StrikeThrough,
		BackColor:     s.BackColor,
		TextColor:     s.TextColor,
		Link:          s.Link,
	}
}

// ToV1 将 ContentTextRun 转换为 ContentTextRunV1
func (t *ContentTextRun) ToV1() *ContentTextRunV1 {
	if t == nil {
		return nil
	}
	return &ContentTextRunV1{
		Text:  t.Text,
		Style: t.Style.ToV1(),
	}
}

// ToV1 将 ContentMention 转换为 ContentPersonV1
func (m *ContentMention) ToV1() *ContentPersonV1 {
	if m == nil {
		return nil
	}
	return &ContentPersonV1{
		OpenID: m.UserID,
	}
}

// ========== ContentBlockV1 转 ContentBlock ==========

// ToV2 将 ContentBlockV1 转换为 ContentBlock
func (c *ContentBlockV1) ToV2() *ContentBlock {
	if c == nil {
		return nil
	}
	result := &ContentBlock{}
	for _, block := range c.Blocks {
		result.Blocks = append(result.Blocks, block.ToV2())
	}
	return result
}

// ToV2 将 ContentBlockElementV1 转换为 ContentBlockElement
func (e *ContentBlockElementV1) ToV2() ContentBlockElement {
	return ContentBlockElement{
		BlockElementType: e.Type,
		Paragraph:        e.Paragraph.ToV2(),
		Gallery:          e.Gallery.ToV2(),
	}
}

// ToV2 将 ContentGalleryV1 转换为 ContentGallery
func (g *ContentGalleryV1) ToV2() *ContentGallery {
	if g == nil {
		return nil
	}
	images := make([]ContentImageItem, 0, len(g.ImageList))
	for _, img := range g.ImageList {
		images = append(images, img.ToV2())
	}
	return &ContentGallery{
		Images: images,
	}
}

// ToV2 将 ContentImageItemV1 转换为 ContentImageItem
func (i *ContentImageItemV1) ToV2() ContentImageItem {
	return ContentImageItem{
		FileToken: i.FileToken,
		Src:       i.Src,
		Width:     i.Width,
		Height:    i.Height,
	}
}

// ToV2 将 ContentParagraphV1 转换为 ContentParagraph
func (p *ContentParagraphV1) ToV2() *ContentParagraph {
	if p == nil {
		return nil
	}
	result := &ContentParagraph{
		Style: p.Style.ToV2(),
	}
	for _, elem := range p.Elements {
		result.Elements = append(result.Elements, elem.ToV2())
	}
	return result
}

// ToV2 将 ContentParagraphElementV1 转换为 ContentParagraphElement
func (e *ContentParagraphElementV1) ToV2() ContentParagraphElement {
	t := ParagraphElementTypeTextRun
	if e.Type != nil {
		t = e.Type.ToV2()
	}
	return ContentParagraphElement{
		ParagraphElementType: t.Ptr(),
		TextRun:              e.TextRun.ToV2(),
		DocsLink:             e.DocsLink,
		Mention:              e.Person.ToV2(),
	}
}

// ToV2 将 ContentParagraphStyleV1 转换为 ContentParagraphStyle
func (s *ContentParagraphStyleV1) ToV2() *ContentParagraphStyle {
	if s == nil {
		return nil
	}
	return &ContentParagraphStyle{
		List: s.List.ToV2(),
	}
}

// ToV2 将 ContentListV1 转换为 ContentList
func (l *ContentListV1) ToV2() *ContentList {
	if l == nil {
		return nil
	}
	return &ContentList{
		ListType:    l.Type,
		IndentLevel: l.IndentLevel,
		Number:      l.Number,
	}
}

// ToV2 将 ContentTextStyleV1 转换为 ContentTextStyle
func (s *ContentTextStyleV1) ToV2() *ContentTextStyle {
	if s == nil {
		return nil
	}
	return &ContentTextStyle{
		Bold:          s.Bold,
		StrikeThrough: s.StrikeThrough,
		BackColor:     s.BackColor,
		TextColor:     s.TextColor,
		Link:          s.Link,
	}
}

// ToV2 将 ContentTextRunV1 转换为 ContentTextRun
func (t *ContentTextRunV1) ToV2() *ContentTextRun {
	if t == nil {
		return nil
	}
	return &ContentTextRun{
		Text:  t.Text,
		Style: t.Style.ToV2(),
	}
}

// ToV2 将 ContentPersonV1 转换为 ContentMention
func (p *ContentPersonV1) ToV2() *ContentMention {
	if p == nil {
		return nil
	}
	return &ContentMention{
		UserID: p.OpenID,
	}
}

// ProgressRateV1 进度率
type ProgressRateV1 struct {
	Percent *float64 `json:"percent,omitempty"`
	Status  *int32   `json:"status,omitempty"`
}

// ProgressV1 进展记录
type ProgressV1 struct {
	ID           string          `json:"progress_id"`
	ModifyTime   string          `json:"modify_time"`
	Content      *ContentBlockV1 `json:"content,omitempty"`
	ProgressRate *ProgressRateV1 `json:"progress_rate,omitempty"`
}

// ToResp converts ProgressV1 to RespProgress
func (p *ProgressV1) ToResp() *RespProgress {
	if p == nil {
		return nil
	}
	resp := &RespProgress{
		ID:         p.ID,
		ModifyTime: formatTimestamp(p.ModifyTime),
	}
	if p.ProgressRate != nil {
		resp.ProgressRate = &RespProgressRate{
			Percent: p.ProgressRate.Percent,
		}
		if p.ProgressRate.Status != nil {
			s := ProgressStatus(*p.ProgressRate.Status).String()
			if s != "" {
				resp.ProgressRate.Status = &s
			}
		}
	}
	// Convert ContentBlockV1 to ContentBlock, then serialize to JSON string
	if p.Content != nil && len(p.Content.Blocks) > 0 {
		if v2 := p.Content.ToV2(); v2 != nil && len(v2.Blocks) > 0 {
			if bytes, err := json.Marshal(v2); err == nil {
				s := string(bytes)
				resp.Content = &s
			}
		}
	}
	return resp
}

// int32Ptr returns a pointer to the given int32 value.
func int32Ptr(v int32) *int32 { return &v }

// ========== Progress (for OKR v2 API ListOkrObjectiveProgress/ListOkrKeyResultProgress) ==========

// ProgressRate 进度率（v2 API）
type ProgressRate struct {
	ProgressPercent *float64 `json:"progress_percent,omitempty"`
	ProgressStatus  *int32   `json:"progress_status,omitempty"`
}

// Progress 进展记录（v2 API）
type Progress struct {
	ID           string        `json:"id"`
	CreateTime   string        `json:"create_time"`
	UpdateTime   string        `json:"update_time"`
	Owner        Owner         `json:"owner"`
	EntityType   *int32        `json:"entity_type,omitempty"`
	EntityID     string        `json:"entity_id"`
	Content      *ContentBlock `json:"content,omitempty"`
	ProgressRate *ProgressRate `json:"progress_rate,omitempty"`
}

// ToResp converts Progress to RespProgress
func (p *Progress) ToResp() *RespProgress {
	if p == nil {
		return nil
	}
	cteateTime := formatTimestamp(p.CreateTime)
	resp := &RespProgress{
		ID:         p.ID,
		ModifyTime: formatTimestamp(p.UpdateTime), // Use UpdateTime as ModifyTime
		CreateTime: &cteateTime,
	}
	if p.ProgressRate != nil {
		resp.ProgressRate = &RespProgressRate{
			Percent: p.ProgressRate.ProgressPercent,
		}
		if p.ProgressRate.ProgressStatus != nil {
			s := ProgressStatus(*p.ProgressRate.ProgressStatus).String()
			if s != "" {
				resp.ProgressRate.Status = &s
			}
		}
	}
	// Serialize ContentBlock to JSON string
	if p.Content != nil && len(p.Content.Blocks) > 0 {
		if bytes, err := json.Marshal(p.Content); err == nil {
			s := string(bytes)
			resp.Content = &s
		}
	}
	return resp
}
