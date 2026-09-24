// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keylesshelper

import (
	"bufio"
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/keysigner"
	"github.com/larksuite/cli/internal/validate"
)

type storeTestSigner struct {
	keysigner.Signer
	name                                             string
	ensureErr, publicErr, signErr                    error
	ensureCalls, publicCalls, signCalls, deleteCalls int
	onEnsure                                         func() error
	onSign                                           func()
}

func (s *storeTestSigner) Name() string                           { return s.name }
func (s *storeTestSigner) SecurityLevel() keysigner.SecurityLevel { return keysigner.SecurityLevelL3 }
func (s *storeTestSigner) EnsureKey(ctx context.Context, ref keysigner.KeyRef) (crypto.PublicKey, error) {
	s.ensureCalls++
	if s.onEnsure != nil {
		if err := s.onEnsure(); err != nil {
			return nil, err
		}
	}
	if s.ensureErr != nil {
		return nil, s.ensureErr
	}
	return s.Signer.EnsureKey(ctx, ref)
}
func (s *storeTestSigner) PublicKey(ctx context.Context, ref keysigner.KeyRef) (crypto.PublicKey, error) {
	s.publicCalls++
	if s.publicErr != nil {
		return nil, s.publicErr
	}
	return s.Signer.PublicKey(ctx, ref)
}
func (s *storeTestSigner) Sign(ctx context.Context, ref keysigner.KeyRef, input []byte) ([]byte, string, error) {
	s.signCalls++
	if s.onSign != nil {
		s.onSign()
	}
	if s.signErr != nil {
		return nil, "", s.signErr
	}
	return s.Signer.Sign(ctx, ref, input)
}
func (s *storeTestSigner) DeleteKey(ctx context.Context, ref keysigner.KeyRef) error {
	s.deleteCalls++
	return s.Signer.DeleteKey(ctx, ref)
}

func newStoreTestSigner(t *testing.T) *storeTestSigner {
	t.Helper()
	signer, err := keysigner.NewSoftwareSigner(t.TempDir(), func(context.Context) ([]byte, error) { return bytes.Repeat([]byte{1}, 32), nil })
	if err != nil {
		t.Fatal(err)
	}
	return &storeTestSigner{Signer: signer, name: "test-software"}
}

func TestKeyStoreLifecycle(t *testing.T) {
	isolateSoftwareStorage(t)
	ctx := context.Background()
	signer := newStoreTestSigner(t)
	unavailable := &storeTestSigner{name: "unavailable", ensureErr: keysigner.ErrUnavailable}
	store := NewKeyStoreWithSigners(nil, unavailable, signer)
	// The entire create + attest operation holds the store lock.
	signer.onSign = func() {
		wait, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
		defer cancel()
		err := withKeyStoreLockContext(wait, func() error { t.Fatal("signing escaped transaction"); return nil })
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("nested lock: %v", err)
		}
	}
	key, attestation, err := store.CreateAttestationContext(ctx, "registration", "nonce", time.Now())
	if err != nil || key == nil || attestation == "" {
		t.Fatalf("create: %v", err)
	}
	if unavailable.ensureCalls != 1 || key.Provider() != signer.Name() {
		t.Fatal("fallback not selected")
	}
	if _, err := key.Thumbprint(); err != nil {
		t.Fatal(err)
	}
	before := signer.ensureCalls
	assertion, err := store.SignClientAssertionContext(ctx, key.Provider(), key.ID(), "cli_test", "https://example.invalid", time.Now())
	if err != nil || assertion == "" || signer.ensureCalls != before {
		t.Fatalf("assertion recreated key: %v", err)
	}
	if _, err := store.LoadContext(ctx, key.Provider(), key.ID()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ProbeKeyContext(ctx, key.Provider(), key.ID()); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteKeyContext(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadContext(ctx, key.Provider(), key.ID()); !errors.Is(err, keysigner.ErrKeyNotFound) {
		t.Fatalf("missing key: %v", err)
	}
	if signer.ensureCalls != before {
		t.Fatal("load recreated key")
	}
}

func TestKeyStoreDoesNotFallbackForExistingOrCorruptKeys(t *testing.T) {
	isolateSoftwareStorage(t)
	signer := newStoreTestSigner(t)
	cause := errors.New("corrupt native storage")
	broken := &storeTestSigner{name: "recorded", ensureErr: cause, publicErr: keysigner.ErrKeyNotFound}
	store := NewKeyStoreWithSigners(nil, broken, signer)
	if _, err := store.EnsureContext(context.Background(), "key"); !errors.Is(err, cause) {
		t.Fatal(err)
	}
	if _, err := store.LoadContext(context.Background(), "recorded", "key"); !errors.Is(err, keysigner.ErrKeyNotFound) {
		t.Fatal(err)
	}
	if signer.ensureCalls != 0 || signer.publicCalls != 0 {
		t.Fatal("silently changed provider")
	}
}

func TestKeyStoreProbeAndFailedAttestationCleanup(t *testing.T) {
	isolateSoftwareStorage(t)
	signer := newStoreTestSigner(t)
	store := NewKeyStoreWithSigner(nil, signer)
	if err := store.ProbeWritableContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if signer.deleteCalls != 1 {
		t.Fatal("probe key leaked")
	}
	signer.signErr = errors.New("signing failed")
	key, _, err := store.CreateAttestationContext(context.Background(), "failed", "nonce", time.Now())
	if !errors.Is(err, signer.signErr) || key == nil {
		t.Fatalf("lost cleanup handle: %v", err)
	}
	if err := store.DeleteKeyContext(context.Background(), key); err != nil {
		t.Fatal(err)
	}
	if signer.deleteCalls != 2 {
		t.Fatal("attestation key leaked")
	}
}

// Native initialization is represented by an exclusive file creation. Reverting
// store locking makes competing child processes race on the shared file.
type processStoreSigner struct{ storeTestSigner }

func (s *processStoreSigner) EnsureKey(context.Context, keysigner.KeyRef) (crypto.PublicKey, error) {
	if os.Getenv("LARK_KEY_STORE_HELPER") == "hold" {
		fmt.Println("locked")
		var b [1]byte
		if _, err := io.ReadFull(os.Stdin, b[:]); err != nil {
			return nil, err
		}
	} else {
		path := filepath.Join(os.Getenv("HOME"), "native-keychain")
		if _, err := os.Stat(path); os.IsNotExist(err) {
			time.Sleep(150 * time.Millisecond)
			f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return nil, err
			}
			if err := f.Close(); err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		}
	}
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	return &private.PublicKey, nil
}

