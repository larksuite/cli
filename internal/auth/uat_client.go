// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package auth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"sync"

	"github.com/larksuite/cli/internal/core"
)

var safeIDChars = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func sanitizeID(id string) string {
	return safeIDChars.ReplaceAllString(id, "_")
}

// UATCallOptions contains options for UAT API calls.
type UATCallOptions struct {
	Context      context.Context
	Store        *CredentialStore
	AuthProtocol string
	Authority    string
	UserOpenId   string
	AppId        string
	AppSecret    string
	Domain       core.LarkBrand
	ErrOut       io.Writer // diagnostic/status output (caller injects f.IOStreams.ErrOut)
}

// UATStatus represents the status of a user access token.
type UATStatus struct {
	Authorized       bool   `json:"authorized"`
	UserOpenId       string `json:"userOpenId"`
	Scope            string `json:"scope,omitempty"`
	ExpiresAt        int64  `json:"expiresAt,omitempty"`
	RefreshExpiresAt int64  `json:"refreshExpiresAt,omitempty"`
	GrantedAt        int64  `json:"grantedAt,omitempty"`
	TokenStatus      string `json:"tokenStatus,omitempty"`
}

// NewUATCallOptions creates UATCallOptions from a CLI config.
func NewUATCallOptions(cfg *core.CliConfig, errOut io.Writer) UATCallOptions {
	if errOut == nil {
		errOut = os.Stderr
	}
	domain := cfg.Brand
	if cfg.APIBaseURL != "" {
		domain = core.LarkBrand(cfg.APIBaseURL)
	}
	return UATCallOptions{
		UserOpenId:   cfg.UserOpenId,
		AppId:        cfg.AppID,
		AppSecret:    cfg.AppSecret,
		Domain:       domain,
		AuthProtocol: cfg.AuthProtocol, Authority: cfg.APIBaseURL,
		ErrOut: errOut,
	}
}

// GetValidAccessToken refreshes on demand under a backend-owned account transaction.
func GetValidAccessToken(httpClient *http.Client, opts UATCallOptions) (string, error) {
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	store := opts.Store
	if store == nil {
		store = defaultCredentialStore()
	}
	record, err := store.Load(ctx, opts.AppId, opts.UserOpenId)
	if err != nil {
		return "", err
	}
	if record == nil {
		return "", &NeedAuthorizationError{UserOpenId: opts.UserOpenId}
	}
	if err := validateCredentialBinding(record, opts); err != nil {
		return "", err
	}
	if record.RefreshInFlight != nil {
		refreshed, err := refreshWithLock(httpClient, opts, record)
		if err != nil {
			return "", err
		}
		return refreshed.AccessToken, nil
	}
	if TokenStatus(record) == "valid" {
		return record.AccessToken, nil
	}
	if TokenStatus(record) == "refresh_lifetime_unknown" {
		return "", &CredentialStateError{Kind: "refresh_lifetime_unknown"}
	}
	if TokenStatus(record) != "needs_refresh" {
		return "", &NeedAuthorizationError{UserOpenId: opts.UserOpenId}
	}
	refreshed, err := refreshWithLock(httpClient, opts, record)
	if err != nil {
		return "", err
	}
	return refreshed.AccessToken, nil
}

type refreshGroup struct {
	done  chan struct{}
	token *StoredUAToken
	err   error
}

var refreshGroups sync.Map

func refreshWithLock(httpClient *http.Client, opts UATCallOptions, stored *StoredUAToken) (*StoredUAToken, error) {
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	store := opts.Store
	if store == nil {
		store = defaultCredentialStore()
		opts.Store = store
	}
	key := store.namespace + "\x00" + opts.AppId + "\x00" + opts.UserOpenId
	group := &refreshGroup{done: make(chan struct{})}
	actual, loaded := refreshGroups.LoadOrStore(key, group)
	if loaded {
		waiting := actual.(*refreshGroup)
		select {
		case <-waiting.done:
			return waiting.token, waiting.err
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	group.token, group.err = refreshAccountTransaction(httpClient, opts, stored)
	close(group.done)
	refreshGroups.Delete(key)
	return group.token, group.err
}
func refreshAccountTransaction(httpClient *http.Client, opts UATCallOptions, _ *StoredUAToken) (*StoredUAToken, error) {
	ctx := opts.Context
	if ctx == nil {
		ctx = context.Background()
	}
	store := opts.Store
	if store == nil {
		store = defaultCredentialStore()
	}
	var result *StoredUAToken
	err := store.transaction(ctx, opts.AppId, opts.UserOpenId, func(current *StoredUAToken) (*StoredUAToken, error) {
		if current == nil {
			return nil, &NeedAuthorizationError{UserOpenId: opts.UserOpenId}
		}
		if current.RefreshInFlight != nil {
			return nil, fmt.Errorf("refresh_outcome_unknown")
		}
		if TokenStatus(current) == "valid" {
			result = current
			return nil, errNoCredentialMutation
		}
		if TokenStatus(current) != "needs_refresh" {
			return nil, &NeedAuthorizationError{UserOpenId: opts.UserOpenId}
		}
		updated, err := refreshCredential(ctx, httpClient, opts, store, current)
		if err != nil {
			return nil, err
		}
		result = updated
		return nil, errNoCredentialMutation
	})
	return result, err
}
