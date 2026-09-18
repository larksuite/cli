// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"errors"
	"io/fs"
	"path/filepath"

	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/shortcuts/common"
)

// statDocResource keeps file access policy in the invocation's FileIO. Only a
// missing relative resource may fall back to the source XML's directory,
// first with its relative path and then with only its basename;
// permission, policy and content failures must not select a different file.
func statDocResource(runtime *common.RuntimeContext, path string) (string, fileio.FileInfo, error) {
	fio := runtime.FileIO()
	info, err := fio.Stat(path)
	if err == nil || !errors.Is(err, fs.ErrNotExist) || errors.Is(err, fileio.ErrPathValidation) || filepath.IsAbs(path) {
		return path, info, err
	}
	source := runtime.Cmd.Annotations[docsContentPathAnnotation]
	if source == "" || runtime.Str("doc-format") == "markdown" {
		return path, info, err
	}
	for _, candidate := range []string{
		filepath.Join(filepath.Dir(source), path),
		filepath.Join(filepath.Dir(source), filepath.Base(path)),
	} {
		if candidate == filepath.Clean(path) {
			continue
		}
		path = candidate
		info, err = fio.Stat(path)
		if err == nil || !errors.Is(err, fs.ErrNotExist) || errors.Is(err, fileio.ErrPathValidation) {
			return path, info, err
		}
	}
	return path, info, err
}
