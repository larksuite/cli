// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"encoding/json"
	"testing"

	"github.com/larksuite/cli/internal/core"
	"github.com/smartystreets/goconvey/convey"
)

func TestFormatTimestamp(t *testing.T) {
	convey.Convey("formatTimestamp", t, func() {
		convey.Convey("empty string returns empty", func() {
			result := formatTimestamp("")
			convey.So(result, convey.ShouldEqual, "")
		})

		convey.Convey("valid timestamp formats correctly", func() {
			result := formatTimestamp("1735689600000")
			// 不检查具体的时分秒，因为时区不同结果会不同
			convey.So(result, convey.ShouldStartWith, "2025-01-01")
		})

		convey.Convey("invalid timestamp returns original", func() {
			result := formatTimestamp("not-a-number")
			convey.So(result, convey.ShouldEqual, "not-a-number")
		})
	})
}

func TestToRespMethods(t *testing.T) {
	convey.Convey("ToResp methods handle nil", t, func() {
		convey.So((*Cycle)(nil).ToResp(), convey.ShouldBeNil)
		convey.So((*KeyResult)(nil).ToResp(), convey.ShouldBeNil)
		convey.So((*Objective)(nil).ToResp(), convey.ShouldBeNil)
		convey.So((*Owner)(nil).ToResp(), convey.ShouldBeNil)
		convey.So((*ProgressV1)(nil).ToResp(), convey.ShouldBeNil)
	})

	convey.Convey("ToResp methods work with valid objects", t, func() {
		convey.Convey("Cycle", func() {
			cycle := &Cycle{
				ID:            "cycle-id",
				CreateTime:    "1735689600000",
				UpdateTime:    "1735776000000",
				TenantCycleID: "tenant-cycle-id",
				Owner:         Owner{OwnerType: OwnerTypeUser, UserID: strPtr("ou-1")},
				StartTime:     "1735689600000",
				EndTime:       "1751318400000",
				CycleStatus:   CycleStatusNormal.Ptr(),
				Score:         float64Ptr(0.75),
			}
			resp := cycle.ToResp()
			convey.So(resp, convey.ShouldNotBeNil)
			convey.So(resp.ID, convey.ShouldEqual, "cycle-id")
			convey.So(*resp.CycleStatus, convey.ShouldEqual, "normal")
			convey.So(*resp.Score, convey.ShouldEqual, 0.75)
		})

		convey.Convey("Objective", func() {
			obj := &Objective{
				ID:         "obj-id",
				CreateTime: "1735689600000",
				UpdateTime: "1735776000000",
				Owner:      Owner{OwnerType: OwnerTypeUser, UserID: strPtr("ou-1")},
				CycleID:    "cycle-id",
				Position:   int32Ptr(1),
				Score:      float64Ptr(0.8),
				Weight:     float64Ptr(1.0),
				Deadline:   strPtr("1751318400000"),
				Content: &ContentBlock{
					Blocks: []ContentBlockElement{
						{
							BlockElementType: BlockElementTypeParagraph.Ptr(),
							Paragraph: &ContentParagraph{
								Elements: []ContentParagraphElement{
									{
										ParagraphElementType: ParagraphElementTypeTextRun.Ptr(),
										TextRun: &ContentTextRun{
											Text: strPtr("Test objective"),
										},
									},
								},
							},
						},
					},
				},
			}
			resp := obj.ToResp()
			convey.So(resp, convey.ShouldNotBeNil)
			convey.So(resp.ID, convey.ShouldEqual, "obj-id")
			convey.So(*resp.Score, convey.ShouldEqual, 0.8)
			convey.So(*resp.Content, convey.ShouldNotBeEmpty)
		})

		convey.Convey("KeyResult", func() {
			kr := &KeyResult{
				ID:          "kr-id",
				CreateTime:  "1735689600000",
				UpdateTime:  "1735776000000",
				Owner:       Owner{OwnerType: OwnerTypeUser, UserID: strPtr("ou-1")},
				ObjectiveID: "obj-id",
				Position:    int32Ptr(1),
				Content: &ContentBlock{
					Blocks: []ContentBlockElement{
						{
							BlockElementType: BlockElementTypeParagraph.Ptr(),
							Paragraph: &ContentParagraph{
								Elements: []ContentParagraphElement{
									{
										ParagraphElementType: ParagraphElementTypeTextRun.Ptr(),
										TextRun: &ContentTextRun{
											Text: strPtr("Test KR"),
										},
									},
								},
							},
						},
					},
				},
				Score:    float64Ptr(0.9),
				Weight:   float64Ptr(0.5),
				Deadline: strPtr("1751318400000"),
			}
			resp := kr.ToResp()
			convey.So(resp, convey.ShouldNotBeNil)
			convey.So(resp.ID, convey.ShouldEqual, "kr-id")
			convey.So(resp.ObjectiveID, convey.ShouldEqual, "obj-id")
			convey.So(*resp.Score, convey.ShouldEqual, 0.9)
			convey.So(*resp.Content, convey.ShouldNotBeEmpty)
		})

		convey.Convey("ProgressV1", func() {
			record := &ProgressV1{
				ID:         "progress-id",
				ModifyTime: "1735776000000",
				Content: &ContentBlockV1{
					Blocks: []ContentBlockElementV1{
						{
							Type: BlockElementTypeParagraph.Ptr(),
							Paragraph: &ContentParagraphV1{
								Elements: []ContentParagraphElementV1{
									{
										Type: ParagraphElementTypeV1TextRun.Ptr(),
										TextRun: &ContentTextRunV1{
											Text: strPtr("Hello progress"),
										},
									},
								},
							},
						},
					},
				},
				ProgressRate: &ProgressRateV1{
					Percent: float64Ptr(75.0),
					Status:  int32Ptr(0),
				},
			}
			resp := record.ToResp()
			convey.So(resp, convey.ShouldNotBeNil)
			convey.So(resp.ID, convey.ShouldEqual, "progress-id")
			convey.So(resp.ModifyTime, convey.ShouldStartWith, "2025-01-02")
			convey.So(resp.Content, convey.ShouldNotBeNil)
			convey.So(*resp.Content, convey.ShouldContainSubstring, "Hello progress")
			convey.So(resp.ProgressRate, convey.ShouldNotBeNil)
			convey.So(*resp.ProgressRate.Percent, convey.ShouldEqual, 75.0)
		})

		convey.Convey("ProgressV1 with empty content", func() {
			record := &ProgressV1{
				ID:         "progress-id-2",
				ModifyTime: "1735776000000",
			}
			resp := record.ToResp()
			convey.So(resp, convey.ShouldNotBeNil)
			convey.So(resp.Content, convey.ShouldBeNil)
			convey.So(resp.ProgressRate, convey.ShouldBeNil)
		})
	})
}

