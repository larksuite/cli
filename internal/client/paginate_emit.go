// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package client

import (
	"context"
	"io"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// PaginateOutputOptions bundles the inputs for PaginateToOutput. Grouping the
// writers, callbacks, and pagination knobs into one struct keeps the call sites
// readable and avoids positional-argument mistakes across the many parameters.
type PaginateOutputOptions struct {
	Client      *APIClient
	Request     RawApiRequest
	Format      output.Format
	JqExpr      string
	Out         io.Writer
	ErrOut      io.Writer
	CommandPath string
	Pagination  PaginationOptions
	CheckErr    func(interface{}, core.Identity) error
	MarkErr     func(error) error
}

// PaginateToOutput fetches all requested pages and emits them in the selected format.
func PaginateToOutput(ctx context.Context, opts PaginateOutputOptions) error {
	ac := opts.Client
	request := opts.Request
	format := opts.Format
	jqExpr := opts.JqExpr
	out := opts.Out
	errOut := opts.ErrOut
	commandPath := opts.CommandPath
	pagOpts := opts.Pagination
	checkErr := opts.CheckErr
	markErr := opts.MarkErr
	if !format.Valid() || format == output.FormatPretty {
		return errs.NewInternalError(errs.SubtypeUnknown,
			"internal: unsupported pagination output format %q", format)
	}
	if markErr == nil {
		markErr = func(err error) error { return err }
	}
	if pagOpts.Identity == "" {
		pagOpts.Identity = request.As
	}
	emitValue := func(data interface{}, valueFormat output.Format) error {
		emitter := output.NewEmitter(output.EmitterConfig{
			Out:         out,
			ErrOut:      errOut,
			CommandPath: commandPath,
			Identity:    string(pagOpts.Identity),
		})
		return emitter.Value(data, output.StreamOptions{Format: valueFormat})
	}
	// When jq is set, always aggregate all pages then filter.
	if jqExpr != "" {
		result, err := ac.PaginateAll(ctx, request, pagOpts)
		if err != nil {
			return markErr(err)
		}
		if apiErr := checkErr(result, pagOpts.Identity); apiErr != nil {
			if emitErr := emitValue(result, output.FormatJSON); emitErr != nil {
				return markErr(emitErr)
			}
			return markErr(apiErr)
		}
		return output.WriteSuccessEnvelope(output.SuccessEnvelopeData(result), output.SuccessEnvelopeOptions{
			CommandPath: commandPath,
			Identity:    string(pagOpts.Identity),
			JqExpr:      jqExpr,
			Out:         out,
			ErrOut:      errOut,
		})
	}

	switch format {
	case output.FormatNDJSON, output.FormatTable, output.FormatCSV:
		emitter := output.NewEmitter(output.EmitterConfig{
			Out:            out,
			ErrOut:         errOut,
			CommandPath:    commandPath,
			Identity:       string(pagOpts.Identity),
			NoticeProvider: output.GetNotice,
		})
		result, hasItems, err := ac.StreamPages(ctx, request, func(items []interface{}) error {
			return emitter.StreamPage(items, output.StreamOptions{Format: format})
		}, pagOpts)
		if err != nil && errs.IsContentSafety(err) {
			return markErr(err)
		}
		if finishErr := emitter.FinishStream(); finishErr != nil {
			return markErr(finishErr)
		}
		if err != nil {
			return markErr(err)
		}
		if apiErr := checkErr(result, pagOpts.Identity); apiErr != nil {
			return markErr(apiErr)
		}
		if !hasItems {
			return emitter.Value(output.SuccessEnvelopeData(result), output.StreamOptions{Format: format})
		}
		return nil
	default:
		result, err := ac.PaginateAll(ctx, request, pagOpts)
		if err != nil {
			return markErr(err)
		}
		if apiErr := checkErr(result, pagOpts.Identity); apiErr != nil {
			if emitErr := emitValue(result, output.FormatJSON); emitErr != nil {
				return markErr(emitErr)
			}
			return markErr(apiErr)
		}
		return output.WriteSuccessEnvelope(output.SuccessEnvelopeData(result), output.SuccessEnvelopeOptions{
			CommandPath: commandPath,
			Identity:    string(pagOpts.Identity),
			Out:         out,
			ErrOut:      errOut,
		})
	}
}
