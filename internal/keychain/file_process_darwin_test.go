//go:build darwin

package keychain

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/zalando/go-keyring"
)

// fileTestRing is an explicitly injected fake keyring shared by child processes.
type fileTestRing struct{ root string }

func (r fileTestRing) Get(_, _ string) (string, error) {
	b, e := os.ReadFile(filepath.Join(r.root, "fake-master-key"))
	if os.IsNotExist(e) {
		return "", keyring.ErrNotFound
	}
	return string(b), e
}
func (r fileTestRing) Set(_, _, value string) error {
	f, e := os.OpenFile(filepath.Join(r.root, "fake-master-key"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.WriteString(value)
	if e == nil {
		e = f.Sync()
	}
	f.Close()
	return e
}
func TestFileCredentialProcessHelper(t *testing.T) {
	if os.Getenv("LARK_TEST_KEYRING_CHILD") != "1" {
		return
	}
	dir := os.Getenv("LARK_TEST_KEYRING_DIR")
	if dir == "" {
		t.Fatal("fake backend required")
	}
	backend := NewFileCredentialBackend(dir, fileTestRing{dir})
	if err := backend.Set("service", os.Getenv("LARK_TEST_ACCOUNT"), "synthetic-complete-record"); err != nil {
		t.Fatal(err)
	}
}
func TestTwoProcessesInitializeOneMasterKeyAndReadableCiphertexts(t *testing.T) {
	dir := t.TempDir()
	children := []*exec.Cmd{}
	for _, account := range []string{"one", "two"} {
		child := exec.Command(os.Args[0], "-test.run=^TestFileCredentialProcessHelper$", "-test.count=1")
		child.Env = append(os.Environ(), "LARK_TEST_KEYRING_CHILD=1", "LARK_TEST_KEYRING_DIR="+dir, "LARK_TEST_ACCOUNT="+account)
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
	encoded, err := os.ReadFile(filepath.Join(dir, "fake-master-key"))
	if err != nil {
		t.Fatal(err)
	}
	key, err := base64.StdEncoding.DecodeString(string(encoded))
	if err != nil || len(key) != 32 {
		t.Fatal("invalid shared key")
	}
	backend := NewFileCredentialBackend(dir, fileTestRing{dir})
	for _, account := range []string{"one", "two"} {
		value, err := backend.Get("service", account)
		if err != nil || value != "synthetic-complete-record" {
			t.Fatalf("lost ciphertext across initialization: %v", err)
		}
	}
}
