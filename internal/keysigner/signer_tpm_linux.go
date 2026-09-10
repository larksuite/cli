//go:build linux

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/asn1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math/big"

	"github.com/google/go-tpm/legacy/tpm2"
	"github.com/google/go-tpm/tpmutil"
)

type tpmSigner struct {
	directory func(string) (string, error)
}

// NewSigner constructs only the named native backend, or returns nil if this
// platform does not support it. The resolver receives the signer name when a
// key operation starts.
func NewSigner(name string, directory func(string) (string, error)) Signer {
	if name != LinuxTPMSignerName {
		return nil
	}
	return &tpmSigner{directory: directory}
}

func (tpmSigner) Name() string                 { return LinuxTPMSignerName }
func (tpmSigner) SecurityLevel() SecurityLevel { return SecurityLevelL1 }

func (s tpmSigner) EnsureKey(ctx context.Context, ref KeyRef) (public crypto.PublicKey, err error) {
	err = s.withKey(ctx, ref, true, func(key *tpmKey, algorithm signingAlgorithm) error {
		public = key.Public()
		return algorithm.validatePublicKey(public)
	})
	return public, err
}

func (s tpmSigner) PublicKey(ctx context.Context, ref KeyRef) (public crypto.PublicKey, err error) {
	err = s.withKey(ctx, ref, false, func(key *tpmKey, algorithm signingAlgorithm) error {
		public = key.Public()
		return algorithm.validatePublicKey(public)
	})
	return public, err
}

func (s tpmSigner) Sign(ctx context.Context, ref KeyRef, input []byte) (signature []byte, algorithm string, err error) {
	err = s.withKey(ctx, ref, false, func(key *tpmKey, selected signingAlgorithm) error {
		var signErr error
		signature, signErr = selected.sign(key, input)
		algorithm = selected.name()
		return signErr
	})
	if err != nil {
		return nil, "", err
	}
	return signature, algorithm, nil
}

func (s tpmSigner) DeleteKey(ctx context.Context, ref KeyRef) error {
	err := s.withKey(ctx, ref, false, func(key *tpmKey, algorithm signingAlgorithm) error {
		if err := algorithm.validatePublicKey(key.Public()); err != nil {
			return err
		}
		return key.Remove()
	})
	if errors.Is(err, ErrKeyNotFound) {
		return nil
	}
	return err
}

// Both algorithms refuse to replace or delete a key
// owned by the other algorithm.
func (s tpmSigner) withKey(ctx context.Context, ref KeyRef, create bool, operation func(*tpmKey, signingAlgorithm) error) error {
	if s.directory == nil {
		return errors.New("keysigner: TPM key directory resolver is required")
	}
	directory, err := s.directory(s.Name())
	if err != nil {
		return err
	}
	return withKeyFile(ctx, directory, ref, func(path string, algorithm signingAlgorithm) (err error) {
		key, err := openTPMKey(ctx, path, ref, algorithm, create)
		if err != nil {
			return err
		}
		defer func() { err = errors.Join(err, key.Close()) }()
		return operation(key, algorithm)
	})
}

