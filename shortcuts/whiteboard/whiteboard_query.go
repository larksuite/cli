// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT
package whiteboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/internal/validate"
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
		// 检查 token 是否包含控制字符
		token := runtime.Str("whiteboard-token")
		if err := validate.RejectControlChars(token, "whiteboard-token"); err != nil {
			return err
		}
		out := runtime.Str("output")
		if out != "" {
			_, err := validate.SafeOutputPath(out)
			if err != nil {
				return err
			}
		}
		if out == "" && runtime.Str("output_as") == WhiteboardQueryAsImage {
			return output.ErrValidation("need a output directory to query whiteboard as image")
		}

		as := runtime.Str("output_as")
		if as != WhiteboardQueryAsImage && as != WhiteboardQueryAsCode && as != WhiteboardQueryAsRaw {
			return common.FlagErrorf("--output_as flag must be one of: image | code | raw")
		}
		return nil
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		as := runtime.Str("output_as")
		switch as {
		case WhiteboardQueryAsImage:
			return common.NewDryRunAPI().
				GET(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/download_as_image", runtime.Str("whiteboard-token"))).
				Desc("Export preview image of given whiteboard")
		case WhiteboardQueryAsCode:
			return common.NewDryRunAPI().
				GET(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", runtime.Str("whiteboard-token"))).
				Desc("Extract Mermaid/Plantuml code from given whiteboard")
		case WhiteboardQueryAsRaw:
			return common.NewDryRunAPI().
				GET(fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", runtime.Str("whiteboard-token"))).
				Desc("Extract raw nodes structure from given whiteboard")
		default:
			return common.NewDryRunAPI().Desc("invalid --as flag, must be one of: image | code | raw")
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
			return output.ErrValidation("--as flag must be one of: image | code | raw")
		}

	},
}

func ensurePNGExtension(path string) string {
	ext := filepath.Ext(path)
	if ext != ".png" {
		if ext == "" {
			path = path + ".png"
		} else {
			path = path[:len(path)-len(ext)] + ".png"
		}
	}
	return path
}

func exportWhiteboardPreview(ctx context.Context, runtime *common.RuntimeContext, wbToken, outDir string) error {
	req := &larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/download_as_image", wbToken),
	}
	// 执行 API 请求
	resp, err := runtime.DoAPI(req, larkcore.WithFileDownload())
	if err != nil {
		return output.ErrNetwork(fmt.Sprintf("get whiteboard preview failed: %v", err))
	}
	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		return output.ErrAPI(resp.StatusCode, string(resp.RawBody), nil)
	}

	outDir = ensurePNGExtension(outDir)
	overwrite := runtime.Bool("overwrite")
	if err := checkFileOverwrite(outDir, overwrite); err != nil {
		return err
	}

	if err := validate.AtomicWrite(outDir, resp.RawBody, 0644); err != nil {
		return output.Errorf(output.ExitInternal, "api_error", "cannot save preview image: %s", err)
	}
	runtime.OutFormat(map[string]interface{}{
		"preview_image_path": outDir,
		"size_bytes":         len(resp.RawBody),
	}, nil, func(w io.Writer) {
		fmt.Fprintf(w, "Preview image saved to %s\n", outDir)
		fmt.Fprintf(w, "Image size: %d bytes", len(resp.RawBody))
	})
	return nil
}

type wbNodesResp struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Nodes []interface{} `json:"nodes"`
	} `json:"data"`
}

func fetchWhiteboardNodes(runtime *common.RuntimeContext, wbToken string) (*wbNodesResp, error) {
	req := &larkcore.ApiReq{
		HttpMethod: http.MethodGet,
		ApiPath:    fmt.Sprintf("/open-apis/board/v1/whiteboards/%s/nodes", wbToken),
	}
	resp, err := runtime.DoAPI(req)
	if err != nil {
		return nil, output.ErrNetwork(fmt.Sprintf("get whiteboard nodes failed: %v", err))
	}
	// 检查响应状态码
	if resp.StatusCode != http.StatusOK {
		return nil, output.ErrAPI(resp.StatusCode, string(resp.RawBody), nil)
	}
	var nodes wbNodesResp
	err = json.Unmarshal(resp.RawBody, &nodes)
	if err != nil {
		return nil, output.Errorf(output.ExitInternal, "parsing", fmt.Sprintf("parse whiteboard nodes failed: %v", err))
	}
	if nodes.Code != 0 {
		return nil, output.ErrAPI(nodes.Code, "get whiteboard nodes failed", fmt.Sprintf("get whiteboard nodes failed: %s", nodes.Msg))
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

	finalPath, err := getFinalOutputPath(outDir, block.syntaxType.ExtensionName(), wbToken)
	if err != nil {
		return err
	}

	overwrite := runtime.Bool("overwrite")
	if err := checkFileOverwrite(finalPath, overwrite); err != nil {
		return err
	}

	safePath, err := validate.SafeOutputPath(finalPath)
	if err != nil {
		return err
	}

	if err := validate.AtomicWrite(safePath, []byte(block.code), 0644); err != nil {
		return output.Errorf(output.ExitInternal, "api_error", "cannot save code file: %s", err)
	}

	runtime.OutFormat(map[string]interface{}{
		"output_path": safePath,
	}, nil, func(w io.Writer) {
		fmt.Fprintf(w, "Whiteboard code saved to %s\n", safePath)
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
		return output.Errorf(output.ExitInternal, "json_error", "cannot marshal whiteboard data: %s", err)
	}

	if outDir == "" {
		runtime.OutFormat(wbNodes.Data, nil, func(w io.Writer) {
			fmt.Fprintf(w, "%s\n", string(jsonData))
		})
		return nil
	}

	finalPath, err := getFinalOutputPath(outDir, ".json", wbToken)
	if err != nil {
		return err
	}

	overwrite := runtime.Bool("overwrite")
	if err := checkFileOverwrite(finalPath, overwrite); err != nil {
		return err
	}

	safePath, err := validate.SafeOutputPath(finalPath)
	if err != nil {
		return err
	}

	if err := validate.AtomicWrite(safePath, jsonData, 0644); err != nil {
		return output.Errorf(output.ExitInternal, "api_error", "cannot save raw data: %s", err)
	}

	runtime.OutFormat(map[string]interface{}{
		"output_path": safePath,
	}, nil, func(w io.Writer) {
		fmt.Fprintf(w, "Whiteboard raw node structure saved to %s\n", safePath)
	})

	return nil
}

func getFinalOutputPath(outPath, ext, token string) (string, error) {
	info, err := os.Stat(outPath)
	if err == nil && info.IsDir() {
		return filepath.Join(outPath, fmt.Sprintf("whiteboard_%s%s", token, ext)), nil
	}
	// 修正路径中的扩展名，确保与实际的扩展名一致
	currentExt := filepath.Ext(outPath)
	if currentExt == ext {
		return outPath, nil
	}
	if currentExt != "" {
		outPath = outPath[:len(outPath)-len(currentExt)]
	}
	return outPath + ext, nil
}

func checkFileOverwrite(path string, overwrite bool) error {
	_, err := os.Stat(path)
	if err == nil {
		if !overwrite {
			return output.ErrValidation(fmt.Sprintf("file already exists: %s (use --overwrite to overwrite)", path))
		}
	} else if !os.IsNotExist(err) {
		return output.Errorf(output.ExitInternal, "io_error", "cannot check file existence: %s", err)
	}
	return nil
}
