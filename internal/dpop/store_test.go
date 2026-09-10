// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package dpop

import (
	"bufio"
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/keysigner"
)

type testMetadataStore struct {
	mu                        sync.Mutex
	values                    map[string]string
	getErr, setErr, removeErr error
}

func (s *testMetadataStore) Get(_, account string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.values[account], s.getErr
}
func (s *testMetadataStore) Set(_, account, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.setErr != nil {
		return s.setErr
	}
	s.values[account] = value
	return nil
}
func (s *testMetadataStore) Remove(_, account string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.removeErr != nil {
		return s.removeErr
	}
	delete(s.values, account)
	return nil
}

type testStoreSigner struct {
	name                                     string
	level                                    keysigner.SecurityLevel
	keys                                     map[string]*ecdsa.PrivateKey
	calls                                    []string
	ensureErr, publicErr, signErr, deleteErr error
	onSign                                   func()
}

func newTestStoreSigner(name string, level keysigner.SecurityLevel) *testStoreSigner {
	return &testStoreSigner{name: name, level: level, keys: map[string]*ecdsa.PrivateKey{}}
}
func (s *testStoreSigner) Name() string                           { return s.name }
func (s *testStoreSigner) SecurityLevel() keysigner.SecurityLevel { return s.level }
func (s *testStoreSigner) EnsureKey(ctx context.Context, ref keysigner.KeyRef) (crypto.PublicKey, error) {
	s.calls = append(s.calls, "ensure:"+ref.Label)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.ensureErr != nil {
		return nil, s.ensureErr
	}
	if s.keys[ref.Label] == nil {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			return nil, err
		}
		s.keys[ref.Label] = key
	}
	return &s.keys[ref.Label].PublicKey, nil
}
func (s *testStoreSigner) PublicKey(ctx context.Context, ref keysigner.KeyRef) (crypto.PublicKey, error) {
	s.calls = append(s.calls, "public:"+ref.Label)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s.publicErr != nil {
		return nil, s.publicErr
	}
	if s.keys[ref.Label] == nil {
		return nil, keysigner.ErrKeyNotFound
	}
	return &s.keys[ref.Label].PublicKey, nil
}
func (s *testStoreSigner) Sign(ctx context.Context, ref keysigner.KeyRef, input []byte) ([]byte, string, error) {
	s.calls = append(s.calls, "sign:"+ref.Label)
	if s.onSign != nil {
		s.onSign()
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if s.signErr != nil {
		return nil, "", s.signErr
	}
	if s.keys[ref.Label] == nil {
		return nil, "", keysigner.ErrKeyNotFound
	}
	return (&transportTestSigner{key: s.keys[ref.Label]}).Sign(ctx, ref, input)
}
func (s *testStoreSigner) DeleteKey(ctx context.Context, ref keysigner.KeyRef) error {
	s.calls = append(s.calls, "delete:"+ref.Label)
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.deleteErr != nil {
		return s.deleteErr
	}
	delete(s.keys, ref.Label)
	return nil
}

func TestKeyStoreCommitsRestoresAndDeletesExactBinding(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	ctx := context.Background()
	kc := &testMetadataStore{values: map[string]string{}}
	signer := newTestStoreSigner("original", keysigner.SecurityLevelL2)
	store := NewKeyStoreWithSigner(kc, signer)
	key, created, err := store.PrepareReplaceableContext(ctx, "stable-id")
	if err != nil || !created {
		t.Fatalf("prepare = %v, %v", created, err)
	}
	if len(kc.values) != 0 {
		t.Fatal("uncommitted key was published")
	}
	if _, err := store.Load(key.ID()); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("uncommitted key loaded: %v", err)
	}
	state := ClockState{OffsetMillis: -3600000, SyncedAtMillis: time.Now().UnixMilli()}
	key.Clock().RestoreState(state)
	if err := store.Save(key); err != nil {
		t.Fatal(err)
	}
	stronger := newTestStoreSigner("new-stronger", keysigner.SecurityLevelL1)
	reopened := newKeyStoreWithSigners(kc, []keysigner.Signer{stronger, signer})
	loaded, created, err := reopened.PrepareReplaceableContext(ctx, key.ID())
	if err != nil || created {
		t.Fatalf("reopen = %v, %v", created, err)
	}
	if loaded.Provider() != "original" || loaded.SecurityLevel() != keysigner.SecurityLevelL2 || loaded.Clock().State() != state {
		t.Fatal("reopen changed persisted binding or clock")
	}
	originalJKT, _ := key.Thumbprint()
	if _, err := RestoreBinding("fixture-token", key.ID(), originalJKT, loaded); err != nil {
		t.Fatal(err)
	}
	if len(stronger.calls) != 0 {
		t.Fatal("existing binding selected a different signer")
	}
	if err := reopened.Delete(key.ID()); err != nil {
		t.Fatal(err)
	}
	if len(signer.keys) != 0 || len(kc.values) != 0 || len(stronger.calls) != 0 {
		t.Fatal("delete did not remove exactly the committed key and metadata")
	}
}

