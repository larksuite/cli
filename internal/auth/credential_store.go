package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/larksuite/cli/internal/keychain"
)

var errNoCredentialMutation = errors.New("no credential mutation")

// CredentialStore binds all credential mutations to one backend-owned lock namespace.
// Tests must inject both the credential backend and its isolated namespace.
type CredentialStore struct {
	backend   keychain.KeychainAccess
	namespace string
}

func NewCredentialStore(backend keychain.KeychainAccess, namespace string) *CredentialStore {
	return &CredentialStore{backend: backend, namespace: namespace}
}
func (s *CredentialStore) Load(_ context.Context, app, user string) (*StoredUAToken, error) {
	raw, err := s.backend.Get(keychain.LarkCliService, accountKey(app, user))
	if errors.Is(err, keychain.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	var record StoredUAToken
	if json.Unmarshal([]byte(raw), &record) != nil {
		return nil, fmt.Errorf("storage_corrupt_record")
	}
	if record.AppId != app || record.UserOpenId != user {
		return nil, fmt.Errorf("storage_identity_mismatch")
	}
	return &record, nil
}
func (s *CredentialStore) transaction(ctx context.Context, app, user string, fn func(*StoredUAToken) (*StoredUAToken, error)) error {
	if s.backend == nil || s.namespace == "" {
		return fmt.Errorf("storage_unavailable")
	}
	if err := os.MkdirAll(s.namespace, 0700); err != nil {
		return err
	}
	sum := sha256.Sum256([]byte(keychain.LarkCliService + "\x00" + app + "\x00" + user))
	lock := flock.New(filepath.Join(s.namespace, hex.EncodeToString(sum[:])+".lock"))
	bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ok, err := lock.TryLockContext(bounded, 10*time.Millisecond)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("storage_lock_timeout")
	}
	defer lock.Unlock()
	record, err := s.Load(ctx, app, user)
	if err != nil {
		return err
	}
	next, err := fn(record)
	if errors.Is(err, errNoCredentialMutation) {
		return nil
	}
	if err != nil {
		return err
	}
	if next == nil {
		return s.backend.Remove(keychain.LarkCliService, accountKey(app, user))
	}
	raw, err := json.Marshal(next)
	if err != nil {
		return err
	}
	return s.backend.Set(keychain.LarkCliService, accountKey(app, user), string(raw))
}
func (s *CredentialStore) Save(ctx context.Context, record *StoredUAToken) error {
	if record == nil || record.AppId == "" || record.UserOpenId == "" {
		return errors.New("invalid credential identity")
	}
	return s.transaction(ctx, record.AppId, record.UserOpenId, func(old *StoredUAToken) (*StoredUAToken, error) {
		next := *record
		if old != nil {
			next.Generation = old.Generation + 1
			if next.UnknownFields == nil {
				next.UnknownFields = old.UnknownFields
			}
		} else {
			next.Generation = 1
		}
		return &next, nil
	})
}
func (s *CredentialStore) Remove(ctx context.Context, app, user string) error {
	return s.transaction(ctx, app, user, func(_ *StoredUAToken) (*StoredUAToken, error) { return nil, nil })
}
