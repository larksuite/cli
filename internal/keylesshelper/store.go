// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylesshelper

import (
	"context"
	"crypto"
	"crypto/x509"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/auth/jwt"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	keyStoreLockTimeout    = 60 * time.Second
	keyStoreLockRetryDelay = 500 * time.Millisecond
)

// The lock follows the shared credential storage, not the selected profile.
// This matches the macOS native keychain's cross-profile ownership.
var keyStoreProcessLocks sync.Map

var ErrKeyNotFound = errors.New("keyless key not found")

// withKeyStoreLockContext follows the UAT token store's cancellable process and
// file locking. It is not reentrant; fn must not call another locking store method.
func withKeyStoreLockContext(ctx context.Context, fn func() error) (err error) {
	lockContext := ctx
	cancel := func() {}
	if lockContext == nil {
		lockContext, cancel = context.WithTimeout(context.Background(), keyStoreLockTimeout)
	} else if _, hasDeadline := lockContext.Deadline(); !hasDeadline {
		lockContext, cancel = context.WithTimeout(lockContext, keyStoreLockTimeout)
	}
	defer cancel()

	directory, err := signerStorageDir()
	if err != nil {
		return err
	}
	lockDir, err := validate.SafeEnvDirPath(filepath.Join(directory, "keysigner"), "key store lock directory")
	if err != nil {
		return err
	}
	// One lock serializes the shared credential store; partition only if
	// contention warrants it. A fixed filename avoids accumulating probe locks.
	lockPath := filepath.Join(lockDir, "key_store.lock")
	candidate := make(chan struct{}, 1)
	candidate <- struct{}{}
	value, _ := keyStoreProcessLocks.LoadOrStore(lockPath, candidate)
	processLock := value.(chan struct{})
	select {
	case <-lockContext.Done():
		return lockContext.Err()
	case <-processLock:
	}
	defer func() { processLock <- struct{}{} }()
	if err := lockContext.Err(); err != nil {
		return err
	}

	if err := vfs.MkdirAll(lockDir, 0700); err != nil {
		return errs.NewInternalError(errs.SubtypeFileIO, "failed to prepare key storage lock").
			WithCause(err).
			WithHint("Check whether local CLI storage is accessible, then retry.")
	}
	fileLock := flock.New(lockPath)
	locked, err := fileLock.TryLockContext(lockContext, keyStoreLockRetryDelay)
	if errors.Is(err, context.DeadlineExceeded) || (err == nil && !locked) {
		return errs.NewInternalError(errs.SubtypeStorage, "timed out waiting for key storage lock").
			WithRetryable().
			WithCause(context.DeadlineExceeded).
			WithHint("Retry the command.")
	}
	if err != nil {
		return errs.NewInternalError(errs.SubtypeFileIO, "failed to acquire key storage lock").
			WithCause(err).
			WithHint("Check whether local CLI storage is accessible, then retry.")
	}
	defer func() {
		if unlockErr := fileLock.Unlock(); err == nil && unlockErr != nil {
			err = errs.NewInternalError(errs.SubtypeFileIO, "failed to release key storage lock").
				WithCause(unlockErr).
				WithHint("Retry the command. If this persists, check whether local CLI storage is accessible.")
		}
	}()
	return fn()
}

// KeyStore coordinates local key lifecycles. Like DPoP's store it owns backend
// selection and locking; PKJWT binding metadata remains in the existing profile.
type KeyStore struct {
	keychain keychain.KeychainAccess
	signers  []keysigner.Signer
}

func NewKeyStore(kc keychain.KeychainAccess) *KeyStore {
	store := newKeyStoreWithSigners(kc, nil)
	store.signers = append(store.signers, keysigner.NewPlatformSigners(signerDirectory)...)
	store.signers = append(store.signers, softwareSigner{keychain: store.keychain})
	return store
}

// NewKeyStoreWithSigner injects a backend while retaining credential ownership.
func NewKeyStoreWithSigner(kc keychain.KeychainAccess, signer keysigner.Signer) *KeyStore {
	var signers []keysigner.Signer
	if signer != nil {
		signers = []keysigner.Signer{signer}
	}
	return newKeyStoreWithSigners(kc, signers)
}

// NewKeyStoreWithSigners is the registration command's ordered test-injection seam.
func NewKeyStoreWithSigners(kc keychain.KeychainAccess, signers ...keysigner.Signer) *KeyStore {
	return newKeyStoreWithSigners(kc, signers)
}

