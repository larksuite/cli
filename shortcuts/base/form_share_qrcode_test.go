// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package base

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/extension/fileio"
	"github.com/larksuite/cli/internal/cmdutil"
)

// nonExclusiveFileIO 只实现基础 FileIO，不提供 SaveExclusive。仓内唯一的
// LocalFileIO 是实现了的，所以拒绝分支只能靠这种宿主注册的 provider 触达。
type nonExclusiveFileIO struct {
	base fileio.FileIO
}

func (f *nonExclusiveFileIO) Open(name string) (fileio.File, error) { return f.base.Open(name) }

func (f *nonExclusiveFileIO) Stat(name string) (fileio.FileInfo, error) { return f.base.Stat(name) }

func (f *nonExclusiveFileIO) ResolvePath(path string) (string, error) {
	return f.base.ResolvePath(path)
}

func (f *nonExclusiveFileIO) Save(path string, opts fileio.SaveOptions, body io.Reader) (fileio.SaveResult, error) {
	return f.base.Save(path, opts, body)
}

type staticFileIOProvider struct {
	fileIO fileio.FileIO
}

func (p *staticFileIOProvider) Name() string { return "static" }

func (p *staticFileIOProvider) ResolveFileIO(context.Context) fileio.FileIO { return p.fileIO }

// provider 做不到独占提交时必须显式拒绝，而不是回落到覆盖式 Save——否则就是
// 「声称不覆盖、实际覆盖」。断言失败的是 provider 的类型，与落点路径无关，所以
// 提示不能把它说成路径冲突：调用方照着去改名或删文件，只会原地打转，还可能删掉
// 一个与本次无关的文件。
func TestSaveFormShareQRCodeRefusesProviderWithoutExclusiveCommit(t *testing.T) {
	runtime := newBaseTestRuntime(nil, map[string]bool{"overwrite": false}, nil)
	prev := fileio.GetProvider()
	fileio.Register(&staticFileIOProvider{
		fileIO: &nonExclusiveFileIO{base: prev.ResolveFileIO(context.Background())},
	})
	t.Cleanup(func() { fileio.Register(prev) })

	cmdutil.TestChdir(t, t.TempDir())
	const target = "qrcode.png"
	_, err := formShareQRCodeExclusiveIO(runtime)
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed error, got %T %v", err, err)
	}
	if p.Subtype != errs.SubtypeFailedPrecondition {
		t.Fatalf("subtype = %v, want FailedPrecondition", p.Subtype)
	}
	if strings.Contains(p.Message, target) {
		t.Fatalf("message names the output path, which reads as a collision: %q", p.Message)
	}
	for _, banned := range []string{"delete", "remove", "already exists"} {
		if strings.Contains(strings.ToLower(p.Hint), banned) {
			t.Fatalf("hint steers the caller at the file (%q): %q", banned, p.Hint)
		}
	}
	if !strings.Contains(p.Hint, "--output") {
		t.Fatalf("hint should say a different --output fails the same way: %q", p.Hint)
	}
	if _, statErr := os.Stat(target); statErr == nil {
		t.Fatalf("refusal still wrote %s", target)
	}
}

// 落点已被占用时换名字是有效的，但删文件不是这条命令该教给调用方的动作——
// 那是用户的文件，不可逆，且服务端返回的文件名本来就可能撞上无关文件。
func TestSaveFormShareQRCodeExistingTargetSuggestsRenameNotDeletion(t *testing.T) {
	runtime := newBaseTestRuntime(nil, map[string]bool{"overwrite": false}, nil)
	cmdutil.TestChdir(t, t.TempDir())
	const target = "qrcode.png"
	if err := os.WriteFile(target, []byte("existing"), 0o600); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}

	exclusive, err := formShareQRCodeExclusiveIO(runtime)
	if err != nil {
		t.Fatalf("resolve exclusive provider: %v", err)
	}
	_, _, err = saveFormShareQRCode(runtime, exclusive, target, []byte("png"))
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected a typed error, got %T %v", err, err)
	}
	if !strings.Contains(p.Message, "already exists") {
		t.Fatalf("message should name the collision: %q", p.Message)
	}
	for _, want := range []string{"--output", "--overwrite"} {
		if !strings.Contains(p.Hint, want) {
			t.Fatalf("hint should point at %s: %q", want, p.Hint)
		}
	}
	for _, banned := range []string{"delete", "remove"} {
		if strings.Contains(strings.ToLower(p.Hint), banned) {
			t.Fatalf("hint tells the caller to destroy a user file (%q): %q", banned, p.Hint)
		}
	}
	kept, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read back target: %v", readErr)
	}
	if string(kept) != "existing" {
		t.Fatalf("existing file was clobbered: %q", kept)
	}
}

