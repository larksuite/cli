// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package credential

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
)

// TestNewAgentTokenPersistsScope pins that a renewal carries the minted scope
// into the stored record; the store overwrites whole records.
func TestNewAgentTokenPersistsScope(t *testing.T) {
	future := time.Now().Add(2 * time.Hour).Unix()
	httpClient := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case core.OAuthTokenV3Path:
			return jsonHTTPResponse(http.StatusOK,
				`{"code":0,"access_token":"tok","expires_in":7200,"scope":"docs:doc bitable:app"}`), nil
		case auth.PathOAuthIntrospect:
			return jsonHTTPResponse(http.StatusOK,
				`{"active":true,"client_id":"cli_a","exp":`+strconv.FormatInt(future, 10)+
					`,"user_type":66,"open_id":"ou_x"}`), nil
		default:
			t.Fatalf("unexpected path %s", req.URL.Path)
			return nil, nil
		}
	})}

	p := NewDefaultTokenProvider(nil, func() (*http.Client, error) { return httpClient, nil }, io.Discard)
	acct := &Account{AppID: "cli_a", PrincipalOpenID: "ou_x"}

	stored, err := p.newAgentToken(acct)(context.Background())
	if err != nil {
		t.Fatalf("refresh: %v", err)
	}
	if stored.Scope != "docs:doc bitable:app" {
		t.Errorf("Scope = %q, want the scope from the freshly minted token", stored.Scope)
	}
}

// TestWarnAgentRefreshFailedRendersLikeTheStatusTimestamps pins that the one
// timestamp this package prints uses the same rendering as whoami and auth
// status: RFC3339 in the local zone with its offset. Two spellings of the same
// instant in one session read like a discrepancy that is not there.
func TestWarnAgentRefreshFailedRendersLikeTheStatusTimestamps(t *testing.T) {
	var buf bytes.Buffer
	p := NewDefaultTokenProvider(nil, nil, &buf)
	expiresAt := time.Date(2026, 9, 17, 12, 0, 0, 0, time.UTC).UnixMilli()

	p.warnAgentRefreshFailed(expiresAt)

	want := time.UnixMilli(expiresAt).Format(time.RFC3339)
	if !strings.Contains(buf.String(), want) {
		t.Fatalf("warning = %q, want it to carry %q", buf.String(), want)
	}
}