func TestKeyStoreFileLockAcrossProcesses(t *testing.T) {
	const helperEnv = "LARK_KEY_STORE_HELPER"
	if os.Getenv(helperEnv) != "" {
		store := NewKeyStoreWithSigner(nil, &processStoreSigner{})
		if _, err := store.EnsureContext(context.Background(), "key"); err != nil {
			t.Fatal(err)
		}
		return
	}
	isolateSoftwareStorage(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var commands []*exec.Cmd
	var outputs []*bytes.Buffer
	for range 6 {
		command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestKeyStoreFileLockAcrossProcesses$")
		command.Env = append(os.Environ(), helperEnv+"=compete")
		out := new(bytes.Buffer)
		command.Stdout, command.Stderr = out, out
		if err := command.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, command)
		outputs = append(outputs, out)
	}
	for i, command := range commands {
		if err := command.Wait(); err != nil {
			t.Fatalf("child: %v: %s", err, outputs[i])
		}
	}
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestKeyStoreFileLockAcrossProcesses$")
	command.Env = append(os.Environ(), helperEnv+"=hold")
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	waited := false
	defer func() {
		if !waited {
			_ = command.Process.Kill()
			_ = command.Wait()
		}
	}()
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "locked\n" {
		t.Fatalf("readiness: %q %v", line, err)
	}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir()) // Another profile still shares the native keychain.
	signer := newStoreTestSigner(t)
	store := NewKeyStoreWithSigner(nil, signer)
	wait, stop := context.WithTimeout(ctx, 30*time.Millisecond)
	_, err = store.EnsureContext(wait, "blocked")
	stop()
	problem, ok := errs.ProblemOf(err)
	if !errors.Is(err, context.DeadlineExceeded) || !ok || !problem.Retryable || signer.ensureCalls != 0 {
		t.Fatalf("timeout: %v", err)
	}
	wait, stop = context.WithCancel(ctx)
	timer := time.AfterFunc(30*time.Millisecond, stop)
	_, err = store.EnsureContext(wait, "blocked")
	timer.Stop()
	stop()
	if !errors.Is(err, context.Canceled) || signer.ensureCalls != 0 {
		t.Fatalf("cancel: %v", err)
	}
	if err := command.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = command.Wait()
	waited = true
	if _, err := store.EnsureContext(ctx, "after-exit"); err != nil {
		t.Fatal(err)
	}
}

