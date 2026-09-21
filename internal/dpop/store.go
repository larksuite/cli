// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package dpop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/gofrs/flock"
	"github.com/google/uuid"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
)

const (
	keyAccountPrefix       = "dpop:key:v1:"
	probeAccountPrefix     = "dpop:probe:v1:"
	probeCredentialMarker  = "writable"
	storedKeyVersion       = 1
	keyStoreLockTimeout    = 60 * time.Second
	keyStoreLockRetryDelay = 500 * time.Millisecond
	// KeyStoreUnavailableHint is safe to use before or after a Token Endpoint
	// exchange because it makes no claim about whether a request was sent.
	KeyStoreUnavailableHint = "restore access to a supported platform KeyStore/Keychain; if the current sandbox or automation environment blocks it, have the agent or user retry the same command from a trusted interactive session outside the sandbox; Bearer fallback was not attempted"
	// KeyStorePreExchangeUnavailableHint adds the stronger guarantee available
	// only when key storage fails before the Token Endpoint request.
	KeyStorePreExchangeUnavailableHint = "no OAuth token request was sent; " + KeyStoreUnavailableHint
	// KeyAccessUnavailableHint covers an already-bound token whose key cannot be
	// loaded or used to sign the protected resource request.
	KeyAccessUnavailableHint = "restore access to the platform KeyStore/Keychain; if the current sandbox or automation environment blocks it, have the agent or user retry the same command from a trusted interactive session outside the sandbox; the protected request was not sent and Bearer fallback was not attempted"
)

var ErrKeyNotFound = errors.New("DPoP key not found")

var keyStoreProcessLocks sync.Map

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
	// ponytail: one lock serializes the shared credential store; partition only if
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

// storedKey is only a commit marker and public-key integrity record. Private
// material is owned by the selected signer and stored outside this metadata.
type storedKey struct {
	Version         int                 `json:"version"`
	Provider        string              `json:"provider"`
	SecurityLevel   string              `json:"securityLevel"`
	JWK             keysigner.PublicJWK `json:"jwk"`
	JKT             string              `json:"jkt"`
	ClockOffsetMs   int64               `json:"clockOffsetMs,omitempty"`
	ClockSyncedAtMs int64               `json:"clockSyncedAtMs,omitempty"`
}

// KeyStore coordinates ordered signing backends with the CLI secret store.
// Metadata records contain no private keys. L3 keeps its file unlock secret in
// a separate keychain entry; the signer owns its encrypted private-key files.
type KeyStore struct {
	keychain keychain.KeychainAccess
	signers  []keysigner.Signer
	pending  sync.Map
}

func NewKeyStore(kc keychain.KeychainAccess) *KeyStore {
	store := newKeyStoreWithSigners(kc, nil)
	store.signers = append(store.signers, keysigner.NewPlatformSigners(signerDirectory)...)
	store.signers = append(store.signers, softwareSigner{keychain: store.keychain})
	return store
}

// signerDirectory keeps application storage policy outside the signing backends.
func signerDirectory(backend string) (string, error) {
	directory, err := signerStorageDir()
	if err != nil {
		return "", err
	}
	return validate.SafeEnvDirPath(filepath.Join(directory, "keysigner", backend), "DPoP signer directory")
}

// NewKeyStoreWithSigner is an injection seam for platform implementations and
// hermetic tests. Production callers should use NewKeyStore.
func NewKeyStoreWithSigner(kc keychain.KeychainAccess, signer keysigner.Signer) *KeyStore {
	var signers []keysigner.Signer
	if signer != nil {
		signers = []keysigner.Signer{signer}
	}
	return newKeyStoreWithSigners(kc, signers)
}

func newKeyStoreWithSigners(kc keychain.KeychainAccess, signers []keysigner.Signer) *KeyStore {
	if kc == nil {
		kc = keychain.Default()
	}
	return &KeyStore{keychain: kc, signers: signers}
}

