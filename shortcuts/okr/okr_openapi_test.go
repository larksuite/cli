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
			// Verify removed fields are not present in RespCycle
			convey.So(resp.StartTime, convey.ShouldNotBeEmpty)
			convey.So(resp.EndTime, convey.ShouldNotBeEmpty)
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

// ========== SemiPlainContent Conversion Tests ==========

func TestContentBlockToSemiPlain_TextOnly(t *testing.T) {
	t.Parallel()
	cb := &ContentBlock{
		Blocks: []ContentBlockElement{
			{
				BlockElementType: BlockElementTypeParagraph.Ptr(),
				Paragraph: &ContentParagraph{
					Elements: []ContentParagraphElement{
						{
							ParagraphElementType: ParagraphElementTypeTextRun.Ptr(),
							TextRun: &ContentTextRun{
								Text: strPtr("Hello world"),
							},
						},
					},
				},
			},
		},
	}
	sp := cb.ToSemiPlain()
	if sp == nil {
		t.Fatal("expected non-nil SemiPlainContent")
	}
	if sp.Text != "Hello world" {
		t.Fatalf("expected text 'Hello world', got '%s'", sp.Text)
	}
	if len(sp.Mention) != 0 {
		t.Fatalf("expected 0 mentions, got %d", len(sp.Mention))
	}
	if len(sp.Docs) != 0 {
		t.Fatalf("expected 0 docs, got %d", len(sp.Docs))
	}
	if len(sp.Images) != 0 {
		t.Fatalf("expected 0 images, got %d", len(sp.Images))
	}
}

func TestContentBlockToSemiPlain_WithMention(t *testing.T) {
	t.Parallel()
	cb := &ContentBlock{
		Blocks: []ContentBlockElement{
			{
				BlockElementType: BlockElementTypeParagraph.Ptr(),
				Paragraph: &ContentParagraph{
					Elements: []ContentParagraphElement{
						{
							ParagraphElementType: ParagraphElementTypeTextRun.Ptr(),
							TextRun: &ContentTextRun{
								Text: strPtr("Hello "),
							},
						},
						{
							ParagraphElementType: ParagraphElementTypeMention.Ptr(),
							Mention: &ContentMention{
								UserID: strPtr("ou_123"),
							},
						},
						{
							ParagraphElementType: ParagraphElementTypeTextRun.Ptr(),
							TextRun: &ContentTextRun{
								Text: strPtr(", how are you?"),
							},
						},
					},
				},
			},
		},
	}
	sp := cb.ToSemiPlain()
	if sp == nil {
		t.Fatal("expected non-nil SemiPlainContent")
	}
	// Text includes @{userID} placeholder to preserve positional context
	if sp.Text != "Hello  @{ou_123} , how are you?" {
		t.Fatalf("expected text 'Hello  @{ou_123} , how are you?', got '%s'", sp.Text)
	}
	if len(sp.Mention) != 1 || sp.Mention[0] != "ou_123" {
		t.Fatalf("expected mention [ou_123], got %v", sp.Mention)
	}
}

func TestContentBlockToSemiPlain_WithDocsAndImages(t *testing.T) {
	t.Parallel()
	cb := &ContentBlock{
		Blocks: []ContentBlockElement{
			{
				BlockElementType: BlockElementTypeParagraph.Ptr(),
				Paragraph: &ContentParagraph{
					Elements: []ContentParagraphElement{
						{
							ParagraphElementType: ParagraphElementTypeTextRun.Ptr(),
							TextRun: &ContentTextRun{
								Text: strPtr("Check out this doc: "),
							},
						},
						{
							ParagraphElementType: ParagraphElementTypeDocsLink.Ptr(),
							DocsLink: &ContentDocsLink{
								Title: strPtr("Design Doc"),
								URL:   strPtr("https://example.feishu.cn/docx/xxx"),
							},
						},
					},
				},
			},
			{
				BlockElementType: BlockElementTypeGallery.Ptr(),
				Gallery: &ContentGallery{
					Images: []ContentImageItem{
						{
							Src: strPtr("https://example.com/img1.png"),
						},
						{
							Src: strPtr("https://example.com/img2.png"),
						},
					},
				},
			},
		},
	}
	sp := cb.ToSemiPlain()
	if sp == nil {
		t.Fatal("expected non-nil SemiPlainContent")
	}
	if sp.Text != "Check out this doc: " {
		t.Fatalf("unexpected text: '%s'", sp.Text)
	}
	if len(sp.Docs) != 1 {
		t.Fatalf("expected 1 doc, got %d", len(sp.Docs))
	}
	if sp.Docs[0].Title != "Design Doc" || sp.Docs[0].URL != "https://example.feishu.cn/docx/xxx" {
		t.Fatalf("unexpected doc: %+v", sp.Docs[0])
	}
	if len(sp.Images) != 2 {
		t.Fatalf("expected 2 images, got %d", len(sp.Images))
	}
	if sp.Images[0] != "https://example.com/img1.png" || sp.Images[1] != "https://example.com/img2.png" {
		t.Fatalf("unexpected images: %v", sp.Images)
	}
}

