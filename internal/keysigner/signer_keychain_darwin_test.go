//go:build darwin

// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package keysigner

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ebitengine/purego"
	"github.com/larksuite/cli/internal/vfs"
)

var (
	keychainTestFFIOnce       sync.Once
	keychainTestFFIErr        error
	secKeychainLock           func(keychain uintptr) int32
	secACLCopyContents        func(acl uintptr, applicationList, description *uintptr, promptSelector *uint32) int32
	secKeychainItemCopyAccess func(item uintptr, access *uintptr) int32
)

func loadKeychainTestFFI(t *testing.T) {
	t.Helper()
	keychainTestFFIOnce.Do(func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				keychainTestFFIErr = fmt.Errorf("load test-only Security framework bindings: %v", recovered)
			}
		}()
		if err := loadFFI(); err != nil {
			keychainTestFFIErr = err
			return
		}
		sec, err := purego.Dlopen(secFrameworkPath, purego.RTLD_NOW|purego.RTLD_GLOBAL)
		if err != nil {
			keychainTestFFIErr = fmt.Errorf("dlopen Security for tests: %w", err)
			return
		}
		purego.RegisterLibFunc(&secKeychainLock, sec, "SecKeychainLock")
		purego.RegisterLibFunc(&secACLCopyContents, sec, "SecACLCopyContents")
		purego.RegisterLibFunc(&secKeychainItemCopyAccess, sec, "SecKeychainItemCopyAccess")
	})
	if keychainTestFFIErr != nil {
		t.Fatal(keychainTestFFIErr)
	}
}

// TestKeychainSignerRegistered confirms the keychain_signer build self-registers
// (init → Register), so keysigner.Active() is non-nil. No keychain access.
func TestKeychainSignerRegistered(t *testing.T) {
	if _, ok := Active().(keychainSigner); !ok {
		t.Fatalf("Active() = %T, want keychainSigner (keychain_signer build must self-register)", Active())
	}
}

func TestKeychainFFIBindings(t *testing.T) {
	loadKeychainTestFFI(t)
	if secKeychainCreate == nil || secKeychainOpen == nil || secKeychainUnlock == nil ||
		secKeychainGetUI == nil || secKeychainSetUI == nil || secAccessCreate == nil ||
		secAccessCopyACLs == nil || secACLSetContents == nil ||
		secKeyCreatePair == nil || secKeyCopyExternal == nil || secKeychainItemDelete == nil ||
		secKeychainLock == nil || secACLCopyContents == nil || secKeychainItemCopyAccess == nil {
		t.Fatal("one or more Keychain functions were not registered")
	}
}

func TestPrivateKeyGenerationAttributesAreNonExtractable(t *testing.T) {
	if privateKeyAttributes&cssmKeyAttrPermanent == 0 {
		t.Fatal("private key must be stored permanently in the dedicated keychain")
	}
	if privateKeyAttributes&cssmKeyAttrSensitive == 0 {
		t.Fatal("private key must be marked sensitive")
	}
	if privateKeyAttributes&cssmKeyAttrExtractable != 0 {
		t.Fatal("private key must not be extractable")
	}
	if publicKeyAttributes&cssmKeyAttrExtractable == 0 {
		t.Fatal("public key must remain exportable")
	}
}

func TestWithKeychainUserInteractionDisabledRestoresPreviousState(t *testing.T) {
	previousGet := getKeychainUserInteractionAllowed
	previousSet := setKeychainUserInteractionAllowed
	var states []bool
	getKeychainUserInteractionAllowed = func() (bool, error) { return true, nil }
	setKeychainUserInteractionAllowed = func(allowed bool) error {
		states = append(states, allowed)
		return nil
	}
	t.Cleanup(func() {
		getKeychainUserInteractionAllowed = previousGet
		setKeychainUserInteractionAllowed = previousSet
	})

	called := false
	if err := withKeychainUserInteractionDisabled(func() error {
		called = true
		if len(states) != 1 || states[0] {
			t.Fatalf("interaction states while operation runs = %v, want [false]", states)
		}
		return nil
	}); err != nil {
		t.Fatalf("withKeychainUserInteractionDisabled: %v", err)
	}
	if !called {
		t.Fatal("operation was not called")
	}
	if want := []bool{false, true}; !reflect.DeepEqual(states, want) {
		t.Fatalf("interaction states = %v, want %v", states, want)
	}
}

