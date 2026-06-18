// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package doc

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"
	"time"

	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/shortcuts/common"
)

const (
	docCoverResourceType    = "cover"
	docCoverUploadParent    = "docx_image"
	docCoverURLMaxBytes     = int64(20 * 1024 * 1024)
	docCoverDownloadName    = "cover"
	docCoverURLDownloadName = "cover"
)

type docCoverHTTPStatusCause int

func (c docCoverHTTPStatusCause) Error() string {
	return http.StatusText(int(c))
}

type docCoverURLGuardError string

func (e docCoverURLGuardError) Error() string {
	return string(e)
}

var docCoverAllowedContentTypes = map[string]string{
	"image/gif":  ".gif",
	"image/jpeg": ".jpg",
	"image/png":  ".png",
	"image/webp": ".webp",
}

var DocResourceDownload = common.Shortcut{
	Service:     "docs",
	Command:     "resource-download",
	Description: "Download a document resource (type=cover downloads the cover image content)",
	Risk:        "read",
	Scopes:      []string{"docx:document:readonly", "docs:document.media:download"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "doc", Desc: "document URL or document_id", Required: true},
		{Name: "type", Default: docCoverResourceType, Desc: "resource type: cover"},
		{Name: "output", Desc: "local save path", Required: true},
		{Name: "overwrite", Type: "bool", Desc: "overwrite existing output file"},
	},
	Validate: validateDocCoverType,
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		docRef, err := parseDocumentRef(runtime.Str("doc"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		documentID := docRef.Token
		d := common.NewDryRunAPI()
		if docRef.Kind == "wiki" {
			documentID = "<resolved_docx_token>"
			d.GET("/open-apis/wiki/v2/spaces/get_node").
				Desc("[1] Resolve wiki node to docx document").
				Params(map[string]interface{}{"token": docRef.Token})
		}
		d.GET("/open-apis/docx/v1/documents/:document_id").
			Desc("Read document cover metadata").
			Set("document_id", documentID)
		d.GET("/open-apis/drive/v1/medias/:cover_token/download").
			Desc("Download cover image content").
			Set("cover_token", "<cover.token>").
			Set("output", runtime.Str("output"))
		return d
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		documentID, err := resolveDocxDocumentIDForResource(runtime, runtime.Str("doc"))
		if err != nil {
			return err
		}
		outputPath := runtime.Str("output")
		if _, err := runtime.ResolveSavePath(outputPath); err != nil {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).WithParam("--output").WithCause(err)
		}

		cover, err := getDocCover(runtime, documentID)
		if err != nil {
			return err
		}
		if cover.Token == "" {
			return errs.NewValidationError(errs.SubtypeFailedPrecondition, "document has no cover (cover is empty): %s", common.MaskToken(documentID)).WithParam("--type")
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Downloading cover: %s\n", common.MaskToken(cover.Token))
		resp, err := runtime.DoAPIStream(ctx, &larkcore.ApiReq{
			HttpMethod: http.MethodGet,
			ApiPath:    fmt.Sprintf("/open-apis/drive/v1/medias/%s/download", validate.EncodePathSegment(cover.Token)),
		})
		if err != nil {
			return wrapDocNetworkErr(err, "download cover failed: %v", err)
		}
		defer resp.Body.Close()

		finalPath, _ := autoAppendDocMediaExtension(outputPath, resp.Header, "")
		if finalPath != outputPath {
			if _, err := runtime.ResolveSavePath(finalPath); err != nil {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsafe output path: %s", err).WithParam("--output").WithCause(err)
			}
		}
		if !runtime.Bool("overwrite") {
			if _, statErr := runtime.FileIO().Stat(finalPath); statErr == nil {
				return errs.NewValidationError(errs.SubtypeFailedPrecondition, "output file already exists: %s (use --overwrite to replace)", finalPath).WithParam("--output")
			}
		}

		result, err := runtime.FileIO().Save(finalPath, fileio.SaveOptions{
			ContentType:   resp.Header.Get("Content-Type"),
			ContentLength: resp.ContentLength,
		}, resp.Body)
		if err != nil {
			return common.WrapSaveErrorTyped(err)
		}
		savedPath, _ := runtime.ResolveSavePath(finalPath)
		if savedPath == "" {
			savedPath = finalPath
		}
		runtime.Out(map[string]interface{}{
			"document_id":  documentID,
			"type":         docCoverResourceType,
			"saved_path":   savedPath,
			"size_bytes":   result.Size(),
			"content_type": resp.Header.Get("Content-Type"),
			"cover":        cover.toOutput(),
		}, nil)
		return nil
	},
}

