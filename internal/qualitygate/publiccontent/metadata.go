// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package publiccontent

import (
	"encoding/json"
	"fmt"

	"github.com/larksuite/cli/internal/vfs"
)

func LoadMetadata(path string) (Metadata, error) {
	if path == "" {
		return Metadata{}, nil
	}
	data, err := vfs.ReadFile(path)
	if err != nil {
		return Metadata{}, fmt.Errorf("public content metadata: %w", err)
	}
	if len(data) == 0 {
		return Metadata{}, nil
	}
	var out Metadata
	if err := json.Unmarshal(data, &out); err != nil {
		return Metadata{}, fmt.Errorf("public content metadata: %w", err)
	}
	return out, nil
}

func scanMetadata(m Metadata) []Finding {
	text := ""
	if m.Title != "" {
		text += "title: " + m.Title + "\n"
	}
	if m.Body != "" {
		text += "body:\n" + m.Body + "\n"
	}
	if text == "" {
		return nil
	}
	out := scanText("pull_request_metadata", "metadata", text, false)
	out = append(out, semanticCandidate("pull_request_metadata", "metadata", text, 1)...)
	return out
}