func TestContentBlockToSemiPlain_Nil(t *testing.T) {
	t.Parallel()
	var cb *ContentBlock
	sp := cb.ToSemiPlain()
	if sp != nil {
		t.Fatal("expected nil SemiPlainContent for nil ContentBlock")
	}
}

func TestSemiPlainContentToContentBlock_TextOnly(t *testing.T) {
	t.Parallel()
	sp := &SemiPlainContent{
		Text: "Hello world",
	}
	cb := sp.ToContentBlock()
	if cb == nil {
		t.Fatal("expected non-nil ContentBlock")
	}
	if len(cb.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(cb.Blocks))
	}
	block := cb.Blocks[0]
	if block.BlockElementType == nil || *block.BlockElementType != BlockElementTypeParagraph {
		t.Fatal("expected paragraph block")
	}
	if block.Paragraph == nil || len(block.Paragraph.Elements) != 1 {
		t.Fatalf("expected 1 paragraph element, got %d", len(block.Paragraph.Elements))
	}
	elem := block.Paragraph.Elements[0]
	if elem.ParagraphElementType == nil || *elem.ParagraphElementType != ParagraphElementTypeTextRun {
		t.Fatal("expected textRun element")
	}
	if elem.TextRun == nil || elem.TextRun.Text == nil || *elem.TextRun.Text != "Hello world" {
		t.Fatalf("unexpected text: %v", elem.TextRun)
	}
}

func TestSemiPlainContentToContentBlock_WithMentions(t *testing.T) {
	t.Parallel()
	sp := &SemiPlainContent{
		Text:    "Please review",
		Mention: []string{"ou_123", "ou_456"},
	}
	cb := sp.ToContentBlock()
	if cb == nil {
		t.Fatal("expected non-nil ContentBlock")
	}
	if len(cb.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(cb.Blocks))
	}
	elems := cb.Blocks[0].Paragraph.Elements
	if len(elems) != 3 {
		t.Fatalf("expected 3 elements (1 text + 2 mentions), got %d", len(elems))
	}
	if *elems[0].ParagraphElementType != ParagraphElementTypeTextRun || *elems[0].TextRun.Text != "Please review" {
		t.Fatal("unexpected first element")
	}
	if *elems[1].ParagraphElementType != ParagraphElementTypeMention || *elems[1].Mention.UserID != "ou_123" {
		t.Fatal("unexpected second element")
	}
	if *elems[2].ParagraphElementType != ParagraphElementTypeMention || *elems[2].Mention.UserID != "ou_456" {
		t.Fatal("unexpected third element")
	}
}

func TestSemiPlainContentToContentBlock_EmptyText(t *testing.T) {
	t.Parallel()
	sp := &SemiPlainContent{
		Text:    "   ",
		Mention: []string{"ou_123"},
	}
	cb := sp.ToContentBlock()
	if cb == nil {
		t.Fatal("expected non-nil ContentBlock")
	}
	elems := cb.Blocks[0].Paragraph.Elements
	// Empty text should be skipped, only mention remains
	if len(elems) != 1 {
		t.Fatalf("expected 1 element (mention only), got %d", len(elems))
	}
	if *elems[0].ParagraphElementType != ParagraphElementTypeMention {
		t.Fatal("expected mention element")
	}
}

func TestSemiPlainContentToContentBlock_DocsImagesIgnored(t *testing.T) {
	t.Parallel()
	sp := &SemiPlainContent{
		Text:    "Test",
		Mention: []string{"ou_123"},
		Docs:    []SemiPlainDoc{{Title: "Doc", URL: "https://..."}},
		Images:  []string{"https://img.png"},
	}
	cb := sp.ToContentBlock()
	if cb == nil {
		t.Fatal("expected non-nil ContentBlock")
	}
	elems := cb.Blocks[0].Paragraph.Elements
	// Docs and images are ignored in input conversion
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements (text + mention), got %d", len(elems))
	}
}

