// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package registry

import "embed"

//go:embed meta_data*.json
var metaFS embed.FS

//go:embed meta_data_default.json
var embeddedMetaDataDefaultJSON []byte

func init() {
	if data, err := metaFS.ReadFile("meta_data.json"); err == nil && len(data) > 0 {
		embeddedMetaJSON = data
	}
	// When meta_data.json is not compiled in, embeddedMetaJSON remains nil,
	// so hasEmbeddedData() correctly returns false and tests that depend on
	// real embedded services will be skipped rather than failing.
}
