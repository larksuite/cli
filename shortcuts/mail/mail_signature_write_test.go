// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"os"
	"strings"
	"testing"

	"github.com/larksuite/cli/internal/httpmock"
	"github.com/larksuite/cli/shortcuts/mail/signature"
)

func clearSignatureTestCache() {
	signature.ClearCache("me")
}

func TestMailSignatureCreate_Happy(t *testing.T) {
	clearSignatureTestCache()
	f, stdout, _, reg := mailShortcutTestFactory(t)

	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/settings/signatures",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"signature": map[string]interface{}{
					"id":               "123",
					"name":             "Agent",
					"signature_type":   "USER",
					"signature_device": "PC",
					"content":          "<p>Regards</p><img src=\"cid:logo1\">",
					"images": []map[string]interface{}{{
						"image_name": "logo.png",
						"file_key":   "file_1",
						"cid":        "logo1",
					}},
				},
			},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailSignatureCreate, []string{
		"+signature-create",
		"--name", "Agent",
		"--content", "<p>Regards</p><img src=\"cid:logo1\">",
		"--images-json", `[{"image_name":"logo.png","file_key":"file_1","cid":"logo1","download_url":"ignored"}]`,
	}, f, stdout)
	if err != nil {
		t.Fatalf("signature-create failed: %v", err)
	}

	capturedBody := decodeCapturedBody(t, stub)
	sig := capturedBody["signature"].(map[string]interface{})
	if sig["signature_type"] != "USER" {
		t.Fatalf("signature_type = %v", sig["signature_type"])
	}
	if sig["signature_device"] != "PC" {
		t.Fatalf("signature_device = %v", sig["signature_device"])
	}
	images := sig["images"].([]interface{})
	img := images[0].(map[string]interface{})
	if _, ok := img["download_url"]; ok {
		t.Fatalf("download_url should not be sent in write payload: %#v", img)
	}

	data := decodeShortcutEnvelopeData(t, stdout)
	if data["id"] != "123" {
		t.Fatalf("id = %v", data["id"])
	}
	if data["content_preview"] == "" {
		t.Fatalf("content_preview missing: %#v", data)
	}
}

func TestMailSignatureCreate_ContentFilePlainTextWrap(t *testing.T) {
	clearSignatureTestCache()
	chdirTemp(t)
	if err := os.WriteFile("sig.txt", []byte("Regards\nAlice"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, stdout, _, reg := mailShortcutTestFactory(t)
	stub := &httpmock.Stub{
		Method: "POST",
		URL:    "/user_mailboxes/me/settings/signatures",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"signature": map[string]interface{}{
					"id":               "124",
					"name":             "Plain",
					"signature_type":   "USER",
					"signature_device": "MOBILE",
					"content":          "<div>Regards<br>Alice</div>",
				},
			},
		},
	}
	reg.Register(stub)

	err := runMountedMailShortcut(t, MailSignatureCreate, []string{
		"+signature-create",
		"--name", "Plain",
		"--content-file", "sig.txt",
		"--device", "mobile",
	}, f, stdout)
	if err != nil {
		t.Fatalf("signature-create failed: %v", err)
	}
	body := decodeCapturedBody(t, stub)
	content := body["signature"].(map[string]interface{})["content"].(string)
	if !strings.Contains(content, "Regards<br>Alice") {
		t.Fatalf("plain text content was not wrapped: %s", content)
	}
	if body["signature"].(map[string]interface{})["signature_device"] != "MOBILE" {
		t.Fatalf("device not normalized: %#v", body)
	}
}