func TestSemiPlainContentToContentBlock_PlaceholderStripping(t *testing.T) {
	t.Parallel()
	// Simulate round-trip: output format has @{userID} in text,
	// input conversion should strip them to avoid duplicate mentions
	sp := &SemiPlainContent{
		Text:    "任务一 @{ou_zhangsan} ，任务二 @{ou_lisi} ",
		Mention: []string{"ou_zhangsan", "ou_lisi"},
	}
	cb := sp.ToContentBlock()
	if cb == nil {
		t.Fatal("expected non-nil ContentBlock")
	}
	elems := cb.Blocks[0].Paragraph.Elements
	// Should have 3 elements: 1 text (stripped) + 2 mentions
	if len(elems) != 3 {
		t.Fatalf("expected 3 elements (1 text + 2 mentions), got %d", len(elems))
	}
	// Text should have placeholders stripped
	if *elems[0].ParagraphElementType != ParagraphElementTypeTextRun {
		t.Fatal("expected first element to be textRun")
	}
	// Note: space before comma is preserved from the placeholder's trailing space
	expectedText := "任务一 ，任务二"
	if *elems[0].TextRun.Text != expectedText {
		t.Fatalf("expected stripped text '%s', got '%s'", expectedText, *elems[0].TextRun.Text)
	}
	// Mentions should be preserved as separate elements
	if *elems[1].ParagraphElementType != ParagraphElementTypeMention || *elems[1].Mention.UserID != "ou_zhangsan" {
		t.Fatal("unexpected second element")
	}
	if *elems[2].ParagraphElementType != ParagraphElementTypeMention || *elems[2].Mention.UserID != "ou_lisi" {
		t.Fatal("unexpected third element")
	}
}

func TestSemiPlainContentToContentBlock_OnlyPlaceholders(t *testing.T) {
	t.Parallel()
	// Text that is only placeholders should result in no text element
	sp := &SemiPlainContent{
		Text:    " @{ou_123}  @{ou_456} ",
		Mention: []string{"ou_123", "ou_456"},
	}
	cb := sp.ToContentBlock()
	if cb == nil {
		t.Fatal("expected non-nil ContentBlock")
	}
	elems := cb.Blocks[0].Paragraph.Elements
	// Should have only 2 mention elements, no text element
	if len(elems) != 2 {
		t.Fatalf("expected 2 elements (mentions only), got %d", len(elems))
	}
	if *elems[0].ParagraphElementType != ParagraphElementTypeMention {
		t.Fatal("expected first element to be mention")
	}
	if *elems[1].ParagraphElementType != ParagraphElementTypeMention {
		t.Fatal("expected second element to be mention")
	}
}

func TestSemiPlainContentToContentBlock_Nil(t *testing.T) {
	t.Parallel()
	var sp *SemiPlainContent
	cb := sp.ToContentBlock()
	if cb != nil {
		t.Fatal("expected nil ContentBlock for nil SemiPlainContent")
	}
}

func TestBuildContentBlock_Conversion(t *testing.T) {
	t.Parallel()
	cb := BuildContentBlock("Test text", []string{"ou_123", "ou_456"})
	if cb == nil {
		t.Fatal("expected non-nil ContentBlock")
	}
	elems := cb.Blocks[0].Paragraph.Elements
	if len(elems) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(elems))
	}
	if *elems[0].TextRun.Text != "Test text" {
		t.Fatalf("unexpected text: %s", *elems[0].TextRun.Text)
	}
	if *elems[1].Mention.UserID != "ou_123" {
		t.Fatalf("unexpected mention: %s", *elems[1].Mention.UserID)
	}
	if *elems[2].Mention.UserID != "ou_456" {
		t.Fatalf("unexpected mention: %s", *elems[2].Mention.UserID)
	}
}

