// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package im

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

func messageExportFlags() []common.Flag {
	return []common.Flag{
		{Name: "output", Desc: "save formatted messages to this file and print a JSON summary instead"},
		{Name: "output-dir", Desc: "save messages under this directory using the resolved chat/thread ID and format extension; conflicts with --output"},
		{Name: "overwrite", Type: "bool", Desc: "replace an existing message export file (requires --output or --output-dir)"},
	}
}

func validateMessageExportFlags(runtime *common.RuntimeContext) error {
	if err := common.MutuallyExclusiveTyped(runtime, "output", "output-dir"); err != nil {
		return err
	}
	for _, flag := range []string{"output", "output-dir"} {
		value := runtime.Str(flag)
		if runtime.Changed(flag) && strings.TrimSpace(value) == "" {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--%s cannot be empty", flag).WithParam("--" + flag)
		}
		if value != "" {
			if runtime.FileIO() == nil {
				return errs.NewInternalError(errs.SubtypeFileIO, "message export requires a file I/O provider")
			}
			if _, err := runtime.ResolveSavePath(value); err != nil {
				return common.WrapSaveErrorTypedForFlag(err, "--"+flag)
			}
		}
	}
	if runtime.Bool("overwrite") && runtime.Str("output") == "" && runtime.Str("output-dir") == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--overwrite requires --output or --output-dir").WithParam("--overwrite")
	}
	return nil
}

type messageExportTarget struct {
	path, location, param string
	format, contentType   string
	fio                   fileio.FileIO
	overwrite             bool
}

func messageExportFormat(runtime *common.RuntimeContext) (format, extension, contentType string) {
	switch {
	case runtime.Bool("concise"):
		return "markdown", ".md", "text/markdown; charset=utf-8"
	case runtime.JqExpr != "":
		return "jq", ".txt", "text/plain; charset=utf-8"
	case runtime.Format == "ndjson":
		return "ndjson", ".ndjson", "application/x-ndjson"
	case runtime.Format == "csv":
		return "csv", ".csv", "text/csv; charset=utf-8"
	case runtime.Format == "table" || runtime.Format == "pretty":
		return runtime.Format, ".txt", "text/plain; charset=utf-8"
	default:
		return "json", ".json", "application/json"
	}
}

func messageExportPath(runtime *common.RuntimeContext, containerID string) (string, string) {
	if path := runtime.Str("output"); path != "" {
		return path, "--output"
	}
	if dir := runtime.Str("output-dir"); dir != "" {
		_, extension, _ := messageExportFormat(runtime)
		return strings.TrimRight(dir, "/\\") + "/" + containerID + extension, "--output-dir"
	}
	return "", ""
}

func messageExportDryRun(runtime *common.RuntimeContext, d *common.DryRunAPI, containerID string) *common.DryRunAPI {
	path, _ := messageExportPath(runtime, containerID)
	if path == "" {
		return d
	}
	policy := "fail"
	if runtime.Bool("overwrite") {
		policy = "overwrite"
	}
	format, _, _ := messageExportFormat(runtime)
	return d.File(cmdutil.DryRunFileIntent{Name: path, IfExists: policy, Content: format + " message output"})
}

