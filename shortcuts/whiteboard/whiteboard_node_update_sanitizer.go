// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

// wbNodeUpdateSanitizer is a best-effort allowlist used only to strip response
// envelopes and known non-node noise from raw/query payloads before
// +node-update sends batch_update. It is not the authoritative WhiteboardNode
// schema and must not be used for validation; the gateway and engine remain the
// source of truth for field support.
type wbNodeUpdateSanitizer map[string]wbNodeUpdateSanitizer

var wbNodeUpdatePointSanitizer = wbNodeUpdateSanitizer{
	"x": nil,
	"y": nil,
}

var wbNodeUpdateRichTextElementTextStyleSanitizer = wbNodeUpdateSanitizer{
	"font_weight":                nil,
	"font_size":                  nil,
	"text_color":                 nil,
	"text_background_color":      nil,
	"line_through":               nil,
	"underline":                  nil,
	"italic":                     nil,
	"dark_text_color":            nil,
	"dark_text_background_color": nil,
}

var wbNodeUpdateRichTextElementTextSanitizer = wbNodeUpdateSanitizer{
	"text":       nil,
	"text_style": wbNodeUpdateRichTextElementTextStyleSanitizer,
}

var wbNodeUpdateRichTextElementLinkSanitizer = wbNodeUpdateSanitizer{
	"herf":       nil,
	"text":       nil,
	"text_style": wbNodeUpdateRichTextElementTextStyleSanitizer,
}

var wbNodeUpdateRichTextElementMentionDocSanitizer = wbNodeUpdateSanitizer{
	"doc_url":    nil,
	"text_style": wbNodeUpdateRichTextElementTextStyleSanitizer,
}

var wbNodeUpdateRichTextElementMentionUserSanitizer = wbNodeUpdateSanitizer{
	"user_id":    nil,
	"text_style": wbNodeUpdateRichTextElementTextStyleSanitizer,
}

var wbNodeUpdateRichTextElementSanitizer = wbNodeUpdateSanitizer{
	"element_type":         nil,
	"text_element":         wbNodeUpdateRichTextElementTextSanitizer,
	"link_element":         wbNodeUpdateRichTextElementLinkSanitizer,
	"mention_user_element": wbNodeUpdateRichTextElementMentionUserSanitizer,
	"mention_doc_element":  wbNodeUpdateRichTextElementMentionDocSanitizer,
}

var wbNodeUpdateRichTextParagraphSanitizer = wbNodeUpdateSanitizer{
	"paragraph_type":   nil,
	"elements":         wbNodeUpdateRichTextElementSanitizer,
	"indent":           nil,
	"list_begin_index": nil,
	"quote":            nil,
}

var wbNodeUpdateRichTextSanitizer = wbNodeUpdateSanitizer{
	"paragraphs": wbNodeUpdateRichTextParagraphSanitizer,
}

var wbNodeUpdateTextSanitizer = wbNodeUpdateSanitizer{
	"text":                                  nil,
	"font_weight":                           nil,
	"font_size":                             nil,
	"horizontal_align":                      nil,
	"vertical_align":                        nil,
	"text_color":                            nil,
	"text_background_color":                 nil,
	"line_through":                          nil,
	"underline":                             nil,
	"italic":                                nil,
	"angle":                                 nil,
	"theme_text_color_code":                 nil,
	"theme_text_background_color_code":      nil,
	"rich_text":                             wbNodeUpdateRichTextSanitizer,
	"text_color_type":                       nil,
	"text_background_color_type":            nil,
	"dark_text_color":                       nil,
	"dark_text_background_color":            nil,
	"dark_theme_text_color_code":            nil,
	"dark_theme_text_background_color_code": nil,
}

var wbNodeUpdateBorderRadiusSanitizer = wbNodeUpdateSanitizer{
	"top_left":     nil,
	"top_right":    nil,
	"bottom_right": nil,
	"bottom_left":  nil,
}

var wbNodeUpdateShadowSanitizer = wbNodeUpdateSanitizer{
	"color":    nil,
	"blur":     nil,
	"offset_x": nil,
	"offset_y": nil,
	"opacity":  nil,
}

var wbNodeUpdateGradientStopSanitizer = wbNodeUpdateSanitizer{
	"position": nil,
	"color":    nil,
}

