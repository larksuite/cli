package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/core"
)

type failingCredentials struct{ err error }

func (f failingCredentials) Get(string, string) (string, error) { return "", f.err }
func (f failingCredentials) Set(string, string, string) error   { return f.err }
func (f failingCredentials) Remove(string, string) error        { return f.err }
func TestCredentialReadFailureIsNotMissingAuthorization(t *testing.T) {
	failure := errors.New("storage_locked")
	store := NewCredentialStore(failingCredentials{failure}, t.TempDir())
	_, err := store.Load(context.Background(), "app", "user")
	if !errors.Is(err, failure) {
		t.Fatalf("storage failure hidden: %v", err)
	}
}

type memoryCredentials struct {
	mu     sync.Mutex
	values map[string]string
	writes int
}

func (m *memoryCredentials) Get(s, a string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.values[s+":"+a], nil
}
func (m *memoryCredentials) Set(s, a, v string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[s+":"+a] = v
	m.writes++
	return nil
}
func (m *memoryCredentials) Remove(s, a string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, s+":"+a)
	return nil
}
func TestCredentialMutationSerializesAcrossStoreInstances(t *testing.T) {
	backend := &memoryCredentials{values: map[string]string{}}
	namespace := t.TempDir()
	a, b := NewCredentialStore(backend, namespace), NewCredentialStore(backend, namespace)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			store := a
			if i%2 == 0 {
				store = b
			}
			if err := store.Save(context.Background(), &StoredUAToken{AppId: "app", UserOpenId: "user", AccessToken: "access"}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	got, err := a.Load(context.Background(), "app", "user")
	if err != nil || got.Generation != 20 {
		t.Fatalf("lost concurrent update: %#v %v", got, err)
	}
}

func TestUnknownRefreshOutcomeIsDurableAndNeverReplayed(t *testing.T) {
	backend := &memoryCredentials{values: map[string]string{}}
	store := NewCredentialStore(backend, t.TempDir())
	ctx := context.Background()
	now := time.Now().UnixMilli()
	if err := store.Save(ctx, &StoredUAToken{AppId: "app", UserOpenId: "user", AuthProtocol: "legacy_oidc", AccessToken: "old", RefreshToken: "single-use", ExpiresAt: now - 1, RefreshExpiresAt: now + 3600000}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/auth/v3/") {
			fmt.Fprint(w, `{"code":0,"app_access_token":"app-token","app_access_token_expire":7200}`)
			return
		}
		calls++
		w.WriteHeader(500)
		fmt.Fprint(w, `{"code":0,"data":{"access_token":"lost-new-token","expires_in":7200}}`)
	}))
	defer s.Close()
	opts := UATCallOptions{AppId: "app", UserOpenId: "user", AppSecret: "secret", Domain: core.LarkBrand(s.URL), Store: store}
	for i := 0; i < 2; i++ {
		_, err := GetValidAccessToken(s.Client(), opts)
		if err == nil || !strings.Contains(err.Error(), "refresh_outcome_unknown") {
			t.Fatalf("must preserve unknown outcome: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("single-use refresh replayed %d times", calls)
	}
	record, err := store.Load(ctx, "app", "user")
	if err != nil || record == nil || record.RefreshToken != "single-use" || record.RefreshInFlight == nil {
		t.Fatalf("lost recovery state: %#v %v", record, err)
	}
}

type failWrites struct {
	*memoryCredentials
	fail bool
}

func (m *failWrites) Set(s, a, v string) error {
	if m.fail {
		return errors.New("storage_write_failed")
	}
	return m.memoryCredentials.Set(s, a, v)
}
func TestRefreshMarkerWriteFailureSendsZeroRefresh(t *testing.T) {
	backend := &failWrites{memoryCredentials: &memoryCredentials{values: map[string]string{}}}
	store := NewCredentialStore(backend, t.TempDir())
	now := time.Now().UnixMilli()
	if err := store.Save(context.Background(), &StoredUAToken{AppId: "app", UserOpenId: "user", AuthProtocol: "legacy_oidc", AccessToken: "old", RefreshToken: "single-use", ExpiresAt: now - 1, RefreshExpiresAt: now + 3600000}); err != nil {
		t.Fatal(err)
	}
	backend.fail = true
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/auth/v3/") {
			fmt.Fprint(w, `{"code":0,"app_access_token":"app","app_access_token_expire":7200}`)
			return
		}
		calls++
	}))
	defer s.Close()
	_, err := GetValidAccessToken(s.Client(), UATCallOptions{AppId: "app", UserOpenId: "user", Domain: core.LarkBrand(s.URL), Store: store})
	if err == nil || calls != 0 {
		t.Fatalf("refresh sent before durable marker: calls=%d err=%v", calls, err)
	}
}
func TestPrivateProtocolAndAuthorityConflictNeverSendCredentials(t *testing.T) {
	for _, mode := range []string{"protocol_unknown", "authority_conflict", "protocol_conflict"} {
		t.Run(mode, func(t *testing.T) {
			backend := &memoryCredentials{values: map[string]string{}}
			store := NewCredentialStore(backend, t.TempDir())
			now := time.Now().UnixMilli()
			token := &StoredUAToken{AppId: "app", UserOpenId: "user", AccessToken: "old", RefreshToken: "keep", ExpiresAt: now - 1, RefreshExpiresAt: now + 3600000}
			if mode != "protocol_unknown" {
				token.AuthProtocol = "legacy_oidc"
			}
			if mode == "authority_conflict" {
				token.Authority = "https://old-authority.test"
			}
			if err := store.Save(context.Background(), token); err != nil {
				t.Fatal(err)
			}
			calls := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
			defer s.Close()
			opts := UATCallOptions{AppId: "app", UserOpenId: "user", Domain: core.LarkBrand(s.URL), Store: store}
			if mode == "protocol_conflict" {
				opts.AuthProtocol = "oauth_v2"
			}
			_, err := GetValidAccessToken(s.Client(), opts)
			if err == nil || calls != 0 {
				t.Fatalf("unsafe credential routing: calls=%d err=%v", calls, err)
			}
			record, _ := store.Load(context.Background(), "app", "user")
			if record.RefreshToken != "keep" || record.Generation != 1 {
				t.Fatal("credential mutated on conflict")
			}
		})
	}
}

