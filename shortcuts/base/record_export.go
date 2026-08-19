// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/base/recordexport"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	maxInlineRecordReadLimit = 200
	ndjsonRecordPageSize     = 500
	maxNDJSONRecordReadLimit = 2000
	recordAnalysisOutputTip  = "If file I/O is available, prefer --format ndjson --output ./records.ndjson for analysis, parsing, or comparison to keep long user data out of model context; process the records file with Python or another data analysis engine. Follow lark-base-record-query-and-analysis-sop.md for engine selection and complete-data checks. Ndjson defaults to limit 2000, so set a smaller --limit only for probes, previews, or an explicitly bounded result."
)

var recordExportNow = time.Now

func recordOutputFlag() common.Flag {
	return common.Flag{
		Name: "output",
		Desc: "preferred analysis output: relative .ndjson output path; implies --format ndjson when format is omitted",
	}
}

func recordMinimalStdoutFlag() common.Flag {
	return common.Flag{
		Name: "minimal-stdout", Type: "bool", Hidden: true,
		Desc: "for ndjson, print only artifact paths, record file size, records_count, and has_more",
	}
}

func recordJQRecordsFlag() common.Flag {
	return common.Flag{
		Name: "jq-records", Hidden: true,
		Desc: "required instead of --jq to process ndjson records with the built-in jq engine; runs once against the exported records array and leaves artifact files unchanged",
	}
}

func recordOverwriteFlag() common.Flag {
	return common.Flag{
		Name: "overwrite", Type: "bool",
		Desc: "replace an existing ndjson artifact and its manifest",
	}
}

func normalizeRecordReadOutput(_ context.Context, flags *common.FlagContext) error {
	if strings.TrimSpace(flags.Str("output")) != "" {
		if flags.Changed("format") && flags.Str("format") != recordexport.FormatNDJSON {
			return errs.NewValidationError(
				errs.SubtypeInvalidArgument,
				"--output writes an ndjson artifact and conflicts with --format %s",
				flags.Str("format"),
			).
				WithParam("--output").
				WithParams(
					errs.InvalidParam{Name: "--output", Reason: "requires ndjson"},
					errs.InvalidParam{Name: "--format", Reason: "conflicts with --output"},
				).
				WithHint("Remove --format or set --format ndjson.")
		}
		if !flags.Changed("format") {
			if err := flags.SetCanonicalFrom("output", "format", recordexport.FormatNDJSON); err != nil {
				return err
			}
		}
	}
	if flags.Str("format") == recordexport.FormatNDJSON && strings.TrimSpace(flags.Str("jq")) != "" {
		return errs.NewValidationError(
			errs.SubtypeInvalidArgument,
			"--jq does not support ndjson; process the saved records file with Python or another data analysis engine",
		).
			WithParam("--jq").
			WithHint("Remove --jq and process the NDJSON records file after export.")
	}
	return nil
}

func normalizeRecordNDJSONLimit(_ context.Context, flags *common.FlagContext) error {
	if flags.Str("format") != recordexport.FormatNDJSON || flags.Changed("limit") {
		return nil
	}
	return flags.SetCanonical("limit", strconv.Itoa(maxNDJSONRecordReadLimit))
}

func normalizeRecordSearchOutput(ctx context.Context, flags *common.FlagContext) error {
	if err := normalizeRecordReadOutput(ctx, flags); err != nil {
		return err
	}
	if strings.TrimSpace(flags.Str("json")) != "" || flags.Changed("limit") {
		return nil
	}
	limit := 10
	if flags.Str("format") == recordexport.FormatNDJSON {
		limit = maxNDJSONRecordReadLimit
	}
	return flags.SetCanonical("limit", strconv.Itoa(limit))
}

func validateRecordExportFlags(runtime *common.RuntimeContext) error {
	format := runtime.Str("format")
	outputPath := strings.TrimSpace(runtime.Str("output"))
	jqRecords := strings.TrimSpace(runtime.Str("jq-records"))
	if format != recordexport.FormatNDJSON {
		switch {
		case outputPath != "":
			return baseFlagErrorf("--output requires --format ndjson")
		case runtime.Bool("minimal-stdout"):
			return baseFlagErrorf("--minimal-stdout requires --format ndjson")
		case jqRecords != "":
			return baseFlagErrorf("--jq-records requires --format ndjson")
		case runtime.Bool("overwrite"):
			return baseFlagErrorf("--overwrite requires --format ndjson")
		}
		return nil
	}
	if jqRecords != "" {
		switch {
		case runtime.JqExpr != "":
			return baseFlagErrorf("--jq-records and --jq are mutually exclusive")
		case runtime.Bool("minimal-stdout"):
			return baseFlagErrorf("--jq-records and --minimal-stdout are mutually exclusive")
		}
		if err := output.ValidateJqExpression(jqRecords); err != nil {
			return err
		}
	}
	if outputPath != "" {
		if filepath.Ext(outputPath) != ".ndjson" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output must end with .ndjson").
				WithParam("--output").
				WithHint("Use a path such as ./exports/records.ndjson.")
		}
		fio := runtime.FileIO()
		if fio == nil {
			return baseMissingFileIOError("record export requires a file I/O provider")
		}
		if _, err := fio.ResolvePath(outputPath); err != nil {
			return baseSaveError(err)
		}
	}
	return nil
}