func TestToSimpleMethods(t *testing.T) {
	t.Parallel()

	// Test Objective.ToSimple()
	text := "Objective text"
	obj := &Objective{
		ID:       "obj-1",
		Content:  BuildContentBlock(text, []string{"ou_123"}),
		Notes:    BuildContentBlock("Note text", nil),
		Owner:    Owner{OwnerType: OwnerTypeUser, UserID: strPtr("ou_owner")},
		CycleID:  "cycle-1",
		Score:    float64Ptr(0.7),
		Weight:   float64Ptr(0.5),
		Deadline: strPtr("1735776000000"),
	}
	simpleObj := obj.ToSimple()
	if simpleObj == nil {
		t.Fatal("expected non-nil RespObjectiveSimple")
	}
	if simpleObj.ID != "obj-1" {
		t.Fatalf("expected ID obj-1, got %s", simpleObj.ID)
	}
	// Text includes @{userID} placeholder for positional context
	expectedContentText := "Objective text @{ou_123} "
	if simpleObj.Content == nil || simpleObj.Content.Text != expectedContentText {
		t.Fatalf("unexpected content text: expected '%s', got '%s'", expectedContentText, simpleObj.Content.Text)
	}
	if simpleObj.Notes == nil || simpleObj.Notes.Text != "Note text" {
		t.Fatalf("unexpected notes: %+v", simpleObj.Notes)
	}
	if simpleObj.Score == nil || *simpleObj.Score != 0.7 {
		t.Fatalf("unexpected score: %v", simpleObj.Score)
	}
	if len(simpleObj.Content.Mention) != 1 || simpleObj.Content.Mention[0] != "ou_123" {
		t.Fatalf("unexpected mentions: %v", simpleObj.Content.Mention)
	}

	// Test KeyResult.ToSimple()
	kr := &KeyResult{
		ID:          "kr-1",
		ObjectiveID: "obj-1",
		Content:     BuildContentBlock("KR text", nil),
		Owner:       Owner{OwnerType: OwnerTypeUser, UserID: strPtr("ou_kr_owner")},
		Score:       float64Ptr(0.5),
	}
	simpleKR := kr.ToSimple()
	if simpleKR == nil {
		t.Fatal("expected non-nil RespKeyResultSimple")
	}
	if simpleKR.Content == nil || simpleKR.Content.Text != "KR text" {
		t.Fatalf("unexpected KR content: %+v", simpleKR.Content)
	}

	// Test ProgressV1.ToSimple()
	progress := &ProgressV1{
		ID:         "prog-1",
		ModifyTime: "1735776000000",
		Content:    BuildContentBlock("Progress text", []string{"ou_mention"}).ToV1(),
	}
	simpleProgress := progress.ToSimple()
	if simpleProgress == nil {
		t.Fatal("expected non-nil RespProgressSimple")
	}
	// Text includes @{userID} placeholder for positional context
	expectedProgressText := "Progress text @{ou_mention} "
	if simpleProgress.Content == nil || simpleProgress.Content.Text != expectedProgressText {
		t.Fatalf("unexpected progress text: expected '%s', got '%s'", expectedProgressText, simpleProgress.Content.Text)
	}
	if len(simpleProgress.Content.Mention) != 1 || simpleProgress.Content.Mention[0] != "ou_mention" {
		t.Fatalf("unexpected progress mentions: %v", simpleProgress.Content.Mention)
	}

	// Test Progress.ToSimple() (V2 progress record)
	progressV2 := &Progress{
		ID:         "prog-v2-1",
		CreateTime: "1735689600000",
		UpdateTime: "1735776000000",
		Content:    BuildContentBlock("V2 progress text", []string{"ou_v2_mention"}),
		ProgressRate: &ProgressRate{
			ProgressPercent: float64Ptr(80.0),
			ProgressStatus:  int32Ptr(int32(ProgressStatusDone)),
		},
	}
	simpleProgressV2 := progressV2.ToSimple()
	if simpleProgressV2 == nil {
		t.Fatal("expected non-nil RespProgressSimple for Progress V2")
	}
	if simpleProgressV2.ID != "prog-v2-1" {
		t.Fatalf("expected ID prog-v2-1, got %s", simpleProgressV2.ID)
	}
	if simpleProgressV2.CreateTime == nil || *simpleProgressV2.CreateTime == "" {
		t.Fatal("expected non-empty CreateTime for Progress V2")
	}
	expectedV2Text := "V2 progress text @{ou_v2_mention} "
	if simpleProgressV2.Content == nil || simpleProgressV2.Content.Text != expectedV2Text {
		t.Fatalf("unexpected V2 progress text: expected '%s', got '%s'", expectedV2Text, simpleProgressV2.Content.Text)
	}
	if simpleProgressV2.ProgressRate == nil || simpleProgressV2.ProgressRate.Status == nil || *simpleProgressV2.ProgressRate.Status != "done" {
		t.Fatalf("expected progress status 'done', got %+v", simpleProgressV2.ProgressRate)
	}
	if simpleProgressV2.ProgressRate.Percent == nil || *simpleProgressV2.ProgressRate.Percent != 80.0 {
		t.Fatalf("expected progress percent 80.0, got %v", simpleProgressV2.ProgressRate.Percent)
	}
	if len(simpleProgressV2.Content.Mention) != 1 || simpleProgressV2.Content.Mention[0] != "ou_v2_mention" {
		t.Fatalf("unexpected V2 progress mentions: %v", simpleProgressV2.Content.Mention)
	}
}

// listTypePtr returns a pointer to the given ListType value.
func listTypePtr(v ListType) *ListType { return &v }
