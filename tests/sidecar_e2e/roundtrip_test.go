// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build authsidecar

// Package sidecar_e2e proves the sidecar auth-proxy wire protocol end-to-end,
// offline and secret-free: a real fork binary (built with -tags authsidecar,
// exercising the REAL extension/transport/sidecar interceptor) signs a
// request with HMAC-SHA256 and routes it to an in-test sidecar, which
// verifies the signature using the REAL sidecar.Verify / sidecar.CanonicalRequest
// from github.com/larksuite/cli/sidecar, injects a synthetic token, and
// forwards to an in-test mock upstream.
//
// DEVIATION FROM THE ORIGINAL PLAN: the plan called for driving the real
// sidecar/server-demo binary (built with -tags authsidecar_demo) as the
// middle process. That is infeasible for an OFFLINE test, for three
// independent reasons, all verified in source:
//
//  1. sidecar/server-demo/handler.go:171 resolves a REAL token via
//     h.cred.ResolveToken(...), which errors out unless the machine has run
//     `lark-cli auth login` — there is no way to make it return a token
//     without live credentials.
//  2. sidecar/server-demo/main.go builds handler.allowedHosts from
//     core.ResolveEndpoints(BrandFeishu/BrandLark) only — real feishu/lark
//     hosts. An in-test mock (127.0.0.1:<port>) is never in that allowlist
//     and would be rejected with 403 (handler.go step 4).
//  3. sidecar/server-demo/handler.go:184 pins the forward scheme to
//     "https://" + targetHost, ignoring the client-supplied scheme. It can
//     never be redirected to an http:// mock.
//
// server-demo's verify+inject logic is ALREADY covered by
// `go test -tags authsidecar_demo ./sidecar/server-demo/` (see the
// sidecar-test Makefile target, item 3) — that is unit-level coverage of the
// same code paths this file would otherwise exercise via a real subprocess.
//
// So instead, this test builds its OWN in-test sidecar (an httptest.Server)
// built on the real protocol package (sidecar.Verify, sidecar.CanonicalRequest,
// sidecar.BodySHA256, the Header* / Sentinel* / Identity* constants) — the
// same symbols server-demo itself uses. Against server-demo/handler.go's
// numbered steps, the coverage accounting is:
//
//   - steps 0-3 (protocol version, timestamp presence, body SHA256, HMAC
//     verification): MIRRORED in the in-test handler. Note server-demo's
//     step 1 checks timestamp presence only — no freshness/skew window
//     exists there either; the timestamp's integrity is covered by the HMAC.
//   - steps 4/5/5.5 (target-host / identity / auth-header allowlists): NOT
//     enforced in the handler (a mock's 127.0.0.1 host can never be in a
//     real allowlist); replaced by post-hoc test assertions that the docs
//     request named the real Feishu host, identity=user, and the committed
//     auth header was present.
//   - step 6 (resolve real token): replaced by a synthetic injected token —
//     the point of the offline design.
//   - steps 7-10 (build forward request, inject, forward, relay response):
//     mirrored in shape, except the forward goes to the in-test mock's URL
//     instead of "https://"+targetHost (deliberate, documented on
//     forwardWithInjectedToken).
//   - step 11 (audit log): not covered; irrelevant to the wire contract.
//
// This is the standard shape for this kind of test: one real external
// process (the fork binary, compiled with the production interceptor code)
// plus two in-process httptest.Server stand-ins (sidecar, upstream). It
// proves the real wire protocol end-to-end without requiring live
// credentials, real feishu/lark hosts, or TLS.
//
// Every key/token/app-id here is an obviously-synthetic placeholder; nothing
// in this file can authenticate against anything real.
package sidecar_e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/larksuite/cli/sidecar"
)

