// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contentread

import (
	"fmt"
	"regexp"
	"strings"
)

// qaImageTagRe matches the materialized image tag the server emits:
// <qa_image image_token="KEY"/> (optional space before the self-close). The
// capture group is the ImageMetaMap key.
var qaImageTagRe = regexp.MustCompile(`<qa_image\s+image_token="([^"]+)"\s*/>`)

// RenderImages rewrites every <qa_image .../> tag in md to a markdown image,
// looking each token up in metas (keyed by image_token). Tags whose token is
// absent from metas degrade to a caption-only / token placeholder.
func RenderImages(md string, metas map[string]*ImageMeta) string {
	if !strings.Contains(md, "<qa_image") {
		return md
	}
	return qaImageTagRe.ReplaceAllStringFunc(md, func(tag string) string {
		m := qaImageTagRe.FindStringSubmatch(tag)
		if len(m) < 2 {
			return tag
		}
		return RenderOneImage(m[1], metas[m[1]])
	})
}

// RenderOneImage renders a single image token as ![caption](token). caption comes
// from the image meta (it degrades to "image" when absent); the URL is the
// image_key itself — the http layer trims qa_image_meta to image_key, so there is
// no separate CDN URL to emit.
func RenderOneImage(token string, meta *ImageMeta) string {
	caption := "image"
	imageKey := token
	if meta != nil {
		if value := strings.TrimSpace(meta.Caption); value != "" {
			caption = value
		}
		if value := strings.TrimSpace(meta.ImageKey); value != "" {
			imageKey = value
		}
	}
	return fmt.Sprintf("![%s](%s)", caption, imageKey)
}