func newKeyStoreWithSigners(kc keychain.KeychainAccess, signers []keysigner.Signer) *KeyStore {
	if kc == nil {
		kc = keychain.Default()
	}
	return &KeyStore{keychain: kc, signers: append([]keysigner.Signer(nil), signers...)}
}

// Key retains a snapshot of the public identity and protection level. PKJWT
// supports both EC and RSA keys; the DPoP clock remains protocol-specific.
type Key struct {
	id            string
	public        crypto.PublicKey
	signer        keysigner.Signer
	provider      string
	securityLevel keysigner.SecurityLevel
}

func newKey(id string, public crypto.PublicKey, signer keysigner.Signer) (*Key, error) {
	if _, err := keysigner.AlgForKey(public); err != nil {
		return nil, err
	}
	// PKIX round-tripping copies both RSA and EC public keys without sharing
	// mutable big.Int values with the backend.
	encoded, err := x509.MarshalPKIXPublicKey(public)
	if err != nil {
		return nil, err
	}
	cloned, err := x509.ParsePKIXPublicKey(encoded)
	if err != nil {
		return nil, err
	}
	return &Key{id: id, public: cloned, signer: signer, provider: signer.Name(), securityLevel: signer.SecurityLevel()}, nil
}

func (k *Key) ID() string {
	if k == nil {
		return ""
	}
	return k.id
}
func (k *Key) Provider() string {
	if k == nil {
		return ""
	}
	return k.provider
}
func (k *Key) SecurityLevel() keysigner.SecurityLevel {
	if k == nil {
		return ""
	}
	return k.securityLevel
}
func (k *Key) PublicJWK() (keysigner.PublicJWK, error) {
	if k == nil || k.public == nil {
		return keysigner.PublicJWK{}, ErrKeyNotFound
	}
	return keysigner.PublicKeyJWK(k.public)
}
func (k *Key) Thumbprint() (string, error) {
	if k == nil || k.public == nil {
		return "", ErrKeyNotFound
	}
	return keysigner.PublicKeyThumbprint(k.public)
}

func (s *KeyStore) EnsureContext(ctx context.Context, id string) (key *Key, err error) {
	if s == nil || s.keychain == nil || len(s.signers) == 0 {
		return nil, keysigner.ErrUnavailable
	}
	if id == "" {
		return nil, errors.New("cannot create keyless key with an empty identifier")
	}
	err = withKeyStoreLockContext(ctx, func() error {
		var err error
		key, err = s.ensureContext(ctx, id)
		return err
	})
	return key, err
}

func (s *KeyStore) ensureContext(ctx context.Context, id string) (*Key, error) {
	if id == "" {
		return nil, fmt.Errorf("keyless: empty key identifier")
	}
	if s == nil || len(s.signers) == 0 {
		return nil, keysigner.ErrUnavailable
	}
	signer, public, err := keysigner.EnsureKeyWithFallback(ctx, s.signers, keysigner.KeyRef{Label: id})
	if err != nil {
		return nil, err
	}
	key, err := newKey(id, public, signer)
	if err != nil {
		cleanupCtx := ctx
		if cleanupCtx == nil {
			cleanupCtx = context.Background()
		} else {
			cleanupCtx = context.WithoutCancel(cleanupCtx)
		}
		return nil, errors.Join(err, signer.DeleteKey(cleanupCtx, keysigner.KeyRef{Label: id}))
	}
	return key, nil
}

// LoadContext restores only the recorded provider. It never creates or falls
// back to another backend. The provider comes from the existing profile.
func (s *KeyStore) LoadContext(ctx context.Context, provider, id string) (key *Key, err error) {
	if s == nil || s.keychain == nil || len(s.signers) == 0 || id == "" {
		return nil, ErrKeyNotFound
	}
	err = withKeyStoreLockContext(ctx, func() error {
		var err error
		key, err = s.loadContext(ctx, provider, id)
		return err
	})
	return key, err
}

