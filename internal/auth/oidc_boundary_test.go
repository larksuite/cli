package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOIDCRejectsHTTPFailureWithoutReflectingSecrets(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		w.Write([]byte(`{"code":0,"data":{"open_id":"reflected-secret"}}`))
	}))
	defer s.Close()
	c := NewOidcAccessTokenClient(s.Client(), s.URL)
	got, err := c.GetUserInfo(context.Background(), "seed-token")
	if err == nil || got != nil {
		t.Fatalf("HTTP 500 must fail, got %#v %v", got, err)
	}
	if strings.Contains(err.Error(), "reflected-secret") || strings.Contains(err.Error(), "seed-token") {
		t.Fatalf("secret reflected: %v", err)
	}
}

func TestOIDCRejectsRedirectBeforeForwardingCredentials(t *testing.T) {
	var forwarded int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded++
		w.Write([]byte(`{"code":0,"data":{"open_id":"user"}}`))
	}))
	defer target.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()
	c := NewOidcAccessTokenClient(source.Client(), source.URL)
	_, err := c.GetUserInfo(context.Background(), "seed-token")
	if err == nil || forwarded != 0 {
		t.Fatalf("redirect must fail without second request: forwarded=%d err=%v", forwarded, err)
	}
}

func TestOIDCResponseBoundary(t *testing.T) {
	for _, tc := range []struct{ name, body string }{
		{"oversize", `{"code":0,"data":{"open_id":"user"},"padding":"` + strings.Repeat("x", 1<<20) + `"}`},
		{"reflected-error", `{"code":1770032,"msg":"seed-secret","data":null}`},
		{"missing-identity", `{"code":0,"data":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte(tc.body)) }))
			defer s.Close()
			_, err := NewOidcAccessTokenClient(s.Client(), s.URL).GetUserInfo(context.Background(), "seed-token")
			if err == nil {
				t.Fatal("invalid response accepted")
			}
			if strings.Contains(err.Error(), "seed-secret") {
				t.Fatal("server secret reflected")
			}
		})
	}
}

func TestOIDCRequiresTokenLifetime(t *testing.T) {
	for _, refresh := range []bool{false, true} {
		t.Run(map[bool]string{false: "exchange", true: "refresh"}[refresh], func(t *testing.T) {
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/auth/v3/") {
					w.Write([]byte(`{"code":0,"app_access_token":"app","app_access_token_expire":7200}`))
					return
				}
				w.Write([]byte(`{"code":0,"data":{"access_token":"access","refresh_token":"refresh"}}`))
			}))
			defer s.Close()
			c := NewOidcAccessTokenClient(s.Client(), s.URL)
			c.SetAppCredentials("app", "secret")
			var err error
			if refresh {
				_, err = c.RefreshAccessToken(context.Background(), RefreshAccessTokenParams{RefreshToken: "refresh"})
			} else {
				_, err = c.CreateAccessToken(context.Background(), CreateAccessTokenParams{Code: "code"})
			}
			if err == nil {
				t.Fatal("missing access lifetime accepted")
			}
		})
	}
}

func TestOIDCErrorCarriesRealStatusAndSafeBusinessCode(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Request-Id", "req-123")
		w.WriteHeader(403)
		w.Write([]byte(`{"code":1770032,"msg":"seed-secret"}`))
	}))
	defer s.Close()
	_, err := NewOidcAccessTokenClient(s.Client(), s.URL).GetUserInfo(context.Background(), "token")
	var diagnostic interface {
		HTTPStatus() int
		BusinessCode() int
		RequestID() string
	}
	if !errors.As(err, &diagnostic) {
		t.Fatalf("missing structured diagnostic: %v", err)
	}
	if diagnostic.HTTPStatus() != 403 || diagnostic.BusinessCode() != 1770032 || diagnostic.RequestID() != "req-123" {
		t.Fatalf("incorrect diagnostic: %v", err)
	}
}

func TestOIDCRejectsConflictingRefreshLifetimes(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/auth/v3/") {
			fmt.Fprint(w, `{"code":0,"app_access_token":"app","app_access_token_expire":7200}`)
			return
		}
		fmt.Fprint(w, `{"code":0,"data":{"access_token":"access","refresh_token":"refresh","expires_in":7200,"refresh_expires_in":100,"refresh_token_expires_in":200}}`)
	}))
	defer s.Close()
	c := NewOidcAccessTokenClient(s.Client(), s.URL)
	c.SetAppCredentials("app", "secret")
	if _, err := c.RefreshAccessToken(context.Background(), RefreshAccessTokenParams{RefreshToken: "old"}); err == nil {
		t.Fatal("conflicting lifetimes accepted")
	}
}

func TestAuthenticationFourEndpointsShareFailureBoundary(t *testing.T) {
	endpoints := []string{"app", "exchange", "userinfo", "refresh"}
	for _, endpoint := range endpoints {
		t.Run(endpoint, func(t *testing.T) {
			for _, tc := range []struct {
				name   string
				status int
				body   string
			}{
				{"html403", 403, "<html>seed-secret</html>"}, {"server-pseudo-success", 500, `{"code":0,"app_access_token":"seed-secret","app_access_token_expire":7200,"data":{"access_token":"seed-secret","expires_in":7200,"open_id":"user"}}`}, {"badjson", 200, "seed-secret"}, {"huge", 200, strings.Repeat("x", (1<<20)+1)}, {"reflection", 200, `{"code":1770032,"msg":"seed-secret"}`}, {"missing-required", 200, `{"code":0,"data":{}}`},
			} {
				t.Run(tc.name, func(t *testing.T) {
					s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if endpoint != "app" && strings.Contains(r.URL.Path, "/auth/v3/") {
							fmt.Fprint(w, `{"code":0,"app_access_token":"app","app_access_token_expire":7200}`)
							return
						}
						w.WriteHeader(tc.status)
						fmt.Fprint(w, tc.body)
					}))
					defer s.Close()
					c := NewOidcAccessTokenClient(s.Client(), s.URL)
					c.SetAppCredentials("app", "seed-secret")
					var err error
					switch endpoint {
					case "app":
						_, err = c.getAppAccessToken(context.Background())
					case "exchange":
						_, err = c.CreateAccessToken(context.Background(), CreateAccessTokenParams{Code: "seed-secret"})
					case "userinfo":
						_, err = c.GetUserInfo(context.Background(), "seed-secret")
					case "refresh":
						_, err = c.RefreshAccessToken(context.Background(), RefreshAccessTokenParams{RefreshToken: "seed-secret"})
					}
					if err == nil || strings.Contains(err.Error(), "seed-secret") {
						t.Fatalf("unsafe authentication response: %v", err)
					}
					var diagnostic interface{ HTTPStatus() int }
					if !errors.As(err, &diagnostic) || diagnostic.HTTPStatus() != tc.status {
						t.Fatalf("real HTTP status lost: %v", err)
					}
				})
			}
		})
	}
}

type boundaryTransport func(*http.Request) (*http.Response, error)

func (f boundaryTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestAuthenticationRejectsUnsafeConfiguredEndpoint(t *testing.T) {
	for _, base := range []string{"http://private.test", "https://user:secret@private.test", "https://private.test?token=secret", "https://private.test#fragment"} {
		calls := 0
		httpClient := &http.Client{Transport: boundaryTransport(func(*http.Request) (*http.Response, error) {
			calls++
			return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(`{"code":0,"data":{"open_id":"user"}}`))}, nil
		})}
		c := NewOidcAccessTokenClient(httpClient, base)
		_, err := c.GetUserInfo(context.Background(), "seed-token")
		if err == nil || calls != 0 || strings.Contains(err.Error(), "secret") {
			t.Fatalf("unsafe base accepted/reflected: calls=%d err=%v", calls, err)
		}
	}
}

func TestLegacyAppTokenAcceptsDocumentedExpireField(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":0,"app_access_token":"app-token","expire":7200}`)
	}))
	defer s.Close()
	c := NewOidcAccessTokenClient(s.Client(), s.URL)
	c.SetAppCredentials("app", "secret")
	token, err := c.getAppAccessToken(context.Background())
	if err != nil || token != "app-token" {
		t.Fatalf("documented app token lifetime unsupported: %v", err)
	}
}

