// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package contact

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	selfAvatarImagePath          = "/open-apis/im/v1/images"
	selfAvatarImageResourceScope = "im:resource"
	selfAvatarImageLimit         = int64(10 * 1024 * 1024)
)

type selfAvatarImageKey struct {
	ImageKey string `json:"image_key"`
}

// Read and close the avatar file successfully before sending any write request.
func uploadSelfAvatar(runtime *common.RuntimeContext, path string) (string, error) {
	files := runtime.FileIO()
	if files == nil {
		return "", errs.NewInternalError(errs.SubtypeUnknown, "avatar FileIO is unavailable")
	}
	info, err := files.Stat(path)
	if err != nil {
		return "", selfAvatarFileError(err, "stat")
	}
	if !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > selfAvatarImageLimit {
		return "", common.ValidationErrorf("--avatar-file must reference a non-empty regular image file at most 10 MiB").WithParam("--avatar-file")
	}
	file, err := files.Open(path)
	if err != nil {
		return "", selfAvatarFileError(err, "open")
	}
	// Bound the actual read even if the file grows after Stat, and close before upload.
	data, readErr := io.ReadAll(io.LimitReader(file, selfAvatarImageLimit+1))
	closeErr := file.Close()
	if readErr != nil || closeErr != nil {
		return "", selfAvatarFileError(errors.Join(readErr, closeErr), "read or close")
	}
	if len(data) == 0 || int64(len(data)) > selfAvatarImageLimit {
		return "", common.ValidationErrorf("--avatar-file image must be non-empty and at most 10 MiB").WithParam("--avatar-file")
	}
	form := larkcore.NewFormdata().AddField("image_type", "avatar").
		AddFileWithName("image", filepath.Base(path), bytes.NewReader(data))
	response, err := callSelfAvatarAPI[selfAvatarImageKey](runtime, "POST", selfAvatarImagePath, form)
	if err != nil {
		return "", err
	}
	key := response.ImageKey
	if strings.TrimSpace(key) == "" {
		return "", contactInvalidResponseError("image upload returned no image_key; avatar update was not sent")
	}
	return key, nil
}

func selfAvatarFileError(err error, operation string) error {
	if errs.IsTyped(err) {
		return err
	}
	return common.ValidationErrorf("cannot %s --avatar-file image; no avatar update was sent", operation).
		WithParam("--avatar-file").WithCause(err)
}
