// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	"fmt"

	"github.com/larksuite/cli/internal/cmdutil"
	intsec "github.com/larksuite/cli/internal/sec"
)

// installer wires up an internal/sec.Installer using the Factory's HTTP client,
// the default platform paths, and a lazy OAPI-client provider used to fetch
// the install manifest. APIClientFunc is a method value, not an eager call —
// commands that short-circuit (or that never install, like sec status / sec
// stop) avoid decrypting credentials from the keychain. Every cmd/sec
// subcommand starts here.
func installer(f *cmdutil.Factory) (*intsec.Installer, *intsec.Paths, error) {
	paths, err := intsec.DefaultPaths()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve sec paths: %w", err)
	}
	httpClient, err := f.HttpClient()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve http client: %w", err)
	}
	return &intsec.Installer{
		Paths:         paths,
		HTTPClient:    httpClient,
		APIClientFunc: f.NewAPIClient,
	}, paths, nil
}
