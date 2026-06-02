// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
	"github.com/larksuite/cli/shortcuts/mail/signature"
)

var MailSignatureUpdate = common.Shortcut{
	Service:     "mail",
	Command:     "+signature-update",
	Description: "Update a personal USER mail signature with GET + local merge + full-replace PUT. Supports inspect, patch-file, flat --set-* flags, and last-write-wins dry-runs.",
	Risk:        "write",
	Scopes:      []string{"mail:user_mailbox.message:modify", "mail:user_mailbox:readonly"},
	AuthTypes:   []string{"user"},
	HasFormat:   true,
	Flags: []common.Flag{
		{Name: "mailbox", Desc: "Mailbox address that owns the signature (default: me)."},
		{Name: "signature-id", Desc: "Signature ID to update. Required except with --print-patch-template.", Required: false},
		{Name: "inspect", Type: "bool", Desc: "Inspect the current signature without updating it."},
		{Name: "print-patch-template", Type: "bool", Desc: "Print a JSON skeleton for --patch-file. No network call is made."},
		{Name: "patch-file", Desc: "Relative JSON patch file. Shape is the same as --print-patch-template output."},
		{Name: "set-name", Desc: "Replace signature name (≤100 chars)."},
		{Name: "set-content", Desc: "Replace signature content. HTML is preserved; plain text is wrapped."},
		{Name: "set-content-file", Desc: "Relative file path for replacement content. Mutually exclusive with --set-content."},
		{Name: "set-device", Desc: "Replace signature device: pc or mobile."},
		{Name: "set-images-json", Desc: "Replace images metadata JSON array. Pass [] to clear images."},
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		if runtime.Bool("print-patch-template") {
			return common.NewDryRunAPI().
				Set("mode", "print-patch-template").
				Set("signature", buildSignaturePatchSkeleton())
		}
		mailboxID := resolveComposeMailboxID(runtime)
		signatureID := runtime.Str("signature-id")
		if signatureID == "" {
			return common.NewDryRunAPI().Set("error", "--signature-id is required except with --print-patch-template")
		}
		if runtime.Bool("inspect") {
			return common.NewDryRunAPI().
				Desc("Inspect the signature without modifying it.").
				GET(mailboxPath(mailboxID, "settings", "signatures"))
		}
		return common.NewDryRunAPI().
			Desc("Update a personal USER mail signature: GET current signatures, merge --patch-file and --set-* flags, then PUT a full-replace signature. No optimistic locking; concurrent updates are last-write-wins.").
			GET(mailboxPath(mailboxID, "settings", "signatures")).
			PUT(mailboxPath(mailboxID, "settings", "signatures", signatureID)).
			Body(map[string]interface{}{
				"signature":      "<merged current signature>",
				"changed_fields": dryRunSignatureChangedFields(runtime),
				"_warning":       "No optimistic locking — last write wins.",
			})
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Bool("print-patch-template") {
			return nil
		}
		if err := validateSignatureID(runtime.Str("signature-id")); err != nil {
			return err
		}
		if runtime.Str("signature-id") == "" {
			return output.ErrValidation("--signature-id is required (or use --print-patch-template to print the patch skeleton)")
		}
		if runtime.Changed("set-content") && runtime.Str("set-content-file") != "" {
			return output.ErrValidation("--set-content and --set-content-file are mutually exclusive")
		}
		if runtime.Changed("set-device") {
			if _, err := normalizeSignatureDevice(runtime.Str("set-device"), "--set-device"); err != nil {
				return err
			}
		}
		if name := runtime.Str("set-name"); runtime.Changed("set-name") {
			if strings.TrimSpace(name) == "" {
				return output.ErrValidation("--set-name must not be empty")
			}
			if len([]rune(strings.TrimSpace(name))) > 100 {
				return output.ErrValidation("--set-name must be at most 100 characters")
			}
		}
		if _, _, err := resolveSignatureContent(runtime, "set-content", "set-content-file"); err != nil {
			return err
		}
		if _, _, err := parseSignatureImagesJSON(runtime.Str("set-images-json"), "set-images-json"); err != nil {
			return err
		}
		patch, err := loadSignaturePatchFile(runtime)
		if err != nil {
			return err
		}
		if patch != nil {
			if patch.ID != nil && *patch.ID != "" && *patch.ID != runtime.Str("signature-id") {
				return output.ErrValidation("signature id in body must match path")
			}
			if patch.Name != nil {
				if strings.TrimSpace(*patch.Name) == "" {
					return output.ErrValidation("patch name must not be empty")
				}
				if len([]rune(strings.TrimSpace(*patch.Name))) > 100 {
					return output.ErrValidation("patch name must be at most 100 characters")
				}
			}
			if patch.Content != nil {
				if _, err := normalizeSignatureContent(*patch.Content); err != nil {
					return err
				}
			}
			if patch.SignatureDevice != nil {
				if _, err := normalizeSignatureDevice(*patch.SignatureDevice, "patch signature_device"); err != nil {
					return err
				}
			}
			if patch.Images != nil {
				if err := validateSignatureImagesOnly(*patch.Images); err != nil {
					return err
				}
			}
		}
		return nil
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		if runtime.Bool("print-patch-template") {
			runtime.Out(buildSignaturePatchSkeleton(), nil)
			return nil
		}
		mailboxID := resolveComposeMailboxID(runtime)
		signatureID := runtime.Str("signature-id")
		current, err := findSignature(runtime, mailboxID, signatureID)
		if err != nil {
			return err
		}
		if runtime.Bool("inspect") {
			out := signatureOutput(current, resolveLang(runtime))
			delete(out, "changed_fields")
			delete(out, "last_write_policy")
			runtime.OutFormat(out, nil, func(w io.Writer) {
				formatSignatureSummary(w, "inspection (read-only)", current, resolveLang(runtime))
			})
			return nil
		}
		if current.SignatureType != signature.SignatureTypeUser {
			return output.ErrValidation("only USER signatures can be updated")
		}

		next := *current
		next.ID = signatureID
		next.SignatureType = signature.SignatureTypeUser
		next.Images = sanitizeSignatureImages(next.Images)
		changedFields := []string{}

		patch, err := loadSignaturePatchFile(runtime)
		if err != nil {
			return err
		}
		if patch != nil {
			if patch.ID != nil && *patch.ID != "" && *patch.ID != signatureID {
				return output.ErrValidation("signature id in body must match path")
			}
			if patch.Name != nil {
				next.Name = strings.TrimSpace(*patch.Name)
				changedFields = append(changedFields, "name")
			}
			if patch.Content != nil {
				content, err := normalizeSignatureContent(*patch.Content)
				if err != nil {
					return err
				}
				next.Content = content
				changedFields = append(changedFields, "content")
			}
			if patch.SignatureDevice != nil {
				device, err := normalizeSignatureDevice(*patch.SignatureDevice, "patch signature_device")
				if err != nil {
					return err
				}
				next.SignatureDevice = device
				changedFields = append(changedFields, "signature_device")
			}
			if patch.Images != nil {
				next.Images = sanitizeSignatureImages(*patch.Images)
				changedFields = append(changedFields, "images")
			}
		}

		if runtime.Changed("set-name") {
			next.Name = strings.TrimSpace(runtime.Str("set-name"))
			changedFields = append(changedFields, "name")
		}
		if content, changed, err := resolveSignatureContent(runtime, "set-content", "set-content-file"); err != nil {
			return err
		} else if changed {
			next.Content = content
			changedFields = append(changedFields, "content")
		}
		if runtime.Changed("set-device") {
			device, err := normalizeSignatureDevice(runtime.Str("set-device"), "--set-device")
			if err != nil {
				return err
			}
			next.SignatureDevice = device
			changedFields = append(changedFields, "signature_device")
		}
		if runtime.Changed("set-images-json") {
			images, _, err := parseSignatureImagesJSON(runtime.Str("set-images-json"), "set-images-json")
			if err != nil {
				return err
			}
			next.Images = sanitizeSignatureImages(images)
			changedFields = append(changedFields, "images")
		}

		if strings.TrimSpace(next.Name) == "" {
			return output.ErrValidation("signature name must not be empty")
		}
		if len([]rune(strings.TrimSpace(next.Name))) > 100 {
			return output.ErrValidation("signature name must be at most 100 characters")
		}
		next.Name = strings.TrimSpace(next.Name)
		if next.SignatureDevice == "" {
			next.SignatureDevice = signature.DevicePC
		}
		if err := validateSignatureImageRefs(next.Content, next.Images); err != nil {
			return err
		}

		updated, err := signature.Update(runtime, mailboxID, signatureID, next)
		if err != nil {
			return err
		}
		out := signatureOutput(updated, resolveLang(runtime))
		out["changed_fields"] = uniqueSignatureFields(changedFields)
		runtime.OutFormat(out, nil, func(w io.Writer) {
			formatSignatureSummary(w, "updated", updated, resolveLang(runtime))
			fmt.Fprintln(w, "warning: no optimistic locking; concurrent updates are last-write-wins.")
		})
		fmt.Fprintln(runtime.IO().ErrOut,
			"warning: signature endpoints have no optimistic locking; concurrent updates are last-write-wins.")
		return nil
	},
}