func prepareMessageExport(runtime *common.RuntimeContext, containerID string) (*messageExportTarget, error) {
	if err := validateMessageExportFlags(runtime); err != nil {
		return nil, err
	}
	path, param := messageExportPath(runtime, containerID)
	if path == "" {
		return nil, nil
	}
	if runtime.Str("output-dir") != "" && (containerID == "" || strings.IndexFunc(containerID, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_')
	}) >= 0) {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "chat/thread ID cannot be used as an export filename").WithParam(param)
	}
	fio := runtime.FileIO()
	location, err := fio.ResolvePath(path)
	if err != nil {
		return nil, common.WrapSaveErrorTypedForFlag(err, param)
	}
	if location == "" {
		return nil, errs.NewInternalError(errs.SubtypeFileIO, "file provider returned an empty export location")
	}
	format, _, contentType := messageExportFormat(runtime)
	target := &messageExportTarget{path: path, location: location, param: param, format: format,
		contentType: contentType, fio: fio, overwrite: runtime.Bool("overwrite")}
	if !target.overwrite {
		if _, ok := fio.(fileio.ExclusiveFileIO); !ok {
			return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition, "file provider cannot preserve existing export files").
				WithParam(param).WithHint("use --overwrite explicitly or a file provider supporting exclusive writes")
		}
	}
	if info, statErr := fio.Stat(path); statErr == nil {
		if info.IsDir() {
			return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "export target is a directory").WithParam(param)
		}
		if !target.overwrite {
			return nil, target.existsError(fs.ErrExist)
		}
	} else if !errors.Is(statErr, fs.ErrNotExist) {
		return nil, common.WrapSaveErrorTypedForFlag(statErr, param)
	}
	return target, nil
}

func (target *messageExportTarget) existsError(cause error) error {
	return errs.NewValidationError(errs.SubtypeFailedPrecondition, "message export %q already exists", target.path).
		WithParam(target.param).WithHint("choose another output path or pass --overwrite to replace it").WithCause(cause)
}

type messageListOutput struct {
	data       map[string]interface{}
	pagination *output.PaginationMeta
	concise    conciseMessageView
	pretty     func(io.Writer)
}

type messageExportSummary struct {
	SavedPath string `json:"saved_path"`
	SizeBytes int64  `json:"size_bytes"`
	Format    string `json:"format"`
	Total     int    `json:"total"`
	HasMore   bool   `json:"has_more"`
	PageToken string `json:"page_token,omitempty"`
}

func emitMessageList(runtime *common.RuntimeContext, target *messageExportTarget, result messageListOutput) error {
	meta := &output.Meta{Pagination: result.pagination}
	if target == nil {
		if runtime.Bool("concise") {
			return outputMessagesConcise(runtime, result.concise)
		}
		runtime.OutFormat(result.data, meta, result.pretty)
		return nil
	}

	var rendered bytes.Buffer
	if runtime.Bool("concise") {
		if err := writeMessagesConcise(runtime, &rendered, result.concise); err != nil {
			return err
		}
	} else {
		emitter := messageExportEmitter(runtime, &rendered)
		if err := emitter.Success(result.data, output.EmitOptions{
			Format: runtime.Format, JQ: runtime.JqExpr, Meta: meta,
			Pretty: func(w io.Writer, _ bool) error { result.pretty(w); return nil },
		}); err != nil {
			return err
		}
	}
	options := fileio.SaveOptions{ContentType: target.contentType, ContentLength: int64(rendered.Len())}
	var saved fileio.SaveResult
	var err error
	if target.overwrite {
		saved, err = target.fio.Save(target.path, options, &rendered)
	} else {
		saved, err = target.fio.(fileio.ExclusiveFileIO).SaveExclusive(target.path, options, &rendered)
	}
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return target.existsError(err)
		}
		return common.WrapSaveErrorTypedForFlag(err, target.param)
	}
	if saved == nil {
		return errs.NewInternalError(errs.SubtypeFileIO, "file provider did not report the saved export size")
	}
	// Filtering belongs to the saved payload; stdout always describes the artifact.
	return messageExportEmitter(runtime, runtime.IO().Out).Success(messageExportSummary{
		SavedPath: target.location, SizeBytes: saved.Size(), Format: target.format,
		Total: result.pagination.Items, HasMore: !result.pagination.Complete, PageToken: result.pagination.NextToken,
	}, output.EmitOptions{Meta: meta})
}

func messageExportEmitter(runtime *common.RuntimeContext, w io.Writer) *output.Emitter {
	return output.NewEmitter(output.EmitterConfig{
		Out: w, ErrOut: runtime.IO().ErrOut, CommandPath: runtime.Cmd.CommandPath(),
		Identity: string(runtime.As()), NoticeProvider: output.GetNotice,
	})
}