func TestCredentialMutationPreservesUnknownVersionFields(t *testing.T) {
	backend := &memoryCredentials{values: map[string]string{"lark-cli:app:user": `{"appId":"app","userOpenId":"user","accessToken":"old","futureMetadata":{"enabled":true}}`}}
	store := NewCredentialStore(backend, t.TempDir())
	record, err := store.Load(context.Background(), "app", "user")
	if err != nil {
		t.Fatal(err)
	}
	record.AccessToken = "new"
	if err = store.Save(context.Background(), record); err != nil {
		t.Fatal(err)
	}
	raw, _ := backend.Get("lark-cli", "app:user")
	if !strings.Contains(raw, `"futureMetadata":{"enabled":true}`) {
		t.Fatal("unknown credential fields erased")
	}
}

func TestConcurrentRefreshSharesOneRotationAndPreservesMetadata(t *testing.T) {
	backend := &memoryCredentials{values: map[string]string{}}
	store := NewCredentialStore(backend, t.TempDir())
	now := time.Now().UnixMilli()
	if err := store.Save(context.Background(), &StoredUAToken{AppId: "app", UserOpenId: "user", AuthProtocol: "legacy_oidc", AccessToken: "old", RefreshToken: "single-use", ExpiresAt: now - 1, RefreshExpiresAt: now + 3600000, IDToken: "expired-metadata", IDTokenExpiresAt: now - 1, GrantedAt: 7, Scope: "old-scope", UserInfo: map[string]interface{}{"name": "kept"}}); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/auth/v3/") {
			fmt.Fprint(w, `{"code":0,"app_access_token":"app","app_access_token_expire":7200}`)
			return
		}
		calls.Add(1)
		time.Sleep(20 * time.Millisecond)
		fmt.Fprint(w, `{"code":0,"data":{"access_token":"new","refresh_token":"rotated","expires_in":7200,"refresh_expires_in":3600}}`)
	}))
	defer s.Close()
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := GetValidAccessToken(s.Client(), UATCallOptions{AppId: "app", UserOpenId: "user", Domain: core.LarkBrand(s.URL), Store: store})
			if err != nil || token != "new" {
				t.Errorf("waiter: token=%q err=%v", token, err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("refresh requests=%d", calls.Load())
	}
	record, err := store.Load(context.Background(), "app", "user")
	if err != nil || record.IDToken != "expired-metadata" || record.Scope != "old-scope" || record.GrantedAt != 7 || record.UserInfo["name"] != "kept" {
		t.Fatal("refresh erased unrelated metadata")
	}
}