func (s *KeyStore) loadContext(ctx context.Context, provider, id string) (*Key, error) {
	if id == "" {
		return nil, fmt.Errorf("keyless: empty key identifier")
	}
	signer := s.signerByName(provider)
	if signer == nil {
		return nil, fmt.Errorf("%w: keyless signer %q is not registered", keysigner.ErrUnavailable, provider)
	}
	public, err := signer.PublicKey(ctx, keysigner.KeyRef{Label: id})
	if err != nil {
		if errors.Is(err, keysigner.ErrKeyNotFound) {
			return nil, fmt.Errorf("%w: %w", ErrKeyNotFound, err)
		}
		return nil, fmt.Errorf("open keyless key with signer %q: %w", signer.Name(), err)
	}
	return newKey(id, public, signer)
}

func (s *KeyStore) signerByName(name string) keysigner.Signer {
	if s != nil {
		for _, signer := range s.signers {
			if signer.Name() == name {
				return signer
			}
		}
	}
	return nil
}

func (s *KeyStore) DeleteKeyContext(ctx context.Context, key *Key) error {
	if key == nil || key.ID() == "" || key.signer == nil {
		return nil
	}
	if s == nil {
		return keysigner.ErrUnavailable
	}
	return withKeyStoreLockContext(ctx, func() error {
		err := key.signer.DeleteKey(ctx, keysigner.KeyRef{Label: key.id})
		if errors.Is(err, keysigner.ErrKeyNotFound) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("delete uncommitted keyless key with signer %q: %w", key.Provider(), err)
		}
		return nil
	})
}

func (s *KeyStore) ProbeWritableContext(ctx context.Context) error {
	if s == nil || s.keychain == nil || len(s.signers) == 0 {
		return keysigner.ErrUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return withKeyStoreLockContext(ctx, func() error {
		if s == nil || len(s.signers) == 0 {
			return keysigner.ErrUnavailable
		}
		var unavailable []error
		for _, signer := range s.signers {
			label, err := keysigner.NewKeyLabel("larksuite-cli-probe-")
			if err != nil {
				return err
			}
			err = keysigner.ProbeSigner(ctx, signer, keysigner.KeyRef{Label: label, Algorithm: keysigner.AlgES256}, []byte("lark-cli key signer probe"))
			if err == nil {
				return nil
			}
			if ctx != nil && ctx.Err() != nil {
				return errors.Join(ctx.Err(), err)
			}
			if !keysigner.CanFallback(err) {
				return err
			}
			unavailable = append(unavailable, err)
		}
		return errors.Join(append([]error{keysigner.ErrUnavailable}, unavailable...)...)
	})
}

// CreateAttestationContext holds one lock across backend selection, key creation
// and signing. It returns the key on signing failure so the caller can clean up.
func (s *KeyStore) CreateAttestationContext(ctx context.Context, id, nonce string, now time.Time) (key *Key, assertion string, err error) {
	err = withKeyStoreLockContext(ctx, func() error {
		var err error
		key, err = s.ensureContext(ctx, id)
		if err != nil {
			return err
		}
		assertion, err = jwt.SignAttestation(ctx, key.signer, keysigner.KeyRef{Label: id}, nonce, now)
		return err
	})
	return key, assertion, err
}

func (s *KeyStore) SignClientAssertionContext(ctx context.Context, provider, id, clientID, audience string, now time.Time) (assertion string, err error) {
	err = withKeyStoreLockContext(ctx, func() error {
		if id == "" {
			return fmt.Errorf("keyless: empty key identifier")
		}
		signer := s.signerByName(provider)
		if signer == nil {
			return fmt.Errorf("%w: keyless signer %q is not registered", keysigner.ErrUnavailable, provider)
		}
		var err error
		assertion, err = jwt.SignClientAssertion(ctx, signer, keysigner.KeyRef{Label: id}, clientID, audience, now)
		return err
	})
	return assertion, err
}

// ProbeKeyContext checks an existing binding without creating or deleting keys.
func (s *KeyStore) ProbeKeyContext(ctx context.Context, provider, id string) (key *Key, err error) {
	err = withKeyStoreLockContext(ctx, func() error {
		var err error
		key, err = s.loadContext(ctx, provider, id)
		if err != nil {
			return err
		}
		algorithm, err := keysigner.AlgForKey(key.public)
		if err != nil {
			return err
		}
		_, signedAlgorithm, err := key.signer.Sign(ctx, keysigner.KeyRef{Label: id, Algorithm: algorithm}, []byte("lark-cli signer health check"))
		if err != nil {
			return err
		}
		if signedAlgorithm != algorithm {
			return fmt.Errorf("keyless: signer returned %s, want %s", signedAlgorithm, algorithm)
		}
		return nil
	})
	return key, err
}
