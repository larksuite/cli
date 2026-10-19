package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/core"
)

// diskTestCredentials stores AES-GCM encrypted synthetic fixtures with an explicit fake keyring file. Child helpers must be
// explicitly injected; they never construct the production credential adapter.
type diskTestCredentials struct{ dir, phase string }

func (d diskTestCredentials) Get(_, account string) (string, error) {
	raw, e := os.ReadFile(filepath.Join(d.dir, account+".enc"))
	if os.IsNotExist(e) {
		return "", nil
	}
	if e != nil {
		return "", e
	}
	key, e := os.ReadFile(filepath.Join(d.dir, "fake-keyring-master"))
	if e != nil {
		return "", e
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return "", e
	}
	gcm, e := cipher.NewGCM(block)
	if e != nil {
		return "", e
	}
	if len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid fixture ciphertext")
	}
	plain, e := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	return string(plain), e
}
func (d diskTestCredentials) Remove(_, account string) error {
	return os.Remove(filepath.Join(d.dir, account+".enc"))
}
func (d diskTestCredentials) Set(_, account, value string) error {
	var record StoredUAToken
	_ = json.Unmarshal([]byte(value), &record)
	if d.phase == "before-commit" && strings.HasPrefix(record.AccessToken, "new-") {
		d.checkpoint()
	}
	p := filepath.Join(d.dir, account+".enc")
	temp := p + ".tmp"
	f, e := os.OpenFile(temp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	key, e := os.ReadFile(filepath.Join(d.dir, "fake-keyring-master"))
	if e != nil {
		f.Close()
		return e
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		f.Close()
		return e
	}
	gcm, e := cipher.NewGCM(block)
	if e != nil {
		f.Close()
		return e
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		f.Close()
		return e
	}
	encrypted := gcm.Seal(nonce, nonce, []byte(value), nil)
	_, e = f.Write(encrypted)
	if e == nil {
		e = f.Sync()
	}
	f.Close()
	if e != nil {
		return e
	}
	if e = os.Rename(temp, p); e != nil {
		return e
	}
	dir, e := os.Open(d.dir)
	if e != nil {
		return e
	}
	e = dir.Sync()
	dir.Close()
	if e != nil {
		return e
	}
	if d.phase == "before-send" && record.RefreshInFlight != nil {
		d.checkpoint()
	}
	return nil
}
func (d diskTestCredentials) checkpoint() {
	_ = os.WriteFile(filepath.Join(d.dir, "checkpoint"), []byte("ready"), 0600)
	select {}
}
func TestRefreshProcessHelper(t *testing.T) {
	if os.Getenv("LARK_TEST_REFRESH_CHILD") != "1" {
		return
	}
	dir := os.Getenv("LARK_TEST_REFRESH_DIR")
	if dir == "" {
		t.Fatal("refusing child without isolated credential backend")
	}
	store := NewCredentialStore(diskTestCredentials{dir: dir, phase: os.Getenv("LARK_TEST_REFRESH_PHASE")}, filepath.Join(dir, "locks"))
	_, err := GetValidAccessToken(&http.Client{Timeout: 3 * time.Second}, UATCallOptions{AppId: "app", UserOpenId: "user", AppSecret: "synthetic", Domain: core.LarkBrand(os.Getenv("LARK_TEST_REFRESH_URL")), Store: store})
	if os.Getenv("LARK_TEST_EXPECT_UNKNOWN") == "1" {
		if err == nil || !strings.Contains(err.Error(), "refresh_outcome_unknown") {
			t.Fatalf("restart must fail closed: %v", err)
		}
	} else if err != nil {
		t.Fatal(err)
	}
}
func refreshChild(dir, base, phase string, unknown bool) *exec.Cmd {
	c := exec.Command(os.Args[0], "-test.run=^TestRefreshProcessHelper$", "-test.count=1")
	c.Env = append(os.Environ(), "LARK_TEST_REFRESH_CHILD=1", "LARK_TEST_REFRESH_DIR="+dir, "LARK_TEST_REFRESH_URL="+base, "LARK_TEST_REFRESH_PHASE="+phase, fmt.Sprintf("LARK_TEST_EXPECT_UNKNOWN=%d", map[bool]int{false: 0, true: 1}[unknown]))
	return c
}
func seedProcessToken(t *testing.T, dir string) {
	t.Helper()
	key := make([]byte, 32)
	if _, e := rand.Read(key); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(dir, "fake-keyring-master"), key, 0600); e != nil {
		t.Fatal(e)
	}
	store := NewCredentialStore(diskTestCredentials{dir: dir}, filepath.Join(dir, "locks"))
	now := time.Now().UnixMilli()
	if e := store.Save(context.Background(), &StoredUAToken{AppId: "app", UserOpenId: "user", AuthProtocol: "legacy_oidc", AccessToken: "old", RefreshToken: "old-refresh", ExpiresAt: now - 1, RefreshExpiresAt: now + 3600000}); e != nil {
		t.Fatal(e)
	}
}
func TestRefreshCrashRecoveryNeverReplaysGeneration(t *testing.T) {
	for _, phase := range []string{"before-send", "consumed-no-response", "before-commit"} {
		t.Run(phase, func(t *testing.T) {
			dir := t.TempDir()
			seedProcessToken(t, dir)
			var calls atomic.Int32
			release := make(chan struct{})
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.Contains(r.URL.Path, "/auth/v3/") {
					fmt.Fprint(w, `{"code":0,"app_access_token":"app","app_access_token_expire":7200}`)
					return
				}
				n := calls.Add(1)
				if phase == "consumed-no-response" {
					_ = os.WriteFile(filepath.Join(dir, "checkpoint"), []byte("consumed"), 0600)
					select {
					case <-r.Context().Done():
					case <-release:
					}
					return
				}
				fmt.Fprintf(w, `{"code":0,"data":{"access_token":"new-%d","refresh_token":"rotated-%d","expires_in":1,"refresh_expires_in":3600}}`, n, n)
			}))
			defer s.Close()
			defer close(release)
			child := refreshChild(dir, s.URL, phase, false)
			if err := child.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = child.Process.Kill() }()
			deadline := time.Now().Add(5 * time.Second)
			for {
				if _, e := os.Stat(filepath.Join(dir, "checkpoint")); e == nil {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("child did not reach durable checkpoint")
				}
				time.Sleep(5 * time.Millisecond)
			}
			if err := child.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			_ = child.Wait()
			if out, err := refreshChild(dir, s.URL, "", true).CombinedOutput(); err != nil {
				t.Fatalf("restart failed: %v %s", err, out)
			}
			want := int32(1)
			if phase == "before-send" {
				want = 0
			}
			if calls.Load() != want {
				t.Fatalf("refresh replay: calls=%d want=%d", calls.Load(), want)
			}
		})
	}
}
func TestRefreshTwoNewProcessesRotateTwice(t *testing.T) {
	dir := t.TempDir()
	seedProcessToken(t, dir)
	var calls atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/auth/v3/") {
			fmt.Fprint(w, `{"code":0,"app_access_token":"app","app_access_token_expire":7200}`)
			return
		}
		n := calls.Add(1)
		var payload map[string]string
		_ = json.NewDecoder(r.Body).Decode(&payload)
		want := "old-refresh"
		if n == 2 {
			want = "rotated-1"
		}
		if payload["refresh_token"] != want {
			t.Errorf("stale refresh consumed")
		}
		fmt.Fprintf(w, `{"code":0,"data":{"access_token":"new-%d","refresh_token":"rotated-%d","expires_in":1,"refresh_expires_in":3600}}`, n, n)
	}))
	defer s.Close()
	for i := 0; i < 2; i++ {
		if out, err := refreshChild(dir, s.URL, "", false).CombinedOutput(); err != nil {
			t.Fatalf("child failed: %v %s", err, out)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("calls=%d", calls.Load())
	}
	store := NewCredentialStore(diskTestCredentials{dir: dir}, filepath.Join(dir, "locks"))
	record, err := store.Load(context.Background(), "app", "user")
	if err != nil || record.Generation != 3 || record.RefreshInFlight != nil || record.RefreshToken != "rotated-2" {
		t.Fatalf("rotation not durable: %v", err)
	}
}

func TestTwoConfigDirectoriesShareOneBackendRefreshTransaction(t *testing.T) {
	dir := t.TempDir()
	seedProcessToken(t, dir)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/auth/v3/") {
			fmt.Fprint(w, `{"code":0,"app_access_token":"app","expire":7200}`)
			return
		}
		calls.Add(1)
		time.Sleep(20 * time.Millisecond)
		fmt.Fprint(w, `{"code":0,"data":{"access_token":"new-1","refresh_token":"rotated-1","expires_in":7200,"refresh_expires_in":3600}}`)
	}))
	defer server.Close()
	children := []*exec.Cmd{}
	for i := 0; i < 2; i++ {
		child := refreshChild(dir, server.URL, "", false)
		child.Env = append(child.Env, "LARKSUITE_CLI_CONFIG_DIR="+t.TempDir())
		if err := child.Start(); err != nil {
			t.Fatal(err)
		}
		children = append(children, child)
	}
	for _, child := range children {
		if err := child.Wait(); err != nil {
			t.Fatal(err)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("config-scoped locks caused duplicate refresh: %d", calls.Load())
	}
}