type signaturePatchFile struct {
	ID              *string                     `json:"id,omitempty"`
	Name            *string                     `json:"name,omitempty"`
	Content         *string                     `json:"content,omitempty"`
	SignatureDevice *string                     `json:"signature_device,omitempty"`
	Images          *[]signature.SignatureImage `json:"images,omitempty"`
}

func loadSignaturePatchFile(runtime *common.RuntimeContext) (*signaturePatchFile, error) {
	pf := strings.TrimSpace(runtime.Str("patch-file"))
	if pf == "" {
		return nil, nil
	}
	f, err := runtime.FileIO().Open(pf)
	if err != nil {
		return nil, output.ErrValidation("open --patch-file %s: %v", pf, err)
	}
	buf, readErr := io.ReadAll(f)
	f.Close()
	if readErr != nil {
		return nil, output.ErrValidation("read --patch-file %s: %v", pf, readErr)
	}
	var patch signaturePatchFile
	if err := json.Unmarshal(buf, &patch); err != nil {
		return nil, output.ErrValidation("parse --patch-file %s: %v", pf, err)
	}
	if patch.Images != nil {
		*patch.Images = sanitizeSignatureImages(*patch.Images)
	}
	return &patch, nil
}

func buildSignaturePatchSkeleton() map[string]interface{} {
	return map[string]interface{}{
		"id":               "string (must match --signature-id when present)",
		"name":             "string (≤100 chars, optional)",
		"content":          "string (HTML or plain text; local <img src> paths are not uploaded)",
		"signature_device": "PC or MOBILE",
		"images": []map[string]interface{}{{
			"image_name":   "logo.png",
			"file_key":     "file_key from an already uploaded signature image",
			"cid":          "logo1",
			"file_size":    "12345",
			"image_width":  120,
			"image_height": 48,
		}},
	}
}

func findSignature(runtime *common.RuntimeContext, mailboxID, signatureID string) (*signature.Signature, error) {
	resp, err := signature.ListAll(runtime, mailboxID)
	if err != nil {
		return nil, err
	}
	for i := range resp.Signatures {
		if resp.Signatures[i].ID == signatureID {
			return &resp.Signatures[i], nil
		}
	}
	return nil, output.ErrValidation("signature not found or already deleted: %s", signatureID)
}

func dryRunSignatureChangedFields(runtime *common.RuntimeContext) []string {
	fields := []string{}
	if runtime.Str("patch-file") != "" {
		fields = append(fields, "patch-file")
	}
	if runtime.Changed("set-name") {
		fields = append(fields, "name")
	}
	if runtime.Changed("set-content") || runtime.Str("set-content-file") != "" {
		fields = append(fields, "content")
	}
	if runtime.Changed("set-device") {
		fields = append(fields, "signature_device")
	}
	if runtime.Changed("set-images-json") {
		fields = append(fields, "images")
	}
	return uniqueSignatureFields(fields)
}

func uniqueSignatureFields(fields []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, field := range fields {
		if field == "" || seen[field] {
			continue
		}
		seen[field] = true
		out = append(out, field)
	}
	return out
}
