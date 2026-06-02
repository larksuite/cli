// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

// bootstrapManifestJSON is the lark-sec-cli release manifest shipped with this
// lark-cli build. It points directly at TOS so a fresh install does not depend
// on any external release-tracking service — first install is fully self-contained.
//
// Updating this file pins a new default version of lark-sec-cli for users who
// install via lark-cli. After install, lark-sec-cli is in charge of finding and
// applying its own updates; lark-cli does not consult any release server.
//
//go:embed bootstrap.json
var bootstrapManifestJSON []byte

// LoadBootstrap parses the embedded bootstrap manifest into a Manifest value.
func LoadBootstrap() (*Manifest, error) {
	var entries []Entry
	if err := json.Unmarshal(bootstrapManifestJSON, &entries); err != nil {
		return nil, fmt.Errorf("decode embedded bootstrap manifest: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("embedded bootstrap manifest is empty")
	}
	return &Manifest{Entries: entries}, nil
}
