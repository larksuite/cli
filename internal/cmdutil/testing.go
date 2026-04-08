// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	lark "github.com/larksuite/oapi-sdk-go/v3"
	larkcore "github.com/larksuite/oapi-sdk-go/v3/core"

	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/credential"
	"github.com/larksuite/cli/internal/httpmock"
)

// noopKeychain is a no-op KeychainAccess for tests that don't need keychain.
type noopKeychain struct{}

func (n *noopKeychain) Get(service, account string) (string, error) { return "", nil }
func (n *noopKeychain) Set(service, account, value string) error    { return nil }
func (n *noopKeychain) Remove(service, account string) error        { return nil }

// TestFactory creates a Factory for testing.
// Returns (factory, stdout buffer, stderr buffer, http mock registry).
func TestFactory(t *testing.T, config *core.CliConfig) (*Factory, *bytes.Buffer, *bytes.Buffer, *httpmock.Registry) {
	t.Helper()

	reg := &httpmock.Registry{}
	t.Cleanup(func() { reg.Verify(t) })

	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}

	mockClient := httpmock.NewClient(reg)
	sdkMockClient := &http.Client{
		Transport: &UserAgentTransport{Base: reg},
	}

	var testLarkClient *lark.Client
	if config != nil && config.AppID != "" {
		opts := []lark.ClientOptionFunc{
			lark.WithEnableTokenCache(false),
			lark.WithLogLevel(larkcore.LogLevelError),
			lark.WithHttpClient(sdkMockClient),
			lark.WithHeaders(BaseSecurityHeaders()),
		}
		if config.Brand != "" {
			opts = append(opts, lark.WithOpenBaseUrl(core.ResolveOpenBaseURL(config.Brand)))
		}
		testLarkClient = lark.NewClient(config.AppID, credential.RuntimeAppSecret(config.AppSecret), opts...)
	}

	testCred := credential.NewCredentialProvider(
		nil,
		&testDefaultAcct{config: config},
		&testDefaultToken{},
		func() (*http.Client, error) { return mockClient, nil },
	)

	f := &Factory{
		Config:     func() (*core.CliConfig, error) { return config, nil },
		HttpClient: func() (*http.Client, error) { return mockClient, nil },
		LarkClient: func() (*lark.Client, error) { return testLarkClient, nil },
		IOStreams:  &IOStreams{In: nil, Out: stdoutBuf, ErrOut: stderrBuf},
		Keychain:   &noopKeychain{},
		Credential: testCred,
	}
	return f, stdoutBuf, stderrBuf, reg
}

type testDefaultAcct struct {
	config *core.CliConfig
}

func (a *testDefaultAcct) ResolveAccount(ctx context.Context) (*credential.Account, error) {
	if a.config == nil {
		return &credential.Account{}, nil
	}
	return credential.AccountFromCliConfig(a.config), nil
}

type testDefaultToken struct{}

func (t *testDefaultToken) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.TokenResult, error) {
	return &credential.TokenResult{Token: "test-token"}, nil
}
