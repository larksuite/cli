// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package whiteboard

type whiteboardNodeUpdateSchema map[string]whiteboardNodeUpdateSchema

var wbNodeUpdatePointSchema = whiteboardNodeUpdateSchema{
	"x": nil,
	"y": nil,
}

var wbNodeUpdateRichTextElementTextStyleSchema = whiteboardNodeUpdateSchema{
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

var wbNodeUpdateRichTextElementTextSchema = whiteboardNodeUpdateSchema{
	"text":       nil,
	"text_style": wbNodeUpdateRichTextElementTextStyleSchema,
}

var wbNodeUpdateRichTextElementLinkSchema = whiteboardNodeUpdateSchema{
	"herf":       nil,
	"text":       nil,
	"text_style": wbNodeUpdateRichTextElementTextStyleSchema,
}

var wbNodeUpdateRichTextElementMentionDocSchema = whiteboardNodeUpdateSchema{
	"doc_url":    nil,
	"text_style": wbNodeUpdateRichTextElementTextStyleSchema,
}

var wbNodeUpdateRichTextElementMentionUserSchema = whiteboardNodeUpdateSchema{
	"user_id":    nil,
	"text_style": wbNodeUpdateRichTextElementTextStyleSchema,
}

var wbNodeUpdateRichTextElementSchema = whiteboardNodeUpdateSchema{
	"element_type":         nil,
	"text_element":         wbNodeUpdateRichTextElementTextSchema,
	"link_element":         wbNodeUpdateRichTextElementLinkSchema,
	"mention_user_element": wbNodeUpdateRichTextElementMentionUserSchema,
	"mention_doc_element":  wbNodeUpdateRichTextElementMentionDocSchema,
}

var wbNodeUpdateRichTextParagraphSchema = whiteboardNodeUpdateSchema{
	"paragraph_type":   nil,
	"elements":         wbNodeUpdateRichTextElementSchema,
	"indent":           nil,
	"list_begin_index": nil,
	"quote":            nil,
}

var wbNodeUpdateRichTextSchema = whiteboardNodeUpdateSchema{
	"paragraphs": wbNodeUpdateRichTextParagraphSchema,
}

var wbNodeUpdateTextSchema = whiteboardNodeUpdateSchema{
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
	"rich_text":                             wbNodeUpdateRichTextSchema,
	"text_color_type":                       nil,
	"text_background_color_type":            nil,
	"dark_text_color":                       nil,
	"dark_text_background_color":            nil,
	"dark_theme_text_color_code":            nil,
	"dark_theme_text_background_color_code": nil,
}

var wbNodeUpdateBorderRadiusSchema = whiteboardNodeUpdateSchema{
	"top_left":     nil,
	"top_right":    nil,
	"bottom_right": nil,
	"bottom_left":  nil,
}

var wbNodeUpdateShadowSchema = whiteboardNodeUpdateSchema{
	"color":    nil,
	"blur":     nil,
	"offset_x": nil,
	"offset_y": nil,
	"opacity":  nil,
}

var wbNodeUpdateGradientStopSchema = whiteboardNodeUpdateSchema{
	"position": nil,
	"color":    nil,
}

var wbNodeUpdateFillGradientSchema = whiteboardNodeUpdateSchema{
	"type":             nil,
	"handle_positions": wbNodeUpdatePointSchema,
	"stops":            wbNodeUpdateGradientStopSchema,
}

var wbNodeUpdateStyleSchema = whiteboardNodeUpdateSchema{
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
	"border_radius":                wbNodeUpdateBorderRadiusSchema,
	"shadow":                       wbNodeUpdateShadowSchema,
	"inner_shadow":                 wbNodeUpdateShadowSchema,
	"fill_gradient":                wbNodeUpdateFillGradientSchema,
}

var wbNodeUpdatePieSchema = whiteboardNodeUpdateSchema{
	"start_radial_line_angle": nil,
	"central_angle":           nil,
	"radius":                  nil,
	"sector_ratio":            nil,
}

var wbNodeUpdateTrapezoidSchema = whiteboardNodeUpdateSchema{
	"top_length": nil,
}

var wbNodeUpdateCubeSchema = whiteboardNodeUpdateSchema{
	"control_point": wbNodeUpdatePointSchema,
}

var wbNodeUpdateCompositeShapeSchema = whiteboardNodeUpdateSchema{
	"type":          nil,
	"pie":           wbNodeUpdatePieSchema,
	"circular_ring": wbNodeUpdatePieSchema,
	"trapezoid":     wbNodeUpdateTrapezoidSchema,
	"cube":          wbNodeUpdateCubeSchema,
}

var wbNodeUpdateConnectorAttachedObjectSchema = whiteboardNodeUpdateSchema{
	"id":       nil,
	"snap_to":  nil,
	"position": wbNodeUpdatePointSchema,
}

var wbNodeUpdateConnectorCaptionSchema = whiteboardNodeUpdateSchema{
	"data": wbNodeUpdateTextSchema,
}

var wbNodeUpdateConnectorInfoSchema = whiteboardNodeUpdateSchema{
	"attached_object": wbNodeUpdateConnectorAttachedObjectSchema,
	"position":        wbNodeUpdatePointSchema,
	"arrow_style":     nil,
}

var wbNodeUpdateConnectorSchema = whiteboardNodeUpdateSchema{
	"start_object":           wbNodeUpdateConnectorAttachedObjectSchema,
	"end_object":             wbNodeUpdateConnectorAttachedObjectSchema,
	"captions":               wbNodeUpdateConnectorCaptionSchema,
	"shape":                  nil,
	"turning_points":         wbNodeUpdatePointSchema,
	"start":                  wbNodeUpdateConnectorInfoSchema,
	"end":                    wbNodeUpdateConnectorInfoSchema,
	"caption_auto_direction": nil,
	"caption_position":       nil,
	"specified_coordinate":   nil,
	"caption_position_type":  nil,
}

var wbNodeUpdateImageSchema = whiteboardNodeUpdateSchema{
	"token": nil,
}

var wbNodeUpdateLifelineSchema = whiteboardNodeUpdateSchema{
	"size": nil,
	"type": nil,
}

var wbNodeUpdateMindMapSchema = whiteboardNodeUpdateSchema{
	"parent_id": nil,
}

var wbNodeUpdateMindMapNodeSchema = whiteboardNodeUpdateSchema{
	"parent_id":       nil,
	"type":            nil,
	"z_index":         nil,
	"layout_position": nil,
	"children":        nil,
	"collapsed":       nil,
}

var wbNodeUpdateMindMapRootSchema = whiteboardNodeUpdateSchema{
	"layout":         nil,
	"type":           nil,
	"line_style":     nil,
	"up_children":    nil,
	"down_children":  nil,
	"left_children":  nil,
	"right_children": nil,
}

var wbNodeUpdatePaintSchema = whiteboardNodeUpdateSchema{
	"type":  nil,
	"lines": wbNodeUpdatePointSchema,
	"width": nil,
	"color": nil,
}

var wbNodeUpdateSectionSchema = whiteboardNodeUpdateSchema{
	"title": nil,
}

var wbNodeUpdateStickyNoteSchema = whiteboardNodeUpdateSchema{
	"user_id":          nil,
	"show_author_info": nil,
}

var wbNodeUpdateSvgSchema = whiteboardNodeUpdateSchema{
	"svg_code": nil,
	"key":      nil,
	"type":     nil,
}

var wbNodeUpdateTableCellMergeInfoSchema = whiteboardNodeUpdateSchema{
	"row_span": nil,
	"col_span": nil,
}

var wbNodeUpdateTableCellSchema = whiteboardNodeUpdateSchema{
	"row_index":  nil,
	"col_index":  nil,
	"merge_info": wbNodeUpdateTableCellMergeInfoSchema,
	"children":   nil,
	"text":       wbNodeUpdateTextSchema,
	"style":      wbNodeUpdateStyleSchema,
}

var wbNodeUpdateTableMetaSchema = whiteboardNodeUpdateSchema{
	"row_num":   nil,
	"col_num":   nil,
	"style":     wbNodeUpdateStyleSchema,
	"text":      wbNodeUpdateTextSchema,
	"row_sizes": nil,
	"col_sizes": nil,
}

var wbNodeUpdateTableSchema = whiteboardNodeUpdateSchema{
	"meta":  wbNodeUpdateTableMetaSchema,
	"title": nil,
	"cells": wbNodeUpdateTableCellSchema,
}

var wbNodeUpdateSyntaxSchema = whiteboardNodeUpdateSchema{
	"syntax_type": nil,
	"code":        nil,
	"style_type":  nil,
}

var wbNodeUpdateSchema = whiteboardNodeUpdateSchema{
	"id":              nil,
	"type":            nil,
	"parent_id":       nil,
	"children":        nil,
	"x":               nil,
	"y":               nil,
	"angle":           nil,
	"height":          nil,
	"text":            wbNodeUpdateTextSchema,
	"style":           wbNodeUpdateStyleSchema,
	"image":           wbNodeUpdateImageSchema,
	"composite_shape": wbNodeUpdateCompositeShapeSchema,
	"connector":       wbNodeUpdateConnectorSchema,
	"width":           nil,
	"section":         wbNodeUpdateSectionSchema,
	"table":           wbNodeUpdateTableSchema,
	"mind_map":        wbNodeUpdateMindMapSchema,
	"locked":          nil,
	"z_index":         nil,
	"lifeline":        wbNodeUpdateLifelineSchema,
	"paint":           wbNodeUpdatePaintSchema,
	"svg":             wbNodeUpdateSvgSchema,
	"sticky_note":     wbNodeUpdateStickyNoteSchema,
	"mind_map_node":   wbNodeUpdateMindMapNodeSchema,
	"mind_map_root":   wbNodeUpdateMindMapRootSchema,
	"syntax":          wbNodeUpdateSyntaxSchema,
}

func sanitizeWhiteboardNodeUpdateNodes(nodes []map[string]interface{}) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(nodes))
	for _, node := range nodes {
		out = append(out, sanitizeWhiteboardNodeUpdateObject(node, wbNodeUpdateSchema))
	}
	return out
}

func sanitizeWhiteboardNodeUpdateObject(input map[string]interface{}, schema whiteboardNodeUpdateSchema) map[string]interface{} {
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		fieldSchema, ok := schema[key]
		if !ok {
			continue
		}
		out[key] = sanitizeWhiteboardNodeUpdateValue(value, fieldSchema)
	}
	return out
}

func sanitizeWhiteboardNodeUpdateValue(value interface{}, schema whiteboardNodeUpdateSchema) interface{} {
	if schema == nil {
		return value
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		return sanitizeWhiteboardNodeUpdateObject(typed, schema)
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, sanitizeWhiteboardNodeUpdateValue(item, schema))
		}
		return out
	default:
		return value
	}
}
