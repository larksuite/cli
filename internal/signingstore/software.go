// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package signingstore

import (
	"context"
	"crypto"

	"github.com/larksuite/cli/internal/keysigner"
)

// SoftwareSigner adapts a domain's lazy opener to Signer. Only EnsureKey may
// initialize unlock material; public/sign/delete operations never initialize it.
type SoftwareSigner struct {
	Open func(allowCreate bool) (keysigner.Signer, error)
}

func (SoftwareSigner) Name() string                           { return keysigner.SoftwareSignerName }
func (SoftwareSigner) SecurityLevel() keysigner.SecurityLevel { return keysigner.SecurityLevelL3 }

func (s SoftwareSigner) EnsureKey(ctx context.Context, ref keysigner.KeyRef) (crypto.PublicKey, error) {
	signer, err := s.Open(true)
	if err != nil {
		return nil, err
	}
	return signer.EnsureKey(ctx, ref)
}

func (s SoftwareSigner) PublicKey(ctx context.Context, ref keysigner.KeyRef) (crypto.PublicKey, error) {
	signer, err := s.Open(false)
	if err != nil {
		return nil, err
	}
	return signer.PublicKey(ctx, ref)
}

func (s SoftwareSigner) Sign(ctx context.Context, ref keysigner.KeyRef, input []byte) ([]byte, string, error) {
	signer, err := s.Open(false)
	if err != nil {
		return nil, "", err
	}
	return signer.Sign(ctx, ref, input)
}

func (s SoftwareSigner) DeleteKey(ctx context.Context, ref keysigner.KeyRef) error {
	signer, err := s.Open(false)
	if err != nil {
		return err
	}
	return signer.DeleteKey(ctx, ref)
}
