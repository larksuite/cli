// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package secplugin

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mustCreateTestCertPEM generates a short-lived self-signed CA certificate for tests.
func mustCreateTestCertPEM(t *testing.T) []byte {
	t.Helper()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}

	der, err := x509.CreateCertificate(rand.Reader, &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "secplugin-test-ca",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}, &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "secplugin-test-ca",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate() error = %v", err)
	}

	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

// TestApplyExtraRootCA_EmptyPathIsNoop verifies that an empty CA path leaves the transport unchanged.
func TestApplyExtraRootCA_EmptyPathIsNoop(t *testing.T) {
	tr := &http.Transport{}

	if err := applyExtraRootCA(tr, "   "); err != nil {
		t.Fatalf("applyExtraRootCA() error = %v", err)
	}
	if tr.TLSClientConfig != nil {
		t.Fatalf("TLSClientConfig = %#v, want nil", tr.TLSClientConfig)
	}
}

// TestApplyExtraRootCA_RejectsRelativePath verifies that CA paths must be absolute.
func TestApplyExtraRootCA_RejectsRelativePath(t *testing.T) {
	tr := &http.Transport{}

	err := applyExtraRootCA(tr, "ca.pem")
	if err == nil || !strings.Contains(err.Error(), "must be an absolute path") {
		t.Fatalf("applyExtraRootCA() error = %v, want absolute-path error", err)
	}
}

// TestApplyExtraRootCA_RejectsMissingFile verifies read errors for missing PEM bundles.
func TestApplyExtraRootCA_RejectsMissingFile(t *testing.T) {
	tr := &http.Transport{}

	err := applyExtraRootCA(tr, filepath.Join(t.TempDir(), "missing.pem"))
	if err == nil || !strings.Contains(err.Error(), "failed to read") {
		t.Fatalf("applyExtraRootCA() error = %v, want read error", err)
	}
}

// TestApplyExtraRootCA_RejectsInvalidPEM verifies validation of malformed PEM bundles.
func TestApplyExtraRootCA_RejectsInvalidPEM(t *testing.T) {
	caPath := filepath.Join(t.TempDir(), "invalid.pem")
	writeFile(t, caPath, []byte("not a pem"), 0600)

	tr := &http.Transport{}
	err := applyExtraRootCA(tr, caPath)
	if err == nil || !strings.Contains(err.Error(), "no certificates parsed from PEM") {
		t.Fatalf("applyExtraRootCA() error = %v, want invalid PEM error", err)
	}
}

// TestApplyExtraRootCA_SetsTLSConfigWhenMissing verifies initialization of TLSClientConfig when absent.
func TestApplyExtraRootCA_SetsTLSConfigWhenMissing(t *testing.T) {
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	writeFile(t, caPath, mustCreateTestCertPEM(t), 0600)

	tr := &http.Transport{}
	if err := applyExtraRootCA(tr, caPath); err != nil {
		t.Fatalf("applyExtraRootCA() error = %v", err)
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("TLSClientConfig = nil, want initialized config")
	}
	if tr.TLSClientConfig.RootCAs == nil {
		t.Fatal("RootCAs = nil, want cert pool")
	}
}

// TestApplyExtraRootCA_ClonesExistingTLSConfig verifies cloning when the base transport already has TLS settings.
func TestApplyExtraRootCA_ClonesExistingTLSConfig(t *testing.T) {
	caPath := filepath.Join(t.TempDir(), "ca.pem")
	writeFile(t, caPath, mustCreateTestCertPEM(t), 0600)

	original := &tls.Config{ServerName: "open.feishu.cn"}
	tr := &http.Transport{TLSClientConfig: original}
	if err := applyExtraRootCA(tr, caPath); err != nil {
		t.Fatalf("applyExtraRootCA() error = %v", err)
	}
	if tr.TLSClientConfig == original {
		t.Fatal("TLSClientConfig pointer reused, want clone")
	}
	if tr.TLSClientConfig.ServerName != original.ServerName {
		t.Fatalf("ServerName = %q, want %q", tr.TLSClientConfig.ServerName, original.ServerName)
	}
	if tr.TLSClientConfig.RootCAs == nil {
		t.Fatal("RootCAs = nil, want cert pool")
	}
}