func TestAuthenticationRedirectMatrixNeverReachesSecondStation(t *testing.T) {
	for _, endpoint := range []string{"app", "exchange", "userinfo", "refresh"} {
		for _, status := range []int{307, 308} {
			for _, destination := range []string{"same-host", "foreign-host", "downgrade"} {
				t.Run(fmt.Sprintf("%s/%d/%s", endpoint, status, destination), func(t *testing.T) {
					second := 0
					target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						second++
						fmt.Fprint(w, `{"code":0,"data":{"open_id":"user"}}`)
					}))
					defer target.Close()
					var source *httptest.Server
					handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path == "/destination" {
							second++
							return
						}
						if endpoint != "app" && strings.Contains(r.URL.Path, "/auth/v3/") {
							fmt.Fprint(w, `{"code":0,"app_access_token":"app","expire":7200}`)
							return
						}
						location := target.URL
						if destination == "same-host" {
							location = source.URL + "/destination"
						}
						http.Redirect(w, r, location, status)
					})
					if destination == "downgrade" {
						source = httptest.NewTLSServer(handler)
					} else {
						source = httptest.NewServer(handler)
					}
					defer source.Close()
					c := NewOidcAccessTokenClient(source.Client(), source.URL)
					c.SetAppCredentials("app", "seed-secret")
					var err error
					switch endpoint {
					case "app":
						_, err = c.getAppAccessToken(context.Background())
					case "exchange":
						_, err = c.CreateAccessToken(context.Background(), CreateAccessTokenParams{Code: "seed-code"})
					case "userinfo":
						_, err = c.GetUserInfo(context.Background(), "seed-token")
					case "refresh":
						_, err = c.RefreshAccessToken(context.Background(), RefreshAccessTokenParams{RefreshToken: "seed-refresh"})
					}
					if err == nil || second != 0 {
						t.Fatalf("redirect exposed secret-bearing request: second=%d err=%v", second, err)
					}
					var diagnostic interface{ HTTPStatus() int }
					if !errors.As(err, &diagnostic) || diagnostic.HTTPStatus() != status {
						t.Fatalf("redirect status lost: %v", err)
					}
				})
			}
		}
	}
}

func TestAuthenticationErrorPreservesSafeBodyLogIDWithoutRawDetail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"code":1770032,"msg":"seed-secret","error":{"log_id":"req-safe","token":"seed-secret"}}`)
	}))
	defer server.Close()
	_, err := NewOidcAccessTokenClient(server.Client(), server.URL).GetUserInfo(context.Background(), "seed-token")
	var diagnostic interface{ RequestID() string }
	if !errors.As(err, &diagnostic) || diagnostic.RequestID() != "req-safe" || strings.Contains(err.Error(), "seed-secret") {
		t.Fatalf("safe body diagnostic lost: %v", err)
	}
}
