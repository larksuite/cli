// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

func downloadWordRenderPDF(
	ctx context.Context,
	runtime *common.RuntimeContext,
	task wordRenderTask,
	output string,
	ifExists string,
) (string, int64, error) {
	if task.PDF == nil || strings.TrimSpace(task.PDF.DownloadURL) == "" {
		return "", 0, errs.NewInternalError(errs.SubtypeInvalidResponse, "Word render task has no PDF download URL")
	}
	finalPath, err := resolveWordRenderOutputPath(runtime, strings.TrimSpace(output), strings.TrimSpace(ifExists))
	if err != nil {
		return "", 0, err
	}
	if err := validate.ValidateDownloadSourceURL(ctx, task.PDF.DownloadURL); err != nil {
		return "", 0, newWordRenderDownloadPolicyError(runtime, "blocked PDF download target: "+err.Error(), task.PDF.DownloadURL, err)
	}
	parsedURL, err := url.Parse(task.PDF.DownloadURL)
	if err != nil {
		return "", 0, newWordRenderDownloadPolicyError(runtime, "PDF download URL is invalid", task.PDF.DownloadURL, err)
	}
	if !strings.EqualFold(parsedURL.Scheme, "https") {
		return "", 0, newWordRenderDownloadPolicyError(runtime, "PDF download URL must use HTTPS", task.PDF.DownloadURL, nil)
	}
	baseClient, err := runtime.Factory.ExternalHTTPClient()
	if err != nil {
		return "", 0, errs.NewNetworkError(errs.SubtypeNetworkTransport, "failed to initialize external download client").WithCause(err)
	}
	downloadClient := *baseClient
	//nolint:forbidigo // Presigned Drive redirects stay outside the gateway; this hook validates every target and strips credentials.
	downloadClient.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return newWordRenderDownloadPolicyError(runtime, "PDF download exceeded the redirect limit", request.URL.String(), nil)
		}
		if !strings.EqualFold(request.URL.Scheme, "https") {
			return newWordRenderDownloadPolicyError(runtime, "PDF download redirect must use HTTPS", request.URL.String(), nil)
		}
		for _, header := range []string{"Authorization", "Cookie", "X-Lark-MCP-UAT", "X-Lark-MCP-TAT"} {
			request.Header.Del(header)
		}
		if err := validate.ValidateDownloadSourceURL(request.Context(), request.URL.String()); err != nil {
			return newWordRenderDownloadPolicyError(runtime, "blocked PDF download redirect: "+err.Error(), request.URL.String(), err)
		}
		return nil
	}
	//nolint:forbidigo // Drive returns a presigned external URL; the download-safe client enforces scheme, DNS/IP and redirect policy without attaching Lark auth.
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, task.PDF.DownloadURL, nil)
	if err != nil {
		return "", 0, errs.NewNetworkError(errs.SubtypeNetworkTransport, "invalid PDF download request").WithCause(err)
	}
	//nolint:forbidigo // Presigned Drive bytes cannot use RuntimeContext.DoAPI because attaching gateway auth would leak credentials.
	response, err := downloadClient.Do(request)
	if err != nil {
		return "", 0, errs.NewNetworkError(errs.SubtypeNetworkTransport, "PDF download failed").WithCause(err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		subtype := errs.SubtypeNetworkTransport
		if response.StatusCode >= http.StatusInternalServerError {
			subtype = errs.SubtypeNetworkServer
		}
		networkErr := errs.NewNetworkError(subtype, "PDF download failed: HTTP %d", response.StatusCode).WithCode(response.StatusCode)
		if response.StatusCode >= http.StatusInternalServerError {
			networkErr = networkErr.WithRetryable()
		}
		return "", 0, networkErr
	}

	prefix := make([]byte, len("%PDF-"))
	if _, err := io.ReadFull(response.Body, prefix); err != nil {
		return "", 0, errs.NewNetworkError(errs.SubtypeNetworkProtocol, "downloaded response is shorter than a PDF header").WithCause(err)
	}
	if string(prefix) != "%PDF-" {
		return "", 0, errs.NewNetworkError(errs.SubtypeNetworkProtocol, "downloaded response is not a PDF")
	}
	saveOptions := fileio.SaveOptions{
		ContentType:   response.Header.Get("Content-Type"),
		ContentLength: response.ContentLength,
	}
	body := io.MultiReader(bytes.NewReader(prefix), response.Body)
	var result fileio.SaveResult
	if strings.TrimSpace(ifExists) == wordRenderIfExistsOverwrite {
		result, err = runtime.FileIO().Save(finalPath, saveOptions, body)
	} else {
		exclusive, ok := runtime.FileIO().(fileio.ExclusiveFileIO)
		if !ok {
			return "", 0, errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"the active file provider cannot guarantee --if-exists %s; use --if-exists overwrite", ifExists).
				WithParam("--if-exists")
		}
		result, err = exclusive.SaveExclusive(finalPath, saveOptions, body)
	}
	if err != nil {
		if errors.Is(err, fs.ErrExist) {
			return "", 0, errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"output file was created concurrently: %s; retry the command", finalPath).WithParam("--output").WithCause(err)
		}
		return "", 0, common.WrapSaveErrorTypedForFlag(err, "--output")
	}
	resolved, resolveErr := runtime.ResolveSavePath(finalPath)
	if resolveErr != nil || resolved == "" {
		resolved = finalPath
	}
	return resolved, result.Size(), nil
}

