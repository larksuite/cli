// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import "github.com/larksuite/cli/shortcuts/common"

func authTypes() []string {
	return []string{"user", "bot"}
}

func baseTokenFlag(required bool) common.Flag {
	return common.Flag{Name: "base-token", Desc: "base token", Required: required}
}

func tableRefFlag(required bool) common.Flag {
	return common.Flag{Name: "table-id", Desc: "table ID (must start with tbl if ID) or name", Required: required}
}

func fieldRefFlag(required bool) common.Flag {
	return common.Flag{Name: "field-id", Desc: "field ID or name", Required: required}
}

func viewRefFlag(required bool) common.Flag {
	return common.Flag{Name: "view-id", Desc: "view ID or name", Required: required}
}

func recordRefFlag(required bool) common.Flag {
	return common.Flag{Name: "record-id", Desc: "record ID", Required: required}
}