func TestWithKeychainUserInteractionDisabledFailsBeforeOperation(t *testing.T) {
	previousGet := getKeychainUserInteractionAllowed
	previousSet := setKeychainUserInteractionAllowed
	getKeychainUserInteractionAllowed = func() (bool, error) { return true, nil }
	setKeychainUserInteractionAllowed = func(bool) error { return errors.New("disable failed") }
	t.Cleanup(func() {
		getKeychainUserInteractionAllowed = previousGet
		setKeychainUserInteractionAllowed = previousSet
	})

	called := false
	err := withKeychainUserInteractionDisabled(func() error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "disable failed") {
		t.Fatalf("error = %v, want disable failure", err)
	}
	if called {
		t.Fatal("operation ran while Keychain UI could not be disabled")
	}
}

func TestEnsureKeychainUnlocksExistingKeychain(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	dir := filepath.Join(home, "Library", "Application Support", "lark-cli", "keysigner")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	keychainPath := filepath.Join(dir, "lark-cli.keychain")
	if err := os.WriteFile(keychainPath, []byte("not-a-real-keychain"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "keychain.pass"), []byte("test-password\n"), 0600); err != nil {
		t.Fatal(err)
	}

	previousCreate := createKeychainFile
	previousUnlock := unlockKeychainFile
	createKeychainFile = func(string, []byte) error {
		t.Fatal("createKeychainFile called for an existing keychain")
		return nil
	}
	type unlockCall struct {
		path     string
		password []byte
	}
	var calls []unlockCall
	var borrowedPassword []byte
	unlockKeychainFile = func(path string, password []byte) error {
		borrowedPassword = password
		calls = append(calls, unlockCall{path: path, password: append([]byte(nil), password...)})
		return nil
	}
	t.Cleanup(func() {
		createKeychainFile = previousCreate
		unlockKeychainFile = previousUnlock
	})

	got, err := ensureKeychain()
	if err != nil {
		t.Fatalf("ensureKeychain: %v", err)
	}
	if got != keychainPath {
		t.Fatalf("ensureKeychain path = %q, want %q", got, keychainPath)
	}
	want := []unlockCall{{path: keychainPath, password: []byte("test-password")}}
	if !reflect.DeepEqual(calls, want) {
		t.Fatalf("unlock calls = %#v, want %#v", calls, want)
	}
	for i, value := range borrowedPassword {
		if value != 0 {
			t.Fatalf("borrowed password byte %d was not cleared after use", i)
		}
	}
}

type keychainFirstCreationRaceFS struct {
	vfs.OsFs
	id       string
	coordDir string
}

func (f keychainFirstCreationRaceFS) ReadFile(name string) ([]byte, error) {
	data, err := f.OsFs.ReadFile(name)
	if filepath.Base(name) != "keychain.pass" || !os.IsNotExist(err) {
		return data, err
	}
	if writeErr := os.WriteFile(filepath.Join(f.coordDir, "password-missing-"+f.id), nil, 0600); writeErr != nil {
		return nil, writeErr
	}
	if !waitForKeychainRaceFile(filepath.Join(f.coordDir, "release-password-"+f.id), 10*time.Second) {
		return nil, fmt.Errorf("timed out waiting to release password read for child %s", f.id)
	}
	return data, err
}

