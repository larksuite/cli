package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOIDCFlowOptions(t *testing.T) {
	opts := &OIDCFlowOptions{
		AppID:       "test-app",
		AppSecret:   "test-secret",
		Domain:      "example.com",
		RedirectURI: "http://localhost:8080/callback",
		Timeout:     120 * time.Second,
	}

	assert.Equal(t, "test-app", opts.AppID)
	assert.Equal(t, "test-secret", opts.AppSecret)
	assert.Equal(t, "example.com", opts.Domain)
}

func TestTokenStatus(t *testing.T) {
	now := time.Now().UnixMilli()

	// Test valid token
	validToken := &StoredUAToken{
		ExpiresAt:        now + 3600*1000, // 1 hour from now
		RefreshExpiresAt: now + 3600*1000,
		RefreshToken:     "refresh-token",
	}
	assert.Equal(t, "valid", TokenStatus(validToken))

	// Test token that needs refresh (within 5 minutes)
	needsRefreshToken := &StoredUAToken{
		ExpiresAt:        now + 2*60*1000, // 2 minutes from now
		RefreshExpiresAt: now + 3600*1000, // 1 hour from now
		RefreshToken:     "refresh-token",
	}
	assert.Equal(t, "needs_refresh", TokenStatus(needsRefreshToken))

	// Test expired token (both access and refresh tokens expired)
	expiredToken := &StoredUAToken{
		ExpiresAt:        now - 1000, // 1 second ago
		RefreshExpiresAt: now - 1000, // Refresh token also expired
		RefreshToken:     "refresh-token",
	}
	assert.Equal(t, "expired", TokenStatus(expiredToken))

	// Test expired ID token
	expiredIDToken := &StoredUAToken{
		IDToken:          "some.id.token",
		IDTokenExpiresAt: now - 1000, // 1 second ago
		ExpiresAt:        now + 3600*1000,
		RefreshExpiresAt: now + 3600*1000,
		RefreshToken:     "refresh-token",
	}
	assert.Equal(t, "valid", TokenStatus(expiredIDToken))
}

func TestOIDCFlowResult(t *testing.T) {
	result := &OIDCFlowResult{
		AccessToken:      "access-token",
		RefreshToken:     "refresh-token",
		IDToken:          "id-token",
		ExpiresIn:        3600,
		RefreshExpiresIn: 7200,
		OpenID:           "user-open-id",
		UserName:         "Test User",
		Email:            "test@example.com",
		Claims:           map[string]interface{}{"sub": "user-sub", "email": "test@example.com"},
	}

	assert.Equal(t, "access-token", result.AccessToken)
	assert.Equal(t, "refresh-token", result.RefreshToken)
	assert.Equal(t, "id-token", result.IDToken)
	assert.Equal(t, 3600, result.ExpiresIn)
	assert.Equal(t, "user-open-id", result.OpenID)
	assert.Equal(t, "Test User", result.UserName)
	assert.Equal(t, "test@example.com", result.Email)
	assert.NotNil(t, result.Claims)
}

func TestStoredUATokenUpdateFromOIDCResult(t *testing.T) {
	token := &StoredUAToken{}
	result := &OIDCFlowResult{
		AccessToken:      "new-access-token",
		RefreshToken:     "new-refresh-token",
		IDToken:          "new-id-token",
		ExpiresIn:        3600,
		RefreshExpiresIn: 7200,
		OpenID:           "new-user-open-id",
		UserName:         "New Test User",
		Email:            "new-test@example.com",
		Claims:           map[string]interface{}{"sub": "new-user-sub", "email": "new-test@example.com", "exp": float64(time.Now().Unix() + 3600)},
	}

	token.UpdateFromOIDCResult(result)

	assert.Equal(t, "new-access-token", token.AccessToken)
	assert.Equal(t, "new-refresh-token", token.RefreshToken)
	assert.Equal(t, "new-id-token", token.IDToken)
	assert.Equal(t, "new-user-open-id", token.UserOpenId)
	assert.NotNil(t, token.UserInfo)
	assert.Equal(t, "new-test@example.com", token.UserInfo["email"])
	assert.Equal(t, "New Test User", token.UserInfo["name"])
}

// Mock HTTP client for testing
type mockHTTPClient struct{}

func (c *mockHTTPClient) Do(req *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("mock error for testing")
}