// ProbeWritableContext verifies both public-metadata persistence and the exact
// generate/sign/delete capability needed by a DPoP flow. It must run before a
// Token Endpoint request when DPoP is required.
func (s *KeyStore) ProbeWritableContext(ctx context.Context) error {
	if s == nil || s.keychain == nil || len(s.signers) == 0 {
		return keysigner.ErrUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return withKeyStoreLockContext(ctx, func() error {
		if err := s.probeMetadataWritable(); err != nil {
			return err
		}
		var unavailable []error
		for _, signer := range s.signers {
			err := probeSigner(ctx, signer)
			if err == nil {
				return nil
			}
			if err != nil && ctx != nil && ctx.Err() != nil {
				return errors.Join(ctx.Err(), err)
			}
			if keysigner.CanFallback(err) {
				unavailable = append(unavailable, err)
				continue
			}
			return err
		}
		return errors.Join(append([]error{keysigner.ErrUnavailable}, unavailable...)...)
	})
}

// ProbeKeyWritableContext validates metadata persistence and the exact signer
// already owned by a binding. It never selects or probes a different provider.
func (s *KeyStore) ProbeKeyWritableContext(ctx context.Context, key *Key) error {
	if s == nil || s.keychain == nil || key == nil || key.signer == nil {
		return keysigner.ErrUnavailable
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return withKeyStoreLockContext(ctx, func() error {
		if err := s.probeMetadataWritable(); err != nil {
			return err
		}
		return probeSigner(ctx, key.signer)
	})
}

// RequireWritableContext fails before a Token Endpoint request when the
// resulting DPoP binding could not be persisted safely.
func (s *KeyStore) RequireWritableContext(ctx context.Context) error {
	return wrapKeyStoreProbeError(s.ProbeWritableContext(ctx))
}

// RequireWritableForReauthorizationContext performs the ordinary writable
// probe and repairs a definitively lost software unlock secret once. The
// repair deletes software keys that can no longer be decrypted, so callers
// must use it only while explicitly issuing a replacement credential.
func (s *KeyStore) RequireWritableForReauthorizationContext(ctx context.Context) (bool, error) {
	err := s.RequireWritableContext(ctx)
	if err == nil || !errors.Is(err, keysigner.ErrUnlockRequired) {
		return false, err
	}
	var found, removed bool
	recoveryErr := withKeyStoreLockContext(ctx, func() error {
		for _, signer := range s.signers {
			software, ok := signer.(softwareSigner)
			if !ok {
				continue
			}
			found = true
			var resetErr error
			removed, resetErr = software.resetUnrecoverableKeys(ctx)
			return resetErr
		}
		return nil
	})
	if recoveryErr != nil {
		return removed, wrapKeyStoreProbeError(fmt.Errorf(
			"remove unrecoverable DPoP software keys: %w",
			errors.Join(keysigner.ErrUnlockRequired, recoveryErr),
		))
	}
	if !found {
		return false, err
	}
	return removed, s.RequireWritableContext(ctx)
}

// RequireKeyWritableContext validates metadata persistence and the exact signer
// already owned by a binding, returning the token-flow error shape on failure.
func (s *KeyStore) RequireKeyWritableContext(ctx context.Context, key *Key) error {
	return wrapKeyStoreProbeError(s.ProbeKeyWritableContext(ctx, key))
}

func wrapKeyStoreProbeError(err error) error {
	if err == nil {
		return nil
	}
	hint := KeyStorePreExchangeUnavailableHint
	if problem, ok := errs.ProblemOf(err); ok && problem.Hint != "" {
		hint = problem.Hint
	}
	return errs.NewAuthenticationError(errs.SubtypeDPoPKeyMissing,
		"DPoP key storage is unavailable: %v", err).
		WithCause(err).
		WithHint("%s", hint)
}

func (s *KeyStore) probeMetadataWritable() error {
	account := probeAccountPrefix + uuid.NewString()
	if err := s.keychain.Set(keychain.LarkCliService, account, probeCredentialMarker); err != nil {
		return fmt.Errorf("probe DPoP metadata storage: %w", err)
	}
	marker, err := s.keychain.Get(keychain.LarkCliService, account)
	if err != nil {
		cleanupErr := s.keychain.Remove(keychain.LarkCliService, account)
		return fmt.Errorf("read DPoP metadata storage probe: %w", errors.Join(err, cleanupErr))
	}
	if marker != probeCredentialMarker {
		cleanupErr := s.keychain.Remove(keychain.LarkCliService, account)
		return errors.Join(errors.New("DPoP metadata storage probe returned unexpected data"), cleanupErr)
	}
	if err := s.keychain.Remove(keychain.LarkCliService, account); err != nil {
		return fmt.Errorf("clean up DPoP metadata storage probe: %w", err)
	}
	return nil
}

func probeSigner(ctx context.Context, signer keysigner.Signer) error {
	ref := keysigner.KeyRef{Label: "probe-" + uuid.NewString(), Algorithm: keysigner.AlgES256}
	if err := keysigner.ProbeSigner(ctx, signer, ref, []byte("lark-cli DPoP key storage probe")); err != nil {
		return fmt.Errorf("probe DPoP signer %q: %w", signer.Name(), err)
	}
	return nil
}

// Generate creates a persistent P-256 key with the strongest usable backend.
// The caller must either Save it after the token transaction commits or delete
// it on failure.
func (s *KeyStore) Generate() (*Key, error) {
	return s.GenerateContext(context.Background())
}

func (s *KeyStore) GenerateContext(ctx context.Context) (*Key, error) {
	return s.EnsureContext(ctx, uuid.NewString())
}

// EnsureContext opens or creates the key for a stable non-secret identifier.
// It is used for process-cached tenant tokens so repeated CLI invocations do
// not leave one new key per token issuance.
func (s *KeyStore) EnsureContext(ctx context.Context, id string) (*Key, error) {
	if s == nil || s.keychain == nil || len(s.signers) == 0 {
		return nil, keysigner.ErrUnavailable
	}
	if id == "" {
		return nil, errors.New("cannot create DPoP key with an empty identifier")
	}
	var key *Key
	err := withKeyStoreLockContext(ctx, func() error {
		var err error
		key, err = s.ensureContext(ctx, id, s.signers)
		return err
	})
	return key, err
}

// ensureContext requires the store lock.
func (s *KeyStore) ensureContext(ctx context.Context, id string, signers []keysigner.Signer) (*Key, error) {
	if metadata, found, err := s.readMetadata(id); err != nil {
		return nil, err
	} else if found {
		return s.loadFromMetadata(ctx, id, metadata)
	}
	ref := keysigner.KeyRef{Label: id, Algorithm: keysigner.AlgES256}
	signer, public, err := keysigner.EnsureKeyWithFallback(ctx, signers, ref)
	if err != nil {
		return nil, fmt.Errorf("create DPoP key: %w", err)
	}
	p256, err := keysigner.P256PublicKey(public)
	if err != nil {
		cleanupCtx := ctx
		if cleanupCtx == nil {
			cleanupCtx = context.Background()
		} else {
			cleanupCtx = context.WithoutCancel(cleanupCtx)
		}
		cleanupErr := signer.DeleteKey(cleanupCtx, ref)
		return nil, errors.Join(err, cleanupErr)
	}
	s.pending.Store(id, signer)
	return newKey(id, p256, signer, NewClock(nil)), nil
}

// PrepareReplaceableContext opens a stable key or replaces stale metadata when
// no persisted token can still be bound to that key. This is intended for TAT,
// whose token and Binding are process-local. UAT callers must use LoadContext
// so a missing bound key requires re-authorization instead of silent rebinding.
// Created is true only when the caller owns a new key and must delete it if the
// token issuance transaction does not commit.
// The identifier is a namespace: each backend uses its own stable key within it.
func (s *KeyStore) PrepareReplaceableContext(ctx context.Context, id string) (*Key, bool, error) {
	if s == nil || s.keychain == nil || len(s.signers) == 0 {
		return nil, false, keysigner.ErrUnavailable
	}
	if id == "" {
		return nil, false, errors.New("cannot create DPoP key with an empty identifier")
	}
	var key *Key
	var created bool
	err := withKeyStoreLockContext(ctx, func() error {
		var unavailable []error
		for _, signer := range s.signers {
			keyID := id + "-" + signer.Name()
			var err error
			key, err = s.ensureContext(ctx, keyID, []keysigner.Signer{signer})
			if errors.Is(err, ErrKeyNotFound) {
				cleanupCtx := ctx
				if cleanupCtx == nil {
					cleanupCtx = context.Background()
				} else {
					cleanupCtx = context.WithoutCancel(cleanupCtx)
				}
				if deleteErr := s.deleteContext(cleanupCtx, keyID); deleteErr != nil {
					return errors.Join(err, deleteErr)
				}
				key, err = s.ensureContext(ctx, keyID, []keysigner.Signer{signer})
			}
			if err != nil && ctx != nil && ctx.Err() != nil {
				return errors.Join(ctx.Err(), err)
			}
			if keysigner.CanFallback(err) {
				unavailable = append(unavailable, err)
				continue
			}
			if err != nil {
				return err
			}
			_, found, err := s.readMetadata(keyID)
			created = !found
			return err
		}
		return errors.Join(append([]error{keysigner.ErrUnavailable}, unavailable...)...)
	})
	return key, created, err
}

func (s *KeyStore) Save(key *Key) error {
	return s.SaveContext(context.Background(), key)
}

func (s *KeyStore) SaveContext(ctx context.Context, key *Key) error {
	if s == nil || s.keychain == nil || key == nil || key.signer == nil || key.ID() == "" {
		return errors.New("cannot store unavailable DPoP key")
	}
	return withKeyStoreLockContext(ctx, func() error {
		public, err := key.signer.PublicKey(ctx, keysigner.KeyRef{Label: key.ID(), Algorithm: keysigner.AlgES256})
		if err != nil {
			return fmt.Errorf("validate DPoP signer key: %w", err)
		}
		p256, err := keysigner.P256PublicKey(public)
		if err != nil {
			return err
		}
		backendKey := newKeyWithMetadata(key.ID(), p256, key.signer, key.Provider(), key.SecurityLevel(), key.clock)
		backendJKT, err := backendKey.Thumbprint()
		if err != nil {
			return err
		}
		keyJKT, err := key.Thumbprint()
		if err != nil {
			return err
		}
		if backendJKT != keyJKT {
			return errors.New("DPoP signer public key changed before storage")
		}
		jwk, err := key.PublicJWK()
		if err != nil {
			return err
		}
		clockState := key.Clock().State()
		payload, err := json.Marshal(storedKey{
			Version:         storedKeyVersion,
			Provider:        key.Provider(),
			SecurityLevel:   string(key.SecurityLevel()),
			JWK:             jwk,
			JKT:             keyJKT,
			ClockOffsetMs:   clockState.OffsetMillis,
			ClockSyncedAtMs: clockState.SyncedAtMillis,
		})
		if err != nil {
			return fmt.Errorf("encode DPoP public key metadata: %w", err)
		}
		if err := s.keychain.Set(keychain.LarkCliService, keyAccountPrefix+key.ID(), string(payload)); err != nil {
			return fmt.Errorf("store DPoP public key metadata: %w", err)
		}
		s.pending.Delete(key.ID())
		return nil
	})
}

func (s *KeyStore) Load(id string) (*Key, error) {
	return s.LoadContext(context.Background(), id)
}

func (s *KeyStore) LoadContext(ctx context.Context, id string) (*Key, error) {
	if s == nil || s.keychain == nil || len(s.signers) == 0 || id == "" {
		return nil, ErrKeyNotFound
	}
	var key *Key
	err := withKeyStoreLockContext(ctx, func() error {
		metadata, found, err := s.readMetadata(id)
		if err != nil {
			return err
		}
		if !found {
			return ErrKeyNotFound
		}
		key, err = s.loadFromMetadata(ctx, id, metadata)
		return err
	})
	return key, err
}

func (s *KeyStore) readMetadata(id string) (storedKey, bool, error) {
	payload, err := s.keychain.Get(keychain.LarkCliService, keyAccountPrefix+id)
	if err != nil {
		if errors.Is(err, keychain.ErrNotFound) {
			return storedKey{}, false, nil
		}
		return storedKey{}, false, fmt.Errorf("read DPoP public key metadata: %w", err)
	}
	if payload == "" {
		return storedKey{}, false, nil
	}
	var metadata storedKey
	if err := json.Unmarshal([]byte(payload), &metadata); err != nil {
		return storedKey{}, false, fmt.Errorf("decode DPoP public key metadata: %w", err)
	}
	if metadata.Version != storedKeyVersion || metadata.JKT == "" {
		return storedKey{}, false, errors.New("unsupported DPoP key metadata; re-authorization is required")
	}
	return metadata, true, nil
}

func (s *KeyStore) loadFromMetadata(ctx context.Context, id string, metadata storedKey) (*Key, error) {
	signer := s.signerByName(metadata.Provider)
	if signer == nil {
		return nil, fmt.Errorf("%w: DPoP signer %q is not registered", keysigner.ErrUnavailable, metadata.Provider)
	}
	public, err := signer.PublicKey(ctx, keysigner.KeyRef{Label: id, Algorithm: keysigner.AlgES256})
	if err != nil {
		if errors.Is(err, keysigner.ErrKeyNotFound) {
			return nil, fmt.Errorf("%w: %w", ErrKeyNotFound, err)
		}
		return nil, fmt.Errorf("open DPoP key with signer %q: %w", signer.Name(), err)
	}
	p256, err := keysigner.P256PublicKey(public)
	if err != nil {
		return nil, err
	}
	level := signer.SecurityLevel()
	if metadata.SecurityLevel != string(level) {
		return nil, errors.New("DPoP signer protection level does not match stored metadata")
	}
	key := newKeyWithMetadata(id, p256, signer, signer.Name(), level, NewClockWithState(nil, ClockState{
		OffsetMillis:   metadata.ClockOffsetMs,
		SyncedAtMillis: metadata.ClockSyncedAtMs,
	}))
	jwk, err := key.PublicJWK()
	if err != nil {
		return nil, err
	}
	jkt, err := key.Thumbprint()
	if err != nil {
		return nil, err
	}
	if jwk != metadata.JWK || jkt != metadata.JKT {
		return nil, errors.New("DPoP signer key does not match stored public metadata")
	}
	return key, nil
}

func (s *KeyStore) signerByName(name string) keysigner.Signer {
	for _, signer := range s.signers {
		if signer.Name() == name {
			return signer
		}
	}
	return nil
}

func (s *KeyStore) Delete(id string) error {
	return s.DeleteContext(context.Background(), id)
}

// DeleteKeyContext removes a key using the signer capability carried by the
// key itself. It is used to roll back a newly issued key before public metadata
// has been committed.
func (s *KeyStore) DeleteKeyContext(ctx context.Context, key *Key) error {
	if key == nil || key.ID() == "" || key.signer == nil {
		return nil
	}
	if s == nil {
		return keysigner.ErrUnavailable
	}
	return withKeyStoreLockContext(ctx, func() error {
		if err := key.signer.DeleteKey(ctx, keysigner.KeyRef{Label: key.ID(), Algorithm: keysigner.AlgES256}); err != nil &&
			!errors.Is(err, keysigner.ErrKeyNotFound) {
			return errors.Join(keysigner.ErrCleanupFailed,
				fmt.Errorf("delete uncommitted DPoP key with signer %q: %w", key.Provider(), err))
		}
		if s.keychain != nil {
			if err := s.keychain.Remove(keychain.LarkCliService, keyAccountPrefix+key.ID()); err != nil {
				return errors.Join(keysigner.ErrCleanupFailed,
					fmt.Errorf("delete DPoP public key metadata: %w", err))
			}
		}
		s.pending.Delete(key.ID())
		return nil
	})
}

func (s *KeyStore) DeleteContext(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	if s == nil || s.keychain == nil {
		return keysigner.ErrUnavailable
	}
	return withKeyStoreLockContext(ctx, func() error { return s.deleteContext(ctx, id) })
}

// deleteContext requires the store lock.
func (s *KeyStore) deleteContext(ctx context.Context, id string) error {
	var signers []keysigner.Signer
	if pending, ok := s.pending.Load(id); ok {
		signers = []keysigner.Signer{pending.(keysigner.Signer)}
	} else if metadata, found, err := s.readMetadata(id); err != nil {
		return err
	} else if found {
		signer := s.signerByName(metadata.Provider)
		if signer == nil {
			return fmt.Errorf("%w: DPoP signer %q is not registered", keysigner.ErrUnavailable, metadata.Provider)
		}
		signers = []keysigner.Signer{signer}
	} else {
		signers = s.signers
	}
	if len(signers) == 0 {
		return keysigner.ErrUnavailable
	}
	var unavailable error
	for _, signer := range signers {
		err := signer.DeleteKey(ctx, keysigner.KeyRef{Label: id, Algorithm: keysigner.AlgES256})
		if err == nil || errors.Is(err, keysigner.ErrKeyNotFound) {
			continue
		}
		if keysigner.CanFallback(err) {
			unavailable = errors.Join(unavailable, err)
			continue
		}
		return fmt.Errorf("delete DPoP key with signer %q: %w", signer.Name(), err)
	}
	if unavailable != nil {
		return fmt.Errorf("delete DPoP key: %w", unavailable)
	}
	if err := s.keychain.Remove(keychain.LarkCliService, keyAccountPrefix+id); err != nil {
		return fmt.Errorf("delete DPoP public key metadata: %w", err)
	}
	s.pending.Delete(id)
	return nil
}
