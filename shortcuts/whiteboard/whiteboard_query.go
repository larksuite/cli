// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
package whiteboard

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/shortcuts/common"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"
)

const (
	// WhiteboardQueryAsImage exports a whiteboard preview image.
	WhiteboardQueryAsImage = "image"
	// WhiteboardQueryAsSvg exports a whiteboard as SVG.
	WhiteboardQueryAsSvg = "svg"
	// WhiteboardQueryAsCode exports Mermaid or PlantUML source extracted from the whiteboard.
	WhiteboardQueryAsCode = "code"
	// WhiteboardQueryAsRaw exports the raw whiteboard node payload.
	WhiteboardQueryAsRaw = "raw"
)

// SyntaxType identifies the diagram syntax extracted from whiteboard code blocks.
type SyntaxType int

const (
	// SyntaxTypePlantUML marks PlantUML code blocks.
	SyntaxTypePlantUML SyntaxType = 1
	// SyntaxTypeMermaid marks Mermaid code blocks.
	SyntaxTypeMermaid SyntaxType = 2
)

// SyntaxTypeNameMap maps whiteboard syntax types to their CLI output names.
var SyntaxTypeNameMap = map[SyntaxType]string{
	SyntaxTypePlantUML: "plantuml",
	SyntaxTypeMermaid:  "mermaid",
}

// SyntaxTypeExtensionMap maps whiteboard syntax types to their default file extensions.
var SyntaxTypeExtensionMap = map[SyntaxType]string{
	SyntaxTypePlantUML: ".puml",
	SyntaxTypeMermaid:  ".mmd",
}

// String returns the CLI-facing name for the syntax type.
func (s SyntaxType) String() string {
	return SyntaxTypeNameMap[s]
}

// ExtensionName returns the default file extension for the syntax type.
func (s SyntaxType) ExtensionName() string {
	return SyntaxTypeExtensionMap[s]
}

// IsValid reports whether the syntax type is one of the supported whiteboard code syntaxes.
func (s SyntaxType) IsValid() bool {
	return s == SyntaxTypePlantUML || s == SyntaxTypeMermaid
}