func TestKeyStoreFallbackOnlyForUnavailableNewKeys(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	denied := errors.New("injected access denied")
	for _, cause := range []error{keysigner.ErrUnavailable, denied, keysigner.ErrCorrupt} {
		t.Run(cause.Error(), func(t *testing.T) {
			kc := &testMetadataStore{values: map[string]string{}}
			first := newTestStoreSigner("L1", keysigner.SecurityLevelL1)
			first.ensureErr = cause
			second := newTestStoreSigner("L2", keysigner.SecurityLevelL2)
			store := newKeyStoreWithSigners(kc, []keysigner.Signer{first, second})
			key, err := store.Generate()
			if errors.Is(cause, keysigner.ErrUnavailable) {
				if err != nil || key.Provider() != "L2" {
					t.Fatalf("fallback = %v, %v", key, err)
				}
			} else if !errors.Is(err, cause) || len(second.calls) != 0 {
				t.Fatalf("unsafe fallback or lost cause: %v", err)
			}
		})
	}
}

func TestKeyStoreRejectsTamperingAndNeverRecreatesBoundKey(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	for _, change := range []string{"provider", "security_level", "jwk", "jkt", "version", "missing_private_key", "backend_replaced_key"} {
		t.Run(change, func(t *testing.T) {
			kc := &testMetadataStore{values: map[string]string{}}
			signer := newTestStoreSigner("original", keysigner.SecurityLevelL2)
			store := NewKeyStoreWithSigner(kc, signer)
			key, err := store.Generate()
			if err != nil {
				t.Fatal(err)
			}
			if err := store.Save(key); err != nil {
				t.Fatal(err)
			}
			var md storedKey
			if err := json.Unmarshal([]byte(kc.values[keyAccountPrefix+key.ID()]), &md); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "provider":
				md.Provider = "unregistered"
			case "security_level":
				md.SecurityLevel = "L1"
			case "jwk":
				md.JWK.X = "different-coordinate"
			case "jkt":
				md.JKT = "different-thumbprint"
			case "version":
				md.Version++
			case "missing_private_key":
				delete(signer.keys, key.ID())
			case "backend_replaced_key":
				replacement, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
				if err != nil {
					t.Fatal(err)
				}
				signer.keys[key.ID()] = replacement
			}
			data, err := json.Marshal(md)
			if err != nil {
				t.Fatal(err)
			}
			kc.values[keyAccountPrefix+key.ID()] = string(data)
			signer.calls = nil
			if _, err := store.Load(key.ID()); err == nil {
				t.Fatal("accepted broken binding")
			}
			for _, call := range signer.calls {
				if strings.HasPrefix(call, "ensure:") || strings.HasPrefix(call, "delete:") {
					t.Fatal("load mutated key storage")
				}
			}
			if kc.values[keyAccountPrefix+key.ID()] != string(data) {
				t.Fatal("load rewrote rejected metadata")
			}
		})
	}
}

