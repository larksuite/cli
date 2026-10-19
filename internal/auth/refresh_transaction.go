package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keychain"
)

func defaultCredentialStore() *CredentialStore {
	backend := keychain.Default()
	if scoped, ok := backend.(interface{ LockNamespace() string }); ok {
		return NewCredentialStore(backend, scoped.LockNamespace())
	}
	dir, _ := os.UserCacheDir()
	if dir == "" {
		dir, _ = os.UserHomeDir()
	}
	return NewCredentialStore(backend, filepath.Join(dir, "lark-cli", "credential-locks"))
}
func (s *CredentialStore) write(record *StoredUAToken) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.backend.Set(keychain.LarkCliService, accountKey(record.AppId, record.UserOpenId), string(raw))
}

func refreshCredential(ctx context.Context, httpClient *http.Client, opts UATCallOptions, store *CredentialStore, current *StoredUAToken) (*StoredUAToken, error) {
	protocol := current.AuthProtocol
	base := core.ResolveOpenBaseURL(opts.Domain)
	if opts.Authority != "" {
		base = opts.Authority
	}
	if current.Authority != "" && strings.TrimRight(current.Authority, "/") != strings.TrimRight(base, "/") {
		return nil, &CredentialStateError{Kind: "credential_authority_conflict"}
	}
	if opts.AuthProtocol != "" && protocol != "" && opts.AuthProtocol != protocol {
		return nil, &CredentialStateError{Kind: "credential_protocol_conflict"}
	}
	if protocol == "" {
		if opts.Domain != core.BrandFeishu && opts.Domain != core.BrandLark && opts.Domain != "" {
			return nil, &CredentialStateError{Kind: "protocol_unknown"}
		}
		protocol = "oauth_v2"
	}
	var send func() (*CreateAccessTokenResponse, error)
	if protocol == "legacy_oidc" {
		client := NewOidcAccessTokenClient(httpClient, base)
		client.SetAppCredentials(opts.AppId, opts.AppSecret)
		// Preparation must complete before the durable send boundary.
		if _, err := client.getAppAccessToken(ctx); err != nil {
			return nil, err
		}
		send = func() (*CreateAccessTokenResponse, error) {
			return client.RefreshAccessToken(ctx, RefreshAccessTokenParams{RefreshToken: current.RefreshToken})
		}
	} else if protocol == "oauth_v2" {
		form := url.Values{"grant_type": {"refresh_token"}, "refresh_token": {current.RefreshToken}, "client_id": {opts.AppId}, "client_secret": {opts.AppSecret}}
		endpoints := ResolveOAuthEndpoints(opts.Domain)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoints.Token, strings.NewReader(form.Encode()))
		if err != nil {
			return nil, fmt.Errorf("refresh_request_invalid")
		}
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if httpClient == nil {
			httpClient = http.DefaultClient
		}
		safe := *httpClient
		safe.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
		send = func() (*CreateAccessTokenResponse, error) {
			resp, err := safe.Do(AuthenticationRequest(req))
			if err != nil {
				return nil, fmt.Errorf("authentication transport_error")
			}
			defer resp.Body.Close()
			body, err := readAuthBody(resp)
			if err != nil {
				return nil, err
			}
			var result CreateAccessTokenResponse
			if json.Unmarshal(body, &result) != nil {
				return nil, fmt.Errorf("authentication invalid_response")
			}
			if result.RefreshExpiresIn < 0 || result.RefreshTokenExpiresIn < 0 || (result.RefreshExpiresIn > 0 && !validAccessLifetime(result.RefreshExpiresIn)) || (result.RefreshTokenExpiresIn > 0 && !validAccessLifetime(result.RefreshTokenExpiresIn)) {
				return nil, authHTTPFailure(resp, "invalid_response", 0)
			}
			if result.RefreshExpiresIn > 0 && result.RefreshTokenExpiresIn > 0 && result.RefreshExpiresIn != result.RefreshTokenExpiresIn {
				return nil, authHTTPFailure(resp, "invalid_response", 0)
			}
			if result.RefreshExpiresIn == 0 {
				result.RefreshExpiresIn = result.RefreshTokenExpiresIn
			}
			if result.AccessToken == "" || !validAccessLifetime(result.ExpiresIn) {
				return nil, fmt.Errorf("authentication invalid_response")
			}
			return &result, nil
		}
	} else {
		return nil, &CredentialStateError{Kind: "protocol_unknown"}
	}
	pending := *current
	pending.RefreshInFlight = &RefreshAttempt{Generation: current.Generation, AttemptID: uuid.NewString(), StartedAt: time.Now().UnixMilli()}
	if err := store.write(&pending); err != nil {
		return nil, err
	}
	response, err := send()
	if err != nil {
		return nil, fmt.Errorf("refresh_outcome_unknown: %w", err)
	}
	if response.RefreshToken == "" {
		return nil, fmt.Errorf("refresh_outcome_unknown: incomplete refresh response")
	}
	next := *current
	now := time.Now().UnixMilli()
	next.AccessToken = response.AccessToken
	next.ExpiresAt = now + int64(response.ExpiresIn)*1000
	next.RefreshToken = response.RefreshToken
	if response.Scope != "" {
		next.Scope = response.Scope
	}
	if response.IDToken != "" {
		next.IDToken = response.IDToken
	}
	if response.RefreshExpiresIn > 0 {
		next.RefreshExpiresAt = now + int64(response.RefreshExpiresIn)*1000
	} else if response.RefreshToken != current.RefreshToken {
		next.RefreshExpiresAt = 0
	}
	next.Generation = current.Generation + 1
	next.RefreshInFlight = nil
	next.AuthProtocol = protocol
	next.Authority = base
	// Only storage commits may be retried; never replay the refresh POST.
	for attempt := 0; attempt < 3; attempt++ {
		if err = store.write(&next); err == nil {
			return &next, nil
		}
	}
	return nil, fmt.Errorf("refresh_outcome_unknown: credential commit failed")
}

