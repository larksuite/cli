// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package signingstore

import (
	"context"
	"errors"
	"testing"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/keysigner"
)

func TestSoftwareAdapterOnlyEnsurePermitsUnlockInitialization(t *testing.T) {
	ctx := context.Background()
	ref := keysigner.KeyRef{Label: "existing"}
	cause := errs.NewInternalError(errs.SubtypeFileIO, "injected opener failure")
	for _, operation := range []string{"ensure", "public", "sign", "delete"} {
		t.Run(operation, func(t *testing.T) {
			signer := SoftwareSigner{Open: func(create bool) (keysigner.Signer, error) {
				if create != (operation == "ensure") {
					t.Fatalf("%s allowCreate=%v", operation, create)
				}
				return nil, cause
			}}
			var err error
			switch operation {
			case "ensure":
				_, err = signer.EnsureKey(ctx, ref)
			case "public":
				_, err = signer.PublicKey(ctx, ref)
			case "sign":
				_, _, err = signer.Sign(ctx, ref, nil)
			case "delete":
				err = signer.DeleteKey(ctx, ref)
			}
			if err != cause || !errors.Is(err, cause) {
				t.Fatalf("opener error reclassified: %v", err)
			}
		})
	}
}