var DocResourceUpdate = common.Shortcut{
	Service:     "docs",
	Command:     "resource-update",
	Description: "Upload and update a document resource (type=cover)",
	Risk:        "write",
	Scopes:      []string{"docx:document:readonly", "docx:document:write_only", "docs:document.media:upload"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "doc", Desc: "document URL or document_id", Required: true},
		{Name: "type", Default: docCoverResourceType, Desc: "resource type: cover"},
		{Name: "file", Desc: "local image file path (files > 20MB use multipart upload automatically)"},
		{Name: "from-clipboard", Type: "bool", Desc: "read image from system clipboard instead of a local file"},
		{Name: "url", Desc: "HTTPS image URL to download and upload"},
		{Name: "offset-ratio-x", Type: "float64", Desc: "cover horizontal offset ratio"},
		{Name: "offset-ratio-y", Type: "float64", Desc: "cover vertical offset ratio"},
	},
	Validate: validateDocCoverUpdate,
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		docRef, err := parseDocumentRef(runtime.Str("doc"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		documentID := docRef.Token
		d := common.NewDryRunAPI()
		if docRef.Kind == "wiki" {
			documentID = "<resolved_docx_token>"
			d.GET("/open-apis/wiki/v2/spaces/get_node").
				Desc("[1] Resolve wiki node to docx document").
				Params(map[string]interface{}{"token": docRef.Token})
		}
		source := docCoverDryRunSource(runtime)
		d.Desc("upload cover image and update document cover").
			POST("/open-apis/drive/v1/medias/upload_all").
			Desc("Upload cover image").
			Body(map[string]interface{}{
				"file":        source,
				"file_name":   "<cover_file_name>",
				"parent_type": docCoverUploadParent,
				"parent_node": documentID,
				"extra":       fmt.Sprintf(`{"drive_route_token":"%s"}`, documentID),
			})
		d.PATCH("/open-apis/docx/v1/documents/:document_id").
			Desc("Update document cover").
			Body(map[string]interface{}{"update_cover": map[string]interface{}{"cover": buildDocCoverUpdateBody("<file_token>", runtime)}})
		d.Set("document_id", documentID)
		if runtime.Str("url") != "" {
			d.Set("url_safety", "HTTPS only; private/loopback/link-local IPs rejected; max 3 redirects; image content-types only; max 20MiB")
		}
		return d
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		documentID, err := resolveDocxDocumentIDForResource(runtime, runtime.Str("doc"))
		if err != nil {
			return err
		}

		source, err := readDocCoverUpdateSource(ctx, runtime)
		if err != nil {
			return err
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Uploading cover image: %s (%d bytes)\n", source.FileName, source.FileSize)
		if source.FileSize > common.MaxDriveMediaUploadSinglePartSize {
			fmt.Fprintf(runtime.IO().ErrOut, "File exceeds 20MB, using multipart upload\n")
		}

		uploadCfg := UploadDocMediaFileConfig{
			FilePath:   source.FilePath,
			Reader:     source.Reader,
			FileName:   source.FileName,
			FileSize:   source.FileSize,
			ParentType: docCoverUploadParent,
			ParentNode: documentID,
			DocID:      documentID,
		}
		fileToken, err := uploadDocMediaFile(runtime, uploadCfg)
		if err != nil {
			return err
		}
		fmt.Fprintf(runtime.IO().ErrOut, "File uploaded: %s\n", common.MaskToken(fileToken))

		coverBody := buildDocCoverUpdateBody(fileToken, runtime)
		if _, err := runtime.CallAPITyped("PATCH",
			fmt.Sprintf("/open-apis/docx/v1/documents/%s", validate.EncodePathSegment(documentID)),
			nil,
			map[string]interface{}{"update_cover": map[string]interface{}{"cover": coverBody}},
		); err != nil {
			return err
		}

		runtime.Out(map[string]interface{}{
			"document_id": documentID,
			"type":        docCoverResourceType,
			"updated":     true,
			"source":      source.Kind,
			"file_token":  fileToken,
			"cover":       coverBody,
		}, nil)
		return nil
	},
}

var DocResourceDelete = common.Shortcut{
	Service:     "docs",
	Command:     "resource-delete",
	Description: "Delete a document resource (type=cover is idempotent when empty)",
	Risk:        "write",
	Scopes:      []string{"docx:document:readonly", "docx:document:write_only"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "doc", Desc: "document URL or document_id", Required: true},
		{Name: "type", Default: docCoverResourceType, Desc: "resource type: cover"},
	},
	Validate: validateDocCoverType,
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		docRef, err := parseDocumentRef(runtime.Str("doc"))
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		documentID := docRef.Token
		d := common.NewDryRunAPI()
		if docRef.Kind == "wiki" {
			documentID = "<resolved_docx_token>"
			d.GET("/open-apis/wiki/v2/spaces/get_node").
				Desc("[1] Resolve wiki node to docx document").
				Params(map[string]interface{}{"token": docRef.Token})
		}
		d.GET("/open-apis/docx/v1/documents/:document_id").
			Desc("Read document cover metadata for idempotency").
			Set("document_id", documentID)
		d.PATCH("/open-apis/docx/v1/documents/:document_id").
			Desc("Clear document cover when one exists").
			Body(map[string]interface{}{"update_cover": map[string]interface{}{"cover": nil}})
		return d
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		documentID, err := resolveDocxDocumentIDForResource(runtime, runtime.Str("doc"))
		if err != nil {
			return err
		}
		cover, err := getDocCover(runtime, documentID)
		if err != nil {
			return err
		}
		if cover.Token == "" {
			runtime.Out(map[string]interface{}{
				"document_id":   documentID,
				"type":          docCoverResourceType,
				"deleted":       false,
				"already_empty": true,
			}, nil)
			return nil
		}

		if _, err := runtime.CallAPITyped("PATCH",
			fmt.Sprintf("/open-apis/docx/v1/documents/%s", validate.EncodePathSegment(documentID)),
			nil,
			map[string]interface{}{"update_cover": map[string]interface{}{"cover": nil}},
		); err != nil {
			return err
		}
		runtime.Out(map[string]interface{}{
			"document_id":    documentID,
			"type":           docCoverResourceType,
			"deleted":        true,
			"already_empty":  false,
			"previous_cover": cover.toOutput(),
		}, nil)
		return nil
	},
}

