//go:build darwin

package keychain

import (
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/zalando/go-keyring"
)

func TestLockedKeyringNeverReplacesMasterKey(t *testing.T) {
	fake := &fakeRing{err: errors.New("locked")}
	store := NewFileCredentialBackend(t.TempDir(), fake)
	_, err := store.Get("test-service", "account")
	if err == nil || fake.sets != 0 {
		t.Fatalf("locked keyring replaced key: sets=%d err=%v", fake.sets, err)
	}
}

type fakeRing struct {
	value string
	err   error
	sets  int
}

func (r *fakeRing) Get(string, string) (string, error) {
	if r.err != nil {
		return "", r.err
	}
	if r.value == "" {
		return "", keyring.ErrNotFound
	}
	return r.value, nil
}
func (r *fakeRing) Set(_, _, value string) error { r.sets++; r.value = value; return nil }
func TestExistingCiphertextMissingKeyNeverInitializes(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "service"), 0700)
	os.WriteFile(filepath.Join(dir, "service", "account.enc"), []byte("ciphertext"), 0600)
	fake := &fakeRing{}
	store := NewFileCredentialBackend(dir, fake)
	_, err := store.Get("service", "account")
	if err == nil || fake.sets != 0 {
		t.Fatalf("missing master key replaced: sets=%d err=%v", fake.sets, err)
	}
}

func TestCorruptMasterKeyAndCiphertextAreNotOverwritten(t *testing.T) {
	for _, mode := range []string{"master-key", "ciphertext"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			ring := &fakeRing{}
			if mode == "master-key" {
				ring.value = "invalid-base64"
			} else {
				ring.value = base64.StdEncoding.EncodeToString(make([]byte, 32))
				os.MkdirAll(filepath.Join(dir, "service"), 0700)
				os.WriteFile(filepath.Join(dir, "service", "account.enc"), []byte("broken-ciphertext"), 0600)
			}
			backend := NewFileCredentialBackend(dir, ring)
			_, err := backend.Get("service", "account")
			if err == nil || ring.sets != 0 {
				t.Fatalf("corrupt state overwritten: %v sets=%d", err, ring.sets)
			}
		})
	}
}

type delayedRing struct {
	release chan struct{}
	sets    int
}

func (r *delayedRing) Get(string, string) (string, error) {
	<-r.release
	return "", keyring.ErrNotFound
}
func (r *delayedRing) Set(string, string, string) error { r.sets++; return nil }
func TestKeyringTimeoutCannotCreateLateReplacementKey(t *testing.T) {
	ring := &delayedRing{release: make(chan struct{})}
	backend := NewFileCredentialBackend(t.TempDir(), ring)
	backend.timeout = 10 * time.Millisecond
	_, err := backend.Get("service", "account")
	if err == nil || !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("missing timeout: %v", err)
	}
	close(ring.release)
	// Wait for the same initialization lock, proving the timed-out worker finished.
	lock := flock.New(filepath.Join(backend.dir("service"), "master-init.lock"))
	if err := lock.Lock(); err != nil {
		t.Fatal(err)
	}
	lock.Unlock()
	if ring.sets != 0 {
		t.Fatal("timed out lookup created a late master key")
	}
}

func TestCredentialCommitFailureDistinguishesBeforeAndAfterRename(t *testing.T) {
	for _, phase := range []string{"before-rename", "after-rename"} {
		t.Run(phase, func(t *testing.T) {
			backend := NewFileCredentialBackend(t.TempDir(), &fakeRing{})
			if err := backend.Set("service", "account", "old-complete-record"); err != nil {
				t.Fatal(err)
			}
			backend.commitHook = func(stage string) error {
				if stage == phase {
					return errors.New("injected")
				}
				return nil
			}
			err := backend.Set("service", "account", "new-complete-record")
			if err == nil {
				t.Fatal("fault reported success")
			}
			value, e := backend.Get("service", "account")
			if e != nil {
				t.Fatal(e)
			}
			if phase == "before-rename" && value != "old-complete-record" {
				t.Fatal("old record lost before rename")
			}
			if phase == "after-rename" && (value != "new-complete-record" || !strings.Contains(err.Error(), "commit_unknown")) {
				t.Fatal("post-rename failure not reported uncertain")
			}
		})
	}
}