// Synthetic, obviously-fake fixtures. None of these are real secrets.
const (
	testProxyKey  = "test-proxy-key-not-a-real-secret-000000000000"
	testAppID     = "cli_test_app_not_real"
	injectedToken = "fake-injected-token-not-real"

	// testDocToken is the --doc argument runFork passes; the docs +fetch call
	// becomes POST /open-apis/docs_ai/v1/documents/<testDocToken>/fetch. Sharing
	// it keeps the request marker below in sync with the command invocation.
	testDocToken = "nonexistent"

	// docsReqPath is the exact path of the TARGET docs +fetch request among
	// every request the fork routes through the proxy. `docs +fetch --as user`
	// resolves a sentinel UAT, and the credential layer then verifies it with
	// a mandatory /open-apis/authen/v1/user_info probe (see
	// internal/credential/credential_provider.go enrichUserInfo) — so a second
	// request also flows through the sidecar. Asserting on whichever arrived
	// last would let that identity probe masquerade as the docs request; we
	// select the docs call by its full path (exact match, not a substring — a
	// wrong API prefix or version must not slip through).
	docsReqPath = "/open-apis/docs_ai/v1/documents/" + testDocToken + "/fetch"

	// wantProxyTargetHost is the real Feishu open-platform host the interceptor
	// must name as the proxy target for BRAND=feishu. The request is never
	// actually forwarded there (the in-test sidecar redirects to the mock); the
	// header only records where the fork BELIEVED it was going, and it is HMAC
	// signing input, so it must be exactly the real host.
	wantProxyTargetHost = "open.feishu.cn"
)

// TestSidecarHMACRoundTrip drives the whole wire protocol as three named
// steps so the flow is readable at a glance; each step's mechanics live in a
// dedicated helper below.
func TestSidecarHMACRoundTrip(t *testing.T) {
	// Two in-process stand-ins: the mock upstream (for open.feishu.cn) and the
	// in-test sidecar (server-demo's verify+inject, via the real protocol pkg).
	upstream := startMockUpstream(t)
	sc := startInTestSidecar(t, []byte(testProxyKey), upstream.URL)

	// One real external process: lark-cli built with -tags authsidecar, run
	// fully offline against the in-test sidecar.
	bin := buildAuthsidecarFork(t)
	res := runFork(t, bin, sc.URL)

	// Diagnostic dump (shown only on failure or -v): the full request set, so a
	// failure makes plain which requests flowed and which one the assertions
	// targeted, instead of guessing about the last-arriving request.
	for _, s := range sc.seenAll() {
		t.Logf("sidecar saw: %s %s target=%q identity=%q verifyRan=%v verifyErr=%v",
			s.req.method, s.req.path, s.req.headers.Get(sidecar.HeaderProxyTarget),
			s.req.headers.Get(sidecar.HeaderProxyIdentity), s.verifyRan, s.verifyErr)
	}
	for _, r := range upstream.sink.all() {
		t.Logf("upstream saw: %s %s auth=%q", r.method, r.path, r.headers.Get("Authorization"))
	}
	t.Logf("fork exit=%d\nstdout=%s\nstderr=%s", res.exit, res.stdout, res.stderr)

	// Assert the fork's command itself succeeded end-to-end, not just that some
	// bytes reached the sidecar.
	assertForkSucceeded(t, res)

	// Assert the three properties of a correct round trip, scoped to the DOCS
	// request (not an auxiliary identity probe).
	assertInterceptorSigned(t, sc)                  // (a)+(c) fork -> sidecar
	assertInjectedTokenReachedUpstream(t, upstream) // (b)     sidecar -> upstream
}

// --- request capture -------------------------------------------------------

// capturedRequest snapshots the parts of an *http.Request that matter for
// assertions, taken before the request (and its body reader) is consumed or
// goes out of scope.
type capturedRequest struct {
	method  string
	path    string
	headers http.Header
	body    []byte
}

// requestSink stores EVERY request a stub server saw, in arrival order,
// guarded so the httptest handler goroutine and the test goroutine can hand
// them over safely. Capturing all requests (not just the last) is what closes
// the false-green window: the fork may route more than one request through the
// proxy, and the target docs request is not guaranteed to be the last.
type requestSink struct {
	mu   sync.Mutex
	reqs []*capturedRequest
}