type docCoverMetadata struct {
	Token        string
	OffsetRatioX *float64
	OffsetRatioY *float64
}

func (c docCoverMetadata) toOutput() map[string]interface{} {
	out := map[string]interface{}{"token": c.Token}
	if c.OffsetRatioX != nil {
		out["offset_ratio_x"] = *c.OffsetRatioX
	}
	if c.OffsetRatioY != nil {
		out["offset_ratio_y"] = *c.OffsetRatioY
	}
	return out
}

type docCoverUpdateSource struct {
	Kind     string
	FilePath string
	Reader   io.Reader
	FileName string
	FileSize int64
}

func validateDocCoverType(ctx context.Context, runtime *common.RuntimeContext) error {
	if runtime.Str("type") != docCoverResourceType {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "unsupported --type %q, expected cover", runtime.Str("type")).WithParam("--type")
	}
	docRef, err := parseDocumentRef(runtime.Str("doc"))
	if err != nil {
		return err
	}
	if docRef.Kind == "doc" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "docs resource-* only supports docx documents; use a docx token/URL or a wiki URL that resolves to docx").WithParam("--doc")
	}
	return nil
}

func validateDocCoverUpdate(ctx context.Context, runtime *common.RuntimeContext) error {
	if err := validateDocCoverType(ctx, runtime); err != nil {
		return err
	}
	sourceCount := 0
	var params []errs.InvalidParam
	if runtime.Str("file") != "" {
		sourceCount++
		params = append(params, errs.InvalidParam{Name: "--file", Reason: "source flag"})
	}
	if runtime.Bool("from-clipboard") {
		sourceCount++
		params = append(params, errs.InvalidParam{Name: "--from-clipboard", Reason: "source flag"})
	}
	if runtime.Str("url") != "" {
		sourceCount++
		params = append(params, errs.InvalidParam{Name: "--url", Reason: "source flag"})
	}
	if sourceCount == 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "one of --file, --from-clipboard or --url is required").WithParams(
			errs.InvalidParam{Name: "--file", Reason: "provide one source"},
			errs.InvalidParam{Name: "--from-clipboard", Reason: "provide one source"},
			errs.InvalidParam{Name: "--url", Reason: "provide one source"},
		)
	}
	if sourceCount > 1 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--file, --from-clipboard and --url are mutually exclusive").WithParams(params...)
	}
	if err := validateCoverOffset(runtime, "offset-ratio-x"); err != nil {
		return err
	}
	if err := validateCoverOffset(runtime, "offset-ratio-y"); err != nil {
		return err
	}
	if rawURL := runtime.Str("url"); rawURL != "" {
		if _, err := parseDocCoverURLSyntax(rawURL); err != nil {
			return err
		}
	}
	return nil
}

