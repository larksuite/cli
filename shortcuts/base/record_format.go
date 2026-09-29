// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

func validateRecordReadFormat(runtime *common.RuntimeContext) error {
	return validateRecordReadFormatValue(runtime.Str("format"))
}

func validateRecordReadFormatValue(format string) error {
	switch format {
	case "ndjson", "json":
		return nil
	case "markdown":
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "deprecated format: markdown is no longer supported").
			WithParam("--format").
			WithHint("Use --format ndjson or omit --format to save records as NDJSON. Use --format json for an inline raw matrix.")
	default:
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--format must be ndjson or json").
			WithParam("--format").WithHint("Use --format ndjson or --format json.")
	}
}