var wbNodeUpdateFillGradientSanitizer = wbNodeUpdateSanitizer{
	"type":             nil,
	"handle_positions": wbNodeUpdatePointSanitizer,
	"stops":            wbNodeUpdateGradientStopSanitizer,
}

var wbNodeUpdateStyleSanitizer = wbNodeUpdateSanitizer{
	"fill_color":                   nil,
	"fill_opacity":                 nil,
	"border_width":                 nil,
	"border_color":                 nil,
	"border_opacity":               nil,
	"h_flip":                       nil,
	"v_flip":                       nil,
	"border_style":                 nil,
	"theme_fill_color_code":        nil,
	"theme_border_color_code":      nil,
	"fill_color_type":              nil,
	"border_color_type":            nil,
	"dark_fill_color":              nil,
	"dark_border_color":            nil,
	"dark_theme_fill_color_code":   nil,
	"dark_theme_border_color_code": nil,
	"border_dasharrays":            nil,
	"border_radius":                wbNodeUpdateBorderRadiusSanitizer,
	"shadow":                       wbNodeUpdateShadowSanitizer,
	"inner_shadow":                 wbNodeUpdateShadowSanitizer,
	"fill_gradient":                wbNodeUpdateFillGradientSanitizer,
}

var wbNodeUpdatePieSanitizer = wbNodeUpdateSanitizer{
	"start_radial_line_angle": nil,
	"central_angle":           nil,
	"radius":                  nil,
	"sector_ratio":            nil,
}

var wbNodeUpdateTrapezoidSanitizer = wbNodeUpdateSanitizer{
	"top_length": nil,
}

var wbNodeUpdateCubeSanitizer = wbNodeUpdateSanitizer{
	"control_point": wbNodeUpdatePointSanitizer,
}

var wbNodeUpdateCompositeShapeSanitizer = wbNodeUpdateSanitizer{
	"type":          nil,
	"pie":           wbNodeUpdatePieSanitizer,
	"circular_ring": wbNodeUpdatePieSanitizer,
	"trapezoid":     wbNodeUpdateTrapezoidSanitizer,
	"cube":          wbNodeUpdateCubeSanitizer,
}

var wbNodeUpdateConnectorAttachedObjectSanitizer = wbNodeUpdateSanitizer{
	"id":       nil,
	"snap_to":  nil,
	"position": wbNodeUpdatePointSanitizer,
}

var wbNodeUpdateConnectorCaptionSanitizer = wbNodeUpdateSanitizer{
	"data": wbNodeUpdateTextSanitizer,
}

var wbNodeUpdateConnectorInfoSanitizer = wbNodeUpdateSanitizer{
	"attached_object": wbNodeUpdateConnectorAttachedObjectSanitizer,
	"position":        wbNodeUpdatePointSanitizer,
	"arrow_style":     nil,
}

var wbNodeUpdateConnectorSanitizer = wbNodeUpdateSanitizer{
	"start_object":           wbNodeUpdateConnectorAttachedObjectSanitizer,
	"end_object":             wbNodeUpdateConnectorAttachedObjectSanitizer,
	"captions":               wbNodeUpdateConnectorCaptionSanitizer,
	"shape":                  nil,
	"turning_points":         wbNodeUpdatePointSanitizer,
	"start":                  wbNodeUpdateConnectorInfoSanitizer,
	"end":                    wbNodeUpdateConnectorInfoSanitizer,
	"caption_auto_direction": nil,
	"caption_position":       nil,
	"specified_coordinate":   nil,
	"caption_position_type":  nil,
}

var wbNodeUpdateImageSanitizer = wbNodeUpdateSanitizer{
	"token": nil,
}

var wbNodeUpdateLifelineSanitizer = wbNodeUpdateSanitizer{
	"size": nil,
	"type": nil,
}

var wbNodeUpdateMindMapSanitizer = wbNodeUpdateSanitizer{
	"parent_id": nil,
}

var wbNodeUpdateMindMapNodeSanitizer = wbNodeUpdateSanitizer{
	"parent_id":       nil,
	"type":            nil,
	"z_index":         nil,
	"layout_position": nil,
	"children":        nil,
	"collapsed":       nil,
}

var wbNodeUpdateMindMapRootSanitizer = wbNodeUpdateSanitizer{
	"layout":         nil,
	"type":           nil,
	"line_style":     nil,
	"up_children":    nil,
	"down_children":  nil,
	"left_children":  nil,
	"right_children": nil,
}

