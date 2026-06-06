// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/mail/signature"
)

const (
	signatureErrInvalidName      = 15180301
	signatureErrInvalidID        = 15180302
	signatureErrNameDuplicate    = 15180303
	signatureErrInvalidParam     = 15180304
	signatureErrPermissionDenied = 15180305
)

type signatureWritePayload struct {
	ID              string                     `json:"id,omitempty"`
	Name            string                     `json:"name"`
	Content         string                     `json:"content"`
	SignatureType   signature.SignatureType    `json:"signature_type"`
	SignatureDevice signature.SignatureDevice  `json:"signature_device"`
	Images          []signature.SignatureImage `json:"images,omitempty"`
}

func signatureMailboxPath(mailboxID string, segments ...string) string {
	all := append([]string{"settings", "signatures"}, segments...)
	return mailboxPath(mailboxID, all...)
}

func resolveSignatureContent(runtime *common.RuntimeContext) (content, sourcePath string, err error) {
	if raw := runtime.Str("content"); raw != "" {
		return raw, "", nil
	}
	path := runtime.Str("content-file")
	if path == "" {
		return "", "", nil
	}
	f, err := runtime.FileIO().Open(path)
	if err != nil {
		return "", path, mailValidationParamError("--content-file", "open --content-file %s: %v", path, err).WithCause(mailInputStatError(err))
	}
	defer f.Close()
	buf, err := io.ReadAll(f)
	if err != nil {
		return "", path, mailValidationParamError("--content-file", "read --content-file %s: %v", path, err).WithCause(err)
	}
	return string(buf), path, nil
}

func signatureDeviceFromRuntime(runtime *common.RuntimeContext) (signature.SignatureDevice, error) {
	device := runtime.Str("device")
	if device == "" {
		return signature.DevicePC, nil
	}
	switch signature.SignatureDevice(device) {
	case signature.DevicePC, signature.DeviceMobile:
		return signature.SignatureDevice(device), nil
	default:
		return "", mailValidationParamError("--device", "--device must be PC or MOBILE")
	}
}

func buildSignaturePayloadFromFlags(ctx context.Context, runtime *common.RuntimeContext, name, content string, device signature.SignatureDevice) (*signatureWritePayload, error) {
	rewritten, images, err := buildSignatureImagesFromContent(ctx, runtime, content)
	if err != nil {
		return nil, err
	}
	return &signatureWritePayload{
		Name:            name,
		Content:         rewritten,
		SignatureType:   signature.SignatureTypeUser,
		SignatureDevice: device,
		Images:          images,
	}, nil
}

func buildSignatureImagesFromContent(ctx context.Context, runtime *common.RuntimeContext, content string) (string, []signature.SignatureImage, error) {
	imgs := parseLocalImgs(content)
	pathToCID := make(map[string]string)
	images := make([]signature.SignatureImage, 0, len(imgs))
	for _, img := range imgs {
		if cid, ok := pathToCID[img.Path]; ok {
			content = replaceImgSrcOnce(content, img.RawSrc, "cid:"+cid)
			continue
		}
		fileKey, size, err := uploadToDriveForTemplate(ctx, runtime, img.Path)
		if err != nil {
			return "", nil, err
		}
		cid, err := generateTemplateCID()
		if err != nil {
			return "", nil, err
		}
		pathToCID[img.Path] = cid
		content = replaceImgSrcOnce(content, img.RawSrc, "cid:"+cid)
		images = append(images, signature.SignatureImage{
			ImageName: filepath.Base(img.Path),
			FileKey:   fileKey,
			CID:       cid,
			FileSize:  strconv.FormatInt(size, 10),
		})
	}
	return content, images, nil
}

func createSignature(runtime *common.RuntimeContext, mailboxID string, payload *signatureWritePayload) (map[string]interface{}, error) {
	return runtime.CallAPITyped("POST", signatureMailboxPath(mailboxID), nil, map[string]interface{}{
		"signature": payload,
	})
}

func updateSignature(runtime *common.RuntimeContext, mailboxID, signatureID string, payload *signatureWritePayload) (map[string]interface{}, error) {
	payload.ID = signatureID
	return runtime.CallAPITyped("PUT", signatureMailboxPath(mailboxID, signatureID), nil, map[string]interface{}{
		"signature": payload,
	})
}

func deleteSignature(runtime *common.RuntimeContext, mailboxID, signatureID string) error {
	_, err := runtime.CallAPITyped("DELETE", signatureMailboxPath(mailboxID, signatureID), nil, nil)
	return err
}

func extractSignaturePayload(data map[string]interface{}) (*signature.Signature, error) {
	if data == nil {
		return nil, mailInvalidResponseError("API response missing signature body")
	}
	raw, ok := data["signature"]
	if !ok {
		raw = data
	}
	buf, err := json.Marshal(raw)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeSDKError, "re-encode signature payload failed: %v", err).WithCause(err)
	}
	var out signature.Signature
	if err := json.Unmarshal(buf, &out); err != nil {
		return nil, mailInvalidResponseError("decode signature payload failed: %v", err).WithCause(err)
	}
	return &out, nil
}

func decorateSignatureWriteError(err error, action string) error {
	err = mailDecorateProblemMessage(err, action)
	p, ok := errs.ProblemOf(err)
	if !ok {
		return err
	}
	var hint string
	switch p.Code {
	case signatureErrInvalidName:
		hint = "pass a non-empty --name for USER signature writes"
	case signatureErrInvalidID:
		hint = "run `lark-cli mail +signature` first and pass an existing USER signature id"
	case signatureErrNameDuplicate:
		hint = "choose a different signature name or update the existing signature"
	case signatureErrInvalidParam:
		hint = "check --signature-id, --device, and the signature HTML payload"
	case signatureErrPermissionDenied:
		hint = "check mailbox access, granted scopes, and avoid --as bot with --mailbox me"
	default:
		if p.Code == 404 || p.Subtype == errs.SubtypeNotFound {
			hint = "signature write APIs may not be available in this environment yet; verify OAPI route rollout and the signature id"
		}
	}
	if strings.TrimSpace(hint) == "" {
		return err
	}
	return mailAppendProblemHint(err, hint)
}

func outputSignatureResult(runtime *common.RuntimeContext, title string, sig *signature.Signature) {
	runtime.OutFormat(map[string]interface{}{"signature": sig}, nil, func(w io.Writer) {
		fmt.Fprintln(w, title)
		if sig != nil {
			fmt.Fprintf(w, "signature_id: %s\n", sig.ID)
			fmt.Fprintf(w, "name: %s\n", sig.Name)
			fmt.Fprintf(w, "device: %s\n", sig.SignatureDevice)
			fmt.Fprintf(w, "images: %d\n", len(sig.Images))
		}
	})
}