func TestKeyStoreFailedCommitKeepsRollbackOwnership(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	kc := &testMetadataStore{values: map[string]string{}}
	signer := newTestStoreSigner("selected", keysigner.SecurityLevelL2)
	other := newTestStoreSigner("other", keysigner.SecurityLevelL1)
	store := newKeyStoreWithSigners(kc, []keysigner.Signer{signer, other})
	key, err := store.Generate()
	if err != nil {
		t.Fatal(err)
	}
	storeErr := errors.New("injected metadata write failure")
	kc.setErr = storeErr
	if err := store.Save(key); !errors.Is(err, storeErr) {
		t.Fatalf("save error = %v", err)
	}
	if len(kc.values) != 0 || len(signer.keys) != 1 {
		t.Fatal("failed save corrupted transaction ownership")
	}
	deleteErr := errors.New("injected signer deletion failure")
	signer.deleteErr = deleteErr
	if err := store.Delete(key.ID()); !errors.Is(err, deleteErr) {
		t.Fatalf("rollback error = %v", err)
	}
	signer.deleteErr = nil
	if err := store.Delete(key.ID()); err != nil {
		t.Fatal(err)
	}
	if len(signer.keys) != 0 || len(other.calls) != 0 {
		t.Fatal("pending rollback touched the wrong signer")
	}
}

func TestKeyStoreProbeCleansUpAfterCancellationAndPreservesErrors(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	kc := &testMetadataStore{values: map[string]string{}}
	signer := newTestStoreSigner("selected", keysigner.SecurityLevelL2)
	store := NewKeyStoreWithSigner(kc, signer)
	ctx, cancel := context.WithCancel(context.Background())
	signer.onSign = cancel
	err := store.RequireWritableContext(ctx)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Subtype != errs.SubtypeDPoPKeyMissing || !errors.Is(err, context.Canceled) || problem.Hint == "" {
		t.Fatalf("probe failure = %v", err)
	}
	if len(signer.keys) != 0 || len(kc.values) != 0 {
		t.Fatal("cancellation prevented probe cleanup")
	}
	signer.onSign = nil
	signErr, deleteErr := errors.New("sign denied"), errors.New("delete denied")
	signer.signErr, signer.deleteErr = signErr, deleteErr
	err = store.ProbeWritableContext(context.Background())
	if !errors.Is(err, signErr) || !errors.Is(err, deleteErr) {
		t.Fatalf("probe lost operation or cleanup failure: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(core.GetConfigDir(), "locks"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "dpop_store.lock" {
		t.Fatalf("probes must reuse one store lock: entries = %v, err = %v", entries, err)
	}
}

func TestKeyStoreOnlyReplaceableKeysRecoverFromMissingPrivateKey(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	ctx := context.Background()
	kc := &testMetadataStore{values: map[string]string{}}
	signer := newTestStoreSigner("selected", keysigner.SecurityLevelL2)
	store := NewKeyStoreWithSigner(kc, signer)
	key, err := store.EnsureContext(ctx, "tenant-key")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(key); err != nil {
		t.Fatal(err)
	}
	oldJKT, _ := key.Thumbprint()
	delete(signer.keys, key.ID())
	if _, err := store.EnsureContext(ctx, key.ID()); !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("existing key was recreated: %v", err)
	}
	replaced, created, err := store.PrepareReplaceableContext(ctx, key.ID())
	if err != nil || !created {
		t.Fatalf("replace missing tenant key: %v, %v", created, err)
	}
	newJKT, _ := replaced.Thumbprint()
	if newJKT == oldJKT || len(kc.values) != 0 {
		t.Fatal("replacement reused stale metadata or committed prematurely")
	}
}

func TestKeyStoreMetadataProbeFailsBeforeNativeKeyCreation(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	cause := errors.New("metadata backend failed")
	for _, operation := range []string{"read", "write", "remove"} {
		t.Run(operation, func(t *testing.T) {
			kc := &testMetadataStore{values: map[string]string{}}
			switch operation {
			case "read":
				kc.getErr = cause
			case "write":
				kc.setErr = cause
			case "remove":
				kc.removeErr = cause
			}
			signer := newTestStoreSigner("selected", keysigner.SecurityLevelL2)
			err := NewKeyStoreWithSigner(kc, signer).RequireWritableContext(context.Background())
			problem, ok := errs.ProblemOf(err)
			if !ok || problem.Subtype != errs.SubtypeDPoPKeyMissing || !errors.Is(err, cause) || problem.Hint == "" {
				t.Fatalf("probe failure = %v", err)
			}
			if len(signer.calls) != 0 {
				t.Fatal("native key creation preceded metadata preflight")
			}
			if operation != "remove" && len(kc.values) != 0 {
				t.Fatal("failed read/write left probe metadata")
			}
		})
	}
}