// validateRecordReadLimit intentionally runs after format normalization so an
// inferred ndjson format receives the 2000-row bound instead of the inline
// 200-row bound.
func validateRecordReadLimit(runtime *common.RuntimeContext, defaultLimit int) error {
	maximum := maxInlineRecordReadLimit
	if runtime.Str("format") == recordexport.FormatNDJSON {
		maximum = maxNDJSONRecordReadLimit
	}
	_, err := common.ValidatePageSizeTyped(runtime, "limit", defaultLimit, 1, maximum)
	return err
}

type recordExportAccumulator struct {
	dataset        recordexport.Dataset
	rev            *int64
	initialized    bool
	pageCount      int
	hasMore        bool
	queryContext   map[string]any
	ignoredFields  []recordexport.IgnoredField
	recordNotFound []string
}

func (a *recordExportAccumulator) append(page recordexport.Page) error {
	if !a.initialized {
		a.dataset = page.Dataset
		a.rev = page.Rev
		a.queryContext = page.QueryContext
		a.initialized = true
	} else if err := a.dataset.AppendPage(page); err != nil {
		return err
	}
	a.pageCount++
	a.hasMore = page.HasMore
	a.ignoredFields = appendUniqueIgnoredFields(a.ignoredFields, page.IgnoredFields)
	a.recordNotFound = appendUniqueStrings(a.recordNotFound, page.RecordNotFound)
	return nil
}

func parseRecordExportPage(data map[string]any) (recordexport.Page, error) {
	page, err := recordexport.ParseMatrix(data)
	if err == nil {
		return page, nil
	}
	return recordexport.Page{}, errs.NewInternalError(
		errs.SubtypeInvalidResponse, "cannot export record matrix: %v", err,
	).WithCause(err)
}

func appendRecordExportPage(accumulator *recordExportAccumulator, page recordexport.Page) error {
	if err := accumulator.append(page); err != nil {
		var schemaChanged *recordexport.SchemaChangedError
		if errors.As(err, &schemaChanged) {
			return errs.NewValidationError(
				errs.SubtypeFailedPrecondition,
				"table schema changed during download; this request failed",
			).
				WithHint("Retry the request so every page uses one consistent schema.").
				WithCause(err)
		}
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "cannot append record page: %v", err).WithCause(err)
	}
	return nil
}

func executeRecordListNDJSON(
	runtime *common.RuntimeContext,
	baseParams map[string]any,
	startOffset int,
	requestedLimit int,
) error {
	accumulator := &recordExportAccumulator{}
	currentOffset := startOffset
	remaining := requestedLimit
	for remaining > 0 {
		pageLimit := min(remaining, ndjsonRecordPageSize)
		params := cloneMap(baseParams)
		params["offset"] = currentOffset
		params["limit"] = pageLimit
		data, err := baseV3Call(runtime, "GET", baseV3Path(
			"bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records",
		), params, nil)
		if err != nil {
			return err
		}
		page, err := parseRecordExportPage(data)
		if err != nil {
			return err
		}
		if len(page.Dataset.Records) > pageLimit {
			return errs.NewInternalError(
				errs.SubtypeInvalidResponse,
				"record API returned %d rows for page limit %d", len(page.Dataset.Records), pageLimit,
			)
		}
		if err := appendRecordExportPage(accumulator, page); err != nil {
			return err
		}
		count := len(page.Dataset.Records)
		remaining -= count
		currentOffset += count
		if !page.HasMore || count == 0 {
			break
		}
	}
	return finalizeRecordExport(runtime, accumulator, startOffset, requestedLimit)
}

