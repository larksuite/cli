// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sidecar

import (
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestBodySHA256_Empty(t *testing.T) {
	// SHA-256 of empty string is a well-known constant.
	want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := BodySHA256(nil); got != want {
		t.Errorf("BodySHA256(nil) = %q, want %q", got, want)
	}
	if got := BodySHA256([]byte{}); got != want {
		t.Errorf("BodySHA256([]byte{}) = %q, want %q", got, want)
	}
}

func TestBodySHA256_NonEmpty(t *testing.T) {
	got := BodySHA256([]byte(`{"key":"value"}`))
	if len(got) != 64 {
		t.Errorf("expected 64-char hex string, got %d chars", len(got))
	}
}

// canonical is a test helper that builds a fully-populated CanonicalRequest
// with reasonable defaults, so individual tests can override just the field
// they want to tamper with.
func canonical(override func(*CanonicalRequest)) CanonicalRequest {
	c := CanonicalRequest{
		Version:      ProtocolV1,
		Method:       "POST",
		Host:         "open.feishu.cn",
		PathAndQuery: "/open-apis/im/v1/messages?receive_id_type=chat_id",
		BodySHA256:   BodySHA256([]byte(`{"content":"hello"}`)),
		Timestamp:    Timestamp(),
		Identity:     IdentityUser,
		AuthHeader:   "Authorization",
	}
	if override != nil {
		override(&c)
	}
	return c
}

func TestSignAndVerify(t *testing.T) {
	key := []byte("test-secret-key-32bytes-long!!!!!")
	req := canonical(nil)

	sig := Sign(key, req)
	if len(sig) != 64 {
		t.Fatalf("signature should be 64-char hex, got %d chars", len(sig))
	}

	// Valid verification
	if err := Verify(key, req, sig); err != nil {
		t.Fatalf("Verify failed for valid signature: %v", err)
	}

	// Wrong key
	if err := Verify([]byte("wrong-key"), req, sig); err == nil {
		t.Error("Verify should fail with wrong key")
	}

	// Each field must be covered by the signature — tampering with any one
	// invalidates it.
	fields := map[string]func(*CanonicalRequest){
		"version":      func(c *CanonicalRequest) { c.Version = "v2" },
		"method":       func(c *CanonicalRequest) { c.Method = "GET" },
		"host":         func(c *CanonicalRequest) { c.Host = "evil.com" },
		"pathAndQuery": func(c *CanonicalRequest) { c.PathAndQuery = "/steal" },
		"bodySHA256":   func(c *CanonicalRequest) { c.BodySHA256 = BodySHA256([]byte("tampered")) },
		"identity":     func(c *CanonicalRequest) { c.Identity = IdentityBot },
		"authHeader":   func(c *CanonicalRequest) { c.AuthHeader = "Cookie" },
	}
	for name, mutate := range fields {
		t.Run("tamper_"+name, func(t *testing.T) {
			tampered := canonical(mutate)
			if err := Verify(key, tampered, sig); err == nil {
				t.Errorf("Verify should fail when %s is tampered", name)
			}
		})
	}
}

// TestVerify_PrivilegeConfusion proves C1: without identity and authHeader in
// the canonical string, an attacker holding a captured user-signed request
// could replay it as bot (or vice versa) by flipping the header. With both
// fields now covered, such a flip must invalidate the signature.
func TestVerify_PrivilegeConfusion(t *testing.T) {
	key := []byte("test-key")
	signed := canonical(func(c *CanonicalRequest) { c.Identity = IdentityUser })
	sig := Sign(key, signed)

	replayed := signed
	replayed.Identity = IdentityBot // attacker flips identity
	if err := Verify(key, replayed, sig); err == nil {
		t.Error("identity flip must invalidate signature")
	}

	replayed = signed
	replayed.AuthHeader = "Cookie" // attacker redirects injection target
	if err := Verify(key, replayed, sig); err == nil {
		t.Error("auth-header flip must invalidate signature")
	}
}

func TestVerify_TimestampDrift(t *testing.T) {
	key := []byte("test-key")

	// Timestamp too old
	oldTs := strconv.FormatInt(time.Now().Unix()-MaxTimestampDrift-10, 10)
	oldReq := canonical(func(c *CanonicalRequest) { c.Timestamp = oldTs })
	sig := Sign(key, oldReq)
	if err := Verify(key, oldReq, sig); err == nil {
		t.Error("Verify should reject expired timestamp")
	}

	// Timestamp too far in future
	futureTs := strconv.FormatInt(time.Now().Unix()+MaxTimestampDrift+10, 10)
	futureReq := canonical(func(c *CanonicalRequest) { c.Timestamp = futureTs })
	sig = Sign(key, futureReq)
	if err := Verify(key, futureReq, sig); err == nil {
		t.Error("Verify should reject future timestamp")
	}

	// Invalid timestamp
	badTs := canonical(func(c *CanonicalRequest) { c.Timestamp = "not-a-number" })
	if err := Verify(key, badTs, "sig"); err == nil {
		t.Error("Verify should reject invalid timestamp")
	}
}

func TestSignDeterministic(t *testing.T) {
	key := []byte("key")
	req := canonical(func(c *CanonicalRequest) { c.Timestamp = "12345" })
	a, b := Sign(key, req), Sign(key, req)
	if a != b {
		t.Errorf("Sign should be deterministic: %q vs %q", a, b)
	}
}

func TestValidateProxyAddr(t *testing.T) {
	valid := []string{
		// loopback IPs
		"http://127.0.0.1:16384",
		"127.0.0.1:16384",
		"[::1]:16384",
		"http://[::1]:16384",
		// recognized same-host aliases
		"http://localhost:8080",
		"localhost:8080",
		"http://host.docker.internal:16384",
		"http://host.containers.internal:16384",
		"http://host.lima.internal:16384",
		"http://gateway.docker.internal:16384",
		// trailing slash is tolerated
		"http://127.0.0.1:8080/",
		// remote managed auth proxy
		"https://auth-proxy.example.com",
		"https://auth-proxy.example.com:443",
	}
	for _, addr := range valid {
		if err := ValidateProxyAddr(addr); err != nil {
			t.Errorf("ValidateProxyAddr(%q) unexpected error: %v", addr, err)
		}
	}

	invalid := []string{
		"",
		"foobar",
		"ftp://127.0.0.1:16384",
		"http://",
		"http://:16384",
		"http://127.0.0.1:16384/some/path",
		"http://127.0.0.1:16384?debug=true",
		"http://127.0.0.1:16384#debug",
		"https://:443",
		"https://auth-proxy.example.com:",
		"https://auth-proxy.example.com:99999",
		"https://open.feishu.cn",
		"https://auth-proxy.example.com/some/path",
		"https://auth-proxy.example.com?debug=true",
		"https://auth-proxy.example.com#debug",
		":16384",
	}
	for _, addr := range invalid {
		if err := ValidateProxyAddr(addr); err == nil {
			t.Errorf("ValidateProxyAddr(%q) expected error, got nil", addr)
		}
	}
}

// TestValidateProxyAddr_HostConstraint pins C2: the sidecar pattern is
// same-machine by definition, so the validator rejects any host that isn't
// loopback or a recognized same-host alias. Tampered /etc/hosts is out of
// scope (attacker already has ambient host access).
func TestValidateProxyAddr_HostConstraint(t *testing.T) {
	sameHost := []string{
		"http://127.0.0.1:16384",
		"http://localhost:8080",
		"http://host.docker.internal:16384",
		"http://host.containers.internal:16384",
		"http://host.lima.internal:16384",
		"http://gateway.docker.internal:16384",
		"http://[::1]:16384",
		// bare form
		"127.0.0.1:16384",
		"localhost:8080",
		"host.docker.internal:16384",
	}
	for _, addr := range sameHost {
		if err := ValidateProxyAddr(addr); err != nil {
			t.Errorf("expected %q to pass as same-host, got: %v", addr, err)
		}
	}

	notSameHost := map[string]string{
		// The interesting ones — plausible misconfigurations / attacks
		"public DNS name":            "http://attacker.com:8080",
		"cloud metadata IMDS":        "http://169.254.169.254",
		"private RFC1918":            "http://10.0.0.1:16384",
		"other RFC1918":              "http://192.168.1.2:16384",
		"link-local IPv4":            "http://169.254.1.1:16384",
		"unspecified IPv4 (0.0.0.0)": "http://0.0.0.0:16384",
		"bare public IP":             "http://8.8.8.8:16384",
		"bare RFC1918":               "10.0.0.1:16384",
	}
	for name, addr := range notSameHost {
		t.Run(name, func(t *testing.T) {
			err := ValidateProxyAddr(addr)
			if err == nil {
				t.Fatalf("expected rejection for %q", addr)
			}
			// Error must name the constraint so users know why.
			msg := err.Error()
			if !strings.Contains(msg, "loopback") && !strings.Contains(msg, "same-host") {
				t.Errorf("error should explain same-host requirement, got: %v", err)
			}
		})
	}
}

// TestValidateProxyAddr_RejectsUserinfo closes the URL-phishing vector
// http://127.0.0.1@attacker.com (where "127.0.0.1" is actually basic-auth
// userinfo and the real host is attacker.com). userinfo has no legitimate
// use in the sidecar protocol.
func TestValidateProxyAddr_RejectsUserinfo(t *testing.T) {
	for _, addr := range []string{
		"http://user@127.0.0.1:16384",
		"http://user:pass@127.0.0.1:16384",
		"http://127.0.0.1@attacker.com:16384",
	} {
		err := ValidateProxyAddr(addr)
		if err == nil {
			t.Errorf("ValidateProxyAddr(%q): expected rejection, got nil", addr)
			continue
		}
		// Either "userinfo" (for addresses parsed with user) or the same-host
		// message (for e.g. http://127.0.0.1@attacker.com where the REAL
		// host parses as attacker.com) is acceptable — both reject the
		// phishing attempt.
		msg := err.Error()
		if !strings.Contains(msg, "userinfo") && !strings.Contains(msg, "same-host") && !strings.Contains(msg, "loopback") {
			t.Errorf("error should reject userinfo or flag wrong host, got: %v", err)
		}
	}
}

func TestValidateProxyAddr_RemoteHTTPSProxy(t *testing.T) {
	for _, addr := range []string{
		"https://auth-proxy.example.com",
		"https://auth-proxy.example.com:443",
		"https://10.0.0.10:8443",
	} {
		if err := ValidateProxyAddr(addr); err != nil {
			t.Errorf("ValidateProxyAddr(%q): unexpected error: %v", addr, err)
		}
	}
}

func TestValidateProxyAddr_RejectsRemoteHTTP(t *testing.T) {
	err := ValidateProxyAddr("http://auth-proxy.example.com:8080")
	if err == nil {
		t.Fatal("expected remote HTTP proxy to be rejected")
	}
	if msg := err.Error(); !strings.Contains(msg, "loopback") && !strings.Contains(msg, "same-host") {
		t.Errorf("error should explain same-host requirement for HTTP, got: %v", err)
	}
}

func TestParseProxyEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		addr       string
		wantMode   ProxyMode
		wantScheme string
		wantHost   string
	}{
		{
			name:       "bare local address",
			addr:       "127.0.0.1:16384",
			wantMode:   ProxyModeLocal,
			wantScheme: "http",
			wantHost:   "127.0.0.1:16384",
		},
		{
			name:       "local HTTP URL",
			addr:       "http://localhost:16384/",
			wantMode:   ProxyModeLocal,
			wantScheme: "http",
			wantHost:   "localhost:16384",
		},
		{
			name:       "remote HTTPS URL",
			addr:       "https://auth-proxy.example.com",
			wantMode:   ProxyModeRemote,
			wantScheme: "https",
			wantHost:   "auth-proxy.example.com",
		},
		{
			name:       "remote HTTPS URL with port",
			addr:       "https://auth-proxy.example.com:8443",
			wantMode:   ProxyModeRemote,
			wantScheme: "https",
			wantHost:   "auth-proxy.example.com:8443",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseProxyEndpoint(tt.addr)
			if err != nil {
				t.Fatalf("ParseProxyEndpoint(%q) unexpected error: %v", tt.addr, err)
			}
			if got.Mode != tt.wantMode {
				t.Errorf("Mode = %q, want %q", got.Mode, tt.wantMode)
			}
			if got.Scheme != tt.wantScheme {
				t.Errorf("Scheme = %q, want %q", got.Scheme, tt.wantScheme)
			}
			if got.Host != tt.wantHost {
				t.Errorf("Host = %q, want %q", got.Host, tt.wantHost)
			}
		})
	}
}

