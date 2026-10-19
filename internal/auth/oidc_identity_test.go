package auth

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestOIDCFlowBindsOnlyTrustedUserInfo(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	callback := "http://" + listener.Addr().String() + "/callback"
	listener.Close()
	idToken := "e30." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"untrusted-sub","iss":"wrong-issuer","aud":"wrong-app"}`)) + ".signature"
	userCalls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open-apis/auth/v3/app_access_token/internal":
			fmt.Fprint(w, `{"code":0,"app_access_token":"app","app_access_token_expire":7200}`)
		case "/open-apis/authen/v1/oidc/access_token":
			fmt.Fprintf(w, `{"code":0,"data":{"access_token":"access","expires_in":7200,"id_token":%q}}`, idToken)
		case "/open-apis/authen/v1/user_info":
			userCalls++
			fmt.Fprint(w, `{"code":0,"data":{"open_id":"trusted-open-id","name":"Trusted"}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer s.Close()
	opts := &OIDCFlowOptions{AuthProtocol: "legacy_oidc", AppID: "app", AppSecret: "secret", Domain: s.URL, OpenDomain: s.URL, RedirectURI: callback, Timeout: time.Second,
		OpenBrowser: func(authURL string) error {
			u, _ := url.Parse(authURL)
			if u.Path != "/open-apis/authen/v1/index" {
				t.Errorf("legacy identity flow used wrong authorization path: %s", u.Path)
			}
			_, err := http.Get(callback + "?state=" + url.QueryEscape(u.Query().Get("state")) + "&code=code")
			return err
		},
	}
	result, err := StartOIDCFlow(context.Background(), opts, s.Client(), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if result.OpenID != "trusted-open-id" || userCalls != 1 || result.AuthProtocol != "legacy_oidc" || result.Authority != s.URL {
		t.Fatalf("bound untrusted identity: %#v calls=%d", result, userCalls)
	}
}

func TestOIDCCallbackRejectsErrorBeforeStateWithoutReflection(t *testing.T) {
	listener, _ := net.Listen("tcp", "127.0.0.1:0")
	callback := "http://" + listener.Addr().String() + "/callback"
	listener.Close()
	var logs strings.Builder
	opts := &OIDCFlowOptions{Domain: "https://tenant.invalid", RedirectURI: callback, Timeout: time.Second, OpenBrowser: func(_ string) error {
		resp, err := http.Get(callback + "?state=seed-wrong-state&error=seed-error&error_description=seed-secret")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode != http.StatusForbidden {
				t.Errorf("wrong state status %d", resp.StatusCode)
			}
		}
		return err
	}}
	_, err := StartOIDCFlow(context.Background(), opts, http.DefaultClient, &logs)
	if err == nil {
		t.Fatal("wrong state accepted")
	}
	if strings.Contains(logs.String()+err.Error(), "seed-") {
		t.Fatal("callback reflected query secrets")
	}
}

func TestLegacyAuthorizationUsesIndexWithoutModernParameters(t *testing.T) {
	u, err := BuildOIDCAuthorizationURL(&OIDCFlowOptions{Domain: "https://login.tenant.test/base/", AuthProtocol: "legacy_oidc", AppID: "app", RedirectURI: "http://127.0.0.1:3000/callback", Scope: "must-not-send"}, "state")
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(u)
	if parsed.Path != "/base/open-apis/authen/v1/index" || parsed.Query().Get("app_id") != "app" || parsed.Query().Get("state") != "state" || len(parsed.Query()) != 3 {
		t.Fatalf("wrong legacy URL: %s", u)
	}
}