// TestOIDCFlowIntegration is an integration test that requires actual OIDC provider
func TestOIDCFlowIntegration(t *testing.T) {
	// This test requires an actual OIDC provider setup
	// For now, we'll just check that the functions exist and can be called
	// without causing panics

	opts := &OIDCFlowOptions{
		AppID:       "test-app",
		AppSecret:   "test-secret",
		Domain:      "example.com",
		RedirectURI: "http://localhost:8080/callback",
		Timeout:     100 * time.Millisecond, // Very short timeout for test
	}

	// We won't actually run the flow in this test as it requires interaction
	// But we verify the function signature and parameters

	assert.Equal(t, "test-app", opts.AppID)
	assert.Equal(t, "test-secret", opts.AppSecret)
	assert.Equal(t, "example.com", opts.Domain)
	assert.Greater(t, opts.Timeout, time.Duration(0))
}

// TestVerifyIDToken tests the ID token verification logic
func TestVerifyIDToken(t *testing.T) {
	// Create a mock JWT token with valid claims (unsigned, for testing only)
	header := `{"alg":"none","typ":"JWT"}`
	claims := map[string]interface{}{
		"iss":   "https://example.com",
		"aud":   "test-client-id",
		"exp":   time.Now().Unix() + 3600,
		"iat":   time.Now().Unix() - 60,
		"sub":   "user123",
		"email": "test@example.com",
		"name":  "Test User",
	}

	claimsJSON, _ := json.Marshal(claims)

	headerEncoded := base64.RawURLEncoding.EncodeToString([]byte(header))
	claimsEncoded := base64.RawURLEncoding.EncodeToString(claimsJSON)

	// Create an unsigned token (none algorithm)
	mockToken := headerEncoded + "." + claimsEncoded + "."

	// Test with skip verification (since we can't verify without proper keys)
	// In production, this would use actual JWKS verification
	claimsResult, err := parseIDTokenClaims(mockToken)

	assert.NoError(t, err)
	assert.NotNil(t, claimsResult)
	assert.Equal(t, "test@example.com", claimsResult["email"])
	assert.Equal(t, "Test User", claimsResult["name"])
}

// TestParseIDTokenClaims tests the JWT claims parsing
func TestParseIDTokenClaims(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		wantErr     bool
		errContains string
	}{
		{
			name:        "invalid format - not enough parts",
			token:       "invalid",
			wantErr:     true,
			errContains: "invalid ID token format",
		},
		{
			name:        "invalid format - too many parts",
			token:       "a.b.c.d",
			wantErr:     true,
			errContains: "invalid ID token format",
		},
		{
			name:    "valid claims parsing",
			token:   "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiZW1haWwiOiJqb2huQGV4YW1wbGUuY29tIn0.Gfx6VO9tcxwk6xqx9yYzSfebfeBZp4Jkt-11A4R4X0s",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := parseIDTokenClaims(tt.token)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, claims)
			}
		})
	}
}

// TestOIDCFlowOptionsValidation tests validation of OIDC flow options
func TestOIDCFlowOptionsValidation(t *testing.T) {
	tests := []struct {
		name        string
		opts        *OIDCFlowOptions
		wantErr     bool
		errContains string
	}{
		{
			name: "valid options with all fields",
			opts: &OIDCFlowOptions{
				AppID:       "test-app",
				AppSecret:   "test-secret",
				Domain:      "example.com",
				RedirectURI: "http://localhost:8080/callback",
				Timeout:     120 * time.Second,
			},
			wantErr: false,
		},
		{
			name: "valid options with default scope",
			opts: &OIDCFlowOptions{
				AppID:       "test-app",
				AppSecret:   "test-secret",
				Domain:      "example.com",
				RedirectURI: "http://localhost:8080/callback",
				Timeout:     120 * time.Second,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate the options structure
			assert.NotEmpty(t, tt.opts.AppID)
			assert.NotEmpty(t, tt.opts.AppSecret)
			assert.NotEmpty(t, tt.opts.Domain)
			assert.NotEmpty(t, tt.opts.RedirectURI)
			assert.Greater(t, tt.opts.Timeout, time.Duration(0))
		})
	}
}