func TestKeyStoreConstructorsAndKeySnapshot(t *testing.T) {
	kc := &testMetadataStore{values: map[string]string{}}
	signer := newStoreTestSigner(t)
	for _, store := range []*KeyStore{NewKeyStore(kc), NewKeyStoreWithSigner(kc, signer), newKeyStoreWithSigners(kc, []keysigner.Signer{signer})} {
		if store.keychain != kc {
			t.Fatal("constructor discarded credential store")
		}
	}
	var absent *Key
	if absent.ID() != "" || absent.Provider() != "" || absent.SecurityLevel() != "" {
		t.Fatal("nil key has metadata")
	}
	if _, err := absent.Thumbprint(); !errors.Is(err, ErrKeyNotFound) {
		t.Fatal(err)
	}
	if _, err := absent.PublicJWK(); !errors.Is(err, ErrKeyNotFound) {
		t.Fatal(err)
	}
	ec, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	for _, public := range []crypto.PublicKey{&ec.PublicKey, &rsaKey.PublicKey} {
		key, err := newKey("key", public, signer)
		if err != nil {
			t.Fatal(err)
		}
		before, err := key.Thumbprint()
		if err != nil {
			t.Fatal(err)
		}
		provider := key.Provider()
		switch value := public.(type) {
		case *ecdsa.PublicKey:
			value.X.SetInt64(1)
		case *rsa.PublicKey:
			value.N.SetInt64(1)
		}
		signer.name = "mutated-backend"
		after, err := key.Thumbprint()
		if err != nil || before != after || key.Provider() != provider || key.SecurityLevel() != keysigner.SecurityLevelL3 {
			t.Fatalf("mutable key snapshot: %v", err)
		}
	}
}

func TestKeyStoreProbeFallbackStopsOnCleanupAndCancellation(t *testing.T) {
	isolateSoftwareStorage(t)
	for _, cause := range []error{keysigner.ErrCleanupFailed, context.Canceled, context.DeadlineExceeded} {
		first := &storeTestSigner{name: "first", ensureErr: errors.Join(keysigner.ErrUnavailable, cause)}
		next := newStoreTestSigner(t)
		err := newKeyStoreWithSigners(nil, []keysigner.Signer{first, next}).ProbeWritableContext(context.Background())
		if !errors.Is(err, cause) || next.ensureCalls != 0 {
			t.Fatalf("unsafe fallback: %v, next=%d", err, next.ensureCalls)
		}
	}
}

func TestSignerDirectoryPreservesSoftwareNamespace(t *testing.T) {
	legacy, err := validate.SafeEnvDirPath(isolateSoftwareStorage(t), "test storage")
	if err != nil {
		t.Fatal(err)
	}
	base, err := signerDirectory(keysigner.SoftwareSignerName)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := signerDirectory(filepath.Join("keyless", keysigner.SoftwareSignerName))
	if err != nil {
		t.Fatal(err)
	}
	if actual != legacy || base == actual {
		t.Fatalf("software namespace changed: base=%s actual=%s legacy=%s", base, actual, legacy)
	}
}

func TestKeyStoreMissingKeyPreservesBothErrorIdentities(t *testing.T) {
	isolateSoftwareStorage(t)
	signer := &storeTestSigner{name: "recorded", publicErr: keysigner.ErrKeyNotFound}
	_, err := NewKeyStoreWithSigner(nil, signer).LoadContext(context.Background(), signer.Name(), "missing")
	if !errors.Is(err, ErrKeyNotFound) || !errors.Is(err, keysigner.ErrKeyNotFound) {
		t.Fatalf("missing cause: %v", err)
	}
	var store *KeyStore
	if _, err := store.LoadContext(context.Background(), "recorded", "missing"); !errors.Is(err, ErrKeyNotFound) {
		t.Fatal(err)
	}
	if err := store.DeleteKeyContext(context.Background(), &Key{}); err != nil {
		t.Fatal(err)
	}
}

func TestKeyStoreSharedLockLocation(t *testing.T) {
	isolateSoftwareStorage(t)
	if err := withKeyStoreLockContext(context.Background(), func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	directory, err := signerStorageDir()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(directory, "keysigner"))
	if err != nil || len(entries) != 1 || entries[0].Name() != "key_store.lock" {
		t.Fatalf("shared lock: %v %v", entries, err)
	}
}