func (s *requestSink) capture(r *http.Request, body []byte) *capturedRequest {
	snap := &capturedRequest{
		method:  r.Method,
		path:    r.URL.RequestURI(),
		headers: r.Header.Clone(),
		body:    body,
	}
	s.mu.Lock()
	s.reqs = append(s.reqs, snap)
	s.mu.Unlock()
	return snap
}

func (s *requestSink) all() []*capturedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]*capturedRequest(nil), s.reqs...)
}

// find returns the first captured request whose path equals path, or nil.
func (s *requestSink) find(path string) *capturedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, r := range s.reqs {
		if r.path == path {
			return r
		}
	}
	return nil
}

// --- mock upstream (stands in for open.feishu.cn) --------------------------

type mockUpstream struct {
	*httptest.Server
	sink requestSink
}

func startMockUpstream(t *testing.T) *mockUpstream {
	t.Helper()
	m := &mockUpstream{}
	m.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		m.sink.capture(r, body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// Respond per path so each forwarded request parses as success: the
		// identity probe needs authen/v1/user_info's {data:{open_id,name}} to
		// resolve cleanly; the docs +fetch is satisfied by the generic code:0
		// envelope. A single canned body would make the identity probe error.
		_, _ = w.Write(mockResponseFor(r.URL.Path))
	}))
	t.Cleanup(m.Close)
	return m
}

// mockResponseFor returns a minimal success body matching the API the given
// path addresses. Unknown paths get a generic code:0 envelope.
func mockResponseFor(path string) []byte {
	if strings.Contains(path, "/authen/v1/user_info") {
		return []byte(`{"code":0,"msg":"success","data":{"open_id":"ou_mock","name":"mock user"}}`)
	}
	return []byte(`{"code":0,"msg":"success","data":{}}`)
}

// --- in-test sidecar (mirrors server-demo/handler.go verify+inject) --------

// sidecarSeen is one request the in-test sidecar received, together with the
// per-request verification outcome. Tracking this per request (not as a single
// last-write-wins field) lets assertions check the verification that belongs
// to the DOCS request specifically.
type sidecarSeen struct {
	req       *capturedRequest
	verifyRan bool
	verifyErr error
}

type inTestSidecar struct {
	*httptest.Server
	key         []byte
	upstreamURL string

	mu   sync.Mutex // guards seen
	seen []sidecarSeen
}

func startInTestSidecar(t *testing.T, key []byte, upstreamURL string) *inTestSidecar {
	t.Helper()
	s := &inTestSidecar{key: key, upstreamURL: upstreamURL}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	t.Cleanup(s.Close)
	return s
}

// handle is the request flow: capture -> verify (steps 0-3) -> inject+forward.
func (s *inTestSidecar) handle(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	snap := &capturedRequest{
		method:  r.Method,
		path:    r.URL.RequestURI(),
		headers: r.Header.Clone(),
		body:    body,
	}

	authHeader, verifyRan, verifyErr, ok := s.verifyProxyRequest(w, r, body)
	s.mu.Lock()
	s.seen = append(s.seen, sidecarSeen{req: snap, verifyRan: verifyRan, verifyErr: verifyErr})
	s.mu.Unlock()
	if !ok {
		return
	}
	s.forwardWithInjectedToken(w, r, body, authHeader)
}

