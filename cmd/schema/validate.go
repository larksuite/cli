// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package schema

import (
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/schema"
)

type inputValidationSuccess struct {
	SchemaPath string `json:"schema_path"`
	Valid      bool   `json:"valid"`
}

func schemaValidateRun(opts *SchemaOptions, visibility CommandVisibility) error {
	factory := opts.Factory
	target, refs, err := resolveSchemaTarget(apicatalog.ParsePath(opts.Args), factory.ResolveStrictMode(opts.Ctx), factory.APICatalog, visibility)
	if err != nil {
		return err
	}
	if target.Kind != apicatalog.TargetMethod {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "input validation requires one API method path").
			WithParam("path").WithHint("use `lark-cli schema <service.resource.method> --validate @input.json`")
	}
	// Resolve selectors and visibility before reading stdin or opening a file.
	input, err := cmdutil.ResolveInput(opts.Validate, factory.IOStreams.In, factory.ResolveFileIO(opts.Ctx))
	if err != nil {
		if _, typed := errs.ProblemOf(err); typed {
			return err
		}
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "cannot read validation input").
			WithParam("--validate").WithCause(err).
			WithHint("provide inline JSON, a readable @file, or pipe JSON with --validate -")
	}
	envelope := schema.EnvelopeOf(factory.Affordance, refs[0])
	if envelope.InputSchema == nil {
		return errs.NewInternalError(errs.SubtypeSDKError, "method input schema is unavailable")
	}
	result, err := schema.ValidateInput(envelope.InputSchema, input)
	if err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid validation JSON document").
			WithParam("--validate").WithCause(err).
			WithHint("provide exactly one JSON document with unique object keys and at most %d nesting levels", schema.MaxInputDepth)
	}
	if len(result.Violations) > 0 {
		params := make([]errs.InvalidParam, 0, len(result.Violations))
		for _, violation := range result.Violations {
			params = append(params, errs.InvalidParam{Name: violation.Path, Reason: violation.Reason})
		}
		validation := errs.NewValidationError(errs.SubtypeInvalidArgument, "input does not match the method input schema").
			WithParam("--validate").WithParams(params...).
			WithHint("inspect `lark-cli schema %s`; params names are JSON Pointers (empty means the root)", refs[0].SchemaPath())
		if result.Truncated {
			validation.WithHint("showing the first %d violations; fix these and validate again; params names are JSON Pointers", schema.MaxInputViolations)
		}
		return validation
	}
	return output.WriteSuccessEnvelope(inputValidationSuccess{SchemaPath: refs[0].SchemaPath(), Valid: true}, output.SuccessEnvelopeOptions{
		CommandPath: "schema", Out: factory.IOStreams.Out, ErrOut: factory.IOStreams.ErrOut,
	})
}