// WhiteboardQuery registers the `whiteboard +query` shortcut.
var WhiteboardQuery = common.Shortcut{
	Service:     "whiteboard",
	Command:     "+query",
	Description: "Query a existing whiteboard, export it as preview image or raw nodes structure.",
	Risk:        "read",
	Scopes:      []string{"board:whiteboard:node:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "whiteboard-token", Desc: "whiteboard token of the whiteboard. You will need read permission to download preview image.", Required: true},
		{Name: "output_as", Desc: "output whiteboard as: image | svg | code | raw.", Required: true},
		{Name: "output", Desc: "output directory. It is required when output as image. If not specified when --output_as svg/code/raw, it will output directly.", Required: false},
		{Name: "overwrite", Desc: "overwrite existing file if it exists", Required: false, Type: "bool"},
	},
	HasFormat: true,
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		// Check if token contains control characters
		token := runtime.Str("whiteboard-token")
		if err := common.RejectDangerousCharsTyped("--whiteboard-token", token); err != nil {
			return err
		}
		out := runtime.Str("output")
		if out != "" {
			if _, err := runtime.ResolveSavePath(out); err != nil {
				return errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid output path: %s", err).WithParam("--output").WithCause(err)
			}
		}
		if out == "" && runtime.Str("output_as") == WhiteboardQueryAsImage {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "need a output directory to query whiteboard as image").WithParam("--output")
		}

		as := runtime.Str("output_as")
		if as != WhiteboardQueryAsImage && as != WhiteboardQueryAsSvg && as != WhiteboardQueryAsCode && as != WhiteboardQueryAsRaw {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output_as flag must be one of: image | svg | code | raw").WithParam("--output_as")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		as := runtime.Str("output_as")
		token := runtime.Str("whiteboard-token")
		switch as {
		case WhiteboardQueryAsImage:
			return common.NewDryRunAPI().
				GET(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/download_as_image", common.MaskToken(url.PathEscape(token)))).
				Desc("Export preview image of given whiteboard")
		case WhiteboardQueryAsCode:
			return common.NewDryRunAPI().
				GET(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", common.MaskToken(url.PathEscape(token)))).
				Desc("Extract Mermaid/Plantuml code from given whiteboard")
		case WhiteboardQueryAsRaw:
			return common.NewDryRunAPI().
				GET(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", common.MaskToken(url.PathEscape(token)))).
				Desc("Extract raw nodes structure from given whiteboard")
		case WhiteboardQueryAsSvg:
			return common.NewDryRunAPI().
				POST(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/export", common.MaskToken(url.PathEscape(token)))).
				Body(map[string]string{"export_type": "svg"}).
				Desc("Export SVG of given whiteboard")
		default:
			return common.NewDryRunAPI().Desc("invalid --output_as flag, must be one of: image | svg | code | raw")
		}
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		// 构建 API 请求
		token := runtime.Str("whiteboard-token")
		outDir := runtime.Str("output")
		as := runtime.Str("output_as")
		switch as {
		case WhiteboardQueryAsImage:
			return exportWhiteboardPreview(ctx, runtime, token, outDir)
		case WhiteboardQueryAsSvg:
			return exportWhiteboardSvg(runtime, token, outDir)
		case WhiteboardQueryAsCode:
			return exportWhiteboardCode(runtime, token, outDir)
		case WhiteboardQueryAsRaw:
			return exportWhiteboardRaw(runtime, token, outDir)
		default:
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output_as flag must be one of: image | svg | code | raw").WithParam("--output_as")
		}

	},
}

// exportReq defines the request body for whiteboard export APIs.
type exportReq struct {
	ExportType string `json:"export_type"`
}

// exportResp models the whiteboard export response envelope.
type exportResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Content  string `json:"content"`
		MimeType string `json:"mime_type"`
	} `json:"data"`
}

// exportWhiteboardSvg exports a whiteboard as SVG and writes it to stdout or a file.
func exportWhiteboardSvg(runtime *common.RuntimeContext, wbToken, outDir string) error {
	reqBody := exportReq{ExportType: "svg"}
	req := &larkcore.ApiReq{
		HttpMethod: http.MethodPost,
		ApiPath:    fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/export", url.PathEscape(wbToken)),
		Body:       reqBody,
	}

	resp, err := runtime.DoAPI(req)
	if err != nil {
		return wrapWbNetworkErr(err, "export whiteboard svg failed: %v", err)
	}

	var exportData exportResp
	if err := json.Unmarshal(resp.RawBody, &exportData); err == nil {
		if exportData.Code != 0 {
			subtype := errs.SubtypeUnknown
			if resp.StatusCode == http.StatusNotFound {
				subtype = errs.SubtypeNotFound
			}
			return errs.NewAPIError(subtype, "export whiteboard svg failed: %s", exportData.Msg).WithCode(exportData.Code)
		}
	} else if resp.StatusCode == http.StatusOK {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "parse export response failed: %v", err).WithCause(err)
	}

	if resp.StatusCode != http.StatusOK {
		body := common.TruncateStr(strings.TrimSpace(string(resp.RawBody)), 500)
		if resp.StatusCode >= 500 {
			return errs.NewNetworkError(errs.SubtypeNetworkServer, "export whiteboard svg failed: HTTP %d: %s", resp.StatusCode, body).
				WithCode(resp.StatusCode).
				WithRetryable()
		}
		subtype := errs.SubtypeUnknown
		if resp.StatusCode == http.StatusNotFound {
			subtype = errs.SubtypeNotFound
		}
		return errs.NewAPIError(subtype, "export whiteboard svg failed: HTTP %d: %s", resp.StatusCode, body).
			WithCode(resp.StatusCode)
	}

	svgBytes, err := base64.StdEncoding.DecodeString(exportData.Data.Content)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "decode svg base64 failed: %v", err).WithCause(err)
	}

	if outDir == "" {
		runtime.OutFormat(map[string]interface{}{
			"svg_content": string(svgBytes),
		}, nil, func(w io.Writer) {
			fmt.Fprintf(w, "%s\n", string(svgBytes))
		})
		return nil
	}

	finalPath, size, err := saveOutputFile(outDir, ".svg", wbToken, runtime, bytes.NewReader(svgBytes))
	if err != nil {
		return err
	}

	runtime.OutFormat(map[string]interface{}{
		"svg_path":   finalPath,
		"size_bytes": size,
	}, nil, func(w io.Writer) {
		fmt.Fprintf(w, "SVG saved to %s\n", finalPath)
		fmt.Fprintf(w, "File size: %d bytes", size)
	})
	return nil
}

