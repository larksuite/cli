// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package html

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/html/payload"
)

type publishFile struct {
	Path    string
	Content []byte
}
type publishPlan struct {
	Files         []publishFile
	Token         string   `json:"token,omitempty"`
	Paths         []string `json:"paths"`
	IdempotentKey string   `json:"idempotent_key"`
	HashTag       string   `json:"hash_tag"`
	Skipped       []payload.Skip
}
type publishPlanKey struct{}
type publishResult struct {
	Token   string `json:"token"`
	LinkURL string `json:"link_url"`
}

// Publish collects a local HTML entry and its dependencies into one publication.
var Publish = common.Shortcut{
	Service: "html", Command: "+publish", Description: "Publish an HTML file and its local static assets",
	Risk: "write", Scopes: []string{publishScope}, AuthTypes: []string{"user"},
	Flags: []common.Flag{
		{Name: "file-path", Required: true, Desc: "HTML entry file, relative to the current directory"},
		{Name: "token", Desc: "Target HTML token; updates it with edit permission, otherwise uses the entry path publication"},
	},
	Tips: []string{"The entry is deployed as index.html. Without --token, its canonical absolute source path identifies the document; moving or renaming the entry creates a different document.", "Local dependencies are collected like apps +deploy; remote resources remain external."},
	Validate: func(ctx context.Context, rt *common.RuntimeContext) error {
		plan, err := preparePublish(rt)
		if err != nil {
			return err
		}
		// Store only on this invocation's command, never on the shared shortcut.
		rt.Cmd.SetContext(context.WithValue(ctx, publishPlanKey{}, plan))
		return nil
	},
	DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		defer rt.Cmd.SetContext(ctx)
		plan, _ := rt.Cmd.Context().Value(publishPlanKey{}).(*publishPlan)
		if plan == nil {
			return common.NewDryRunAPI()
		}
		return common.NewDryRunAPI().POST(publishPath).Body(struct {
			Token         string   `json:"token,omitempty"`
			Paths         []string `json:"paths"`
			IdempotentKey string   `json:"idempotent_key"`
			HashTag       string   `json:"hash_tag"`
		}{Token: plan.Token, Paths: plan.Paths, IdempotentKey: plan.IdempotentKey, HashTag: plan.HashTag}).Set("file_count", len(plan.Files)).Set("skipped_dependencies", plan.Skipped).
			Desc("Upload ordered multipart files with matching paths; no file contents are shown")
	},
	Execute: executePublish,
}