// verifyProxyRequest mirrors server-demo/handler.go steps 0-3 (protocol
// version, timestamp presence, body SHA256, HMAC signature verification —
// including the target parse and identity/auth-header reads that feed the
// canonical signing string). The allowlist steps 4/5/5.5 are intentionally
// absent; the package comment's coverage accounting explains what replaces
// them. It returns the auth header the client committed to, whether HMAC
// verification (step 3) actually ran, and its result. On any earlier failure
// it writes the HTTP error and returns ok=false with verifyRan=false.
func (s *inTestSidecar) verifyProxyRequest(w http.ResponseWriter, r *http.Request, body []byte) (authHeader string, verifyRan bool, verifyErr error, ok bool) {
	// Step 0: protocol version.
	version := r.Header.Get(sidecar.HeaderProxyVersion)
	if version != sidecar.ProtocolV1 {
		http.Error(w, "unsupported "+sidecar.HeaderProxyVersion+": "+version, http.StatusBadRequest)
		return "", false, nil, false
	}

	// Step 1: timestamp presence (matching server-demo, which enforces
	// presence only — the value's integrity is covered by the HMAC below;
	// an empty-but-signed timestamp would otherwise verify fine).
	ts := r.Header.Get(sidecar.HeaderProxyTimestamp)
	if ts == "" {
		http.Error(w, "missing "+sidecar.HeaderProxyTimestamp, http.StatusBadRequest)
		return "", false, nil, false
	}

	// Step 2: body SHA256.
	claimedSHA := r.Header.Get(sidecar.HeaderBodySHA256)
	if claimedSHA == "" || claimedSHA != sidecar.BodySHA256(body) {
		http.Error(w, "body SHA256 mismatch", http.StatusBadRequest)
		return "", false, nil, false
	}

	// Step 3 inputs: target host, identity, auth-header (all covered by the sig).
	targetHost, perr := parseTargetHost(r.Header.Get(sidecar.HeaderProxyTarget))
	if perr != nil {
		http.Error(w, "invalid "+sidecar.HeaderProxyTarget+": "+perr.Error(), http.StatusForbidden)
		// verifyRan=false: HMAC verification never ran; surface the parse error
		// so the diagnostic dump shows why this request was rejected early.
		return "", false, perr, false
	}
	identity := r.Header.Get(sidecar.HeaderProxyIdentity)
	authHeader = r.Header.Get(sidecar.HeaderProxyAuthHeader)

	// Step 3: verify HMAC signature over the canonical request.
	err := sidecar.Verify(s.key, sidecar.CanonicalRequest{
		Version:      version,
		Method:       r.Method,
		Host:         targetHost,
		PathAndQuery: r.URL.RequestURI(),
		BodySHA256:   claimedSHA,
		Timestamp:    ts,
		Identity:     identity,
		AuthHeader:   authHeader,
	}, r.Header.Get(sidecar.HeaderProxySignature))
	if err != nil {
		http.Error(w, "HMAC verification failed: "+err.Error(), http.StatusUnauthorized)
		return "", true, err, false
	}
	return authHeader, true, nil, true
}

