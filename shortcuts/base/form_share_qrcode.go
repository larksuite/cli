// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/shortcuts/common"
)

const formShareQRCodeSegment = "share-qrcode"

// 服务端固定返回 PNG。落盘前同时校验 mime_type 和真实字节头，
// 避免把一个错误响应体写成后缀是 .png 的垃圾文件。
const formShareQRCodeMIMEType = "image/png"

// PNG 签名（PNG 规范 §5.2）：\x89 P N G \r \n \x1a \n。
var formShareQRCodePNGMagic = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}

// 服务端 file_name 缺失时的兜底名。
const formShareQRCodeFallbackName = "form-qrcode.png"

var BaseFormShareQRCodeGet = common.Shortcut{
	Service:     "base",
	Command:     "+form-share-qrcode-get",
	Description: "Get the share QR code of a form as base64 PNG",
	Risk:        "read",
	Scopes:      []string{"base:form:read"},
	AuthTypes:   authTypes(),
	Flags:       formRefFlags(),
	Tips: []string{
		"Form sharing must already be enabled: with sharing off the API returns invalid_request. Tell the user to enable sharing (+form-share-update --enabled=true) — this command never turns it on.",
		"The response carries share_url, file_name, mime_type and content_base64 (the raw PNG payload). To put the image on disk use +form-share-qrcode-download instead of piping base64 through a shell.",
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return setFormRefDryRun(
			common.NewDryRunAPI().GET(formSubDryRunPath(formShareQRCodeSegment)),
			runtime,
		)
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		data, err := fetchFormShareQRCode(runtime)
		if err != nil {
			return err
		}
		runtime.Out(data, nil)
		return nil
	},
}

var BaseFormShareQRCodeDownload = common.Shortcut{
	Service:     "base",
	Command:     "+form-share-qrcode-download",
	Description: "Download the share QR code of a form to a local PNG file",
	// 与 +record-download-attachment / drive +download / doc +media-download 一致：
	// 下载类命令按远端副作用定级，本地落盘不算 write。
	Risk:      "read",
	Scopes:    []string{"base:form:read"},
	AuthTypes: authTypes(),
	Flags: append(formRefFlags(),
		common.Flag{
			Name: "output",
			Desc: "output file path or directory (within the allowed roots: cwd, /tmp, ~/files); defaults to the API-provided file_name in the working directory",
		},
		common.Flag{Name: "overwrite", Type: "bool", Desc: "overwrite existing output file"},
	),
	Tips: []string{
		"Form sharing must already be enabled: with sharing off the API returns invalid_request. Tell the user to enable sharing (+form-share-update --enabled=true) — this command never turns it on.",
		"Same request as +form-share-qrcode-get; the only difference is that the PNG is written to disk and the final path is returned instead of the base64 payload.",
		"--output is optional. Omitted, the file lands in the working directory under the API-provided file_name; a directory value keeps that file name inside it. Paths must stay inside cwd, /tmp or ~/files.",
		"An existing file at the target is refused unless --overwrite is passed; without --output the file name comes from the server, so repeated downloads collide.",
		"The response is validated before anything is written: mime_type must be image/png, content_base64 must decode, and the decoded bytes must start with the PNG signature.",
		"If the local write fails, rerun with a writable --output; the request is a read-only GET, so re-sending it has no side effects.",
	},
	Validate: func(_ context.Context, runtime *common.RuntimeContext) error {
		return validateFormShareQRCodeOutput(runtime)
	},
	DryRun: func(_ context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		return setFormRefDryRun(
			common.NewDryRunAPI().GET(formSubDryRunPath(formShareQRCodeSegment)),
			runtime,
		).Set("output", formShareQRCodeOutputPreview(runtime))
	},
	Execute: func(_ context.Context, runtime *common.RuntimeContext) error {
		// 落盘能力先于请求判定：provider 能不能独占提交是环境属性，与响应无关，
		// 放在请求之后只会每次都先白花一次 API 调用才发现它必然失败。
		exclusive, err := formShareQRCodeExclusiveIO(runtime)
		if err != nil {
			return err
		}

		data, err := fetchFormShareQRCode(runtime)
		if err != nil {
			return err
		}
		qrcode := projectFormShareQRCode(data)
		png, err := decodeFormShareQRCode(qrcode)
		if err != nil {
			return err
		}

		targetPath := formShareQRCodeTargetPath(runtime, qrcode)
		savedPath, size, err := saveFormShareQRCode(runtime, exclusive, targetPath, png)
		if err != nil {
			return err
		}

		runtime.Out(map[string]interface{}{
			"saved_path": savedPath,
			"file_name":  filepath.Base(savedPath),
			"mime_type":  formShareQRCodeMIMEType,
			"size_bytes": size,
			"share_url":  qrcode.ShareURL,
		}, nil)
		return nil
	},
}