func classifyTPMError(err error) error {
	var pathErr *fs.PathError
	// A missing or inaccessible TPM is unavailable to make downgrades possible; I/O and key-file errors stay hard failures.
	if errors.As(err, &pathErr) && pathErr.Path == "/dev/tpmrm0" &&
		(errors.Is(pathErr.Err, fs.ErrNotExist) || errors.Is(pathErr.Err, fs.ErrPermission)) {
		return fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	return err
}

// Private is an integrity-protected TPM blob, never a software private key.
// FixedTPM and FixedParent prevent duplication to a different TPM or parent.
type tpmKeyBlob struct {
	Public  []byte `json:"public"`
	Private []byte `json:"private"`
}

type tpmKey struct {
	device io.ReadWriteCloser
	parent tpmutil.Handle
	handle tpmutil.Handle
	public crypto.PublicKey
	path   string
}

func openTPMKey(ctx context.Context, path string, ref KeyRef, algorithm signingAlgorithm, create bool) (_ *tpmKey, err error) {
	template := tpm2.Public{
		NameAlg: tpm2.AlgSHA256,
		Attributes: tpm2.FlagSign | tpm2.FlagFixedTPM | tpm2.FlagFixedParent |
			tpm2.FlagSensitiveDataOrigin | tpm2.FlagUserWithAuth | tpm2.FlagNoDA,
	}
	switch algorithm.name() {
	case AlgES256:
		template.Type = tpm2.AlgECC
		template.ECCParameters = &tpm2.ECCParams{
			CurveID: tpm2.CurveNISTP256,
			Sign:    &tpm2.SigScheme{Alg: tpm2.AlgECDSA, Hash: tpm2.AlgSHA256},
		}
	case AlgRS256:
		template.Type = tpm2.AlgRSA
		template.RSAParameters = &tpm2.RSAParams{
			KeyBits: 2048, Sign: &tpm2.SigScheme{Alg: tpm2.AlgRSASSA, Hash: tpm2.AlgSHA256},
		}
	default:
		return nil, ErrUnsupportedAlgorithm
	}
	record, readErr := readKeyFile(path, ref, LinuxTPMSignerName, algorithm)
	if readErr != nil && (!create || !errors.Is(readErr, ErrKeyNotFound)) {
		return nil, readErr
	}
	var blob tpmKeyBlob
	if readErr == nil {
		if err := decodeKeyJSON(record.Data, &blob); err != nil {
			return nil, err
		}
	}
	device, err := tpm2.OpenTPM("/dev/tpmrm0")
	if err != nil {
		return nil, classifyTPMError(err)
	}
	key := &tpmKey{device: device, path: path}
	defer func() {
		if err != nil {
			err = errors.Join(err, key.Close())
		}
	}()
	// Recreate this deterministic storage parent per operation. No persistent
	// TPM handles are allocated or evicted, so unrelated applications are untouched.
	key.parent, _, err = tpm2.CreatePrimary(device, tpm2.HandleOwner, tpm2.PCRSelection{}, "", "", tpm2.Public{
		Type: tpm2.AlgRSA, NameAlg: tpm2.AlgSHA256,
		Attributes: tpm2.FlagStorageDefault | tpm2.FlagNoDA,
		RSAParameters: &tpm2.RSAParams{
			KeyBits:   2048,
			Symmetric: &tpm2.SymScheme{Alg: tpm2.AlgAES, KeyBits: 128, Mode: tpm2.AlgCFB},
		},
	})
	if err != nil {
		return nil, err
	}
	if errors.Is(readErr, ErrKeyNotFound) {
		blob.Private, blob.Public, _, _, _, err = tpm2.CreateKey(device, key.parent, tpm2.PCRSelection{}, "", "", template)
		if err != nil {
			return nil, err
		}
	}
	area, err := tpm2.DecodePublic(blob.Public)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	if !area.MatchesTemplate(template) || len(blob.Private) == 0 {
		return nil, ErrCorrupt
	}
	public, err := area.Key()
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	if err := algorithm.validatePublicKey(public); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCorrupt, err)
	}
	key.public = public
	publicDER, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		return nil, err
	}
	if readErr == nil && !bytes.Equal(publicDER, record.PublicKey) {
		return nil, ErrCorrupt
	}
	key.handle, _, err = tpm2.Load(device, key.parent, "", blob.Public, blob.Private)
	if err != nil {
		return nil, err
	}
	if errors.Is(readErr, ErrKeyNotFound) {
		data, err := json.Marshal(blob)
		if err != nil {
			return nil, err
		}
		record = keyFileRecord{Version: 1, Label: ref.Label, Backend: LinuxTPMSignerName, PublicKey: publicDER, Data: data}
		if err := writeKeyFile(ctx, path, record); err != nil {
			return nil, err
		}
	}
	return key, nil
}

func (k *tpmKey) Public() crypto.PublicKey { return k.public }

func (k *tpmKey) Sign(_ io.Reader, digest []byte, opts crypto.SignerOpts) ([]byte, error) {
	if opts == nil || opts.HashFunc() != crypto.SHA256 || len(digest) != crypto.SHA256.Size() {
		return nil, ErrUnsupportedAlgorithm
	}
	if _, pss := opts.(*rsa.PSSOptions); pss {
		return nil, ErrUnsupportedAlgorithm
	}
	scheme := tpm2.SigScheme{Hash: tpm2.AlgSHA256}
	switch k.public.(type) {
	case *ecdsa.PublicKey:
		scheme.Alg = tpm2.AlgECDSA
	case *rsa.PublicKey:
		scheme.Alg = tpm2.AlgRSASSA
	default:
		return nil, ErrUnsupportedAlgorithm
	}
	signature, err := tpm2.Sign(k.device, k.handle, "", digest, nil, &scheme)
	if err != nil {
		return nil, err
	}
	if signature == nil || signature.Alg != scheme.Alg {
		return nil, ErrCorrupt
	}
	if scheme.Alg == tpm2.AlgECDSA {
		if signature.ECC == nil || signature.ECC.HashAlg != tpm2.AlgSHA256 || signature.ECC.R == nil || signature.ECC.S == nil {
			return nil, ErrCorrupt
		}
		return asn1.Marshal(struct{ R, S *big.Int }{signature.ECC.R, signature.ECC.S})
	}
	if signature.RSA == nil || signature.RSA.HashAlg != tpm2.AlgSHA256 || len(signature.RSA.Signature) != k.public.(*rsa.PublicKey).Size() {
		return nil, ErrCorrupt
	}
	return signature.RSA.Signature, nil
}

func (k *tpmKey) Remove() error { return removeKeyFile(k.path) }

func (k *tpmKey) Close() error {
	var err error
	if k.handle != 0 {
		err = tpm2.FlushContext(k.device, k.handle)
	}
	if k.parent != 0 {
		err = errors.Join(err, tpm2.FlushContext(k.device, k.parent))
	}
	return errors.Join(err, k.device.Close())
}
