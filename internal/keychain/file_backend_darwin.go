//go:build darwin

package keychain

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/google/uuid"
	"github.com/zalando/go-keyring"
)

// MasterKeyring permits complete test isolation from the host Keychain.
type MasterKeyring interface {
	Get(string, string) (string, error)
	Set(string, string, string) error
}
type systemMasterKeyring struct{}

func (systemMasterKeyring) Get(s, a string) (string, error) { return keyring.Get(s, a) }
func (systemMasterKeyring) Set(s, a, v string) error        { return keyring.Set(s, a, v) }

// FileCredentialBackend is the macOS Keychain-master-key + encrypted-file backend.
// Its namespace must be shared by every process using the same credential files.
type FileCredentialBackend struct {
	root       string
	ring       MasterKeyring
	timeout    time.Duration
	commitHook func(string) error
}

func NewFileCredentialBackend(root string, ring MasterKeyring) *FileCredentialBackend {
	return &FileCredentialBackend{root: root, ring: ring}
}
func (b *FileCredentialBackend) dir(service string) string { return filepath.Join(b.root, service) }
func (b *FileCredentialBackend) masterKey(service string) ([]byte, error) {
	timeout := b.timeout
	if timeout <= 0 {
		timeout = keychainTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	type outcome struct {
		key []byte
		err error
	}
	done := make(chan outcome, 1)
	go func() { key, err := b.masterKeyWithin(ctx, service); done <- outcome{key, err} }()
	select {
	case got := <-done:
		return got.key, got.err
	case <-ctx.Done():
		return nil, fmt.Errorf("storage_keyring_timeout")
	}
}
func (b *FileCredentialBackend) masterKeyWithin(ctx context.Context, service string) ([]byte, error) {
	dir := b.dir(service)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("storage_unavailable")
	}
	lock := flock.New(filepath.Join(dir, "master-init.lock"))

	ok, err := lock.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil || !ok {
		return nil, fmt.Errorf("storage_keyring_timeout")
	}
	defer lock.Unlock()
	value, err := b.ring.Get(service, "master.key")
	if ctx.Err() != nil {
		return nil, fmt.Errorf("storage_keyring_timeout")
	}
	if err == nil {
		key, e := base64.StdEncoding.DecodeString(value)
		if e != nil || len(key) != masterKeyBytes {
			return nil, fmt.Errorf("storage_corrupt_master_key")
		}
		return key, nil
	}
	if !errors.Is(err, keyring.ErrNotFound) {
		return nil, fmt.Errorf("storage_keyring_unavailable: %w", err)
	}
	entries, e := filepath.Glob(filepath.Join(dir, "*.enc"))
	if e != nil || len(entries) > 0 {
		return nil, fmt.Errorf("storage_missing_master_key")
	}
	key := make([]byte, masterKeyBytes)
	if _, e = rand.Read(key); e != nil {
		return nil, fmt.Errorf("storage_unavailable")
	}
	if e = b.ring.Set(service, "master.key", base64.StdEncoding.EncodeToString(key)); e != nil {
		return nil, fmt.Errorf("storage_keyring_unavailable: %w", e)
	}
	return key, nil
}
func (b *FileCredentialBackend) Get(service, account string) (string, error) {
	key, err := b.masterKey(service)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(filepath.Join(b.dir(service), safeFileName(account)))
	if os.IsNotExist(err) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", fmt.Errorf("storage_unavailable")
	}
	value, err := decryptData(data, key)
	if err != nil {
		return "", fmt.Errorf("storage_corrupt_ciphertext")
	}
	return value, nil
}
func (b *FileCredentialBackend) Set(service, account, value string) error {
	key, err := b.masterKey(service)
	if err != nil {
		return err
	}
	encrypted, err := encryptData(value, key)
	if err != nil {
		return fmt.Errorf("storage_encrypt_failed")
	}
	dir := b.dir(service)
	temp := filepath.Join(dir, safeFileName(account)+"."+uuid.NewString()+".tmp")
	defer os.Remove(temp)
	f, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("storage_write_failed")
	}
	_, err = f.Write(encrypted)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return fmt.Errorf("storage_write_failed")
	}
	if b.commitHook != nil {
		if e := b.commitHook("before-rename"); e != nil {
			return fmt.Errorf("storage_write_failed")
		}
	}
	if err = os.Rename(temp, filepath.Join(dir, safeFileName(account))); err != nil {
		return fmt.Errorf("storage_write_failed")
	}
	if b.commitHook != nil {
		if e := b.commitHook("after-rename"); e != nil {
			return fmt.Errorf("storage_commit_unknown")
		}
	}
	d, err := os.Open(dir)
	if err != nil {
		return fmt.Errorf("storage_commit_unknown")
	}
	defer d.Close()
	if d.Sync() != nil {
		return fmt.Errorf("storage_commit_unknown")
	}
	return nil
}
func (b *FileCredentialBackend) Remove(service, account string) error {
	err := os.Remove(filepath.Join(b.dir(service), safeFileName(account)))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("storage_remove_failed")
	}
	d, err := os.Open(b.dir(service))
	if err != nil {
		return fmt.Errorf("storage_commit_unknown")
	}
	defer d.Close()
	if d.Sync() != nil {
		return fmt.Errorf("storage_commit_unknown")
	}
	return nil
}

// LockNamespace identifies the physical backend, not a CLI config directory.
func (b *FileCredentialBackend) LockNamespace() string {
	p, _ := filepath.Abs(b.root)
	p = filepath.Clean(p)
	if canonical, err := filepath.EvalSymlinks(p); err == nil {
		p = canonical
	}
	sum := sha256.Sum256([]byte(p))
	return filepath.Join(b.root, ".locks", hex.EncodeToString(sum[:]))
}