// formShareQRCodeResponse 是下载命令从 share-qrcode 响应里消费的字段。
// 落点和写盘都依赖这几个值，先投影成定型结构再往下传，避免各处各自从 map 断言。
type formShareQRCodeResponse struct {
	ShareURL      string
	FileName      string
	MIMEType      string
	ContentBase64 string
}

func projectFormShareQRCode(data map[string]interface{}) formShareQRCodeResponse {
	return formShareQRCodeResponse{
		ShareURL:      common.GetString(data, "share_url"),
		FileName:      common.GetString(data, "file_name"),
		MIMEType:      common.GetString(data, "mime_type"),
		ContentBase64: common.GetString(data, "content_base64"),
	}
}

// validateFormShareQRCodeOutput 在 Validate 阶段校验显式给出的 --output，
// 这样 dry-run 与真实执行拒绝同一批路径，越界也不会先花掉一次请求。
// 未给 --output 时落点来自响应里的 file_name，只能在拿到响应后由写盘前的校验兜住。
func validateFormShareQRCodeOutput(runtime *common.RuntimeContext) error {
	output := strings.TrimSpace(runtime.Str("output"))
	if output == "" {
		return nil
	}
	if _, err := runtime.ResolveSavePath(output); err != nil {
		// 点名 --output，与 drive/doc/im 的同类下载命令一致；也与写盘前对
		// 「--output 拼上服务端 file_name 之后的完整路径」的校验区分开。
		return baseValidationErrorf("--output is not a safe output path: %s", err)
	}
	return nil
}

// fetchFormShareQRCode 发起两条二维码命令共用的那一次 GET：无 body、无 query 参数。
func fetchFormShareQRCode(runtime *common.RuntimeContext) (map[string]interface{}, error) {
	return baseV3Call(runtime, "GET", formSubPath(runtime, formShareQRCodeSegment), nil, nil)
}

// decodeFormShareQRCode 在落盘前把响应校验成真实 PNG 字节：先看声明的
// mime_type，再看 base64 能否解码，最后看解码结果的文件头。三道校验都属于
// 响应契约问题（不是用户输入问题），所以归类为 invalid_response。
func decodeFormShareQRCode(qrcode formShareQRCodeResponse) ([]byte, error) {
	if got := stripMIMEParams(strings.TrimSpace(qrcode.MIMEType)); !strings.EqualFold(got, formShareQRCodeMIMEType) {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"form QR code mime_type is %q; expected %s", qrcode.MIMEType, formShareQRCodeMIMEType)
	}

	encoded := strings.TrimSpace(qrcode.ContentBase64)
	if encoded == "" {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"form QR code response is missing content_base64")
	}

	png, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"form QR code content_base64 is not valid base64: %s", err).WithCause(err)
	}
	if !bytes.HasPrefix(png, formShareQRCodePNGMagic) {
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"form QR code content_base64 does not decode to a PNG image")
	}
	return png, nil
}

// formShareQRCodeTargetPath 决定落盘位置：--output 未传时用响应里的
// file_name；传的是已存在的目录时把 file_name 拼进去；否则原样当文件路径。
func formShareQRCodeTargetPath(runtime *common.RuntimeContext, qrcode formShareQRCodeResponse) string {
	fileName := formShareQRCodeFileName(qrcode.FileName)
	output := strings.TrimSpace(runtime.Str("output"))
	if output == "" {
		return fileName
	}
	if outputPathLooksDirectory(runtime, output) {
		return filepath.Join(output, fileName)
	}
	return output
}

// formShareQRCodeFileName 取服务端返回的文件名，并只保留基名：响应里的
// 相对路径不能把写入位置带出 --output 指定的目录。
func formShareQRCodeFileName(serverName string) string {
	name := filepath.Base(strings.TrimSpace(serverName))
	// ".." 也会原样穿过 filepath.Base，漏掉它就等于允许响应把落点抬高一层目录。
	if name == "" || name == "." || name == ".." || name == string(filepath.Separator) {
		return formShareQRCodeFallbackName
	}
	return name
}

