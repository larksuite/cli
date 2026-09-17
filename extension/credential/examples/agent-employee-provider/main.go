// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Command agent-employee-provider is a runnable fork of lark-cli whose
// credentials for an Agent Employee come from an external system instead of the
// local keychain.
//
// It shows the two things such a provider owes the CLI. First, its account
// declares up front that it is governed by an Agent Employee credential and
// names the principal it has already verified — the CLI checks every credential
// against those declarations rather than trusting each one on arrival. Second,
// it returns the credential and its subject together through AccessProvider: the
// legacy token interface can carry only a string, which leaves nothing to check
// a principal against.
//
// Build & run:
//
//	cd extension/credential/examples/agent-employee-provider
//	go build -o agent-cli .
//	export EXAMPLE_AGENT_APP_ID=cli_xxx
//	export EXAMPLE_AGENT_OPEN_ID=ou_xxx
//	export EXAMPLE_AGENT_TOKEN=<token minted by your system>
//	./agent-cli whoami
//	# {"identity":"user","credential":"aat","principal":"agent_employee",...}
//
//	./agent-cli api --url /open-apis/im/v1/chats --method GET
//	# the Agent Employee credential is sent on the user lane
//
//	./agent-cli api --url /open-apis/im/v1/chats --as bot
//	# refused before any request: an Agent Employee has no tenant lane
package main

import (
	"context"
	"os"
	"time"

	"github.com/larksuite/cli/cmd"
	"github.com/larksuite/cli/extension/credential"
)

// Environment inputs stand in for whatever real transport a deployment uses to
// reach its credential service.
const (
	envAppID  = "EXAMPLE_AGENT_APP_ID"
	envOpenID = "EXAMPLE_AGENT_OPEN_ID"
	envToken  = "EXAMPLE_AGENT_TOKEN"
)

// exampleTokenTTL is how long this example claims its credentials last. A real
// provider reports what its own issuer returned; the CLI refuses an Access that
// is already past its expiry.
const exampleTokenTTL = time.Hour

type provider struct{}

func (p *provider) Name() string { return "agent-employee-example" }

// ResolveAccount declares the account before any credential is resolved for it.
// Returning nil means "not configured here", which lets the next credential
// source answer instead.
func (p *provider) ResolveAccount(ctx context.Context) (*credential.Account, error) {
	appID := os.Getenv(envAppID)
	principal := os.Getenv(envOpenID)
	if appID == "" || principal == "" {
		return nil, nil
	}
	return &credential.Account{
		AppID: appID,
		// The real secret stays in the external system; this provider mints
		// nothing locally and the CLI never needs it.
		AppSecret: credential.NoAppSecret,
		Brand:     credential.BrandFeishu,
		// An Agent Employee is a user principal with no tenant lane, so the
		// account says so once here instead of every call site guessing.
		CredentialKind:      credential.CredentialAAT,
		SupportedIdentities: credential.SupportsUser,
		DefaultAs:           credential.IdentityUser,
		// OpenID carries the principal this provider has already verified. The
		// CLI compares every credential it later receives against this value, so
		// a rebinding on the platform side cannot go unnoticed.
		OpenID: principal,
	}, nil
}

// ResolveToken is part of the long-standing Provider interface and stays here so
// this file compiles against it unchanged. It is never consulted for this
// account: an Agent Employee account is served through ResolveAccess, and
// answering here would hand back a credential with no subject attached.
func (p *provider) ResolveToken(ctx context.Context, req credential.TokenSpec) (*credential.Token, error) {
	return nil, nil
}

// ResolveAccess returns the credential together with the facts the CLI checks it
// against. Kind, OpenID and ExpiresAt are not decoration: a mismatch with the
// declared account fails the request rather than sending the credential.
func (p *provider) ResolveAccess(ctx context.Context, req credential.AccessSpec) (*credential.Access, error) {
	token, err := fetchAgentCredential(ctx, req.AppID)
	if err != nil {
		return nil, err
	}
	return &credential.Access{
		Value:     token,
		Kind:      credential.CredentialAAT,
		OpenID:    os.Getenv(envOpenID),
		ExpiresAt: time.Now().Add(exampleTokenTTL),
		// An Agent Employee credential is issued without scopes: the platform
		// decides what the Agent may do, so there is nothing for a local scope
		// pre-check to compare against.
		Scopes: "",
	}, nil
}

// fetchAgentCredential stands in for a call to whatever service holds the
// credential. A real implementation performs its own verification here; the CLI
// does not re-verify an external credential, it only checks it against the
// account this provider declared.
func fetchAgentCredential(ctx context.Context, appID string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	token := os.Getenv(envToken)
	if token == "" {
		// Returning an error rather than an empty credential keeps the failure
		// attributable to this provider instead of surfacing later as a puzzling
		// rejection from the API.
		return "", &credential.BlockError{
			Provider: "agent-employee-example",
			Reason:   "no Agent Employee credential available from the external system",
		}
	}
	return token, nil
}

func init() {
	credential.Register(&provider{})
}

func main() {
	os.Exit(cmd.Execute())
}
