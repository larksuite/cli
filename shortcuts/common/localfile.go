// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"errors"
	"io"
	"io/fs"
	"math"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/validate"
)

// ValidateLocalFileFlag validates that a local input path exists, is a regular
// file, and does not exceed maxBytes. Absolute and relative paths use
// the process filesystem namespace.
func (ctx *RuntimeContext) ValidateLocalFileFlag(flagName string, maxBytes int64) error {
	path, param, err := ctx.localFileFlag(flagName, maxBytes)
	if err != nil {
		return err
	}

	info, err := cmdutil.StatLocalFile(path)
	if err != nil {
		return localFileReadError(param, path, "inspect", err)
	}
	if err := localFileRegularError(param, path, info.Mode()); err != nil {
		return err
	}
	if info.Size() > maxBytes {
		return localFileSizeError(param, path, info.Size(), maxBytes)
	}
	return nil
}

// ReadLocalFileFlag is the shared replacement for direct os.ReadFile calls in
// shortcuts. It accepts absolute and relative paths, enforces a hard size
// limit, and returns command-facing typed errors.
func (ctx *RuntimeContext) ReadLocalFileFlag(flagName string, maxBytes int64) (data []byte, retErr error) {
	path, param, err := ctx.localFileFlag(flagName, maxBytes)
	if err != nil {
		return nil, err
	}
	f, err := cmdutil.OpenLocalFile(path)
	if err != nil {
		return nil, localFileReadError(param, path, "open", err)
	}
	defer func() {
		if err := f.Close(); err != nil && retErr == nil {
			data = nil
			retErr = errs.NewInternalError(errs.SubtypeFileIO, "cannot close %s %q: %v", param, path, err).WithCause(err)
		}
	}()

	openedInfo, err := f.Stat()
	if err != nil {
		return nil, localFileReadError(param, path, "inspect opened", err)
	}
	if err := localFileRegularError(param, path, openedInfo.Mode()); err != nil {
		return nil, err
	}
	if openedInfo.Size() > maxBytes {
		return nil, localFileSizeError(param, path, openedInfo.Size(), maxBytes)
	}

	readLimit := maxBytes + 1
	if maxBytes == math.MaxInt64 {
		readLimit = maxBytes
	}
	data, err = io.ReadAll(io.LimitReader(f, readLimit))
	if err != nil {
		return nil, localFileReadError(param, path, "read", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument,
			"%s %q grew beyond the %d-byte limit while being read", param, path, maxBytes).
			WithParam(param)
	}
	return data, nil
}

func (ctx *RuntimeContext) localFileFlag(flagName string, maxBytes int64) (path, param string, err error) {
	name, param, err := localFileFlagNames(flagName)
	if err != nil {
		return "", "", err
	}
	if ctx == nil || ctx.Cmd == nil {
		return "", param, errs.NewInternalError(errs.SubtypeUnknown, "cannot read %s: runtime command is unavailable", param)
	}

	path = strings.TrimSpace(ctx.Str(name))
	if path == "" {
		return "", param, errs.NewValidationError(errs.SubtypeInvalidArgument, "%s is required", param).WithParam(param)
	}
	if _, err := validate.LocalInputPath(path); err != nil {
		return "", param, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid %s path: %v", param, err).
			WithParam(param).
			WithCause(err)
	}
	if maxBytes < 0 {
		return "", param, errs.NewInternalError(errs.SubtypeUnknown, "invalid read limit configured for %s", param)
	}
	return path, param, nil
}

func localFileRegularError(param, path string, mode fs.FileMode) error {
	if mode.IsRegular() {
		return nil
	}
	return errs.NewValidationError(errs.SubtypeInvalidArgument,
		"%s %q is not a regular file", param, path).
		WithParam(param)
}

func localFileReadError(param, path, op string, cause error) error {
	if errors.Is(cause, fileio.ErrPathValidation) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid %s %q: %v", param, path, cause).
			WithParam(param).
			WithCause(cause)
	}
	if errors.Is(cause, fs.ErrNotExist) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s %q does not exist", param, path).
			WithParam(param).
			WithCause(cause)
	}
	return errs.NewInternalError(errs.SubtypeFileIO, "cannot %s %s %q: %v", op, param, path, cause).WithCause(cause)
}

func localFileSizeError(param, path string, size, limit int64) error {
	return errs.NewValidationError(errs.SubtypeInvalidArgument,
		"%s %q is %d bytes; limit is %d bytes", param, path, size, limit).
		WithParam(param)
}

func localFileFlagNames(flagName string) (name, param string, err error) {
	name = strings.TrimLeft(strings.TrimSpace(flagName), "-")
	if name == "" {
		return "", "", errs.NewInternalError(errs.SubtypeUnknown, "local file flag name must not be empty")
	}
	return name, "--" + name, nil
}