// formShareQRCodeOutputPreview 供 dry-run 展示落点。此时还没有发请求，拿不到
// file_name，所以未传 --output 时只说明会用服务端返回的文件名。
func formShareQRCodeOutputPreview(runtime *common.RuntimeContext) string {
	if output := strings.TrimSpace(runtime.Str("output")); output != "" {
		return output
	}
	return "<file_name from response>"
}

// formShareQRCodeExclusiveIO 取出能独占提交的 FileIO。放在发请求之前调用：
// provider 支不支持独占提交是环境属性，与响应无关。
// 传了 --overwrite 时不需要独占提交，返回 (nil, nil)，写盘走普通 Save——与
// commandhost 下载只在 IfExistsFail 时才要求 ExclusiveFileIO 的策略一致。
func formShareQRCodeExclusiveIO(runtime *common.RuntimeContext) (fileio.ExclusiveFileIO, error) {
	fileIO := runtime.FileIO()
	if fileIO == nil {
		return nil, baseMissingFileIOError("form QR code download requires a file I/O provider")
	}
	if runtime.Bool("overwrite") {
		return nil, nil
	}
	// SaveExclusive 只在 ExclusiveFileIO 上，做不到独占提交的 provider 按接口约定不实现它；
	// 断言失败必须显式拒绝而不是回落到 Save，否则就是「声称不覆盖、实际覆盖」。
	//
	// 这里失败的是 provider 的类型，与落点路径无关——换任何文件名结果都一样。
	// 所以文案不能提落点，否则调用方会当成路径冲突去改名重试。
	exclusive, ok := fileIO.(fileio.ExclusiveFileIO)
	if !ok {
		return nil, errs.NewValidationError(errs.SubtypeFailedPrecondition,
			"the configured file provider cannot commit files exclusively, so this command cannot guarantee it will not overwrite").
			WithHint("this is a capability of the host's file provider, not a collision at the output path: a different --output will fail the same way. Pass --overwrite to allow replacing an existing file, or run where the provider implements exclusive commit")
	}
	return exclusive, nil
}

// saveFormShareQRCode 通过 FileIO 抽象写盘（路径受 cwd / /tmp / ~/files 允许
// 根目录约束），返回规范化后的最终路径和实际字节数。exclusive 为 nil 表示调用方
// 传了 --overwrite，走普通 Save；否则走独占提交。
func saveFormShareQRCode(runtime *common.RuntimeContext, exclusive fileio.ExclusiveFileIO, targetPath string, png []byte) (string, int64, error) {
	if _, err := runtime.ResolveSavePath(targetPath); err != nil {
		return "", 0, baseValidationErrorf("unsafe output path: %s", err)
	}

	opts := fileio.SaveOptions{
		ContentType:   formShareQRCodeMIMEType,
		ContentLength: int64(len(png)),
	}
	var (
		result fileio.SaveResult
		err    error
	)
	if exclusive != nil {
		// 落点文件名可能来自服务端的 file_name（用户和模型都没指定过），覆盖已有文件是
		// 不可逆的——底层是 tmp+rename 的原子替换，连回收站都不留。所以默认由提交本身
		// 保证不覆盖，而不是先 Stat 再写（两步之间文件可能被创建）。
		result, err = exclusive.SaveExclusive(targetPath, opts, bytes.NewReader(png))
		if errors.Is(err, fs.ErrExist) {
			return "", 0, errs.NewValidationError(errs.SubtypeFailedPrecondition,
				"output file %s already exists", targetPath).
				WithHint("pass --overwrite to replace it, or a different --output. Without --output the file name comes from the server response, so an explicit --output is the reliable way to avoid a collision")
		}
	} else {
		result, err = runtime.FileIO().Save(targetPath, opts, bytes.NewReader(png))
	}
	if err != nil {
		return "", 0, formShareQRCodeSaveError(err, targetPath)
	}

	savedPath, _ := runtime.ResolveSavePath(targetPath)
	if savedPath == "" {
		savedPath = targetPath
	}
	return savedPath, result.Size(), nil
}

// formShareQRCodeSaveError 沿用 baseSaveError 的分类，补一条重试提示：失败的是
// 本地写入，换个可写位置重跑即可；请求是只读 GET，重跑时重发没有副作用。
func formShareQRCodeSaveError(err error, targetPath string) error {
	saveErr := baseSaveError(err)
	if p, ok := errs.ProblemOf(saveErr); ok && p.Hint == "" {
		p.Hint = fmt.Sprintf(
			"The local write to %s failed. Rerun with a writable --output (cwd, /tmp or ~/files); the request is a read-only GET, so re-sending it has no side effects.",
			targetPath,
		)
	}
	return saveErr
}