var wbNodeUpdatePaintSanitizer = wbNodeUpdateSanitizer{
	"type":  nil,
	"lines": wbNodeUpdatePointSanitizer,
	"width": nil,
	"color": nil,
}

var wbNodeUpdateSectionSanitizer = wbNodeUpdateSanitizer{
	"title": nil,
}

var wbNodeUpdateStickyNoteSanitizer = wbNodeUpdateSanitizer{
	"user_id":          nil,
	"show_author_info": nil,
}

var wbNodeUpdateSvgSanitizer = wbNodeUpdateSanitizer{
	"svg_code": nil,
	"key":      nil,
	"type":     nil,
}

var wbNodeUpdateTableCellMergeInfoSanitizer = wbNodeUpdateSanitizer{
	"row_span": nil,
	"col_span": nil,
}

var wbNodeUpdateTableCellSanitizer = wbNodeUpdateSanitizer{
	"row_index":  nil,
	"col_index":  nil,
	"merge_info": wbNodeUpdateTableCellMergeInfoSanitizer,
	"children":   nil,
	"text":       wbNodeUpdateTextSanitizer,
	"style":      wbNodeUpdateStyleSanitizer,
}

var wbNodeUpdateTableMetaSanitizer = wbNodeUpdateSanitizer{
	"row_num":   nil,
	"col_num":   nil,
	"style":     wbNodeUpdateStyleSanitizer,
	"text":      wbNodeUpdateTextSanitizer,
	"row_sizes": nil,
	"col_sizes": nil,
}

var wbNodeUpdateTableSanitizer = wbNodeUpdateSanitizer{
	"meta":  wbNodeUpdateTableMetaSanitizer,
	"title": nil,
	"cells": wbNodeUpdateTableCellSanitizer,
}

var wbNodeUpdateSyntaxSanitizer = wbNodeUpdateSanitizer{
	"syntax_type": nil,
	"code":        nil,
	"style_type":  nil,
}

var wbNodeUpdateSanitizerRoot = wbNodeUpdateSanitizer{
	"id":              nil,
	"type":            nil,
	"parent_id":       nil,
	"children":        nil,
	"x":               nil,
	"y":               nil,
	"angle":           nil,
	"height":          nil,
	"text":            wbNodeUpdateTextSanitizer,
	"style":           wbNodeUpdateStyleSanitizer,
	"image":           wbNodeUpdateImageSanitizer,
	"composite_shape": wbNodeUpdateCompositeShapeSanitizer,
	"connector":       wbNodeUpdateConnectorSanitizer,
	"width":           nil,
	"section":         wbNodeUpdateSectionSanitizer,
	"table":           wbNodeUpdateTableSanitizer,
	"mind_map":        wbNodeUpdateMindMapSanitizer,
	"locked":          nil,
	"z_index":         nil,
	"lifeline":        wbNodeUpdateLifelineSanitizer,
	"paint":           wbNodeUpdatePaintSanitizer,
	"svg":             wbNodeUpdateSvgSanitizer,
	"sticky_note":     wbNodeUpdateStickyNoteSanitizer,
	"mind_map_node":   wbNodeUpdateMindMapNodeSanitizer,
	"mind_map_root":   wbNodeUpdateMindMapRootSanitizer,
	"syntax":          wbNodeUpdateSyntaxSanitizer,
}

func sanitizeWbNodeUpdateNodes(nodes []map[string]interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, sanitizeWbNodeUpdateObject(node, wbNodeUpdateSanitizerRoot))
	}
	return out
}

func sanitizeWbNodeUpdateObject(input map[string]interface{}, schema wbNodeUpdateSanitizer) map[string]interface{} {
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		fieldSchema, ok := schema[key]
		if !ok {
			continue
		}
		out[key] = sanitizeWbNodeUpdateValue(value, fieldSchema)
	}
	return out
}

func sanitizeWbNodeUpdateValue(value interface{}, schema wbNodeUpdateSanitizer) interface{} {
	if schema == nil {
		return value
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		return sanitizeWbNodeUpdateObject(typed, schema)
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, sanitizeWbNodeUpdateValue(item, schema))
		}
		return out
	default:
		return value
	}
}
