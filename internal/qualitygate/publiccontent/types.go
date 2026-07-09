// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package publiccontent

import "github.com/larksuite/cli/internal/qualitygate/report"

type Options struct {
	Repo         string
	ChangedFrom  string
	MetadataPath string
	BranchName   string
}

type Metadata struct {
	Title  string `json:"title"`
	Body   string `json:"body"`
	Branch string `json:"branch"`
}

type Finding struct {
	Rule       string
	Action     report.Action
	File       string
	Line       int
	Source     string
	Excerpt    string
	Message    string
	Suggestion string
}