func exportWhiteboardPreview(ctx context.Context, runtime *common.RuntimeContext, wbToken, outDir string) error {
	req := &larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/download_as_image", url.PathEscape(wbToken)),
	}
	// Execute API request. The preview endpoint streams raw image bytes (not a
	// JSON envelope), so classify by HTTP status: 5xx is retryable network,
	// while 4xx remains an API-side rejection.
	resp, err := runtime.DoAPI(req, larkcore.WithFileDownload())
	if err != nil {
		return wrapWbNetworkErr(err, "get whiteboard preview failed: %v", err)
	}
	if resp.StatusCode >= 400 {
		body := common.TruncateStr(strings.TrimSpace(string(resp.RawBody)), 500)
		if resp.StatusCode >= 500 {
			return errs.NewNetworkError(errs.SubtypeNetworkServer, "get whiteboard preview failed: HTTP %d: %s", resp.StatusCode, body).
				WithCode(resp.StatusCode).
				WithRetryable()
		}
		subtype := errs.SubtypeUnknown
		if resp.StatusCode == http.StatusNotFound {
			subtype = errs.SubtypeNotFound
		}
		return errs.NewAPIError(subtype, "get whiteboard preview failed: HTTP %d: %s", resp.StatusCode, body).
			WithCode(resp.StatusCode)
	}

	finalPath, size, err := saveOutputFile(outDir, ".png", wbToken, runtime, bytes.NewReader(resp.RawBody))
	if err != nil {
		return err
	}

	runtime.OutFormat(map[string]interface{}{
		"preview_image_path": finalPath,
		"size_bytes":         size,
	}, nil, func(w io.Writer) {
		fmt.Fprintf(w, "Preview image saved to %s\n", finalPath)
		fmt.Fprintf(w, "Image size: %d bytes", size)
	})
	return nil
}

type wbNodesResp struct {
	Data struct {
		Nodes []interface{} `json:"nodes"`
	} `json:"data"`
}

func fetchWhiteboardNodes(runtime *common.RuntimeContext, wbToken string) (*wbNodesResp, error) {
	data, err := runtime.CallAPITyped(http.MethodGet, fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", url.PathEscape(wbToken)), nil, nil)
	if err != nil {
		return nil, err
	}
	var nodes wbNodesResp
	rawNodes, _ := data["nodes"]
	if rawNodes != nil {
		var ok bool
		nodes.Data.Nodes, ok = rawNodes.([]interface{})
		if !ok {
			return nil, wbInvalidResponse("get whiteboard nodes failed: data.nodes must be an array")
		}
	}
	return &nodes, nil
}

type syntaxInfo struct {
	code       string
	syntaxType SyntaxType
}

func exportWhiteboardCode(runtime *common.RuntimeContext, wbToken, outDir string) error {
	wbNodes, err := fetchWhiteboardNodes(runtime, wbToken)
	if err != nil {
		return err
	}
	if wbNodes == nil || wbNodes.Data.Nodes == nil {
		runtime.OutFormat(map[string]interface{}{
			"msg": "whiteboard is empty",
		}, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Whiteboard is empty\n")
		})
		return nil
	}

	var syntaxBlocks []syntaxInfo
	for _, node := range wbNodes.Data.Nodes {
		nodeMap, ok := node.(map[string]interface{})
		if !ok {
			continue
		}
		syntax, ok := nodeMap["syntax"]
		if !ok {
			continue
		}
		syntaxMap, ok := syntax.(map[string]interface{})
		if !ok {
			continue
		}
		code, _ := syntaxMap["code"].(string)
		var syntaxType SyntaxType
		switch v := syntaxMap["syntax_type"].(type) {
		case json.Number:
			// runtime.ClassifyAPIResponse decodes the response with UseNumber,
			// so numeric fields arrive as json.Number rather than float64.
			if n, err := v.Int64(); err == nil {
				syntaxType = SyntaxType(n)
			}
		case float64:
			syntaxType = SyntaxType(v)
		case SyntaxType:
			syntaxType = v
		}
		if code != "" && syntaxType.IsValid() {
			syntaxBlocks = append(syntaxBlocks, syntaxInfo{code: code, syntaxType: syntaxType})
		}
	}

	if len(syntaxBlocks) == 0 {
		runtime.OutFormat(map[string]interface{}{
			"msg": "no code blocks found in whiteboard",
		}, nil, func(w io.Writer) {
			fmt.Fprintf(w, "No code blocks found in whiteboard\n")
		})
		return nil
	}
	// 目前的标准操作是导出到单一文件，和 Doc 展示画板代码块采用相同的逻辑
	// 如果有需求，可以调整到导出到多个文件的模式
	if len(syntaxBlocks) > 1 {
		runtime.OutFormat(map[string]interface{}{
			"msg": "multiple code blocks found, cannot export directly",
		}, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Multiple code blocks found, cannot export directly\n")
		})
		return nil
	}
	block := syntaxBlocks[0]

	if outDir == "" {
		runtime.OutFormat(map[string]interface{}{
			"code":        block.code,
			"syntax_type": block.syntaxType.String(),
		}, nil, func(w io.Writer) {
			fmt.Fprintf(w, "%s\n", block.code)
		})
		return nil
	}

	finalPath, _, err := saveOutputFile(outDir, block.syntaxType.ExtensionName(), wbToken, runtime, strings.NewReader(block.code))
	if err != nil {
		return err
	}

	runtime.OutFormat(map[string]interface{}{
		"output_path": finalPath,
	}, nil, func(w io.Writer) {
		fmt.Fprintf(w, "Whiteboard code saved to %s\n", finalPath)
	})

	return nil
}

