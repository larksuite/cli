// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
//
// note +detail — get note metadata and document tokens by a known note_id.

package note

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

// NoteDetail queries note metadata, display type and document tokens by note_id.
var NoteDetail = common.Shortcut{
	Service:     "note",
	Command:     "+detail",
	Description: "Get note detail (display type, document tokens) by note_id",
	Risk:        "read",
	Scopes:      []string{"vc:note:read"},
	AuthTypes:   []string{"user"},
	Flags: []common.Flag{
		{Name: "note-id", Desc: "note ID", Required: true},
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		noteID := strings.TrimSpace(runtime.Str("note-id"))
		if noteID == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--note-id is required").WithParam("--note-id")
		}
		if err := validate.ResourceName(noteID, "--note-id"); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "%s", err).WithParam("--note-id").WithCause(err)
		}
		return nil
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		noteID := strings.TrimSpace(runtime.Str("note-id"))
		return common.NewDryRunAPI().
			GET(fmt.Sprintf("/open-apis/vc/v1/notes/%s", validate.EncodePathSegment(noteID)))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		noteID := strings.TrimSpace(runtime.Str("note-id"))
		detail, err := FetchDetail(ctx, runtime, noteID)
		if err != nil {
			return mapNoteError(err)
		}
		runtime.OutFormat(map[string]any{"note": detail.ToMap()}, nil, nil)
		return nil
	},
}

// mapNoteError surfaces the no-permission case explicitly and passes through
// any other typed API error unchanged.
func mapNoteError(err error) error {
	if problem, ok := errs.ProblemOf(err); ok && problem.Code == NoNoteReadPermissionCode {
		message := strings.TrimSpace(problem.Message)
		if message == "" {
			message = "no read permission for this note"
		} else if !strings.Contains(message, "no read permission for this note") {
			message = fmt.Sprintf("no read permission for this note: %s", message)
		}
		var permErr *errs.PermissionError
		if errors.As(err, &permErr) {
			mapped := *permErr
			mapped.Problem.Message = message
			if mapped.Problem.Hint == "" {
				mapped.Problem.Hint = "Ask the note owner to grant read permission, then retry"
			}
			mapped.Cause = err
			return &mapped
		}
		mappedProblem := *problem
		mappedProblem.Category = errs.CategoryAuthorization
		mappedProblem.Subtype = errs.SubtypePermissionDenied
		mappedProblem.Message = message
		if mappedProblem.Hint == "" {
			mappedProblem.Hint = "Ask the note owner to grant read permission, then retry"
		}
		return &errs.PermissionError{Problem: mappedProblem, Cause: err}
	}
	return err
}