func waitForKeychainRaceFile(path string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

type keychainRaceChild struct {
	cmd      *exec.Cmd
	output   *bytes.Buffer
	waitOnce sync.Once
	waitErr  error
}

func (c *keychainRaceChild) wait(kill bool) error {
	c.waitOnce.Do(func() {
		if kill && c.cmd.ProcessState == nil {
			_ = c.cmd.Process.Kill()
		}
		c.waitErr = c.cmd.Wait()
	})
	return c.waitErr
}

func TestEnsureKeychainSerializesFirstCreationAcrossProcesses(t *testing.T) {
	const childEnv = "LARK_KEYCHAIN_FIRST_CREATION_CHILD"
	if childID := os.Getenv(childEnv); childID != "" {
		coordDir := os.Getenv("LARK_KEYCHAIN_FIRST_CREATION_COORD_DIR")
		vfs.DefaultFS = keychainFirstCreationRaceFS{id: childID, coordDir: coordDir}
		createKeychainFile = func(path string, password []byte) error {
			if err := os.WriteFile(filepath.Join(coordDir, "create-ready-"+childID), nil, 0600); err != nil {
				return err
			}
			if !waitForKeychainRaceFile(filepath.Join(coordDir, "release-create-"+childID), 10*time.Second) {
				return fmt.Errorf("timed out waiting to create keychain for child %s", childID)
			}
			file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			if _, err := file.Write(password); err != nil {
				_ = file.Close()
				return err
			}
			if err := file.Close(); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(coordDir, "created-"+childID), nil, 0600)
		}
		unlockKeychainFile = func(path string, password []byte) error {
			createdWith, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if !bytes.Equal(createdWith, password) {
				return fmt.Errorf("keychain password mismatch")
			}
			return nil
		}
		if _, err := ensureKeychain(); err != nil {
			t.Fatalf("ensureKeychain child %s: %v", childID, err)
		}
		return
	}

	home := t.TempDir()
	coordDir := t.TempDir()
	startChild := func(id string) *keychainRaceChild {
		t.Helper()
		output := &bytes.Buffer{}
		cmd := exec.Command(os.Args[0], "-test.run=^TestEnsureKeychainSerializesFirstCreationAcrossProcesses$")
		cmd.Env = append(os.Environ(),
			"HOME="+home,
			childEnv+"="+id,
			"LARK_KEYCHAIN_FIRST_CREATION_COORD_DIR="+coordDir,
		)
		cmd.Stdout = output
		cmd.Stderr = output
		if err := cmd.Start(); err != nil {
			t.Fatalf("start child %s: %v", id, err)
		}
		child := &keychainRaceChild{cmd: cmd, output: output}
		t.Cleanup(func() { _ = child.wait(true) })
		return child
	}

	childA := startChild("a")
	childB := startChild("b")
	passwordMissing := func(id string) string {
		return filepath.Join(coordDir, "password-missing-"+id)
	}
	keychainDir := filepath.Join(home, "Library", "Application Support", "lark-cli", "keysigner")
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		aMissing := waitForKeychainRaceFile(passwordMissing("a"), 10*time.Millisecond)
		bMissing := waitForKeychainRaceFile(passwordMissing("b"), 10*time.Millisecond)
		_, lockErr := os.Stat(filepath.Join(keychainDir, "keychain.init.lock"))
		if aMissing && bMissing || (aMissing || bMissing) && lockErr == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	aMissing := waitForKeychainRaceFile(passwordMissing("a"), 10*time.Millisecond)
	bMissing := waitForKeychainRaceFile(passwordMissing("b"), 10*time.Millisecond)
	if !aMissing && !bMissing {
		t.Fatal("neither child observed a missing password file")
	}
	for _, id := range []string{"a", "b"} {
		if (id == "a" && aMissing) || (id == "b" && bMissing) {
			if err := os.WriteFile(filepath.Join(coordDir, "release-password-"+id), nil, 0600); err != nil {
				t.Fatal(err)
			}
		}
	}

	winner := "a"
	if !aMissing {
		winner = "b"
	}
	if !waitForKeychainRaceFile(filepath.Join(coordDir, "create-ready-"+winner), 10*time.Second) {
		t.Fatalf("child %s did not reach keychain creation", winner)
	}
	if aMissing && bMissing {
		loser := "b"
		if winner == "b" {
			loser = "a"
		}
		if !waitForKeychainRaceFile(filepath.Join(coordDir, "create-ready-"+loser), 10*time.Second) {
			t.Fatalf("child %s did not reach competing keychain creation", loser)
		}
	}
	if err := os.WriteFile(filepath.Join(coordDir, "release-create-"+winner), nil, 0600); err != nil {
		t.Fatal(err)
	}
	if !waitForKeychainRaceFile(filepath.Join(coordDir, "created-"+winner), 10*time.Second) {
		t.Fatalf("child %s did not create the keychain", winner)
	}
	loser := "b"
	if winner == "b" {
		loser = "a"
	}
	if err := os.WriteFile(filepath.Join(coordDir, "release-create-"+loser), nil, 0600); err != nil {
		t.Fatal(err)
	}

	errA := childA.wait(false)
	errB := childB.wait(false)
	if errA != nil || errB != nil {
		t.Fatalf("concurrent ensureKeychain failed: child a: %v\n%s\nchild b: %v\n%s", errA, childA.output, errB, childB.output)
	}
	storedPassword, err := os.ReadFile(filepath.Join(keychainDir, "keychain.pass"))
	if err != nil {
		t.Fatal(err)
	}
	createdWith, err := os.ReadFile(filepath.Join(keychainDir, "lark-cli.keychain"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(storedPassword), createdWith) {
		t.Fatal("stored password does not unlock the concurrently created keychain")
	}
}

// TestKeychainSignerRoundTrip creates a real non-extractable RSA key, signs, and
// verifies RS256 against the returned public key. Gated by LARK_KEYCHAIN_IT
// because it mutates the dedicated lark-cli keychain store. The signer is now
// cgo-free (purego runtime FFI), so it runs with CGO_ENABLED=0. Run with:
//
//	LARK_KEYCHAIN_IT=1 go test -run RoundTrip ./internal/keysigner/
func TestKeychainSignerRoundTrip(t *testing.T) {
	if os.Getenv("LARK_KEYCHAIN_IT") == "" {
		t.Skip("set LARK_KEYCHAIN_IT=1 to run (mutates the macOS keychain)")
	}
	loadKeychainTestFFI(t)
	t.Setenv("HOME", t.TempDir())
	s := keychainSigner{}
	ref := KeyRef{Label: "lark-cli-keychain-it"}

	pub, err := s.EnsureKey(context.Background(), ref)
	if err != nil {
		t.Fatalf("EnsureKey: %v", err)
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("public key = %T, want *rsa.PublicKey", pub)
	}
	if alg, err := AlgForKey(pub); err != nil || alg != AlgRS256 {
		t.Fatalf("AlgForKey = %q, %v; want RS256", alg, err)
	}

	input := []byte("header.payload")
	sig, alg, err := s.Sign(context.Background(), ref, input)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if alg != AlgRS256 {
		t.Errorf("Sign alg = %q, want RS256", alg)
	}
	h := sha256.Sum256(input)
	if err := rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, h[:], sig); err != nil {
		t.Errorf("RS256 signature did not verify: %v", err)
	}

	keychainPath, err := keychainFilePath()
	if err != nil {
		t.Fatalf("keychain path: %v", err)
	}
	var keychainRef uintptr
	if status := secKeychainOpen(cstr(keychainPath), &keychainRef); status != errSecSuccess {
		t.Fatalf("open keychain before lock: %v", keychainError("open keychain before lock", int(status)))
	}
	if keychainRef == 0 {
		t.Fatal("open keychain before lock returned an empty reference")
	}
	if status := secKeychainLock(keychainRef); status != errSecSuccess {
		cfRelease(keychainRef)
		t.Fatalf("lock keychain: %v", keychainError("lock keychain", int(status)))
	}
	cfRelease(keychainRef)

	lockedSig, _, err := s.Sign(context.Background(), ref, input)
	if err != nil {
		t.Fatalf("Sign after explicit Keychain lock: %v", err)
	}
	if err := rsa.VerifyPKCS1v15(rsaPub, crypto.SHA256, h[:], lockedSig); err != nil {
		t.Errorf("signature after explicit Keychain lock did not verify: %v", err)
	}

	md, err := readKeyMetadata(ref.Label)
	if err != nil {
		t.Fatalf("read metadata: %v", err)
	}
	appLabel, err := hex.DecodeString(md.AppLabel)
	if err != nil {
		t.Fatalf("decode app label: %v", err)
	}
	if err := withKeychainUserInteractionDisabled(func() error {
		keychain, err := ensureKeychain()
		if err != nil {
			return err
		}
		privateKeyRef, err := findPrivateKey(appLabel, keychain)
		if err != nil {
			return err
		}
		defer cfRelease(privateKeyRef)

		var access uintptr
		if status := secKeychainItemCopyAccess(privateKeyRef, &access); status != errSecSuccess {
			return keychainError("copy private-key access policy", int(status))
		}
		if access == 0 {
			return errors.New("private-key access policy was empty")
		}
		defer cfRelease(access)
		aclList := secAccessCopyACLs(access, kSecACLAuthorizationSign)
		if aclList == 0 {
			return errors.New("private-key signing ACL was empty")
		}
		defer cfRelease(aclList)
		for i := 0; i < cfArrayGetCount(aclList); i++ {
			acl := cfArrayGetValue(aclList, i)
			var applications, description uintptr
			var promptSelector uint32
			if status := secACLCopyContents(acl, &applications, &description, &promptSelector); status != errSecSuccess {
				return keychainError("copy private-key signing ACL contents", int(status))
			}
			if applications != 0 {
				cfRelease(applications)
				if description != 0 {
					cfRelease(description)
				}
				return errors.New("private-key signing ACL did not allow no-prompt access")
			}
			if description != 0 {
				cfRelease(description)
			}
			if promptSelector != 0 {
				return errors.New("private-key signing ACL requested user interaction")
			}
		}

		var exportErr uintptr
		if exported := secKeyCopyExternal(privateKeyRef, &exportErr); exported != 0 {
			cfRelease(exported)
			return errors.New("private key was exportable")
		}
		if exportErr != 0 {
			cfRelease(exportErr)
		}
		return nil
	}); err != nil {
		t.Fatalf("private-key export check: %v", err)
	}
}