func exportWhiteboardRaw(runtime *common.RuntimeContext, wbToken, outDir string) error {
	wbNodes, err := fetchWhiteboardNodes(runtime, wbToken)
	if err != nil {
		return err
	}
	if wbNodes == nil || wbNodes.Data.Nodes == nil {
		runtime.OutFormat(map[string]interface{}{
			"msg": "whiteboard is empty",
		}, nil, func(w io.Writer) {
			fmt.Fprintf(w, "Whiteboard is empty\n")
		})
		return nil
	}

	jsonData, err := json.MarshalIndent(wbNodes.Data, "", "  ")
	if err != nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "cannot marshal whiteboard data: %s", err).WithCause(err)
	}

	if outDir == "" {
		runtime.OutFormat(wbNodes.Data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "%s\n", string(jsonData))
		})
		return nil
	}

	finalPath, _, err := saveOutputFile(outDir, ".json", wbToken, runtime, bytes.NewReader(jsonData))
	if err != nil {
		return err
	}

	runtime.OutFormat(map[string]interface{}{
		"output_path": finalPath,
	}, nil, func(w io.Writer) {
		fmt.Fprintf(w, "Whiteboard raw node structure saved to %s\n", finalPath)
	})

	return nil
}

func saveOutputFile(outPath, ext, token string, runtime *common.RuntimeContext, data io.Reader) (string, int64, error) {
	// Step 1: Get final output path
	info, err := runtime.FileIO().Stat(outPath)
	var finalPath string
	if err == nil && info.IsDir() {
		finalPath = filepath.Join(outPath, fmt.Sprintf("whiteboard_%s%s", token, ext))
	} else {
		// Fix extension in path
		currentExt := filepath.Ext(outPath)
		if currentExt != ext {
			if currentExt != "" {
				outPath = outPath[:len(outPath)-len(currentExt)]
			}
			outPath += ext
		}
		finalPath = outPath
	}
	if _, err := runtime.ResolveSavePath(finalPath); err != nil { // double check
		return "", 0, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid output path: %s", err).WithParam("--output").WithCause(err)
	}

	// Step 2: Check overwrite
	_, err = runtime.FileIO().Stat(finalPath)
	if err == nil {
		if !runtime.Bool("overwrite") {
			return "", 0, errs.NewValidationError(errs.SubtypeInvalidArgument, "file already exists: %s (use --overwrite to overwrite)", finalPath).WithParam("--overwrite")
		}
	} else if !os.IsNotExist(err) {
		return "", 0, errs.NewInternalError(errs.SubtypeFileIO, "cannot check file existence: %s", err).WithCause(err)
	}

	// Step 3: Save file
	var contentType string
	switch ext {
	case ".png":
		contentType = "image/png"
	case ".svg":
		contentType = "image/svg+xml"
	case ".json":
		contentType = "application/json"
	case ".mmd", ".puml":
		contentType = "text/plain"
	}

	savResult, err := runtime.FileIO().Save(finalPath, fileio.SaveOptions{
		ContentType: contentType,
	}, data)
	if err != nil {
		return "", 0, wbSaveError(err)
	}

	return finalPath, savResult.Size(), nil
}
