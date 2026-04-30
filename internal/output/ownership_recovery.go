// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package output

func buildOwnershipRecoveryHint() string {
	return "This resource belongs to another user — you can't send it directly. Download it with 'im +messages-resources-download --output <output_path>', then send the local file via 'im +send..'. For post or interactive, upload first and use the new image_key or file_key."
}