func preparePublish(rt *common.RuntimeContext) (*publishPlan, error) {
	entry := rt.Str("file-path")
	if !strings.EqualFold(path.Ext(entry), ".html") && !strings.EqualFold(path.Ext(entry), ".htm") {
		return nil, common.ValidationErrorf("--file-path must name an HTML file").WithParam("--file-path")
	}
	stat, err := rt.FileIO().Stat(entry)
	if err != nil {
		return nil, common.ValidationErrorf("--file-path: %v", err).WithParam("--file-path").WithCause(err)
	}
	if !stat.Mode().IsRegular() || stat.Size() <= 0 || stat.Size() > maxPublishBytes {
		return nil, common.ValidationErrorf("--file-path must be a nonempty regular file of at most %d bytes", maxPublishBytes).WithParam("--file-path")
	}
	candidates, key, skipped, err := payload.CollectFileWithBudget(rt.FileIO(), entry, maxPublishBytes)
	if err != nil {
		return nil, publishRecoveryError(err)
	}
	// Keep recovery guidance aligned with the HTML command's input mode.
	for i := range skipped {
		switch skipped[i].Kind {
		case payload.SkipUnparsed:
			skipped[i].Advice = "fix the file so its local references can be parsed and collected, then retry"
		case payload.SkipDynamic:
			skipped[i].Advice = "use literal local paths for required assets so they can be collected, then retry"
		}
	}
	if len(candidates) > maxFiles || len(key) > 1256 {
		return nil, common.ValidationErrorf("publication exceeds %d files or the 1256-byte entry path limit", maxFiles).WithParam("--file-path")
	}
	if _, err = payload.Guard(candidates, false, payload.Limits{SingleHTMLBytes: maxPublishBytes, RawTotalBytes: maxPublishBytes}); err != nil {
		return nil, publishRecoveryError(err)
	}
	entries, _, err := payload.BuildManifest(candidates, candidates[0].RelPath)
	if err != nil {
		return nil, publishRecoveryError(err)
	}
	plan := &publishPlan{Token: rt.Str("token"), IdempotentKey: key, Skipped: skipped}
	hashes := make([]payload.HashFile, 0, len(entries))
	var total int64
	for _, item := range entries {
		rel := strings.TrimPrefix(item.ZipPath, "output/")
		if err := validateDeploymentPath(rel); err != nil {
			return nil, err
		}
		file, err := rt.FileIO().Open(item.AbsPath)
		if err != nil {
			return nil, errs.NewInternalError(errs.SubtypeFileIO, "read publication file: %v", err).WithCause(err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(file, maxPublishBytes-total+1))
		closeErr := file.Close()
		if readErr != nil {
			return nil, errs.NewInternalError(errs.SubtypeFileIO, "read publication file: %v", readErr).WithCause(readErr)
		}
		if closeErr != nil {
			return nil, errs.NewInternalError(errs.SubtypeFileIO, "close publication file: %v", closeErr).WithCause(closeErr)
		}
		total += int64(len(raw))
		if len(raw) == 0 || total > maxPublishBytes {
			return nil, common.ValidationErrorf("publication contains an empty file or exceeds %d bytes", maxPublishBytes).WithParam("--file-path")
		}
		plan.Paths = append(plan.Paths, rel)
		plan.Files = append(plan.Files, publishFile{Path: rel, Content: raw})
		hashes = append(hashes, payload.HashFile{Path: rel, Raw: raw})
	}
	plan.HashTag, err = payload.ContentHash(hashes)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

// encodePublication uses repeated files and paths parts in matching order.
// SDK Formdata is map-backed, so it cannot represent ordered repeated fields.
func encodePublication(plan *publishPlan) (*bytes.Buffer, string, error) {
	body := new(bytes.Buffer)
	writer := multipart.NewWriter(body)
	for _, file := range plan.Files {
		part, err := writer.CreateFormFile("files", file.Path)
		if err != nil {
			return nil, "", err
		}
		if _, err = part.Write(file.Content); err != nil {
			return nil, "", err
		}
	}
	for _, rel := range plan.Paths {
		if err := writer.WriteField("paths", rel); err != nil {
			return nil, "", err
		}
	}
	for _, field := range [][2]string{{"idempotent_key", plan.IdempotentKey}, {"hash_tag", plan.HashTag}} {
		if err := writer.WriteField(field[0], field[1]); err != nil {
			return nil, "", err
		}
	}
	if plan.Token != "" {
		if err := writer.WriteField("token", plan.Token); err != nil {
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return body, writer.FormDataContentType(), nil
}

func executePublish(ctx context.Context, rt *common.RuntimeContext) error {
	defer rt.Cmd.SetContext(ctx)
	plan, _ := rt.Cmd.Context().Value(publishPlanKey{}).(*publishPlan)
	if plan == nil {
		return errs.NewInternalError(errs.SubtypeSDKError, "publication was not validated")
	}
	for _, skip := range plan.Skipped {
		fmt.Fprintln(rt.IO().ErrOut, skip.String())
	}
	body, contentType, err := encodePublication(plan)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeSDKError, "encode publication: %v", err).WithCause(err)
	}
	resp, err := rt.DoAPIStream(ctx, &larkcore.ApiReq{HttpMethod: http.MethodPost, ApiPath: publishPath, Body: body}, client.WithHeaders(http.Header{"Content-Type": {contentType}}))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxEnvelopeBytes+1))
	if err != nil {
		return errs.NewNetworkError(errs.SubtypeNetworkTransport, "read publication response: %v", err).WithCause(err)
	}
	if int64(len(raw)) > maxEnvelopeBytes {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "publication response is too large")
	}
	if _, err = rt.ClassifyAPIResponse(&larkcore.ApiResp{StatusCode: resp.StatusCode, Header: resp.Header, RawBody: raw}); err != nil {
		return err
	}
	var envelope struct {
		Data publishResult `json:"data"`
	}
	if err = json.Unmarshal(raw, &envelope); err != nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "decode publication response: %v", err).WithCause(err)
	}
	if envelope.Data.Token == "" || envelope.Data.LinkURL == "" {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "publication response is missing token or link_url")
	}
	rt.OutFormat(envelope.Data, nil, func(w io.Writer) { fmt.Fprintln(w, envelope.Data.LinkURL) })
	return nil
}

// Keep deployment paths within the HTML backend contract, independently of the
// collector's zip-safety checks (the app archive accepts a broader path set).
func validateDeploymentPath(rel string) error {
	first := strings.Split(rel, "/")[0]
	invalid := !utf8.ValidString(rel) || utf8.RuneCountInString(rel) > 512 || strings.ContainsAny(rel, "\\\x00:%?#") || first == ".git" || first == ".well-known" || first == "_headers" || first == "_redirects"
	for _, r := range rel {
		if unicode.IsControl(r) {
			invalid = true
		}
	}
	if invalid {
		return common.ValidationErrorf("unsupported HTML deployment path %q", rel).WithParam("--file-path")
	}
	return nil
}