// forwardWithInjectedToken mirrors server-demo's inject+forward. Unlike
// server-demo (which forwards to "https://"+targetHost), this test forwards to
// the in-test MOCK's URL — proving the sidecar's inject step without needing a
// real upstream or a route to targetHost. It strips any client-supplied auth
// headers first (the sidecar is the sole source of auth material), injects the
// synthetic token into the committed header, and relays the response back.
func (s *inTestSidecar) forwardWithInjectedToken(w http.ResponseWriter, r *http.Request, body []byte, authHeader string) {
	freq, err := http.NewRequest(r.Method, s.upstreamURL+r.URL.RequestURI(), bytes.NewReader(body))
	if err != nil {
		http.Error(w, "failed to build forward request", http.StatusInternalServerError)
		return
	}
	for k, vs := range r.Header {
		if isProxyHeader(k) {
			continue
		}
		for _, v := range vs {
			freq.Header.Add(k, v)
		}
	}
	freq.Header.Del("Authorization")
	freq.Header.Del(sidecar.HeaderMCPUAT)
	freq.Header.Del(sidecar.HeaderMCPTAT)

	if authHeader == "Authorization" {
		freq.Header.Set("Authorization", "Bearer "+injectedToken)
	} else {
		freq.Header.Set(authHeader, injectedToken)
	}

	resp, err := http.DefaultClient.Do(freq)
	if err != nil {
		http.Error(w, "forward failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

// seenAll returns a copy of every request the sidecar received, in order.
func (s *inTestSidecar) seenAll() []sidecarSeen {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]sidecarSeen(nil), s.seen...)
}

// findSeen returns the first received request whose path equals path, or nil.
func (s *inTestSidecar) findSeen(path string) *sidecarSeen {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.seen {
		if s.seen[i].req.path == path {
			return &s.seen[i]
		}
	}
	return nil
}

// isProxyHeader reports whether name is one of the sidecar wire-protocol
// headers that must not be copied through to the forwarded (mock upstream)
// request. Mirrors sidecar/server-demo/handler.go's isProxyHeader.
func isProxyHeader(name string) bool {
	switch http.CanonicalHeaderKey(name) {
	case http.CanonicalHeaderKey(sidecar.HeaderProxyVersion),
		http.CanonicalHeaderKey(sidecar.HeaderProxyTarget),
		http.CanonicalHeaderKey(sidecar.HeaderProxyIdentity),
		http.CanonicalHeaderKey(sidecar.HeaderProxySignature),
		http.CanonicalHeaderKey(sidecar.HeaderProxyTimestamp),
		http.CanonicalHeaderKey(sidecar.HeaderBodySHA256),
		http.CanonicalHeaderKey(sidecar.HeaderProxyAuthHeader):
		return true
	}
	return false
}

// parseTargetHost validates X-Lark-Proxy-Target and returns its host.
// Mirrors sidecar/server-demo/handler.go's parseTarget: the header must be
// "https://<host>" with no path, query, fragment, or userinfo. Only the host
// is used, both as HMAC signing input and to record what the fork believed
// its real destination was — the actual forward in this test always goes to
// the in-test mock, never to this host.
func parseTargetHost(target string) (string, error) {
	u, err := url.Parse(target)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}
	if u.Scheme != "https" {
		return "", fmt.Errorf("scheme must be https, got %q", u.Scheme)
	}
	if u.Host == "" {
		return "", fmt.Errorf("missing host")
	}
	if u.User != nil {
		return "", fmt.Errorf("userinfo not allowed")
	}
	if u.Path != "" && u.Path != "/" {
		return "", fmt.Errorf("path not allowed (got %q)", u.Path)
	}
	if u.RawQuery != "" {
		return "", fmt.Errorf("query not allowed")
	}
	if u.Fragment != "" {
		return "", fmt.Errorf("fragment not allowed")
	}
	return u.Host, nil
}

// --- fork build + run ------------------------------------------------------

// buildAuthsidecarFork builds the REAL lark-cli with -tags authsidecar (the
// production interceptor) and returns the binary path.
func buildAuthsidecarFork(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "forkbin")
	build := exec.Command("go", "build", "-tags", "authsidecar", "-o", bin, ".")
	build.Dir = repoRoot(t)
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fork binary: %v\n%s", err, out)
	}
	return bin
}

// forkResult is the fork subprocess outcome.
type forkResult struct {
	exit   int
	stdout string
	stderr string
}