func TestMailSignatureCreate_ValidateImages(t *testing.T) {
	clearSignatureTestCache()
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailSignatureCreate, []string{
		"+signature-create",
		"--name", "Bad",
		"--content", `<p><img src="./logo.png"></p>`,
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "local image paths are not supported") {
		t.Fatalf("expected local image validation error, got %v", err)
	}

	err = runMountedMailShortcut(t, MailSignatureCreate, []string{
		"+signature-create",
		"--name", "Bad",
		"--content", `<p><img src="cid:logo1"></p>`,
		"--images-json", `[{"cid":"other","file_key":"file_1"}]`,
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "not referenced") {
		t.Fatalf("expected cid mismatch validation error, got %v", err)
	}
}

func TestMailSignatureUpdate_Happy(t *testing.T) {
	clearSignatureTestCache()
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/settings/signatures",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"signatures": []map[string]interface{}{{
					"id":               "123",
					"name":             "Old",
					"signature_type":   "USER",
					"signature_device": "PC",
					"content":          "<p>old</p>",
				}},
			},
		},
	})
	putStub := &httpmock.Stub{
		Method: "PUT",
		URL:    "/user_mailboxes/me/settings/signatures/123",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"signature": map[string]interface{}{
					"id":               "123",
					"name":             "New",
					"signature_type":   "USER",
					"signature_device": "MOBILE",
					"content":          "<p>new</p>",
				},
			},
		},
	}
	reg.Register(putStub)

	err := runMountedMailShortcut(t, MailSignatureUpdate, []string{
		"+signature-update",
		"--signature-id", "123",
		"--set-name", "New",
		"--set-device", "mobile",
		"--set-content", "<p>new</p>",
	}, f, stdout)
	if err != nil {
		t.Fatalf("signature-update failed: %v", err)
	}

	body := decodeCapturedBody(t, putStub)
	sig := body["signature"].(map[string]interface{})
	if sig["id"] != "123" || sig["name"] != "New" || sig["signature_device"] != "MOBILE" {
		t.Fatalf("unexpected PUT signature: %#v", sig)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["id"] != "123" {
		t.Fatalf("id = %v", data["id"])
	}
}

func TestMailSignatureUpdate_InspectAndTenantReject(t *testing.T) {
	clearSignatureTestCache()
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/settings/signatures",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"signatures": []map[string]interface{}{{
					"id":               "123",
					"name":             "Tenant",
					"signature_type":   "TENANT",
					"signature_device": "PC",
					"content":          "<p>tenant</p>",
				}},
			},
		},
		Reusable: true,
	})
	err := runMountedMailShortcut(t, MailSignatureUpdate, []string{
		"+signature-update",
		"--signature-id", "123",
		"--inspect",
	}, f, stdout)
	if err != nil {
		t.Fatalf("inspect should pass for TENANT: %v", err)
	}

	err = runMountedMailShortcut(t, MailSignatureUpdate, []string{
		"+signature-update",
		"--signature-id", "123",
		"--set-name", "Nope",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "only USER signatures") {
		t.Fatalf("expected TENANT rejection, got %v", err)
	}
}

func TestMailSignatureUpdate_PatchIDConflict(t *testing.T) {
	clearSignatureTestCache()
	chdirTemp(t)
	if err := os.WriteFile("patch.json", []byte(`{"id":"999","name":"New"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailSignatureUpdate, []string{
		"+signature-update",
		"--signature-id", "123",
		"--patch-file", "patch.json",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "signature id in body must match path") {
		t.Fatalf("expected patch id conflict, got %v", err)
	}
}

func TestMailSignatureDelete_Happy(t *testing.T) {
	clearSignatureTestCache()
	f, stdout, _, reg := mailShortcutTestFactory(t)
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/settings/signatures",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"signatures": []map[string]interface{}{{
					"id":               "123",
					"name":             "Agent",
					"signature_type":   "USER",
					"signature_device": "PC",
					"content":          "<p>bye</p>",
				}},
			},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "DELETE",
		URL:    "/user_mailboxes/me/settings/signatures/123",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{},
		},
	})
	reg.Register(&httpmock.Stub{
		Method: "GET",
		URL:    "/user_mailboxes/me/settings/signatures",
		Body: map[string]interface{}{
			"code": 0,
			"data": map[string]interface{}{
				"signatures": []map[string]interface{}{},
			},
		},
	})

	err := runMountedMailShortcut(t, MailSignatureDelete, []string{
		"+signature-delete",
		"--signature-id", "123",
		"--yes",
	}, f, stdout)
	if err != nil {
		t.Fatalf("signature-delete failed: %v", err)
	}
	data := decodeShortcutEnvelopeData(t, stdout)
	if data["deleted_signature_id"] != "123" || data["verify_status"] != "absent" {
		t.Fatalf("unexpected delete output: %#v", data)
	}
}

func TestMailSignatureDelete_RequiresYes(t *testing.T) {
	clearSignatureTestCache()
	f, stdout, _, _ := mailShortcutTestFactory(t)
	err := runMountedMailShortcut(t, MailSignatureDelete, []string{
		"+signature-delete",
		"--signature-id", "123",
	}, f, stdout)
	if err == nil || !strings.Contains(err.Error(), "--yes is required") {
		t.Fatalf("expected --yes validation error, got %v", err)
	}
}