func validateCoverOffset(runtime *common.RuntimeContext, name string) error {
	if !runtime.Changed(name) {
		return nil
	}
	value := runtime.Float64(name)
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--%s must be a finite number", name).WithParam("--" + name)
	}
	return nil
}

func resolveDocxDocumentIDForResource(runtime *common.RuntimeContext, input string) (string, error) {
	docRef, err := parseDocumentRef(input)
	if err != nil {
		return "", err
	}
	switch docRef.Kind {
	case "docx":
		return docRef.Token, nil
	case "wiki":
		return resolveDocxDocumentID(runtime, input)
	case "doc":
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "docs resource-* only supports docx documents; use a docx token/URL or a wiki URL that resolves to docx").WithParam("--doc")
	default:
		return "", errs.NewValidationError(errs.SubtypeInvalidArgument, "docs resource-* only supports docx documents").WithParam("--doc")
	}
}

func getDocCover(runtime *common.RuntimeContext, documentID string) (docCoverMetadata, error) {
	data, err := runtime.CallAPITyped("GET",
		fmt.Sprintf("/open-apis/docx/v1/documents/%s", validate.EncodePathSegment(documentID)),
		nil, nil)
	if err != nil {
		return docCoverMetadata{}, err
	}
	coverData := common.GetMap(data, "document", "cover")
	if len(coverData) == 0 {
		coverData = common.GetMap(data, "cover")
	}
	cover := docCoverMetadata{Token: common.GetString(coverData, "token")}
	if value, ok := getOptionalFloat(coverData, "offset_ratio_x"); ok {
		cover.OffsetRatioX = &value
	}
	if value, ok := getOptionalFloat(coverData, "offset_ratio_y"); ok {
		cover.OffsetRatioY = &value
	}
	return cover, nil
}

func getOptionalFloat(m map[string]interface{}, key string) (float64, bool) {
	if m == nil {
		return 0, false
	}
	switch v := m[key].(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	}
	return 0, false
}

func readDocCoverUpdateSource(ctx context.Context, runtime *common.RuntimeContext) (docCoverUpdateSource, error) {
	if runtime.Bool("from-clipboard") {
		fmt.Fprintf(runtime.IO().ErrOut, "Reading image from clipboard...\n")
		content, err := readClipboardImage()
		if err != nil {
			return docCoverUpdateSource{}, err
		}
		return docCoverUpdateSource{
			Kind:     "clipboard",
			Reader:   bytes.NewReader(content),
			FileName: "clipboard.png",
			FileSize: int64(len(content)),
		}, nil
	}
	if rawURL := runtime.Str("url"); rawURL != "" {
		content, fileName, err := downloadDocCoverURL(ctx, runtime, rawURL)
		if err != nil {
			return docCoverUpdateSource{}, err
		}
		return docCoverUpdateSource{
			Kind:     "url",
			Reader:   bytes.NewReader(content),
			FileName: fileName,
			FileSize: int64(len(content)),
		}, nil
	}

	filePath := runtime.Str("file")
	stat, err := runtime.FileIO().Stat(filePath)
	if err != nil {
		return docCoverUpdateSource{}, wrapDocInputFileErr(err, "file not found")
	}
	if !stat.Mode().IsRegular() {
		return docCoverUpdateSource{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "file must be a regular file: %s", filePath).WithParam("--file")
	}
	return docCoverUpdateSource{
		Kind:     "file",
		FilePath: filePath,
		FileName: filepath.Base(filePath),
		FileSize: stat.Size(),
	}, nil
}

func buildDocCoverUpdateBody(fileToken string, runtime *common.RuntimeContext) map[string]interface{} {
	cover := map[string]interface{}{"token": fileToken}
	if runtime.Changed("offset-ratio-x") {
		cover["offset_ratio_x"] = runtime.Float64("offset-ratio-x")
	}
	if runtime.Changed("offset-ratio-y") {
		cover["offset_ratio_y"] = runtime.Float64("offset-ratio-y")
	}
	return cover
}