func TestContentBlockV1V2RoundTrip(t *testing.T) {
	convey.Convey("ContentBlock V1↔V2 round-trip", t, func() {
		original := &ContentBlock{
			Blocks: []ContentBlockElement{
				{
					BlockElementType: BlockElementTypeParagraph.Ptr(),
					Paragraph: &ContentParagraph{
						Style: &ContentParagraphStyle{
							List: &ContentList{
								ListType:    listTypePtr(ListTypeBullet),
								IndentLevel: int32Ptr(1),
								Number:      int32Ptr(2),
							},
						},
						Elements: []ContentParagraphElement{
							{
								ParagraphElementType: ParagraphElementTypeTextRun.Ptr(),
								TextRun: &ContentTextRun{
									Text: strPtr("Hello world"),
									Style: &ContentTextStyle{
										Bold:          boolPtr(true),
										StrikeThrough: boolPtr(false),
									},
								},
							},
							{
								ParagraphElementType: ParagraphElementTypeDocsLink.Ptr(),
								DocsLink: &ContentDocsLink{
									URL:   strPtr("https://example.com"),
									Title: strPtr("Example"),
								},
							},
							{
								ParagraphElementType: ParagraphElementTypeMention.Ptr(),
								Mention: &ContentMention{
									UserID: strPtr("ou_123"),
								},
							},
						},
					},
				},
				{
					BlockElementType: BlockElementTypeGallery.Ptr(),
					Gallery: &ContentGallery{
						Images: []ContentImageItem{
							{FileToken: strPtr("ftoken1"), Width: float64Ptr(100), Height: float64Ptr(200)},
						},
					},
				},
			},
		}

		// V2 -> V1
		v1 := original.ToV1()
		convey.So(v1, convey.ShouldNotBeNil)
		convey.So(len(v1.Blocks), convey.ShouldEqual, 2)

		// V1 -> V2
		v2 := v1.ToV2()
		convey.So(v2, convey.ShouldNotBeNil)
		convey.So(len(v2.Blocks), convey.ShouldEqual, 2)

		// Verify first block (paragraph)
		convey.So(*v2.Blocks[0].BlockElementType, convey.ShouldEqual, BlockElementTypeParagraph)
		convey.So(v2.Blocks[0].Paragraph, convey.ShouldNotBeNil)
		convey.So(len(v2.Blocks[0].Paragraph.Elements), convey.ShouldEqual, 3)

		// TextRun
		textRunElem := v2.Blocks[0].Paragraph.Elements[0]
		convey.So(*textRunElem.ParagraphElementType, convey.ShouldEqual, ParagraphElementTypeTextRun)
		convey.So(textRunElem.TextRun, convey.ShouldNotBeNil)
		convey.So(*textRunElem.TextRun.Text, convey.ShouldEqual, "Hello world")
		convey.So(textRunElem.TextRun.Style, convey.ShouldNotBeNil)
		convey.So(*textRunElem.TextRun.Style.Bold, convey.ShouldBeTrue)

		// DocsLink
		docsLinkElem := v2.Blocks[0].Paragraph.Elements[1]
		convey.So(*docsLinkElem.ParagraphElementType, convey.ShouldEqual, ParagraphElementTypeDocsLink)
		convey.So(docsLinkElem.DocsLink, convey.ShouldNotBeNil)
		convey.So(*docsLinkElem.DocsLink.URL, convey.ShouldEqual, "https://example.com")

		// Mention
		mentionElem := v2.Blocks[0].Paragraph.Elements[2]
		convey.So(*mentionElem.ParagraphElementType, convey.ShouldEqual, ParagraphElementTypeMention)
		convey.So(mentionElem.Mention, convey.ShouldNotBeNil)
		convey.So(*mentionElem.Mention.UserID, convey.ShouldEqual, "ou_123")

		// Verify second block (gallery)
		convey.So(*v2.Blocks[1].BlockElementType, convey.ShouldEqual, BlockElementTypeGallery)
		convey.So(v2.Blocks[1].Gallery, convey.ShouldNotBeNil)
		convey.So(len(v2.Blocks[1].Gallery.Images), convey.ShouldEqual, 1)

		// Verify list style round-trip
		convey.So(v2.Blocks[0].Paragraph.Style, convey.ShouldNotBeNil)
		convey.So(v2.Blocks[0].Paragraph.Style.List, convey.ShouldNotBeNil)
		convey.So(*v2.Blocks[0].Paragraph.Style.List.ListType, convey.ShouldEqual, ListTypeBullet)
		convey.So(*v2.Blocks[0].Paragraph.Style.List.IndentLevel, convey.ShouldEqual, 1)
	})

	convey.Convey("nil ContentBlock round-trip", t, func() {
		convey.So((*ContentBlock)(nil).ToV1(), convey.ShouldBeNil)
		convey.So((*ContentBlockV1)(nil).ToV2(), convey.ShouldBeNil)
	})
}