// runFork runs the fork against the in-test sidecar, fully offline, and returns
// its exit code and captured output. LARKSUITE_CLI_REMOTE_META=off is essential:
// without it the fork's startup metadata refresh hits the real
// open.feishu.cn/api/tools/open/api_definition (internal/registry/remote.go),
// which both breaks the "offline, secret-free" contract and makes the run
// depend on live network. With it set, the command still completes and the
// docs request still flows through the sidecar, but nothing leaves the machine.
func runFork(t *testing.T, binPath, sidecarURL string) forkResult {
	t.Helper()
	scURL, err := url.Parse(sidecarURL)
	if err != nil {
		t.Fatalf("parse sidecar URL: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binPath, "docs", "+fetch", "--doc", testDocToken, "--as", "user")
	// Strip the host's LARKSUITE_CLI_* namespace before appending overrides: a
	// developer machine exporting, say, LARKSUITE_CLI_DEFAULT_AS or
	// LARKSUITE_CLI_STRICT_MODE would otherwise leak into the fork (the sidecar
	// credential provider reads them via os.Getenv), so the fork's CLI-facing
	// environment is exactly the variables set below, on any machine.
	env := os.Environ()
	base := env[:0]
	for _, kv := range env {
		if !strings.HasPrefix(kv, "LARKSUITE_CLI_") {
			base = append(base, kv)
		}
	}
	cmd.Env = append(base,
		"LARKSUITE_CLI_AUTH_PROXY=http://"+scURL.Host,
		"LARKSUITE_CLI_PROXY_KEY="+testProxyKey,
		"LARKSUITE_CLI_APP_ID="+testAppID,
		"LARKSUITE_CLI_BRAND=feishu",
		"LARKSUITE_CLI_CONFIG_DIR="+t.TempDir(),
		"LARKSUITE_CLI_REMOTE_META=off",
		"LARKSUITE_CLI_NO_UPDATE_NOTIFIER=1",
		"LARKSUITE_CLI_NO_SKILLS_NOTIFIER=1",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	runErr := cmd.Run()
	exit := 0
	if runErr != nil {
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			exit = ee.ExitCode()
		} else {
			t.Fatalf("run fork: %v", runErr)
		}
	}
	return forkResult{exit: exit, stdout: stdout.String(), stderr: stderr.String()}
}

// repoRoot resolves the lark-cli module root from the test's working
// directory (which `go test` sets to the package dir, tests/sidecar_e2e).
func repoRoot(t *testing.T) string {
	t.Helper()
	out, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Fatalf("resolve repo root: %v", err)
	}
	return strings.TrimSpace(string(out))
}

// --- assertions ------------------------------------------------------------

// assertForkSucceeded checks the fork command itself completed the round trip:
// exit 0 and an ok:true JSON envelope on stdout. This is what makes the docs
// request a genuine success path, not merely bytes that happened to flow.
func assertForkSucceeded(t *testing.T, res forkResult) {
	t.Helper()
	if res.exit != 0 {
		t.Fatalf("fork exit=%d want 0; stdout=%s stderr=%s", res.exit, res.stdout, res.stderr)
	}
	// Parse rather than substring-match: the CLI pretty-prints stdout, so the
	// envelope reads "ok": true (with a space), and the field's truth — not its
	// serialized spelling — is what proves the round trip succeeded.
	var env struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal([]byte(res.stdout), &env); err != nil {
		t.Fatalf("fork stdout is not a JSON envelope: %v; stdout=%s stderr=%s", err, res.stdout, res.stderr)
	}
	if !env.OK {
		t.Fatalf("fork stdout ok != true (round trip did not succeed); stdout=%s stderr=%s", res.stdout, res.stderr)
	}
}

// assertInterceptorSigned checks the fork -> sidecar hop (assertions a + c) for
// the DOCS request specifically: the real interceptor ran (all proxy headers
// present, identity=user, method+path+target as expected), stripped every
// real/sentinel auth header before signing, and produced a signature that
// verified against the shared key.
func assertInterceptorSigned(t *testing.T, sc *inTestSidecar) {
	t.Helper()
	seen := sc.findSeen(docsReqPath)
	if seen == nil {
		t.Fatalf("sidecar never received the docs request (path %q) — interceptor did not route it to AUTH_PROXY; saw %v",
			docsReqPath, sidecarPaths(sc.seenAll()))
	}
	got := seen.req
	if !seen.verifyRan {
		t.Fatal("sidecar received the docs request but never reached HMAC verification (rejected earlier — see handler headers)")
	}
	if seen.verifyErr != nil {
		t.Fatalf("HMAC verification failed on the fork's own signed docs request: %v", seen.verifyErr)
	}
	t.Logf("fork->sidecar docs headers: %v", got.headers)

	// Target/method/path: prove we asserted on the real docs call to the real
	// Feishu open platform, not an auxiliary request.
	if got.method != http.MethodPost {
		t.Errorf("docs request method = %q, want POST", got.method)
	}
	if targetHost, err := parseTargetHost(got.headers.Get(sidecar.HeaderProxyTarget)); err != nil {
		t.Errorf("docs request %s invalid: %v", sidecar.HeaderProxyTarget, err)
	} else if targetHost != wantProxyTargetHost {
		t.Errorf("docs request proxy target host = %q, want %q", targetHost, wantProxyTargetHost)
	}

	// No real/sentinel auth ever left the fork: the interceptor strips the
	// sentinel before signing, so this hop must carry no auth header at all.
	if auth := got.headers.Get("Authorization"); auth != "" {
		t.Fatalf("fork->sidecar hop leaked an Authorization header (want none, interceptor should have stripped it): %q", auth)
	}
	if v := got.headers.Get(sidecar.HeaderMCPUAT); v != "" {
		t.Fatalf("fork->sidecar hop leaked %s (want none): %q", sidecar.HeaderMCPUAT, v)
	}
	if v := got.headers.Get(sidecar.HeaderMCPTAT); v != "" {
		t.Fatalf("fork->sidecar hop leaked %s (want none): %q", sidecar.HeaderMCPTAT, v)
	}

	// Proxy headers must be present (proves the interceptor actually ran).
	for _, h := range []string{
		sidecar.HeaderProxyVersion, sidecar.HeaderProxyTarget, sidecar.HeaderProxyIdentity,
		sidecar.HeaderProxySignature, sidecar.HeaderProxyTimestamp, sidecar.HeaderBodySHA256,
		sidecar.HeaderProxyAuthHeader,
	} {
		if got.headers.Get(h) == "" {
			t.Fatalf("fork->sidecar hop missing required proxy header %s", h)
		}
	}
	if id := got.headers.Get(sidecar.HeaderProxyIdentity); id != sidecar.IdentityUser {
		t.Fatalf("fork->sidecar identity = %q, want %q", id, sidecar.IdentityUser)
	}
}

// assertInjectedTokenReachedUpstream checks the sidecar -> upstream hop
// (assertion b) for the DOCS request: the mock saw exactly the sidecar-injected
// synthetic token, never a sentinel or a real one — proving injection happened.
func assertInjectedTokenReachedUpstream(t *testing.T, up *mockUpstream) {
	t.Helper()
	got := up.sink.find(docsReqPath)
	if got == nil {
		t.Fatalf("mock upstream never received the forwarded docs request (path %q) — sidecar did not forward it after verification; saw %v",
			docsReqPath, requestPaths(up.sink.all()))
	}
	t.Logf("sidecar->mock docs headers: %v", got.headers)

	wantAuth := "Bearer " + injectedToken
	gotAuth := got.headers.Get("Authorization")
	if gotAuth != wantAuth {
		t.Fatalf("mock upstream Authorization = %q, want %q", gotAuth, wantAuth)
	}
	// Belt-and-suspenders: the value the mock saw must not be either sentinel,
	// proving the only token that ever reached "upstream" was the injected one.
	if gotAuth == "Bearer "+sidecar.SentinelUAT || gotAuth == "Bearer "+sidecar.SentinelTAT {
		t.Fatalf("mock upstream received a sentinel token instead of the injected one: %q", gotAuth)
	}
	// The sidecar wire-protocol headers are between fork and sidecar only —
	// the forward must strip every one of them. Token injection alone passing
	// would still be a leak if signatures/timestamps/digests reached upstream.
	for _, h := range []string{
		sidecar.HeaderProxyVersion, sidecar.HeaderProxyTarget, sidecar.HeaderProxyIdentity,
		sidecar.HeaderProxySignature, sidecar.HeaderProxyTimestamp, sidecar.HeaderBodySHA256,
		sidecar.HeaderProxyAuthHeader,
	} {
		if v := got.headers.Get(h); v != "" {
			t.Errorf("proxy protocol header %s leaked to upstream (want stripped): %q", h, v)
		}
	}
}

// sidecarPaths / requestPaths render captured paths for failure messages.
func sidecarPaths(seen []sidecarSeen) []string {
	paths := make([]string, len(seen))
	for i, s := range seen {
		paths[i] = s.req.method + " " + s.req.path
	}
	return paths
}

func requestPaths(reqs []*capturedRequest) []string {
	paths := make([]string, len(reqs))
	for i, r := range reqs {
		paths[i] = r.method + " " + r.path
	}
	return paths
}