func TestValidAccessTokenCannotCrossChangedAuthority(t *testing.T) {
	backend := &memoryCredentials{values: map[string]string{}}
	store := NewCredentialStore(backend, t.TempDir())
	now := time.Now().UnixMilli()
	if err := store.Save(context.Background(), &StoredUAToken{AppId: "app", UserOpenId: "user", AuthProtocol: "legacy_oidc", Authority: "https://original.test", AccessToken: "private-access", ExpiresAt: now + 3600000}); err != nil {
		t.Fatal(err)
	}
	token, err := GetValidAccessToken(http.DefaultClient, UATCallOptions{AppId: "app", UserOpenId: "user", Domain: core.LarkBrand("https://changed.test"), Store: store})
	if err == nil || token != "" {
		t.Fatal("valid token escaped to changed authority")
	}
}

func TestExplicitProtocolConfirmationPreservesCredentialAndUnknownMetadata(t *testing.T) {
	backend := &memoryCredentials{values: map[string]string{}}
	store := NewCredentialStore(backend, t.TempDir())
	if err := store.Save(context.Background(), &StoredUAToken{AppId: "app", UserOpenId: "user", RefreshToken: "preserve", IDToken: "non-authoritative"}); err != nil {
		t.Fatal(err)
	}
	if err := ConfirmCredentialProtocol(context.Background(), store, "app", "user", "legacy_oidc", "https://tenant.test"); err != nil {
		t.Fatal(err)
	}
	record, _ := store.Load(context.Background(), "app", "user")
	if record.AuthProtocol != "legacy_oidc" || record.Authority != "https://tenant.test" || record.RefreshToken != "preserve" || record.IDToken != "non-authoritative" {
		t.Fatal("confirmation lost credential data")
	}
}

type blockingCredentials struct {
	*memoryCredentials
	once             sync.Once
	entered, release chan struct{}
}

func (b *blockingCredentials) Set(s, a, v string) error {
	b.once.Do(func() { close(b.entered); <-b.release })
	return b.memoryCredentials.Set(s, a, v)
}
func TestCredentialLockCancellationNeverFallsBackUnlocked(t *testing.T) {
	backend := &blockingCredentials{memoryCredentials: &memoryCredentials{values: map[string]string{}}, entered: make(chan struct{}), release: make(chan struct{})}
	namespace := t.TempDir()
	one, two := NewCredentialStore(backend, namespace), NewCredentialStore(backend, namespace)
	done := make(chan error, 1)
	go func() {
		done <- one.Save(context.Background(), &StoredUAToken{AppId: "app", UserOpenId: "user", AccessToken: "first"})
	}()
	<-backend.entered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	err := two.Save(ctx, &StoredUAToken{AppId: "app", UserOpenId: "user", AccessToken: "unsafe-second"})
	close(backend.release)
	if first := <-done; first != nil {
		t.Fatal(first)
	}
	if err == nil {
		t.Fatal("contended canceled write ran without account lock")
	}
	record, _ := one.Load(context.Background(), "app", "user")
	if record.AccessToken != "first" || record.Generation != 1 {
		t.Fatal("canceled waiter mutated credential")
	}
}
func TestRefreshAndLogoutDoNotResurrectAccount(t *testing.T) {
	backend := &memoryCredentials{values: map[string]string{}}
	store := NewCredentialStore(backend, t.TempDir())
	now := time.Now().UnixMilli()
	if err := store.Save(context.Background(), &StoredUAToken{AppId: "app", UserOpenId: "user", AuthProtocol: "legacy_oidc", AccessToken: "old", RefreshToken: "old-refresh", ExpiresAt: now - 1, RefreshExpiresAt: now + 3600000}); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/auth/v3/") {
			fmt.Fprint(w, `{"code":0,"app_access_token":"app","expire":7200}`)
			return
		}
		close(entered)
		<-release
		fmt.Fprint(w, `{"code":0,"data":{"access_token":"new","refresh_token":"rotated","expires_in":7200,"refresh_expires_in":3600}}`)
	}))
	defer s.Close()
	refreshed := make(chan error, 1)
	go func() {
		_, err := GetValidAccessToken(s.Client(), UATCallOptions{AppId: "app", UserOpenId: "user", Domain: core.LarkBrand(s.URL), Store: store})
		refreshed <- err
	}()
	<-entered
	removed := make(chan error, 1)
	go func() { removed <- store.Remove(context.Background(), "app", "user") }()
	close(release)
	if err := <-refreshed; err != nil {
		t.Fatal(err)
	}
	if err := <-removed; err != nil {
		t.Fatal(err)
	}
	record, err := store.Load(context.Background(), "app", "user")
	if err != nil || record != nil {
		t.Fatal("in-flight refresh resurrected logged-out record")
	}
}

