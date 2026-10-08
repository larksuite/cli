// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package slides

import (
	"context"
	"net/http"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
)

const slidesLintAPIPath = "/open-apis/slides_ai/v1/xml_presentation/lint"

// SlidesLint validates a complete presentation XML document without creating
// or changing a presentation.
var SlidesLint = common.Shortcut{
	Service:     "slides",
	Command:     "+lint",
	Description: "Validate Slides presentation XML without creating or changing a presentation",
	Risk:        "read",
	Scopes:      []string{"slides:presentation:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "content", Aliases: contentFlagAliases, Desc: "presentation XML content to validate", Required: true, Input: []string{common.File, common.Stdin}},
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if strings.TrimSpace(runtime.Str("content")) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--content cannot be empty").WithParam("--content")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().
			Desc("Validate presentation XML").
			POST(slidesLintAPIPath).
			Body(slidesLintBody(runtime.Str("content")))
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		data, err := runtime.CallAPITyped(http.MethodPost, slidesLintAPIPath, nil, slidesLintBody(runtime.Str("content")))
		if err != nil {
			return err
		}
		runtime.OutRaw(data, nil)
		return nil
	},
}

func slidesLintBody(content string) map[string]interface{} {
	return map[string]interface{}{"content": content}
}
