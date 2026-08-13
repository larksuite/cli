// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// This file is overlaid into the real CLI main package only for the local
// protocol E2E. It gives every supported OS the same explicit test trust root
// without weakening production TLS behavior or modifying a host trust store.
package main

import (
	"crypto/x509"
	"os"

	"github.com/larksuite/cli/internal/vfs"
)

func init() {
	path := os.Getenv("LARKSUITE_CLI_E2E_CA_PATH")
	if path == "" {
		return
	}
	pemBytes, err := vfs.ReadFile(path)
	if err != nil {
		panic("read external credential E2E root CA: " + err.Error())
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(pemBytes) {
		panic("parse external credential E2E root CA")
	}
	x509.SetFallbackRoots(roots)
}
