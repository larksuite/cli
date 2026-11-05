// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package html

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/charcheck"
	"github.com/larksuite/cli/shortcuts/common"
)

func fetchPath(token string) string {
	return "/open-apis/html/v1/" + url.PathEscape(token) + "/download"
}
func fetchOutput(rt *common.RuntimeContext) string {
	if rt.Str("output") != "" {
		return rt.Str("output")
	}
	return rt.Str("token") + ".zip"
}

// Fetch saves the source ZIP without unpacking or interpreting output as a directory.
var Fetch = common.Shortcut{
	Service: "html", Command: "+fetch", Description: "Download an HTML document's source ZIP",
	Risk: "read", Scopes: []string{downloadScope}, AuthTypes: []string{"user"},
	Flags: []common.Flag{
		{Name: "token", Required: true, Desc: "Bare HTML document token, not a URL"},
		{Name: "output", Desc: "Full relative output file path (default: <token>.zip); the archive is not unpacked"},
	},
	Validate: func(ctx context.Context, rt *common.RuntimeContext) error {
		token := rt.Str("token")
		if err := charcheck.RejectControlChars(token, "--token"); err != nil {
			return common.ValidationErrorf("%v", err).WithParam("--token").WithCause(err)
		}
		if token == "" || len(token) > 128 || token == "." || token == ".." || strings.ContainsAny(token, "/\\?# %\t\r\n") {
			return common.ValidationErrorf("--token must be a bare HTML token of at most 128 bytes").WithParam("--token")
		}
		if _, err := rt.FileIO().ResolvePath(fetchOutput(rt)); err != nil {
			return common.ValidationErrorf("--output: %v", err).WithParam("--output").WithCause(err)
		}
		return nil
	},
	DryRun: func(ctx context.Context, rt *common.RuntimeContext) *common.DryRunAPI {
		return common.NewDryRunAPI().GET(fetchPath(rt.Str("token"))).Set("output", fetchOutput(rt)).Desc("Save the source ZIP to a file without unpacking")
	},
	Execute: executeFetch,
}

type fetchResult struct {
	Token     string `json:"token"`
	Output    string `json:"output"`
	SizeBytes int64  `json:"size_bytes"`
}

func executeFetch(ctx context.Context, rt *common.RuntimeContext) error {
	resp, err := rt.DoAPIStream(ctx, &larkcore.ApiReq{HttpMethod: http.MethodGet, ApiPath: fetchPath(rt.Str("token"))})
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	magic, peekErr := reader.Peek(4)
	if peekErr != nil && !errors.Is(peekErr, io.EOF) {
		return errs.NewNetworkError(errs.SubtypeNetworkTransport, "read source archive: %v", peekErr).WithCause(peekErr)
	}
	// ZIP signatures are checked without extracting the archive. This also catches
	// a 200 JSON error when the gateway omitted its Content-Type header.
	zip := len(magic) == 4 && string(magic[:2]) == "PK" && (string(magic[2:]) == "\x03\x04" || string(magic[2:]) == "\x05\x06" || string(magic[2:]) == "\x07\x08")
	if !zip {
		raw, readErr := io.ReadAll(io.LimitReader(reader, maxEnvelopeBytes+1))
		if readErr != nil {
			return errs.NewNetworkError(errs.SubtypeNetworkTransport, "read download error: %v", readErr).WithCause(readErr)
		}
		if int64(len(raw)) <= maxEnvelopeBytes {
			if _, err = rt.ClassifyAPIResponse(&larkcore.ApiResp{StatusCode: resp.StatusCode, Header: resp.Header, RawBody: raw}); err != nil {
				return err
			}
		}
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "download response is not a ZIP archive")
	}
	out := fetchOutput(rt)
	saved, err := rt.FileIO().Save(out, fileio.SaveOptions{ContentType: "application/zip", ContentLength: resp.ContentLength}, reader)
	if err != nil {
		return common.WrapSaveErrorTypedForFlag(err, "--output")
	}
	resolved, err := rt.FileIO().ResolvePath(out)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeFileIO, "resolve saved archive: %v", err).WithCause(err)
	}
	result := fetchResult{Token: rt.Str("token"), Output: resolved, SizeBytes: saved.Size()}
	rt.OutFormat(result, nil, func(w io.Writer) { fmt.Fprintf(w, "Saved %s (%d bytes)\n", resolved, saved.Size()) })
	return nil
}