func validateCredentialBinding(record *StoredUAToken, opts UATCallOptions) error {
	if record.AuthProtocol == "" && record.IDToken != "" {
		return &CredentialStateError{Kind: "protocol_unknown"}
	}
	base := core.ResolveOpenBaseURL(opts.Domain)
	if opts.Authority != "" {
		base = opts.Authority
	}
	if record.Authority != "" && strings.TrimRight(record.Authority, "/") != strings.TrimRight(base, "/") {
		return &CredentialStateError{Kind: "credential_authority_conflict"}
	}
	if opts.AuthProtocol != "" && record.AuthProtocol != "" && opts.AuthProtocol != record.AuthProtocol {
		return &CredentialStateError{Kind: "credential_protocol_conflict"}
	}
	if record.AuthProtocol == "" && opts.Domain != core.BrandFeishu && opts.Domain != core.BrandLark && opts.Domain != "" {
		return &CredentialStateError{Kind: "protocol_unknown"}
	}
	return nil
}

// ConfirmCredentialProtocol records an explicit app/account protocol confirmation,
// preserving tokens and refusing authority movement or unresolved refresh attempts.
func ConfirmCredentialProtocol(ctx context.Context, store *CredentialStore, app, user, protocol, authority string) error {
	if protocol != "legacy_oidc" && protocol != "oauth_v2" {
		return fmt.Errorf("unsupported auth protocol")
	}
	if authority == "" {
		return fmt.Errorf("authority required")
	}
	if store == nil {
		store = defaultCredentialStore()
	}
	return store.transaction(ctx, app, user, func(record *StoredUAToken) (*StoredUAToken, error) {
		if record == nil {
			return nil, &NeedAuthorizationError{UserOpenId: user}
		}
		if record.RefreshInFlight != nil {
			return nil, fmt.Errorf("refresh_outcome_unknown")
		}
		if record.Authority != "" && strings.TrimRight(record.Authority, "/") != strings.TrimRight(authority, "/") {
			return nil, &CredentialStateError{Kind: "credential_authority_conflict"}
		}
		next := *record
		next.AuthProtocol = protocol
		next.Authority = authority
		next.Generation++
		return &next, nil
	})
}

// CredentialStateError preserves a safe, actionable classification without secrets.
type CredentialStateError struct{ Kind string }

func (e *CredentialStateError) Error() string {
	return e.Kind + ": credentials preserved; explicit account confirmation or recovery required"
}
func (e *CredentialStateError) CredentialState() string { return e.Kind }
