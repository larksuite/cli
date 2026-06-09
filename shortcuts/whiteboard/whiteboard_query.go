// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
package whiteboard

import (
	"bytes"
	"context"
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
	WhiteboardQueryAsImage = "image"
	WhiteboardQueryAsCode  = "code"
	WhiteboardQueryAsRaw   = "raw"
)

type SyntaxType int

const (
	SyntaxTypePlantUML SyntaxType = 1
	SyntaxTypeMermaid  SyntaxType = 2
)

var SyntaxTypeNameMap = map[SyntaxType]string{
	SyntaxTypePlantUML: "plantuml",
	SyntaxTypeMermaid:  "mermaid",
}

var SyntaxTypeExtensionMap = map[SyntaxType]string{
	SyntaxTypePlantUML: ".puml",
	SyntaxTypeMermaid:  ".mmd",
}

func (s SyntaxType) String() string {
	return SyntaxTypeNameMap[s]
}

func (s SyntaxType) ExtensionName() string {
	return SyntaxTypeExtensionMap[s]
}

func (s SyntaxType) IsValid() bool {
	return s == SyntaxTypePlantUML || s == SyntaxTypeMermaid
}

var WhiteboardQuery = common.Shortcut{
	Service:     "whiteboard",
	Command:     "+query",
	Description: "Query a existing whiteboard, export it as preview image or raw nodes structure.",
	Risk:        "read",
	Scopes:      []string{"board:whiteboard:node:read"},
	AuthTypes:   []string{"user", "bot"},
	Flags: []common.Flag{
		{Name: "whiteboard-token", Desc: "whiteboard token of the whiteboard. You will need read permission to download preview image.", Required: true},
		{Name: "output_as", Desc: "output whiteboard as: image | code | raw.", Required: true},
		{Name: "output", Desc: "output directory. It is required when output as image. If not specified when --output_as code/raw, it will output directly.", Required: false},
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
		if as != WhiteboardQueryAsImage && as != WhiteboardQueryAsCode && as != WhiteboardQueryAsRaw {
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output_as flag must be one of: image | code | raw").WithParam("--output_as")
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
		default:
			return common.NewDryRunAPI().Desc("invalid --output_as flag, must be one of: image | code | raw")
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
		case WhiteboardQueryAsCode:
			return exportWhiteboardCode(runtime, token, outDir)
		case WhiteboardQueryAsRaw:
			return exportWhiteboardRaw(runtime, token, outDir)
		default:
			return errs.NewValidationError(errs.SubtypeInvalidArgument, "--output_as flag must be one of: image | code | raw").WithParam("--output_as")
		}

	},
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
	rawNodes, ok := data["nodes"]
	if !ok {
		return nil, wbInvalidResponse("get whiteboard nodes failed: missing data.nodes")
	}
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