func executeRecordSearchNDJSON(runtime *common.RuntimeContext, requestBody map[string]any) error {
	startOffset, requestedLimit, err := recordSearchPagination(requestBody)
	if err != nil {
		return err
	}
	accumulator := &recordExportAccumulator{}
	currentOffset := startOffset
	remaining := requestedLimit
	for remaining > 0 {
		pageLimit := min(remaining, ndjsonRecordPageSize)
		body := cloneMap(requestBody)
		body["offset"] = currentOffset
		body["limit"] = pageLimit
		data, err := baseV3Call(runtime, "POST", baseV3Path(
			"bases", runtime.Str("base-token"), "tables", baseTableID(runtime), "records", "search",
		), nil, body)
		if err != nil {
			return err
		}
		page, err := parseRecordExportPage(data)
		if err != nil {
			return err
		}
		if len(page.Dataset.Records) > pageLimit {
			return errs.NewInternalError(
				errs.SubtypeInvalidResponse,
				"record search API returned %d rows for page limit %d", len(page.Dataset.Records), pageLimit,
			)
		}
		if err := appendRecordExportPage(accumulator, page); err != nil {
			return err
		}
		count := len(page.Dataset.Records)
		remaining -= count
		currentOffset += count
		if !page.HasMore || count == 0 {
			break
		}
	}
	return finalizeRecordExport(runtime, accumulator, startOffset, requestedLimit)
}

func executeRecordGetNDJSON(runtime *common.RuntimeContext, data map[string]any, requestedRecordCount int) error {
	page, err := parseRecordExportPage(data)
	if err != nil {
		return err
	}
	page.QueryContext = cloneMap(page.QueryContext)
	if page.QueryContext == nil {
		page.QueryContext = make(map[string]any, 2)
	}
	page.QueryContext["record_scope"] = "selected_record_ids"
	page.QueryContext["requested_record_count"] = requestedRecordCount
	accumulator := &recordExportAccumulator{}
	if err := appendRecordExportPage(accumulator, page); err != nil {
		return err
	}
	return finalizeRecordExport(runtime, accumulator, 0, 0)
}

func finalizeRecordExport(
	runtime *common.RuntimeContext,
	accumulator *recordExportAccumulator,
	startOffset int,
	requestedLimit int,
) error {
	if !accumulator.initialized {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "record export received no matrix page")
	}
	paths, err := resolveRecordExportPaths(runtime)
	if err != nil {
		return err
	}
	fio := runtime.FileIO()
	if fio == nil {
		return baseMissingFileIOError("record export requires a file I/O provider")
	}
	if err := ensureRecordExportTargets(fio, paths, runtime.Bool("overwrite"), runtime.Changed("output")); err != nil {
		return err
	}

	scanResult := output.ScanForSafety(runtime.Cmd.CommandPath(), accumulator.dataset, runtime.IO().ErrOut)
	if scanResult.Blocked {
		return baseContentSafetyBlockError(scanResult)
	}
	if scanResult.Alert != nil {
		output.WriteAlertWarning(runtime.IO().ErrOut, scanResult.Alert)
	}

	recordFileSizeBytes, err := saveRecordNDJSON(fio, paths.recordRelative, accumulator.dataset)
	if err != nil {
		return err
	}
	manifest := recordexport.BuildManifest(accumulator.dataset, recordexport.ManifestOptions{
		BaseToken:           runtime.Str("base-token"),
		TableID:             baseTableID(runtime),
		Rev:                 accumulator.rev,
		QueryContext:        accumulator.queryContext,
		Offset:              startOffset,
		RequestedLimit:      requestedLimit,
		PageCount:           accumulator.pageCount,
		HasMore:             accumulator.hasMore,
		RecordFile:          paths.recordAbsolute,
		RecordFileSizeBytes: recordFileSizeBytes,
		ManifestFile:        paths.manifestAbsolute,
		IgnoredFields:       accumulator.ignoredFields,
		RecordNotFound:      accumulator.recordNotFound,
	})
	if err := saveRecordManifest(fio, paths.manifestRelative, manifest); err != nil {
		return err
	}
	return outputRecordExportResult(runtime, accumulator.dataset, manifest)
}

type recordExportPaths struct {
	recordRelative   string
	recordAbsolute   string
	manifestRelative string
	manifestAbsolute string
}

func resolveRecordExportPaths(runtime *common.RuntimeContext) (recordExportPaths, error) {
	recordPath := strings.TrimSpace(runtime.Str("output"))
	if recordPath == "" {
		now := recordExportNow()
		recordPath = fmt.Sprintf(
			"%s_%s_%03d.ndjson",
			baseTableID(runtime), now.Format("20060102_150405"), now.Nanosecond()/int(time.Millisecond),
		)
	}
	manifestPath := strings.TrimSuffix(recordPath, ".ndjson") + ".manifest.json"
	fio := runtime.FileIO()
	if fio == nil {
		return recordExportPaths{}, baseMissingFileIOError("record export requires a file I/O provider")
	}
	recordAbsolute, err := fio.ResolvePath(recordPath)
	if err != nil {
		return recordExportPaths{}, baseSaveError(err)
	}
	manifestAbsolute, err := fio.ResolvePath(manifestPath)
	if err != nil {
		return recordExportPaths{}, baseSaveError(err)
	}
	return recordExportPaths{
		recordRelative: recordPath, recordAbsolute: recordAbsolute,
		manifestRelative: manifestPath, manifestAbsolute: manifestAbsolute,
	}, nil
}

