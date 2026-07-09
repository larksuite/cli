// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package gitcred

import (
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/keychain"
)

type SecretStore struct {
	kc keychain.KeychainAccess
}

func NewSecretStore(kc keychain.KeychainAccess) *SecretStore {
	return &SecretStore{kc: kc}
}

func (s *SecretStore) Get(ref string) (string, error) {
	if s == nil || ref == "" {
		return "", nil
	}
	if s.kc == nil {
		return "", nil
	}
	return s.kc.Get(KeychainService, ref)
}

func (s *SecretStore) Set(ref, pat string) error {
	if s == nil || s.kc == nil {
		return &errs.ConfigError{Problem: errs.Problem{
			Category: errs.CategoryConfig,
			Subtype:  errs.SubtypeInvalidConfig,
			Message:  "local keychain is unavailable",
			Hint:     "make sure the system credential store is available, then retry lark-cli apps +git-credential-init",
		}}
	}
	if ref == "" {
		return &errs.InternalError{Problem: errs.Problem{
			Category: errs.CategoryInternal,
			Subtype:  errs.SubtypeUnknown,
			Message:  "keychain PAT reference is empty",
		}}
	}
	if err := s.kc.Set(KeychainService, ref, pat); err != nil {
		return &errs.ConfigError{Problem: errs.Problem{
			Category: errs.CategoryConfig,
			Subtype:  errs.SubtypeInvalidConfig,
			Message:  "save local Git credential PAT to keychain failed",
			Hint:     "make sure the system credential store is available, then retry lark-cli apps +git-credential-init",
		}, Cause: err}
	}
	return nil
}

func (s *SecretStore) Remove(ref string) error {
	if s == nil {
		return nil
	}
	if ref == "" {
		return nil
	}
	if s.kc == nil {
		return &errs.ConfigError{Problem: errs.Problem{
			Category: errs.CategoryConfig,
			Subtype:  errs.SubtypeInvalidConfig,
			Message:  "local keychain is unavailable",
			Hint:     "make sure the system credential store is available, then retry lark-cli apps +git-credential-remove",
		}}
	}
	if err := s.kc.Remove(KeychainService, ref); err != nil {
		return &errs.ConfigError{Problem: errs.Problem{
			Category: errs.CategoryConfig,
			Subtype:  errs.SubtypeInvalidConfig,
			Message:  "remove local Git credential PAT from keychain failed",
			Hint:     "make sure the system credential store is available, then retry lark-cli apps +git-credential-remove",
		}, Cause: err}
	}
	return nil
}
