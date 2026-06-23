// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package report

import (
	"fmt"
	"io"
	"sort"
)

type Action string

const (
	ActionReject  Action = "REJECT"
	ActionLabel   Action = "LABEL"
	ActionWarning Action = "WARNING"
)

type Diagnostic struct {
	Rule        string `json:"rule"`
	Action      Action `json:"action"`
	File        string `json:"file"`
	Line        int    `json:"line"`
	Message     string `json:"message"`
	Suggestion  string `json:"suggestion,omitempty"`
	SubjectType string `json:"subject_type,omitempty"`
	CommandPath string `json:"command_path,omitempty"`
	FlagName    string `json:"flag_name,omitempty"`
}

func Print(w io.Writer, ds []Diagnostic) {
	sort.SliceStable(ds, func(i, j int) bool {
		if ds[i].File != ds[j].File {
			return ds[i].File < ds[j].File
		}
		if ds[i].Line != ds[j].Line {
			return ds[i].Line < ds[j].Line
		}
		return ds[i].Rule < ds[j].Rule
	})

	for _, d := range ds {
		fmt.Fprintf(w, "%s:%d: [%s/%s] %s\n", d.File, d.Line, d.Action, d.Rule, d.Message)
		if d.Suggestion != "" {
			fmt.Fprintf(w, "    hint: %s\n", d.Suggestion)
		}
	}
}

func ExitCode(ds []Diagnostic) int {
	for _, d := range ds {
		if d.Action == ActionReject {
			return 1
		}
	}
	return 0
}
