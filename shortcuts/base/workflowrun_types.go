// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import "github.com/larksuite/cli/extension/fileio"

func fileSaveOptions() fileio.SaveOptions {
	return fileio.SaveOptions{ContentType: "application/x-ndjson"}
}
