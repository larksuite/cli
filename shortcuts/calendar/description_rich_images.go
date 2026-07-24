// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package calendar

import (
	"fmt"
	"image"

	// Register the common image decoders so DecodeConfig can read intrinsic
	// dimensions for PNG/JPEG/GIF sources.
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const calendarMediaParentType = "calendar"

var markdownImageRe = regexp.MustCompile(`!\[([^\]]*)\]\(([^)]*)\)`)

func resolveDescriptionRichImages(runtime *common.RuntimeContext, calendarID string) error {
	md := runtime.Str("description-rich")
	if md == "" || !strings.Contains(md, "![") {
		return nil
	}
	rewritten, changed, err := uploadLocalDescriptionImages(runtime, calendarID, md)
	if err != nil {
		return err
	}
	if changed {
		if err := runtime.Cmd.Flags().Set("description-rich", rewritten); err != nil {
			return errs.NewInternalError(errs.SubtypeUnknown, "failed to update --description-rich after image upload: %v", err).WithCause(err)
		}
	}
	return nil
}

func uploadLocalDescriptionImages(runtime *common.RuntimeContext, calendarID, md string) (string, bool, error) {
	matches := markdownImageRe.FindAllStringSubmatchIndex(md, -1)
	if len(matches) == 0 {
		return md, false, nil
	}
	var out strings.Builder
	last := 0
	changed := false
	cache := map[string]string{}
	for _, m := range matches {
		altStart, altEnd, srcStart, srcEnd := m[2], m[3], m[4], m[5]
		src := strings.TrimSpace(md[srcStart:srcEnd])
		if !isLocalImageSrc(src) {
			continue
		}
		alt := md[altStart:altEnd]
		uploadedURL, err := resolveLocalImage(runtime, calendarID, src, alt, cache)
		if err != nil {
			return "", false, err
		}
		out.WriteString(md[last:srcStart])
		out.WriteString(uploadedURL)
		last = srcEnd
		changed = true
	}
	if !changed {
		return md, false, nil
	}
	out.WriteString(md[last:])
	return out.String(), true, nil
}

func resolveLocalImage(runtime *common.RuntimeContext, calendarID, src, alt string, cache map[string]string) (string, error) {
	localPath := localImagePath(src)
	if cached, ok := cache[localPath]; ok {
		return cached, nil
	}

	safePath, err := validate.SafeInputPath(localPath)
	if err != nil {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"--description-rich image %q could not be read: %v", src, err).
			WithParam("--description-rich").
			WithHint("reference local images by a path inside the current working directory (e.g. ./images/pic.png; cd there first), or use an already-uploaded Lark image URL").
			WithCause(err)
	}

	info, err := runtime.FileIO().Stat(localPath)
	if err != nil {
		return "", common.WrapInputStatErrorTyped(err)
	}

	fileToken, err := common.UploadDriveMediaAllTyped(runtime, common.DriveMediaUploadAllConfig{
		FilePath:   localPath,
		FileName:   filepath.Base(safePath),
		FileSize:   info.Size(),
		ParentType: calendarMediaParentType,
		ParentNode: &calendarID,
	})
	if err != nil {
		return "", err
	}

	width, height := decodeImageDimensions(runtime, localPath)
	uploadedURL := buildCalendarImagePreviewURL(runtime.Config.Brand, fileToken, width, height, info.Size())
	cache[localPath] = uploadedURL
	return uploadedURL, nil
}

func decodeImageDimensions(runtime *common.RuntimeContext, path string) (int, int) {
	f, err := runtime.FileIO().Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

func isLocalImageSrc(src string) bool {
	if src == "" {
		return false
	}
	lower := strings.ToLower(src)
	switch {
	case strings.HasPrefix(lower, "http://"), strings.HasPrefix(lower, "https://"), strings.HasPrefix(lower, "data:"):
		return false
	case strings.HasPrefix(lower, "file://"):
		return true
	}
	if i := strings.Index(src, "://"); i > 0 {
		return false
	}
	return true
}

func localImagePath(src string) string {
	s := strings.TrimSpace(src)
	if strings.HasPrefix(strings.ToLower(s), "file://") {
		if u, err := url.Parse(s); err == nil && u.Path != "" {
			s = u.Path
		}
	}
	if decoded, err := url.PathUnescape(s); err == nil {
		return decoded
	}
	return s
}

func buildCalendarImagePreviewURL(brand core.LarkBrand, fileToken string, width, height int, size int64) string {
	host := "internal-api-drive-stream.larkoffice.com"
	if brand == core.BrandLark {
		host = "internal-api-drive-stream.larksuite.com"
	}
	u := fmt.Sprintf("https://%s/space/api/box/stream/download/preview/%s?preview_type=16", host, fileToken)
	if width > 0 && height > 0 {
		u += fmt.Sprintf("&im_w=%d&im_h=%d", width, height)
	}
	if size > 0 {
		u += fmt.Sprintf("&im_size=%d", size)
	}
	return u
}