func ensureRecordExportTargets(fio fileio.FileIO, paths recordExportPaths, overwrite bool, explicitOutput bool) error {
	if overwrite {
		return nil
	}
	for _, path := range []string{paths.recordRelative, paths.manifestRelative} {
		if _, err := fio.Stat(path); err == nil {
			validationErr := errs.NewValidationError(
				errs.SubtypeFailedPrecondition, "output file already exists: %s", path,
			).WithHint("Pass --overwrite to replace the ndjson artifact and manifest.")
			if explicitOutput {
				validationErr = validationErr.WithParam("--output")
			}
			return validationErr
		} else if !errors.Is(err, fs.ErrNotExist) {
			return baseSaveError(err)
		}
	}
	return nil
}

func saveRecordNDJSON(fio fileio.FileIO, path string, dataset recordexport.Dataset) (int64, error) {
	reader, writer := io.Pipe()
	writeDone := make(chan error, 1)
	go func() {
		err := recordexport.WriteNDJSON(writer, dataset)
		_ = writer.CloseWithError(err)
		writeDone <- err
	}()
	saveResult, saveErr := fio.Save(path, fileio.SaveOptions{
		ContentType: "application/x-ndjson", ContentLength: -1,
	}, reader)
	_ = reader.CloseWithError(saveErr)
	writeErr := <-writeDone
	if saveErr != nil {
		return 0, baseSaveError(saveErr)
	}
	if writeErr != nil {
		return 0, errs.NewInternalError(errs.SubtypeInvalidResponse, "cannot encode ndjson records: %v", writeErr).WithCause(writeErr)
	}
	if saveResult == nil {
		return 0, errs.NewInternalError(errs.SubtypeFileIO, "record export did not report the saved ndjson size")
	}
	return saveResult.Size(), nil
}

func saveRecordManifest(fio fileio.FileIO, path string, manifest recordexport.Manifest) error {
	var buffer bytes.Buffer
	if err := recordexport.WriteManifest(&buffer, manifest); err != nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "cannot encode record manifest: %v", err).WithCause(err)
	}
	if _, err := fio.Save(path, fileio.SaveOptions{
		ContentType: "application/json", ContentLength: int64(buffer.Len()),
	}, &buffer); err != nil {
		return baseSaveError(err)
	}
	return nil
}

func outputRecordExportResult(
	runtime *common.RuntimeContext,
	dataset recordexport.Dataset,
	manifest recordexport.Manifest,
) error {
	if jqRecords := strings.TrimSpace(runtime.Str("jq-records")); jqRecords != "" {
		records := make([]any, 0, len(dataset.Records))
		for _, record := range dataset.Records {
			object := make(map[string]any, len(dataset.Columns))
			for columnIndex, column := range dataset.Columns {
				object[column.Name] = record.Values[columnIndex]
			}
			records = append(records, object)
		}
		return output.JqFilter(runtime.IO().Out, records, jqRecords)
	}

	var value any = manifest
	if runtime.Bool("minimal-stdout") {
		value = manifest.Minimal()
	}
	scanResult := output.ScanForSafety(runtime.Cmd.CommandPath(), value, runtime.IO().ErrOut)
	if scanResult.Blocked {
		return baseContentSafetyBlockError(scanResult)
	}
	if scanResult.Alert != nil {
		output.WriteAlertWarning(runtime.IO().ErrOut, scanResult.Alert)
	}
	if runtime.JqExpr != "" {
		return output.JqFilter(runtime.IO().Out, value, runtime.JqExpr)
	}
	if err := output.WriteJSON(runtime.IO().Out, value); err != nil {
		return errs.NewInternalError(errs.SubtypeUnknown, "cannot write record manifest to stdout: %v", err).WithCause(err)
	}
	return nil
}

func appendUniqueIgnoredFields(current, incoming []recordexport.IgnoredField) []recordexport.IgnoredField {
	seen := make(map[string]bool, len(current)+len(incoming))
	for _, item := range current {
		seen[item.ID+"\x00"+item.Name+"\x00"+item.Reason] = true
	}
	for _, item := range incoming {
		key := item.ID + "\x00" + item.Name + "\x00" + item.Reason
		if !seen[key] {
			current = append(current, item)
			seen[key] = true
		}
	}
	return current
}

func appendUniqueStrings(current, incoming []string) []string {
	seen := make(map[string]bool, len(current)+len(incoming))
	for _, item := range current {
		seen[item] = true
	}
	for _, item := range incoming {
		if !seen[item] {
			current = append(current, item)
			seen[item] = true
		}
	}
	return current
}