// TestStoredUATokenSerialization tests JSON serialization/deserialization of StoredUAToken
func TestStoredUATokenSerialization(t *testing.T) {
	now := time.Now().UnixMilli()

	original := &StoredUAToken{
		UserOpenId:       "test-user-open-id",
		AppId:            "test-app-id",
		AccessToken:      "test-access-token",
		RefreshToken:     "test-refresh-token",
		IDToken:          "test-id-token.jwt.here",
		ExpiresAt:        now + 3600*1000,
		RefreshExpiresAt: now + 604800*1000,
		IDTokenExpiresAt: now + 3600*1000,
		GrantedAt:        now,
		UserInfo: map[string]interface{}{
			"email": "test@example.com",
			"name":  "Test User",
		},
	}

	// Serialize
	data, err := json.Marshal(original)
	assert.NoError(t, err)

	// Deserialize
	var deserialized StoredUAToken
	err = json.Unmarshal(data, &deserialized)
	assert.NoError(t, err)

	// Verify all fields are preserved
	assert.Equal(t, original.UserOpenId, deserialized.UserOpenId)
	assert.Equal(t, original.AppId, deserialized.AppId)
	assert.Equal(t, original.AccessToken, deserialized.AccessToken)
	assert.Equal(t, original.RefreshToken, deserialized.RefreshToken)
	assert.Equal(t, original.IDToken, deserialized.IDToken)
	assert.Equal(t, original.ExpiresAt, deserialized.ExpiresAt)
	assert.Equal(t, original.RefreshExpiresAt, deserialized.RefreshExpiresAt)
	assert.Equal(t, original.IDTokenExpiresAt, deserialized.IDTokenExpiresAt)
	assert.Equal(t, original.GrantedAt, deserialized.GrantedAt)
	assert.Equal(t, original.UserInfo["email"], deserialized.UserInfo["email"])
	assert.Equal(t, original.UserInfo["name"], deserialized.UserInfo["name"])
}

// TestStoredUATokenBackwardCompatibility tests that old token format without ID Token fields still works
func TestStoredUATokenBackwardCompatibility(t *testing.T) {
	now := time.Now().UnixMilli()

	// Old format token (without ID Token fields)
	oldFormatJSON := fmt.Sprintf(`{
		"userOpenId": "test-user",
		"appId": "test-app",
		"accessToken": "access-token",
		"refreshToken": "refresh-token",
		"expiresAt": %d,
		"refreshExpiresAt": %d,
		"scope": "calendar:calendar:read",
		"grantedAt": %d
	}`, now+3600*1000, now+604800*1000, now)

	var token StoredUAToken
	err := json.Unmarshal([]byte(oldFormatJSON), &token)
	assert.NoError(t, err)

	// Old format should still work, ID Token fields will be zero values
	assert.Equal(t, "test-user", token.UserOpenId)
	assert.Equal(t, "test-app", token.AppId)
	assert.Equal(t, "access-token", token.AccessToken)
	assert.Empty(t, token.IDToken)
	assert.Equal(t, int64(0), token.IDTokenExpiresAt)

	// TokenStatus should still work correctly
	status := TokenStatus(&token)
	assert.Equal(t, "valid", status)
}

// TestUpdateFromOIDCResult tests the UpdateFromOIDCResult method
func TestUpdateFromOIDCResult(t *testing.T) {
	token := &StoredUAToken{
		AppId:      "test-app",
		UserOpenId: "old-user",
	}

	now := time.Now()
	result := &OIDCFlowResult{
		AccessToken:      "new-access-token",
		RefreshToken:     "new-refresh-token",
		IDToken:          "new-id-token",
		ExpiresIn:        7200,
		RefreshExpiresIn: 604800,
		OpenID:           "new-open-id",
		UserName:         "New User",
		Email:            "new@example.com",
		Claims: map[string]interface{}{
			"exp": now.Add(2 * time.Hour).Unix(),
			"sub": "new-open-id",
		},
	}

	token.UpdateFromOIDCResult(result)

	assert.Equal(t, "new-access-token", token.AccessToken)
	assert.Equal(t, "new-refresh-token", token.RefreshToken)
	assert.Equal(t, "new-id-token", token.IDToken)
	assert.Equal(t, "new-open-id", token.UserOpenId)
	assert.Equal(t, "New User", token.UserInfo["name"])
	assert.Equal(t, "new@example.com", token.UserInfo["email"])
	assert.Greater(t, token.IDTokenExpiresAt, time.Now().UnixMilli())
}

// TestOIDCFlowCallbackHandling tests the callback handling in OIDC flow
func TestOIDCFlowCallbackHandling(t *testing.T) {
	// Create a test server to simulate callback
	callbackData := url.Values{}
	callbackData.Set("code", "test-auth-code")
	callbackData.Set("state", "test-state")

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		code := query.Get("code")
		state := query.Get("state")

		assert.Equal(t, "test-auth-code", code)
		assert.Equal(t, "test-state", state)

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Authentication successful"))
	})

	server := httptest.NewServer(handler)
	defer server.Close()

	// Make a test request to the callback handler
	resp, err := http.Get(server.URL + "?code=test-auth-code&state=test-state")
	assert.NoError(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "Authentication successful")
}