func TestKeyStoreBoundProbeAndDeletionCannotSwitchProvider(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	kc := &testMetadataStore{values: map[string]string{}}
	signer := newTestStoreSigner("original", keysigner.SecurityLevelL2)
	store := NewKeyStoreWithSigner(kc, signer)
	key, err := store.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(key); err != nil {
		t.Fatal(err)
	}
	metadata := kc.values[keyAccountPrefix+key.ID()]
	other := newTestStoreSigner("new-stronger", keysigner.SecurityLevelL1)
	reopened := newKeyStoreWithSigners(kc, []keysigner.Signer{other, signer})
	signer.ensureErr = keysigner.ErrUnavailable
	if err := reopened.RequireKeyWritableContext(context.Background(), key); !errors.Is(err, keysigner.ErrUnavailable) {
		t.Fatalf("bound probe = %v", err)
	}
	for _, failure := range []error{keysigner.ErrUnavailable, errors.New("access denied")} {
		signer.deleteErr = failure
		if err := reopened.Delete(key.ID()); !errors.Is(err, failure) {
			t.Fatalf("delete = %v", err)
		}
		if len(signer.keys) != 1 || kc.values[keyAccountPrefix+key.ID()] != metadata {
			t.Fatal("failed deletion lost binding needed for retry")
		}
	}
	signer.deleteErr = nil
	if err := reopened.Delete(key.ID()); err != nil {
		t.Fatal(err)
	}
	if len(signer.keys) != 0 || len(kc.values) != 0 || len(other.calls) != 0 {
		t.Fatal("bound probe/deletion touched wrong provider or did not clean up")
	}
}

func TestKeyStoreCommitRejectsKeyChangedAfterIssuance(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	kc := &testMetadataStore{values: map[string]string{}}
	signer := newTestStoreSigner("selected", keysigner.SecurityLevelL2)
	store := NewKeyStoreWithSigner(kc, signer)
	key, err := store.Generate()
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer.keys[key.ID()] = replacement
	if err := store.Save(key); err == nil {
		t.Fatal("committed token binding with a different private key")
	}
	if len(kc.values) != 0 {
		t.Fatal("published mismatched binding")
	}
	if err := store.DeleteKeyContext(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if len(signer.keys) != 0 {
		t.Fatal("uncommitted key could not be rolled back")
	}
}

func TestKeyStoreOperationsWaitForSharedLock(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	kc := &testMetadataStore{values: map[string]string{}}
	signer := newTestStoreSigner("selected", keysigner.SecurityLevelL2)
	store := NewKeyStoreWithSigner(kc, signer)
	key, err := store.Generate()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(key); err != nil {
		t.Fatal(err)
	}
	// Hold the lock inside a real probe while another store instance waits.
	entered, release := make(chan struct{}), make(chan struct{})
	signer.onSign = func() { close(entered); <-release }
	done := make(chan error, 1)
	go func() { done <- store.ProbeWritableContext(context.Background()) }()
	t.Cleanup(func() {
		close(release)
		if err := <-done; err != nil {
			t.Errorf("probe after release: %v", err)
		}
	})
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("probe did not acquire the store lock")
	}
	waiting := NewKeyStoreWithSigner(kc, signer)
	calls := len(signer.calls)
	for _, tc := range []struct {
		name string
		run  func(context.Context) error
	}{
		{"probe", waiting.ProbeWritableContext},
		{"bound_probe", func(ctx context.Context) error { return waiting.ProbeKeyWritableContext(ctx, key) }},
		{"ensure", func(ctx context.Context) error { _, err := waiting.EnsureContext(ctx, key.ID()); return err }},
		{"prepare", func(ctx context.Context) error {
			_, _, err := waiting.PrepareReplaceableContext(ctx, key.ID())
			return err
		}},
		{"save", func(ctx context.Context) error { return waiting.SaveContext(ctx, key) }},
		{"load", func(ctx context.Context) error { _, err := waiting.LoadContext(ctx, key.ID()); return err }},
		{"delete_key", func(ctx context.Context) error { return waiting.DeleteKeyContext(ctx, key) }},
		{"delete", func(ctx context.Context) error { return waiting.DeleteContext(ctx, key.ID()) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			if err := tc.run(ctx); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("operation did not wait for the store lock: %v", err)
			}
			if len(signer.calls) != calls || len(kc.values) != 1 {
				t.Fatal("waiting operation accessed the signer or changed metadata")
			}
		})
	}
}