func docCoverDryRunSource(runtime *common.RuntimeContext) string {
	if runtime.Bool("from-clipboard") {
		return "<clipboard image>"
	}
	if rawURL := runtime.Str("url"); rawURL != "" {
		return rawURL
	}
	if filePath := runtime.Str("file"); filePath != "" {
		return "@" + filePath
	}
	return "<cover image>"
}

func downloadDocCoverURL(ctx context.Context, runtime *common.RuntimeContext, raw string) ([]byte, string, error) {
	u, err := parseAndValidateDocCoverURL(ctx, raw)
	if err != nil {
		return nil, "", err
	}

	baseClient, err := runtime.Factory.HttpClient()
	if err != nil {
		return nil, "", errs.NewInternalError(errs.SubtypeSDKError, "http client: %v", err).WithCause(err)
	}
	client := newDocCoverHTTPClient(baseClient)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil) //nolint:forbidigo // cover --url fetches external user content; RuntimeContext API helpers are Lark-API only.
	if err != nil {
		return nil, "", errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --url: %v", err).WithParam("--url").WithCause(err)
	}
	resp, err := client.Do(req) //nolint:forbidigo // cover --url uses a guarded external downloader, not Lark API transport.
	if err != nil {
		return nil, "", wrapDocNetworkErr(err, "download cover URL failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		subtype := errs.SubtypeNetworkTransport
		if resp.StatusCode >= 500 {
			subtype = errs.SubtypeNetworkServer
		}
		cause := docCoverHTTPStatusCause(resp.StatusCode)
		return nil, "", errs.NewNetworkError(subtype, "download cover URL failed: HTTP %d", resp.StatusCode).WithCode(resp.StatusCode).WithCause(cause)
	}

	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil || mediaType == "" {
		return nil, "", errs.NewValidationError(errs.SubtypeInvalidArgument, "cover URL response must include an image Content-Type").WithParam("--url")
	}
	mediaType = strings.ToLower(mediaType)
	ext, ok := docCoverAllowedContentTypes[mediaType]
	if !ok {
		return nil, "", errs.NewValidationError(errs.SubtypeInvalidArgument, "cover URL Content-Type %q is not supported; expected image/png, image/jpeg, image/gif or image/webp", mediaType).WithParam("--url")
	}

	limited := io.LimitReader(resp.Body, docCoverURLMaxBytes+1)
	content, err := io.ReadAll(limited)
	if err != nil {
		return nil, "", wrapDocNetworkErr(err, "read cover URL response failed: %v", err)
	}
	if int64(len(content)) > docCoverURLMaxBytes {
		return nil, "", errs.NewValidationError(errs.SubtypeInvalidArgument, "cover URL response exceeds 20MiB limit").WithParam("--url")
	}

	fileName := docCoverURLFileName(resp.Request.URL, ext)
	return content, fileName, nil
}

func parseAndValidateDocCoverURL(ctx context.Context, raw string) (*url.URL, error) {
	u, err := parseDocCoverURLSyntax(raw)
	if err != nil {
		return nil, err
	}
	if err := validateDocCoverURLHost(ctx, u.Hostname()); err != nil {
		return nil, err
	}
	return u, nil
}

func parseDocCoverURLSyntax(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid --url: %v", err).WithParam("--url").WithCause(err)
	}
	if u.Scheme != "https" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--url must use https").WithParam("--url")
	}
	if u.User != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--url must not include userinfo").WithParam("--url")
	}
	host := u.Hostname()
	if host == "" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--url host cannot be empty").WithParam("--url")
	}
	return u, nil
}

func docCoverURLFileName(u *url.URL, ext string) string {
	base := path.Base(u.EscapedPath())
	if base == "." || base == "/" || base == "" {
		return docCoverURLDownloadName + ext
	}
	unescaped, err := url.PathUnescape(base)
	if err == nil {
		base = unescaped
	}
	base = filepath.Base(base)
	if strings.TrimSpace(base) == "" || base == "." || base == string(filepath.Separator) {
		return docCoverURLDownloadName + ext
	}
	if filepath.Ext(base) == "" {
		base += ext
	}
	return base
}

func validateDocCoverURLHost(ctx context.Context, host string) error {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--url host cannot be empty").WithParam("--url")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "--url must not resolve to a local or internal address").WithParam("--url")
	}
	if ip := net.ParseIP(host); ip != nil {
		if isUnsafeDocCoverIP(ip) {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--url must not resolve to a local or internal address").WithParam("--url")
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "failed to resolve --url host: %v", err).WithParam("--url").WithCause(err)
	}
	if len(ips) == 0 {
		return errs.NewValidationError(errs.SubtypeInvalidArgument, "failed to resolve --url host: no addresses").WithParam("--url")
	}
	for _, ip := range ips {
		if isUnsafeDocCoverIP(ip) {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--url must not resolve to a local or internal address").WithParam("--url")
		}
	}
	return nil
}

func isUnsafeDocCoverIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 10 || v4[0] == 127 {
			return true
		}
		if v4[0] == 169 && v4[1] == 254 {
			return true
		}
		if v4[0] == 172 && v4[1] >= 16 && v4[1] <= 31 {
			return true
		}
		if v4[0] == 192 && v4[1] == 168 {
			return true
		}
		if v4[0] == 100 && v4[1] >= 64 && v4[1] <= 127 {
			return true
		}
		if v4[0] == 198 && (v4[1] == 18 || v4[1] == 19) {
			return true
		}
		if v4[0] >= 240 {
			return true
		}
		return false
	}
	return ip.IsPrivate()
}

func newDocCoverHTTPClient(base *http.Client) *http.Client { //nolint:forbidigo // guarded external --url downloader cannot use Lark API runtime helpers.
	if base == nil {
		base = &http.Client{} //nolint:forbidigo // fallback only; caller normally supplies Factory.HttpClient.
	}
	cloned := *base
	if cloned.Timeout == 0 { //nolint:forbidigo // external download timeout guard on cloned client.
		cloned.Timeout = 30 * time.Second //nolint:forbidigo // external download timeout guard on cloned client.
	}
	cloned.Transport = cloneDocCoverTransport(base.Transport)                   //nolint:forbidigo // external download transport adds proxy/IP guards.
	cloned.CheckRedirect = func(req *http.Request, via []*http.Request) error { //nolint:forbidigo // redirects must be validated for external --url downloads.
		if len(via) >= 3 {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "cover URL redirects too many times").WithParam("--url")
		}
		if len(via) > 0 {
			prev := via[len(via)-1]
			if strings.EqualFold(prev.URL.Scheme, "https") && strings.EqualFold(req.URL.Scheme, "http") {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "cover URL redirect from https to http is not allowed").WithParam("--url")
			}
		}
		_, err := parseAndValidateDocCoverURL(req.Context(), req.URL.String())
		return err
	}
	return &cloned
}

func cloneDocCoverTransport(base http.RoundTripper) *http.Transport { //nolint:forbidigo // external --url downloader wraps caller transport with IP/proxy guards.
	var cloned *http.Transport
	if src, ok := base.(*http.Transport); ok && src != nil {
		cloned = src.Clone()
	} else if def, ok := http.DefaultTransport.(*http.Transport); ok && def != nil { //nolint:forbidigo // fallback for guarded external downloader only.
		cloned = def.Clone()
	} else {
		cloned = &http.Transport{}
	}
	cloned.Proxy = nil

	origDial := cloned.DialContext
	cloned.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		conn, err := dialDocCoverConn(ctx, origDial, network, addr)
		if err != nil {
			return nil, err
		}
		if err := validateDocCoverConnRemoteIP(conn); err != nil {
			conn.Close()
			return nil, err
		}
		return conn, nil
	}
	if cloned.DialTLSContext != nil {
		origDialTLS := cloned.DialTLSContext
		cloned.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			conn, err := dialDocCoverConn(ctx, origDialTLS, network, addr)
			if err != nil {
				return nil, err
			}
			if err := validateDocCoverConnRemoteIP(conn); err != nil {
				conn.Close()
				return nil, err
			}
			return conn, nil
		}
	}
	return cloned
}

func dialDocCoverConn(ctx context.Context, dialFn func(context.Context, string, string) (net.Conn, error), network, addr string) (net.Conn, error) {
	if dialFn != nil {
		return dialFn(ctx, network, addr)
	}
	var dialer net.Dialer
	return dialer.DialContext(ctx, network, addr)
}

func validateDocCoverConnRemoteIP(conn net.Conn) error {
	if conn == nil {
		return docCoverURLGuardError("nil connection")
	}
	addr := conn.RemoteAddr()
	if addr == nil {
		return docCoverURLGuardError("missing remote address")
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		host = addr.String()
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return docCoverURLGuardError("invalid remote IP")
	}
	if isUnsafeDocCoverIP(ip) {
		return docCoverURLGuardError("local/internal host is not allowed")
	}
	return nil
}
