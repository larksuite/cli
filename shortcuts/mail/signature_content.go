// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/mail/signature"
)

var imgSrcRe = regexp.MustCompile(`(?is)<img\b[^>]*\bsrc\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))`)

func validateSignatureID(id string) error {
	if id == "" {
		return nil
	}
	if _, err := strconv.ParseInt(id, 10, 64); err != nil {
		return output.ErrValidation("--signature-id must be a decimal integer string")
	}
	return nil
}

func normalizeSignatureDevice(raw, flagName string) (signature.SignatureDevice, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "pc":
		return signature.DevicePC, nil
	case "mobile":
		return signature.DeviceMobile, nil
	default:
		return "", output.ErrValidation("%s must be pc or mobile", flagName)
	}
}

func normalizeSignatureContent(raw string) (string, error) {
	if err := rejectSignatureLocalImages(raw); err != nil {
		return "", err
	}
	if bodyIsHTML(raw) {
		return raw, nil
	}
	return buildBodyDiv(raw, false), nil
}

func resolveSignatureContent(runtime *common.RuntimeContext, valueFlag, fileFlag string) (string, bool, error) {
	if runtime.Changed(valueFlag) {
		content, err := normalizeSignatureContent(runtime.Str(valueFlag))
		return content, true, err
	}
	path := strings.TrimSpace(runtime.Str(fileFlag))
	if path == "" {
		return "", false, nil
	}
	f, err := runtime.FileIO().Open(path)
	if err != nil {
		return "", false, output.ErrValidation("open --%s %s: %v", fileFlag, path, err)
	}
	defer f.Close()
	buf, err := io.ReadAll(f)
	if err != nil {
		return "", false, output.ErrValidation("read --%s %s: %v", fileFlag, path, err)
	}
	if !utf8.Valid(buf) {
		return "", false, output.ErrValidation("--%s %s must be UTF-8 text", fileFlag, path)
	}
	content, err := normalizeSignatureContent(string(buf))
	return content, true, err
}

func parseSignatureImagesJSON(raw, flagName string) ([]signature.SignatureImage, bool, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, false, nil
	}
	var items []map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, true, output.ErrValidation("--%s must be a JSON array of signature images: %v", flagName, err)
	}
	images := make([]signature.SignatureImage, 0, len(items))
	for i, item := range items {
		img := signature.SignatureImage{
			ImageName:   stringField(item, "image_name"),
			FileKey:     stringField(item, "file_key"),
			CID:         stringField(item, "cid"),
			FileSize:    stringField(item, "file_size"),
			ImageWidth:  int32Field(item, "image_width"),
			ImageHeight: int32Field(item, "image_height"),
		}
		if strings.TrimSpace(img.CID) == "" {
			return nil, true, output.ErrValidation("--%s[%d].cid is required", flagName, i)
		}
		images = append(images, img)
	}
	return images, true, nil
}

func sanitizeSignatureImages(images []signature.SignatureImage) []signature.SignatureImage {
	if len(images) == 0 {
		return nil
	}
	out := make([]signature.SignatureImage, 0, len(images))
	for _, img := range images {
		out = append(out, signature.SignatureImage{
			ImageName:   img.ImageName,
			FileKey:     img.FileKey,
			CID:         img.CID,
			FileSize:    img.FileSize,
			ImageWidth:  img.ImageWidth,
			ImageHeight: img.ImageHeight,
		})
	}
	return out
}

func validateSignatureImageRefs(content string, images []signature.SignatureImage) error {
	refs := extractSignatureCIDs(content)
	seenImages := make(map[string]bool, len(images))
	for _, img := range images {
		if strings.TrimSpace(img.CID) == "" {
			return output.ErrValidation("image cid is required")
		}
		if seenImages[img.CID] {
			return output.ErrValidation("duplicate image cid %q in images metadata", img.CID)
		}
		seenImages[img.CID] = true
		if !refs[img.CID] {
			return output.ErrValidation("image cid %q is not referenced by signature content", img.CID)
		}
	}
	for cid := range refs {
		if !seenImages[cid] {
			return output.ErrValidation("signature content references cid:%s but images metadata is missing", cid)
		}
	}
	return nil
}

func validateSignatureImagesOnly(images []signature.SignatureImage) error {
	seenImages := make(map[string]bool, len(images))
	for _, img := range images {
		if strings.TrimSpace(img.CID) == "" {
			return output.ErrValidation("image cid is required")
		}
		if seenImages[img.CID] {
			return output.ErrValidation("duplicate image cid %q in images metadata", img.CID)
		}
		seenImages[img.CID] = true
	}
	return nil
}

func rejectSignatureLocalImages(content string) error {
	for _, src := range extractSignatureImageSources(content) {
		if src == "" {
			continue
		}
		if strings.HasPrefix(src, "//") {
			continue
		}
		u, err := url.Parse(src)
		if err == nil && u.Scheme != "" {
			continue
		}
		return output.ErrValidation("local image paths are not supported for signature content; use --images-json with cid/file_key")
	}
	return nil
}

func extractSignatureCIDs(content string) map[string]bool {
	refs := map[string]bool{}
	for _, src := range extractSignatureImageSources(content) {
		if strings.HasPrefix(strings.ToLower(src), "cid:") {
			cid := strings.TrimSpace(src[len("cid:"):])
			if cid != "" {
				refs[cid] = true
			}
		}
	}
	return refs
}

func extractSignatureImageSources(content string) []string {
	matches := imgSrcRe.FindAllStringSubmatch(content, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		for _, idx := range []int{2, 3, 4} {
			if match[idx] != "" {
				out = append(out, strings.TrimSpace(match[idx]))
				break
			}
		}
	}
	return out
}

func stringField(item map[string]interface{}, key string) string {
	switch v := item[key].(type) {
	case string:
		return v
	case float64:
		return strconv.FormatInt(int64(v), 10)
	case json.Number:
		return v.String()
	default:
		return ""
	}
}

func int32Field(item map[string]interface{}, key string) int32 {
	switch v := item[key].(type) {
	case float64:
		return int32(v)
	case string:
		n, _ := strconv.ParseInt(v, 10, 32)
		return int32(n)
	case json.Number:
		n, _ := strconv.ParseInt(v.String(), 10, 32)
		return int32(n)
	default:
		return 0
	}
}

func signatureOutput(sig *signature.Signature, lang string) map[string]interface{} {
	if sig == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"signature":         sig,
		"id":                sig.ID,
		"name":              sig.Name,
		"signature_device":  sig.SignatureDevice,
		"content_preview":   contentPreview(sig.Content, 200, lang),
		"signature_type":    sig.SignatureType,
		"images_count":      len(sig.Images),
		"changed_fields":    []string{},
		"last_write_policy": "last-write-wins",
	}
}

func signatureChangedFields(fields ...string) []string {
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field) != "" {
			out = append(out, field)
		}
	}
	return out
}

func formatSignatureSummary(w io.Writer, action string, sig *signature.Signature, lang string) {
	fmt.Fprintf(w, "Signature %s.\n", action)
	if sig == nil {
		return
	}
	fmt.Fprintf(w, "signature_id: %s\n", sig.ID)
	fmt.Fprintf(w, "name: %s\n", sig.Name)
	fmt.Fprintf(w, "signature_device: %s\n", sig.SignatureDevice)
	if preview := contentPreview(sig.Content, 200, lang); preview != "" {
		fmt.Fprintf(w, "content_preview: %s\n", preview)
	}
}