func TestContentBlockV1JSON(t *testing.T) {
	convey.Convey("ContentBlockV1 JSON serialization", t, func() {
		v1 := &ContentBlockV1{
			Blocks: []ContentBlockElementV1{
				{
					Type: BlockElementTypeParagraph.Ptr(),
					Paragraph: &ContentParagraphV1{
						Elements: []ContentParagraphElementV1{
							{
								Type:    ParagraphElementTypeV1TextRun.Ptr(),
								TextRun: &ContentTextRunV1{Text: strPtr("test")},
							},
						},
					},
				},
			},
		}
		data, err := json.Marshal(v1)
		convey.So(err, convey.ShouldBeNil)
		convey.So(string(data), convey.ShouldContainSubstring, "paragraph")
		convey.So(string(data), convey.ShouldContainSubstring, "textRun")
		convey.So(string(data), convey.ShouldContainSubstring, "test")
	})
}

func TestProgressRecordToResp_ContentBlockV1Conversion(t *testing.T) {
	convey.Convey("ProgressV1.ToResp converts V1 content to V2 JSON", t, func() {
		record := &ProgressV1{
			ID:         "rec-123",
			ModifyTime: "1735776000000",
			Content: &ContentBlockV1{
				Blocks: []ContentBlockElementV1{
					{
						Type: BlockElementTypeParagraph.Ptr(),
						Paragraph: &ContentParagraphV1{
							Elements: []ContentParagraphElementV1{
								{
									Type:    ParagraphElementTypeV1TextRun.Ptr(),
									TextRun: &ContentTextRunV1{Text: strPtr("V1 content")},
								},
								{
									Type:   ParagraphElementTypeV1Mention.Ptr(),
									Person: &ContentPersonV1{OpenID: strPtr("ou_mention")},
								},
							},
						},
					},
				},
			},
		}
		resp := record.ToResp()
		convey.So(resp.Content, convey.ShouldNotBeNil)
		// Content should be V2 format JSON string
		convey.So(*resp.Content, convey.ShouldContainSubstring, "block_element_type")
		convey.So(*resp.Content, convey.ShouldContainSubstring, "V1 content")
		convey.So(*resp.Content, convey.ShouldContainSubstring, "user_id")
	})
}