func TestKeyStoreFileLockAcrossProcesses(t *testing.T) {
	const helperEnv = "LARK_CLI_DPOP_STORE_LOCK_TEST_HELPER"
	if os.Getenv(helperEnv) == "1" {
		store := NewKeyStoreWithSigner(&testMetadataStore{values: map[string]string{}},
			newTestStoreSigner("selected", keysigner.SecurityLevelL2))
		signer := store.signers[0].(*testStoreSigner)
		signer.onSign = func() {
			if _, err := fmt.Fprintln(os.Stdout, "LOCKED"); err != nil {
				t.Fatal(err)
			}
			var release [1]byte
			if _, err := io.ReadFull(os.Stdin, release[:]); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.ProbeWritableContext(context.Background()); err != nil {
			t.Fatal(err)
		}
		return
	}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestKeyStoreFileLockAcrossProcesses$")
	command.Env = append(os.Environ(), helperEnv+"=1")
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	t.Cleanup(func() {
		_ = stdin.Close()
		if !waited {
			cancel()
			_ = command.Wait()
		}
	})
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "LOCKED\n" {
		t.Fatalf("helper readiness = %q, err = %v", line, err)
	}
	signer := newTestStoreSigner("selected", keysigner.SecurityLevelL2)
	store := NewKeyStoreWithSigner(&testMetadataStore{values: map[string]string{}}, signer)
	waitCtx, stop := context.WithTimeout(ctx, 30*time.Millisecond)
	defer stop()
	_, err = store.GenerateContext(waitCtx)
	problem, ok := errs.ProblemOf(err)
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeStorage ||
		!problem.Retryable || !errors.Is(err, context.DeadlineExceeded) || len(signer.calls) != 0 {
		t.Fatalf("file-lock timeout = %v, signer calls = %v", err, signer.calls)
	}
	cancelCtx, stopWaiting := context.WithCancel(ctx)
	defer stopWaiting()
	timer := time.AfterFunc(30*time.Millisecond, stopWaiting)
	defer timer.Stop()
	if _, err := store.GenerateContext(cancelCtx); !errors.Is(err, context.Canceled) {
		t.Fatalf("file-lock cancellation = %v", err)
	}
	if _, err := io.WriteString(stdin, "x"); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("helper failed: %v; %s", err, stderr.String())
	}
	waited = true
	key, err := store.GenerateContext(ctx)
	if err != nil {
		t.Fatalf("store remained locked after release: %v", err)
	}
	if err := store.DeleteKeyContext(ctx, key); err != nil {
		t.Fatal(err)
	}
}

func TestKeyStoreLockDirectoryFailure(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	if err := os.WriteFile(filepath.Join(core.GetConfigDir(), "locks"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	signer := newTestStoreSigner("selected", keysigner.SecurityLevelL2)
	store := NewKeyStoreWithSigner(&testMetadataStore{values: map[string]string{}}, signer)
	_, err := store.Generate()
	problem, ok := errs.ProblemOf(err)
	var pathErr *os.PathError
	if !ok || problem.Category != errs.CategoryInternal || problem.Subtype != errs.SubtypeFileIO ||
		!errors.As(err, &pathErr) || len(signer.calls) != 0 {
		t.Fatalf("lock directory failure = %v, signer calls = %v", err, signer.calls)
	}
}