func newWordRenderDownloadPolicyError(
	runtime *common.RuntimeContext,
	message string,
	downloadURL string,
	cause error,
) *errs.SecurityPolicyError {
	policyErr := errs.NewSecurityPolicyError(errs.SubtypeAccessDenied, "%s", message)
	if runtime != nil && strings.EqualFold(strings.TrimSpace(runtime.Format), "json") {
		policyErr.WithDownloadURL(strings.TrimSpace(downloadURL))
	}
	if cause != nil {
		policyErr.WithCause(cause)
	}
	return policyErr
}

func resolveWordRenderOutputPath(runtime *common.RuntimeContext, output, ifExists string) (string, error) {
	if _, err := runtime.ResolveSavePath(output); err != nil {
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe --output path: %s", err).
			WithParam("--output").
			WithCause(err)
	}
	switch ifExists {
	case "", wordRenderIfExistsError:
		if _, ok := runtime.FileIO().(fileio.ExclusiveFileIO); !ok {
			return "", errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"the active file provider cannot guarantee --if-exists error; use --if-exists overwrite").WithParam("--if-exists")
		}
		if _, err := runtime.FileIO().Stat(output); err == nil {
			return "", errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"output file already exists: %s (use --if-exists overwrite or rename)", output).WithParam("--output")
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", errs.NewInternalError(errs.SubtypeFileIO, "cannot access output path %s: %s", output, err).WithCause(err)
		}
		return output, nil
	case wordRenderIfExistsOverwrite:
		return output, nil
	case wordRenderIfExistsRename:
		if _, ok := runtime.FileIO().(fileio.ExclusiveFileIO); !ok {
			return "", errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"the active file provider cannot guarantee --if-exists rename; use --if-exists overwrite").WithParam("--if-exists")
		}
		return nextAvailableWordRenderPath(runtime.FileIO(), output)
	default:
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument,
			"invalid --if-exists %q: allowed values are error, overwrite, rename", ifExists).WithParam("--if-exists")
	}
}

func nextAvailableWordRenderPath(fio fileio.FileIO, output string) (string, error) {
	if _, err := fio.Stat(output); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return output, nil
		}
		return "", errs.NewInternalError(errs.SubtypeFileIO, "cannot access output path %s: %s", output, err).WithCause(err)
	}
	dir := filepath.Dir(output)
	ext := filepath.Ext(output)
	base := strings.TrimSuffix(filepath.Base(output), ext)
	for index := 1; index < 10_000; index++ {
		candidate := filepath.Join(dir, fmt.Sprintf("%s (%d)%s", base, index, ext))
		if _, err := fio.Stat(candidate); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return candidate, nil
			}
			return "", errs.NewInternalError(errs.SubtypeFileIO, "cannot access output candidate %s: %s", candidate, err).WithCause(err)
		}
	}
	return "", errs.NewInternalError(errs.SubtypeFileIO, "cannot allocate a unique PDF output path")
}