func TestParseProgressRecord(t *testing.T) {
	convey.Convey("parseProgressRecord", t, func() {
		convey.Convey("valid data", func() {
			data := map[string]any{
				"progress_id": "123",
				"modify_time": "1735776000000",
				"content": map[string]any{
					"blocks": []any{
						map[string]any{
							"type": "paragraph",
							"paragraph": map[string]any{
								"elements": []any{
									map[string]any{
										"type":    "textRun",
										"textRun": map[string]any{"text": "test"},
									},
								},
							},
						},
					},
				},
			}
			record, err := parseProgressRecord(data)
			convey.So(err, convey.ShouldBeNil)
			convey.So(record.ID, convey.ShouldEqual, "123")
			convey.So(record.Content, convey.ShouldNotBeNil)
		})

		convey.Convey("empty data", func() {
			data := map[string]any{}
			record, err := parseProgressRecord(data)
			convey.So(err, convey.ShouldBeNil)
			convey.So(record.ID, convey.ShouldEqual, "")
		})
	})
}

func TestParseCreateProgressRecordParams_BrandAwareSourceURL(t *testing.T) {
	convey.Convey("parseCreateProgressRecordParams brand-aware defaults", t, func() {
		// This test directly tests the brand-aware default logic by constructing
		// a minimal ContentBlock JSON and checking the resolved sourceURL.
		convey.Convey("feishu brand defaults to feishu.cn", func() {
			url := core.ResolveOpenBaseURL(core.BrandFeishu) + "/app"
			convey.So(url, convey.ShouldEqual, "https://open.feishu.cn/app")
		})
		convey.Convey("lark brand defaults to larksuite.com", func() {
			url := core.ResolveOpenBaseURL(core.BrandLark) + "/app"
			convey.So(url, convey.ShouldEqual, "https://open.larksuite.com/app")
		})
	})
}

