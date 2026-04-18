// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package okr

import (
	"testing"

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
	})
}

// strPtr returns a pointer to the given string value.
func strPtr(v string) *string { return &v }

// int32Ptr returns a pointer to the given int32 value.
func int32Ptr(v int32) *int32 { return &v }

// float64Ptr returns a pointer to the given float64 value.
func float64Ptr(v float64) *float64 { return &v }
