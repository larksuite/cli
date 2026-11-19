// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylesshelper

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/keysigner"
)

func writeECPrivateKey(t *testing.T, path string) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}), 0600))
	return key
}

func TestFileSignerRejectsUnsafeFiles(t *testing.T) {
	dir := t.TempDir()
	privateDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", privateDir)
	target := filepath.Join(privateDir, "private.pem")
	writeECPrivateKey(t, target)
	data, err := os.ReadFile(target)
	require.NoError(t, err)
	// The oversized file is otherwise a valid PEM.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "oversized.pem"), append(data, bytes.Repeat([]byte(" "), maxPrivateKeyFileBytes)...), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "invalid.pem"), []byte("not PEM"), 0600))

	reject := func(t *testing.T, path string) {
		t.Helper()
		_, err := (&FileSigner{}).PublicKey(context.Background(), keysigner.KeyRef{Label: path})
		var invalid *errs.ConfigError
		require.ErrorAs(t, err, &invalid, "path %q", path)
		require.Equal(t, errs.SubtypeInvalidConfig, invalid.Subtype)
	}
	for _, name := range []string{".", "missing.pem", "oversized.pem", "invalid.pem"} {
		reject(t, filepath.Join(dir, name))
	}
	t.Run("denylisted symlink", func(t *testing.T) {
		path := filepath.Join(dir, "linked.pem")
		if err := os.Symlink(target, path); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		reject(t, path)
	})
}

func TestFileSignerReadsRotatedKey(t *testing.T) {
	dir := filepath.Join(t.TempDir(), strings.Repeat("a", 128), strings.Repeat("b", 128))
	require.NoError(t, os.MkdirAll(dir, 0700))
	ref := keysigner.KeyRef{Label: filepath.Join(dir, "private.pem")}
	signer := &FileSigner{}
	ctx := context.Background()
	input := []byte("header.claims")
	digest := sha256.Sum256(input)
	for range 2 {
		key := writeECPrivateKey(t, ref.Label)
		public, err := signer.PublicKey(ctx, ref)
		require.NoError(t, err)
		require.True(t, key.PublicKey.Equal(public))
		sig, alg, err := signer.Sign(ctx, ref, input)
		require.NoError(t, err)
		require.Equal(t, keysigner.AlgES256, alg)
		require.Len(t, sig, 64)
		require.True(t, ecdsa.Verify(&key.PublicKey, digest[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])))
	}
	require.ErrorIs(t, signer.DeleteKey(ctx, ref), ErrLifecycleNotSupported)
	require.NoError(t, os.Remove(ref.Label))
	_, _, err := signer.Sign(ctx, ref, input)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestFileSignerImportsAlgorithms(t *testing.T) {
	for _, algorithm := range []string{keysigner.AlgES384, keysigner.AlgES512, keysigner.AlgEdDSA, keysigner.AlgRS256} {
		t.Run(algorithm, func(t *testing.T) {
			var key crypto.Signer
			var err error
			switch algorithm {
			case keysigner.AlgES384:
				key, err = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
			case keysigner.AlgES512:
				key, err = ecdsa.GenerateKey(elliptic.P521(), rand.Reader)
			case keysigner.AlgEdDSA:
				_, key, err = ed25519.GenerateKey(rand.Reader)
			case keysigner.AlgRS256:
				key, err = rsa.GenerateKey(rand.Reader, 2048)
			}
			require.NoError(t, err)
			block := &pem.Block{Type: "PRIVATE KEY"}
			if rsaKey, ok := key.(*rsa.PrivateKey); ok {
				block.Type, block.Bytes = "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(rsaKey)
			} else {
				block.Bytes, err = x509.MarshalPKCS8PrivateKey(key)
				require.NoError(t, err)
			}
			ref := keysigner.KeyRef{Label: filepath.Join(t.TempDir(), "key.pem"), Algorithm: algorithm}
			require.NoError(t, os.WriteFile(ref.Label, pem.EncodeToMemory(block), 0600))
			signer := &FileSigner{}
			sig, got, err := signer.Sign(context.Background(), ref, []byte("header.claims"))
			require.NoError(t, err)
			require.Equal(t, algorithm, got)
			require.NotEmpty(t, sig)
			ref.Algorithm = keysigner.AlgES256
			_, _, err = signer.Sign(context.Background(), ref, nil)
			require.ErrorIs(t, err, keysigner.ErrUnsupportedAlgorithm)
			_, err = signer.PublicKey(context.Background(), ref)
			require.ErrorIs(t, err, keysigner.ErrUnsupportedAlgorithm)
		})
	}
}