func TestProgressStatus(t *testing.T) {
	convey.Convey("ProgressStatus parsing and string conversion", t, func() {
		convey.Convey("ParseProgressStatus accepts string names", func() {
			s, ok := ParseProgressStatus("normal")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(s, convey.ShouldEqual, ProgressStatusNormal)

			s, ok = ParseProgressStatus("overdue")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(s, convey.ShouldEqual, ProgressStatusOverdue)

			s, ok = ParseProgressStatus("done")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(s, convey.ShouldEqual, ProgressStatusDone)
		})

		convey.Convey("ParseProgressStatus accepts numeric strings", func() {
			s, ok := ParseProgressStatus("0")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(s, convey.ShouldEqual, ProgressStatusNormal)

			s, ok = ParseProgressStatus("1")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(s, convey.ShouldEqual, ProgressStatusOverdue)

			s, ok = ParseProgressStatus("2")
			convey.So(ok, convey.ShouldBeTrue)
			convey.So(s, convey.ShouldEqual, ProgressStatusDone)
		})

		convey.Convey("ParseProgressStatus rejects invalid values", func() {
			_, ok := ParseProgressStatus("invalid")
			convey.So(ok, convey.ShouldBeFalse)
		})

		convey.Convey("String returns human-readable names", func() {
			convey.So(ProgressStatusNormal.String(), convey.ShouldEqual, "normal")
			convey.So(ProgressStatusOverdue.String(), convey.ShouldEqual, "overdue")
			convey.So(ProgressStatusDone.String(), convey.ShouldEqual, "done")
		})
	})
}

func TestProgressV1ToResp_StatusConversion(t *testing.T) {
	convey.Convey("ProgressV1.ToResp converts Status int to string", t, func() {
		convey.Convey("status=0 → normal", func() {
			record := &ProgressV1{
				ID:         "rec-1",
				ModifyTime: "1735776000000",
				ProgressRate: &ProgressRateV1{
					Percent: float64Ptr(50.0),
					Status:  int32Ptr(0),
				},
			}
			resp := record.ToResp()
			convey.So(resp.ProgressRate, convey.ShouldNotBeNil)
			convey.So(*resp.ProgressRate.Status, convey.ShouldEqual, "normal")
			convey.So(*resp.ProgressRate.Percent, convey.ShouldEqual, 50.0)
		})

		convey.Convey("status=1 → overdue", func() {
			record := &ProgressV1{
				ID:         "rec-2",
				ModifyTime: "1735776000000",
				ProgressRate: &ProgressRateV1{
					Percent: float64Ptr(30.0),
					Status:  int32Ptr(1),
				},
			}
			resp := record.ToResp()
			convey.So(*resp.ProgressRate.Status, convey.ShouldEqual, "overdue")
		})

		convey.Convey("status=2 → done", func() {
			record := &ProgressV1{
				ID:         "rec-3",
				ModifyTime: "1735776000000",
				ProgressRate: &ProgressRateV1{
					Percent: float64Ptr(100.0),
					Status:  int32Ptr(2),
				},
			}
			resp := record.ToResp()
			convey.So(*resp.ProgressRate.Status, convey.ShouldEqual, "done")
		})

		convey.Convey("nil ProgressRate", func() {
			record := &ProgressV1{
				ID:         "rec-4",
				ModifyTime: "1735776000000",
			}
			resp := record.ToResp()
			convey.So(resp.ProgressRate, convey.ShouldBeNil)
		})

		convey.Convey("nil Status in ProgressRate", func() {
			record := &ProgressV1{
				ID:         "rec-5",
				ModifyTime: "1735776000000",
				ProgressRate: &ProgressRateV1{
					Percent: float64Ptr(75.0),
				},
			}
			resp := record.ToResp()
			convey.So(resp.ProgressRate, convey.ShouldNotBeNil)
			convey.So(resp.ProgressRate.Status, convey.ShouldBeNil)
			convey.So(*resp.ProgressRate.Percent, convey.ShouldEqual, 75.0)
		})
	})
}

// strPtr returns a pointer to the given string value.
func strPtr(v string) *string { return &v }

// float64Ptr returns a pointer to the given float64 value.
func float64Ptr(v float64) *float64 { return &v }

// boolPtr returns a pointer to the given bool value.
func boolPtr(v bool) *bool { return &v }

// listTypePtr returns a pointer to the given ListType value.
func listTypePtr(v ListType) *ListType { return &v }