func TestAppTokenPreparationFailureCreatesNoRefreshInflight(t *testing.T) {
	backend := &memoryCredentials{values: map[string]string{}}
	store := NewCredentialStore(backend, t.TempDir())
	now := time.Now().UnixMilli()
	if err := store.Save(context.Background(), &StoredUAToken{AppId: "app", UserOpenId: "user", AuthProtocol: "legacy_oidc", AccessToken: "old", RefreshToken: "keep", ExpiresAt: now - 1, RefreshExpiresAt: now + 3600000}); err != nil {
		t.Fatal(err)
	}
	refreshes := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/auth/v3/") {
			w.WriteHeader(500)
			fmt.Fprint(w, `{"code":0,"app_access_token":"fake","expire":7200}`)
			return
		}
		refreshes++
	}))
	defer s.Close()
	_, err := GetValidAccessToken(s.Client(), UATCallOptions{AppId: "app", UserOpenId: "user", Domain: core.LarkBrand(s.URL), Store: store})
	if err == nil || refreshes != 0 {
		t.Fatalf("refresh sent after failed prerequisite: %v count=%d", err, refreshes)
	}
	record, _ := store.Load(context.Background(), "app", "user")
	if record.RefreshInFlight != nil || record.RefreshToken != "keep" || record.Generation != 1 {
		t.Fatal("unsent refresh incorrectly marked inflight")
	}
}

func TestOldOIDCRecordNeedsExplicitProtocolConfirmationNotReauthorization(t *testing.T) {
	backend := &memoryCredentials{values: map[string]string{}}
	store := NewCredentialStore(backend, t.TempDir())
	now := time.Now().UnixMilli()
	if err := store.Save(context.Background(), &StoredUAToken{AppId: "app", UserOpenId: "user", IDToken: "unverified-metadata", AccessToken: "still-valid", ExpiresAt: now + 3600000}); err != nil {
		t.Fatal(err)
	}
	opts := UATCallOptions{AppId: "app", UserOpenId: "user", Domain: core.BrandFeishu, Store: store}
	if _, err := GetValidAccessToken(http.DefaultClient, opts); err == nil || !strings.Contains(err.Error(), "protocol_unknown") {
		t.Fatalf("old OIDC protocol guessed: %v", err)
	}
	if err := ConfirmCredentialProtocol(context.Background(), store, "app", "user", "legacy_oidc", "https://open.feishu.cn"); err != nil {
		t.Fatal(err)
	}
	token, err := GetValidAccessToken(http.DefaultClient, opts)
	if err != nil || token != "still-valid" {
		t.Fatalf("explicit migration lost usable access: %v", err)
	}
}

func TestOAuthRefreshNormalizesRefreshTokenLifetimeSeconds(t *testing.T) {
	backend := &memoryCredentials{values: map[string]string{}}
	store := NewCredentialStore(backend, t.TempDir())
	now := time.Now().UnixMilli()
	if err := store.Save(context.Background(), &StoredUAToken{AppId: "app", UserOpenId: "user", AuthProtocol: "oauth_v2", AccessToken: "old", RefreshToken: "old-refresh", ExpiresAt: now - 1, RefreshExpiresAt: now + 1000000}); err != nil {
		t.Fatal(err)
	}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/open-apis/authen/v2/oauth/token" {
			t.Errorf("wrong OAuth path %s", r.URL.Path)
		}
		fmt.Fprint(w, `{"access_token":"new","refresh_token":"rotated","expires_in":7200,"refresh_token_expires_in":3600}`)
	}))
	defer s.Close()
	_, err := GetValidAccessToken(s.Client(), UATCallOptions{AppId: "app", UserOpenId: "user", Domain: core.LarkBrand(s.URL), Store: store})
	if err != nil {
		t.Fatal(err)
	}
	record, _ := store.Load(context.Background(), "app", "user")
	if record.RefreshExpiresAt < now+3599000 || record.RefreshExpiresAt > now+3601000 {
		t.Fatalf("refresh lifetime seconds lost or wrong unit: %d", record.RefreshExpiresAt-now)
	}
}