// 服务端返回的 file_name 是外部输入，它唯一的过滤就是这个函数。filepath.Base("..")
// 仍然是 ".."，漏掉它等于允许响应把落点抬高一层目录。
func TestFormShareQRCodeFileNameRejectsTraversalRemnants(t *testing.T) {
	for _, tc := range []struct{ name, in string }{
		{"dotdot", ".."},
		{"nested dotdot", "x/../.."},
		{"absolute", "/etc/passwd"},
		{"parent escape", "../../etc/passwd"},
		{"dot", "."},
		{"blank", "   "},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := formShareQRCodeFileName(tc.in)
			if got == ".." || got == "." || got == "" || strings.ContainsRune(got, '/') {
				t.Fatalf("file_name %q resolved to %q, which can still move the write target", tc.in, got)
			}
		})
	}
	// 正常文件名要原样保留，别被守卫误伤。
	if got := formShareQRCodeFileName("qrcode.png"); got != "qrcode.png" {
		t.Fatalf("plain file name was rewritten to %q", got)
	}
}

// 越界的 --output 是纯本地判定，放在 Validate 里：runner 在 dry-run 和发请求之前
// 都会先跑 Validate，所以 dry-run 与真实执行拒绝同一批路径，也不会白花一次 API 调用。
func TestFormShareQRCodeDownloadValidateRejectsUnsafeOutput(t *testing.T) {
	cmdutil.TestChdir(t, t.TempDir())
	rt := newBaseTestRuntime(map[string]string{
		"base-token": "app_x", "table-id": "tbl_x", "form-id": "vew_x",
		"output": "../../etc/passwd.png",
	}, nil, nil)

	err := BaseFormShareQRCodeDownload.Validate(context.Background(), rt)
	if err == nil {
		t.Fatal("an out-of-allowlist --output must be refused")
	}
	// 写盘前对「拼接后完整路径」的校验报的是同一句 unsafe output path，只有这条
	// 校验点名了 --output。断言 Param 才能区分两者。
	var verr *errs.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected a validation error, got %T %v", err, err)
	}
	if verr.Param != "--output" {
		t.Fatalf("refusal should be the pre-request --output check (param=%q): %v", verr.Param, err)
	}
}

func TestFormShareQRCodeDownloadValidateAcceptsSafeOrOmittedOutput(t *testing.T) {
	cmdutil.TestChdir(t, t.TempDir())
	for _, output := range []string{"", "qrcode.png", "out/"} {
		rt := newBaseTestRuntime(map[string]string{
			"base-token": "app_x", "table-id": "tbl_x", "form-id": "vew_x",
			"output": output,
		}, nil, nil)
		if err := BaseFormShareQRCodeDownload.Validate(context.Background(), rt); err != nil {
			t.Fatalf("--output %q should pass validation: %v", output, err)
		}
	}
}

// --overwrite 是调用方显式同意替换已有文件：此时不要求 provider 支持独占提交，
// 写盘走普通 Save，并真的替换掉旧内容。
func TestSaveFormShareQRCodeOverwriteReplacesExistingFile(t *testing.T) {
	prev := fileio.GetProvider()
	fileio.Register(&staticFileIOProvider{
		fileIO: &nonExclusiveFileIO{base: prev.ResolveFileIO(context.Background())},
	})
	t.Cleanup(func() { fileio.Register(prev) })

	cmdutil.TestChdir(t, t.TempDir())
	const target = "qrcode.png"
	if err := os.WriteFile(target, []byte("existing"), 0o600); err != nil {
		t.Fatalf("seed existing file: %v", err)
	}
	rt := newBaseTestRuntime(nil, map[string]bool{"overwrite": true}, nil)

	exclusive, err := formShareQRCodeExclusiveIO(rt)
	if err != nil {
		t.Fatalf("--overwrite must not require exclusive commit: %v", err)
	}
	if exclusive != nil {
		t.Fatalf("--overwrite should select the plain Save path, got %T", exclusive)
	}
	savedPath, size, err := saveFormShareQRCode(rt, exclusive, target, []byte("new-png"))
	if err != nil {
		t.Fatalf("overwrite save failed: %v", err)
	}
	if size != int64(len("new-png")) || !strings.HasSuffix(savedPath, target) {
		t.Fatalf("unexpected save result path=%q size=%d", savedPath, size)
	}
	got, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatalf("read back target: %v", readErr)
	}
	if string(got) != "new-png" {
		t.Fatalf("existing file was not replaced: %q", got)
	}
}

