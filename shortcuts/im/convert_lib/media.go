// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package convertlib

import "fmt"

type imageConverter struct{}

func (imageConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("image")
	}
	if key, _ := parsed["image_key"].(string); key != "" {
		return fmt.Sprintf("[Image: %s]", key)
	}
	return "[Image]"
}

type fileConverter struct{}

func (fileConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("file")
	}
	key, _ := parsed["file_key"].(string)
	if key == "" {
		return "[File]"
	}
	name, _ := parsed["file_name"].(string)
	if name == "" {
		name = key
	}
	return fmt.Sprintf(`<file key="%s" name="%s"/>`, cardEscapeAttr(key), cardEscapeAttr(name))
}

type audioMsgConverter struct{}

// Convert renders an audio message: when body.content carries a file_key it
// emits <audio key="..." duration="Xs"/> (duration omitted when absent);
// otherwise it falls back to [Voice: Xs] (duration only) or [Voice].
func (audioMsgConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("audio")
	}
	if key, _ := parsed["file_key"].(string); key != "" {
		result := fmt.Sprintf(`<audio key="%s"`, cardEscapeAttr(key))
		if dur, ok := parsed["duration"].(float64); ok && dur > 0 {
			result += fmt.Sprintf(` duration="%.0fs"`, dur/1000)
		}
		return result + "/>"
	}
	if dur, ok := parsed["duration"].(float64); ok && dur > 0 {
		return fmt.Sprintf("[Voice: %.0fs]", dur/1000)
	}
	return "[Voice]"
}

type videoMsgConverter struct{}

func (videoMsgConverter) Convert(ctx *ConvertContext) string {
	parsed, err := ParseJSONObject(ctx.RawContent)
	if err != nil {
		return invalidJSONPlaceholder("video")
	}
	key, _ := parsed["file_key"].(string)
	if key == "" {
		return "[Video]"
	}
	name, _ := parsed["file_name"].(string)
	if name == "" {
		name = key
	}
	result := fmt.Sprintf(`<video key="%s" name="%s"`, cardEscapeAttr(key), cardEscapeAttr(name))
	if dur, ok := parsed["duration"].(float64); ok && dur > 0 {
		result += fmt.Sprintf(` duration="%.0fs"`, dur/1000)
	}
	if coverKey, _ := parsed["image_key"].(string); coverKey != "" {
		result += fmt.Sprintf(` cover_image_key="%s"`, cardEscapeAttr(coverKey))
	}
	return result + "/>"
}
