// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package signature

import "testing"

func TestFindSendDefault(t *testing.T) {
	resp := &GetSignaturesResponse{
		Signatures: []Signature{
			{ID: "sig_main", Content: "main"},
			{ID: "sig_alias", Content: "alias"},
		},
		Usages: []SignatureUsage{
			{EmailAddress: "main@example.com", SendMailSignatureID: "sig_main"},
			{EmailAddress: "Alias@Example.com", SendMailSignatureID: "sig_alias"},
			{EmailAddress: "empty@example.com", SendMailSignatureID: ""},
			{EmailAddress: "zero@example.com", SendMailSignatureID: "0"},
			{EmailAddress: "dangling@example.com", SendMailSignatureID: "sig_missing"},
		},
	}

	cases := []struct {
		name      string
		sender    string
		wantID    string
		wantFound bool
	}{
		{name: "case insensitive match", sender: "alias@example.com", wantID: "sig_alias", wantFound: true},
		{name: "main match", sender: "MAIN@example.com", wantID: "sig_main", wantFound: true},
		{name: "empty default", sender: "empty@example.com"},
		{name: "zero default", sender: "zero@example.com"},
		{name: "dangling default", sender: "dangling@example.com"},
		{name: "unknown sender", sender: "other@example.com"},
		{name: "empty sender", sender: " "},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, found := FindSendDefault(resp, tc.sender)
			if found != tc.wantFound {
				t.Fatalf("found = %v, want %v", found, tc.wantFound)
			}
			if tc.wantID == "" {
				if got != nil {
					t.Fatalf("signature = %#v, want nil", got)
				}
				return
			}
			if got == nil || got.ID != tc.wantID {
				t.Fatalf("signature ID = %#v, want %q", got, tc.wantID)
			}
		})
	}
}