func TestSaveFormShareQRCodeDefaultStillRequiresExclusiveCommit(t *testing.T) {
	rt := newBaseTestRuntime(nil, map[string]bool{"overwrite": false}, nil)
	exclusive, err := formShareQRCodeExclusiveIO(rt)
	if err != nil {
		t.Fatalf("local provider implements exclusive commit: %v", err)
	}
	if exclusive == nil {
		t.Fatal("without --overwrite the exclusive commit path must be selected")
	}
}

// 三道校验都属于响应契约问题，统一报 invalid_response 且不产生字节；
// 任何一道被删掉，错误响应体就可能被写成后缀为 .png 的垃圾文件。
func TestDecodeFormShareQRCode(t *testing.T) {
	pngBytes := append(append([]byte(nil), formShareQRCodePNGMagic...), 0x00, 0x01)
	validBase64 := base64.StdEncoding.EncodeToString(pngBytes)

	t.Run("valid png", func(t *testing.T) {
		got, err := decodeFormShareQRCode(formShareQRCodeResponse{MIMEType: "image/png; charset=binary", ContentBase64: " " + validBase64 + " "})
		if err != nil {
			t.Fatalf("valid payload rejected: %v", err)
		}
		if string(got) != string(pngBytes) {
			t.Fatalf("decoded bytes = %v, want %v", got, pngBytes)
		}
	})

	for _, tc := range []struct {
		name    string
		in      formShareQRCodeResponse
		wantMsg string
	}{
		{"wrong mime type", formShareQRCodeResponse{MIMEType: "application/json", ContentBase64: validBase64}, "mime_type"},
		{"missing mime type", formShareQRCodeResponse{ContentBase64: validBase64}, "mime_type"},
		{"missing content", formShareQRCodeResponse{MIMEType: "image/png"}, "missing content_base64"},
		{"invalid base64", formShareQRCodeResponse{MIMEType: "image/png", ContentBase64: "not base64!"}, "not valid base64"},
		{"not a png", formShareQRCodeResponse{MIMEType: "image/png", ContentBase64: base64.StdEncoding.EncodeToString([]byte(`{"code":1}`))}, "does not decode to a PNG"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeFormShareQRCode(tc.in)
			if got != nil {
				t.Fatalf("rejected payload still produced bytes: %v", got)
			}
			p, ok := errs.ProblemOf(err)
			if !ok {
				t.Fatalf("expected a typed error, got %T %v", err, err)
			}
			if p.Subtype != errs.SubtypeInvalidResponse {
				t.Fatalf("subtype = %v, want invalid_response", p.Subtype)
			}
			if !strings.Contains(p.Message, tc.wantMsg) {
				t.Fatalf("message %q should mention %q", p.Message, tc.wantMsg)
			}
		})
	}
}

func TestProjectFormShareQRCodeKeepsOnlyTypedStrings(t *testing.T) {
	got := projectFormShareQRCode(map[string]interface{}{
		"share_url":      "https://example.com/share",
		"file_name":      "form.png",
		"mime_type":      "image/png",
		"content_base64": "iVBORw0KGgo=",
		"extra":          "ignored",
	})
	want := formShareQRCodeResponse{
		ShareURL: "https://example.com/share", FileName: "form.png",
		MIMEType: "image/png", ContentBase64: "iVBORw0KGgo=",
	}
	if got != want {
		t.Fatalf("projection = %+v, want %+v", got, want)
	}
	// 非字符串字段按缺失处理，交给 decode 报 invalid_response，而不是 panic。
	if got := projectFormShareQRCode(map[string]interface{}{"mime_type": 1}); got.MIMEType != "" {
		t.Fatalf("non-string mime_type should project to empty, got %q", got.MIMEType)
	}
}