func TestNormalizeRemoteProxyTrustHost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"https://auth-proxy.example.com", "auth-proxy.example.com"},
		{"https://AUTH-PROXY.example.com:443", "auth-proxy.example.com"},
		{"auth-proxy.example.com", "auth-proxy.example.com"},
		{"auth-proxy.example.com:8443", "auth-proxy.example.com:8443"},
		{"https://[::1]:8443", "[::1]:8443"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := NormalizeRemoteProxyTrustHost(tt.input)
			if err != nil {
				t.Fatalf("NormalizeRemoteProxyTrustHost(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeRemoteProxyTrustHost(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNormalizeRemoteProxyTrustHostRejectsInvalidInput(t *testing.T) {
	for _, input := range []string{
		"",
		"http://auth-proxy.example.com",
		"https://",
		"https://:443",
		"https://auth-proxy.example.com/path",
		"https://auth-proxy.example.com?debug=true",
		"https://auth-proxy.example.com#debug",
		"https://user@auth-proxy.example.com",
		"https://auth-proxy.example.com:99999",
	} {
		t.Run(input, func(t *testing.T) {
			if _, err := NormalizeRemoteProxyTrustHost(input); err == nil {
				t.Fatalf("expected NormalizeRemoteProxyTrustHost(%q) to fail", input)
			}
		})
	}
}

func TestIsTrustedRemoteProxyHost(t *testing.T) {
	trusted := []string{
		"https://auth-proxy.example.com",
		"gate.example.com:8443",
	}
	for _, host := range []string{
		"auth-proxy.example.com",
		"AUTH-PROXY.EXAMPLE.COM:443",
		"gate.example.com:8443",
	} {
		t.Run("trusted_"+host, func(t *testing.T) {
			if !IsTrustedRemoteProxyHost(host, trusted) {
				t.Fatalf("expected %q to match trusted hosts %v", host, trusted)
			}
		})
	}
	for _, host := range []string{
		"evil.example.com",
		"auth-proxy.example.com:8443",
		"gate.example.com",
	} {
		t.Run("untrusted_"+host, func(t *testing.T) {
			if IsTrustedRemoteProxyHost(host, trusted) {
				t.Fatalf("expected %q not to match trusted hosts %v", host, trusted)
			}
		})
	}
}

func TestParseProxyTarget(t *testing.T) {
	tests := []struct {
		target string
		want   string
	}{
		{"https://open.feishu.cn", "open.feishu.cn"},
		{"https://open.feishu.cn:443", "open.feishu.cn:443"},
		{"https://open.larksuite.com", "open.larksuite.com"},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			got, err := ParseProxyTarget(tt.target)
			if err != nil {
				t.Fatalf("ParseProxyTarget(%q) unexpected error: %v", tt.target, err)
			}
			if got != tt.want {
				t.Fatalf("ParseProxyTarget(%q) = %q, want %q", tt.target, got, tt.want)
			}
		})
	}
}

func TestParseProxyTargetRejectsUnsafeTargets(t *testing.T) {
	for _, target := range []string{
		"",
		"http://open.feishu.cn",
		"https://",
		"https://:443",
		"https://open.feishu.cn:",
		"https://open.feishu.cn:99999",
		"https://open.feishu.cn/path",
		"https://open.feishu.cn?tenant=abc",
		"https://open.feishu.cn#frag",
		"https://user@open.feishu.cn",
	} {
		t.Run(target, func(t *testing.T) {
			if _, err := ParseProxyTarget(target); err == nil {
				t.Fatalf("expected ParseProxyTarget(%q) to fail", target)
			}
		})
	}
}

func TestProxyHost(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"http://127.0.0.1:16384", "127.0.0.1:16384"},
		{"http://0.0.0.0:8080", "0.0.0.0:8080"},
		{"http://host.docker.internal:16384/", "host.docker.internal:16384"},
		{"https://auth-proxy.example.com:8443", "auth-proxy.example.com:8443"},
		{"127.0.0.1:16384", "127.0.0.1:16384"}, // no scheme
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := ProxyHost(tt.input); got != tt.want {
				t.Errorf("ProxyHost(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
