// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package gitcred

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/errs"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "gitcred-test-config-*")
	if err != nil {
		panic(err)
	}
	_ = os.Setenv("LARKSUITE_CLI_CONFIG_DIR", dir)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

type fakeKeychain struct {
	values    map[string]string
	removed   []string
	services  []string
	getErr    error
	setErr    error
	removeErr error
	onGet     func(account string)
	onSet     func(account, value string)
}

func newFakeKeychain() *fakeKeychain {
	return &fakeKeychain{values: map[string]string{}}
}

func (f *fakeKeychain) Get(service, account string) (string, error) {
	if f.getErr != nil {
		return "", f.getErr
	}
	f.services = append(f.services, "get:"+service)
	if f.onGet != nil {
		f.onGet(account)
	}
	return f.values[account], nil
}

func (f *fakeKeychain) Set(service, account, value string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.services = append(f.services, "set:"+service)
	f.values[account] = value
	if f.onSet != nil {
		f.onSet(account, value)
	}
	return nil
}

func (f *fakeKeychain) Remove(service, account string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	f.services = append(f.services, "remove:"+service)
	delete(f.values, account)
	f.removed = append(f.removed, account)
	return nil
}

type fakeIssuer struct {
	calls   int
	next    *IssuedCredential
	err     error
	onIssue func()
}

func (f *fakeIssuer) Issue(ctx context.Context, appID string, profile ProfileContext) (*IssuedCredential, error) {
	f.calls++
	if f.onIssue != nil {
		f.onIssue()
	}
	if f.err != nil {
		return nil, f.err
	}
	out := *f.next
	if out.AppID == "" {
		out.AppID = appID
	}
	return &out, nil
}

type fakeGitConfig struct {
	set   []string
	unset []string
	err   error
}

func (f *fakeGitConfig) SetHelper(ctx context.Context, gitHTTPURL, appID string) error {
	f.set = append(f.set, gitHTTPURL+" "+appID)
	return f.err
}

func (f *fakeGitConfig) UnsetHelper(ctx context.Context, gitHTTPURL string) error {
	f.unset = append(f.unset, gitHTTPURL)
	return f.err
}

type splitFakeGitConfig struct {
	setErr   error
	unsetErr error
}

func (f splitFakeGitConfig) SetHelper(ctx context.Context, gitHTTPURL, appID string) error {
	return f.setErr
}

func (f splitFakeGitConfig) UnsetHelper(ctx context.Context, gitHTTPURL string) error {
	return f.unsetErr
}

type fakeAppStorage struct {
	values map[string][]byte
	err    error
}

func newFakeAppStorage() *fakeAppStorage {
	return &fakeAppStorage{values: map[string][]byte{}}
}

func (s *fakeAppStorage) Read(appID, key string) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	data := s.values[appID+"/"+key]
	if data == nil {
		return nil, nil
	}
	return append([]byte(nil), data...), nil
}

func (s *fakeAppStorage) Write(appID, key string, data []byte) error {
	if s.err != nil {
		return s.err
	}
	s.values[appID+"/"+key] = append([]byte(nil), data...)
	return nil
}

func (s *fakeAppStorage) Delete(appID, key string) error {
	if s.err != nil {
		return s.err
	}
	delete(s.values, appID+"/"+key)
	return nil
}

type sequenceAppStorage struct {
	reads [][]byte
}

func (s *sequenceAppStorage) Read(appID, key string) ([]byte, error) {
	if len(s.reads) == 0 {
		return nil, nil
	}
	data := s.reads[0]
	s.reads = s.reads[1:]
	return append([]byte(nil), data...), nil
}

func (s *sequenceAppStorage) Write(appID, key string, data []byte) error {
	return nil
}

func (s *sequenceAppStorage) Delete(appID, key string) error {
	return nil
}

func TestNormalizeGitHTTPURL(t *testing.T) {
	got, err := NormalizeGitHTTPURL("HTTPS://Example.COM:443//git/u_abc/app.git/?x=1#frag")
	if err != nil {
		t.Fatalf("NormalizeGitHTTPURL returned error: %v", err)
	}
	want := "https://example.com/git/u_abc/app.git"
	if got != want {
		t.Fatalf("NormalizeGitHTTPURL() = %q, want %q", got, want)
	}
}

func TestManagerInitStoresPATThroughInternalKeychainAndMetadataOnly(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	gitConfig := &fakeGitConfig{}
	issuer := &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		Username:   "x-access-token",
		PAT:        "secret-pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}}
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), gitConfig, issuer)
	manager.Now = func() time.Time { return now }

	result, err := manager.Init(context.Background(), testProfile(), "app_xxx")
	if err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	if result.GitHTTPURL != "https://example.com/git/u/app.git" {
		t.Fatalf("GitHTTPURL = %q", result.GitHTTPURL)
	}
	record, err := manager.Store.FindByURL(result.GitHTTPURL)
	if err != nil {
		t.Fatalf("FindByURL returned error: %v", err)
	}
	if record == nil || record.Status != StatusConfirmed {
		t.Fatalf("record = %#v, want confirmed", record)
	}
	if bytes.Contains(mustReadMetadata(t, manager), []byte("secret-pat")) {
		t.Fatalf("metadata must not contain PAT")
	}
	if got := kc.values[record.PATRef]; got != "secret-pat" {
		t.Fatalf("keychain PAT = %q, want secret-pat", got)
	}
	if !slices.Contains(kc.services, "set:"+KeychainService) {
		t.Fatalf("keychain services = %#v, want Set through %q", kc.services, KeychainService)
	}
	if len(gitConfig.set) != 1 || gitConfig.set[0] != result.GitHTTPURL+" app_xxx" {
		t.Fatalf("git config set = %#v", gitConfig.set)
	}
}

func TestManagerInitFailsWhenKeychainUnavailable(t *testing.T) {
	now := time.Unix(1780000000, 0)
	issuer := &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "secret-pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}}
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(nil), nil, issuer)
	manager.Now = func() time.Time { return now }

	_, err := manager.Init(context.Background(), testProfile(), "app_xxx")
	if err == nil {
		t.Fatal("Init returned nil error, want keychain unavailable error")
	}
	record, findErr := manager.Store.FindByURL("https://example.com/git/u/app.git")
	if findErr != nil {
		t.Fatalf("FindByURL returned error: %v", findErr)
	}
	if record != nil {
		t.Fatalf("record after failed init = %#v, want nil", record)
	}
}

func TestManagerInitRestoresExistingRecordWhenKeychainSetFails(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	issuer := &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "old-pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}}
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), nil, issuer)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("initial Init returned error: %v", err)
	}
	before, err := manager.Store.FindByURL("https://example.com/git/u/app.git")
	if err != nil {
		t.Fatalf("FindByURL returned error: %v", err)
	}

	kc.setErr = errors.New("keychain locked")
	issuer.next = &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "new-pat",
		ExpiresAt:  now.Add(48 * time.Hour).Unix(),
	}
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err == nil {
		t.Fatal("second Init returned nil error, want keychain error")
	}
	after, err := manager.Store.FindByURL("https://example.com/git/u/app.git")
	if err != nil {
		t.Fatalf("FindByURL returned error: %v", err)
	}
	if after == nil || after.ExpiresAt != before.ExpiresAt || after.Status != StatusConfirmed {
		t.Fatalf("record after failed refresh init = %#v, want original %#v", after, before)
	}
	if got := kc.values[before.PATRef]; got != "old-pat" {
		t.Fatalf("keychain PAT after failed refresh init = %q, want old-pat", got)
	}
}

func TestManagerInitCleansOldURLHelperAfterRepositoryURLChanges(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	gitConfig := &fakeGitConfig{}
	issuer := &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/old.git",
		PAT:        "old-pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}}
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), gitConfig, issuer)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("initial Init returned error: %v", err)
	}

	issuer.next = &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/new.git",
		PAT:        "new-pat",
		ExpiresAt:  now.Add(48 * time.Hour).Unix(),
	}
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("second Init returned error: %v", err)
	}
	if len(gitConfig.unset) != 1 || gitConfig.unset[0] != "https://example.com/git/u/old.git" {
		t.Fatalf("git config unset = %#v, want old URL cleanup", gitConfig.unset)
	}
}

func TestManagerInitReportsOldURLCleanupWarning(t *testing.T) {
	now := time.Unix(1780000000, 0)
	issuer := &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/old.git",
		PAT:        "old-pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}}
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(newFakeKeychain()), splitFakeGitConfig{unsetErr: errors.New("unset failed")}, issuer)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("initial Init returned error: %v", err)
	}
	issuer.next = &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/new.git",
		PAT:        "new-pat",
		ExpiresAt:  now.Add(48 * time.Hour).Unix(),
	}
	result, err := manager.Init(context.Background(), testProfile(), "app_xxx")
	if err != nil {
		t.Fatalf("second Init returned error: %v", err)
	}
	if !strings.Contains(result.ConfigWarning, "unset failed") {
		t.Fatalf("ConfigWarning = %q, want unset warning", result.ConfigWarning)
	}
}

func TestManagerInitRemovesPreviousPATRefAfterLoginChanges(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	issuer := &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "old-pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}}
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), nil, issuer)
	manager.Now = func() time.Time { return now }
	oldProfile := testProfile()
	if _, err := manager.Init(context.Background(), oldProfile, "app_xxx"); err != nil {
		t.Fatalf("initial Init returned error: %v", err)
	}
	oldRef := BuildPATRef(oldProfile, "app_xxx")

	newProfile := ProfileContext{Profile: "default", ProfileAppID: "cli_xxx", UserOpenID: "ou_new"}
	issuer.next = &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "new-pat",
		ExpiresAt:  now.Add(48 * time.Hour).Unix(),
	}
	if _, err := manager.Init(context.Background(), newProfile, "app_xxx"); err != nil {
		t.Fatalf("second Init returned error: %v", err)
	}
	newRef := BuildPATRef(newProfile, "app_xxx")
	if got := kc.values[oldRef]; got != "" {
		t.Fatalf("old keychain PAT = %q, want removed", got)
	}
	if got := kc.values[newRef]; got != "new-pat" {
		t.Fatalf("new keychain PAT = %q, want new-pat", got)
	}
}

func TestManagerInitDoesNotTreatOtherAppRecordAsRefresh(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	storage := newFakeAppStorage()
	otherStore := NewAppStore("app_other", storage)
	otherRecord := CredentialRecord{
		AppID:      "app_other",
		GitHTTPURL: "https://example.com/git/u/other.git",
		PATRef:     "other-ref",
		Status:     StatusConfirmed,
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}
	if err := otherStore.Upsert(otherRecord); err != nil {
		t.Fatalf("seed other app record: %v", err)
	}
	kc.values[otherRecord.PATRef] = "other-pat"
	manager := NewManager(NewAppStore("app_xxx", storage), NewSecretStore(kc), nil, &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "app-pat",
		ExpiresAt:  now.Add(48 * time.Hour).Unix(),
	}})
	manager.Now = func() time.Time { return now }

	result, err := manager.Init(context.Background(), testProfile(), "app_xxx")
	if err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	if result.Refreshed {
		t.Fatalf("Init marked refreshed with only another app record present")
	}
	if got := kc.values[otherRecord.PATRef]; got != "other-pat" {
		t.Fatalf("other app PAT = %q, want untouched", got)
	}
	if record, err := otherStore.FindByURL(otherRecord.GitHTTPURL); err != nil || record == nil {
		t.Fatalf("other app record = %#v, %v; want untouched", record, err)
	}
}

func TestManagerGetRefreshesWithinTenMinutes(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	issuer := &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		Username:   "x-access-token",
		PAT:        "old-pat",
		ExpiresAt:  now.Add(9 * time.Minute).Unix(),
	}}
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), nil, issuer)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	issuer.next = &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		Username:   "x-access-token",
		PAT:        "new-pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}

	var out bytes.Buffer
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got := out.String(); got != "username=x-access-token\npassword=new-pat\n\n" {
		t.Fatalf("credential output = %q", got)
	}
	if issuer.calls != 2 {
		t.Fatalf("issuer calls = %d, want 2", issuer.calls)
	}
}

func TestManagerGetDoesNotReuseUnusableRecordWhenRefreshReturnsOlderExpiry(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	issuer := &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		Username:   "x-access-token",
		PAT:        "old-pat",
		ExpiresAt:  now.Add(5 * time.Minute).Unix(),
	}}
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), nil, issuer)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	issuer.next = &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		Username:   "x-access-token",
		PAT:        "older-pat",
		ExpiresAt:  now.Add(time.Minute).Unix(),
	}

	var out bytes.Buffer
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty because reread record is still not usable", out.String())
	}
	if issuer.calls != 2 {
		t.Fatalf("issuer calls = %d, want 2", issuer.calls)
	}
}

func TestManagerGetUsesValidPATWithoutRefresh(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	issuer := &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		Username:   "x-access-token",
		PAT:        "valid-pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}}
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), nil, issuer)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	var out bytes.Buffer
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got := out.String(); got != "username=x-access-token\npassword=valid-pat\n\n" {
		t.Fatalf("credential output = %q", got)
	}
	if issuer.calls != 1 {
		t.Fatalf("issuer calls = %d, want 1", issuer.calls)
	}
}

func TestManagerGetKeepsStdoutEmptyWhenRefreshFails(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	issuer := &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "old-pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}}
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), nil, issuer)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	record, err := manager.Store.FindByURL("https://example.com/git/u/app.git")
	if err != nil {
		t.Fatalf("FindByURL returned error: %v", err)
	}
	record.ExpiresAt = now.Add(-time.Minute).Unix()
	if err := manager.Store.Upsert(*record); err != nil {
		t.Fatalf("Upsert expired record returned error: %v", err)
	}
	samplePAT := testPublicSafeJoin("pat", "-sample")
	samplePassword := "sample-password"
	issuer.err = errs.NewAPIError(
		errs.SubtypeUnknown,
		"permission denied: "+
			testCredentialAssignment("token", samplePAT)+" "+
			testCredentialAssignment("password", samplePassword)+" "+
			testCredentialURLWithUserInfo("example.com/git/u/app.git", samplePAT),
	).WithHint("retry without " + testCredentialAssignment("token", samplePAT)).WithLogID("log_x")

	var out bytes.Buffer
	var errOut bytes.Buffer
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &out, &errOut); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
	stderr := errOut.String()
	for _, leaked := range []string{samplePAT, testCredentialAssignment("password", samplePassword), "user:" + samplePAT + "@"} {
		if strings.Contains(stderr, leaked) {
			t.Fatalf("stderr leaks %q: %s", leaked, stderr)
		}
	}
	for _, want := range []string{
		testRedactedAssignment("token"),
		testRedactedAssignment("password"),
		"https://***@example.com/git/u/app.git",
		"log_id=log_x",
	} {
		if !strings.Contains(stderr, want) {
			t.Fatalf("stderr missing %q in %s", want, stderr)
		}
	}
	if !bytes.Contains(errOut.Bytes(), []byte("lark-cli apps +git-credential-init --app-id app_xxx")) {
		t.Fatalf("stderr missing actionable hint: %q", errOut.String())
	}
}

func TestManagerGetKeepsStdoutEmptyOnLoginMismatch(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	issuer := &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}}
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), nil, issuer)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer
	other := ProfileContext{Profile: "work", ProfileAppID: "cli_other", UserOpenID: "ou_other"}
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, other, &out, &errOut); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
	if !bytes.Contains(errOut.Bytes(), []byte("current login does not match")) {
		t.Fatalf("stderr missing login mismatch: %q", errOut.String())
	}
}

func TestManagerGetAllowsProfileRenameForSameLogin(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), nil, &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		Username:   "x-access-token",
		PAT:        "pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}})
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	renamed := testProfile()
	renamed.Profile = "renamed-profile"
	var out bytes.Buffer
	var errOut bytes.Buffer
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, renamed, &out, &errOut); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got := out.String(); got != "username=x-access-token\npassword=pat\n\n" {
		t.Fatalf("credential output = %q", got)
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", errOut.String())
	}
}

func TestEraseMarksInvalidatedWithCooldown(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	issuer := &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}}
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), nil, issuer)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	input := "protocol=https\nhost=example.com\npath=/git/u/app.git\n\n"
	if err := manager.Erase(bytes.NewBufferString(input)); err != nil {
		t.Fatalf("Erase returned error: %v", err)
	}
	if err := manager.Erase(bytes.NewBufferString(input)); err != nil {
		t.Fatalf("second Erase returned error: %v", err)
	}
	if len(kc.removed) != 1 {
		t.Fatalf("removed count = %d, want 1", len(kc.removed))
	}
	record, err := manager.Store.FindByURL("https://example.com/git/u/app.git")
	if err != nil {
		t.Fatalf("FindByURL returned error: %v", err)
	}
	if record.InvalidatedAt == 0 || record.LastEraseAt == 0 {
		t.Fatalf("record was not invalidated: %#v", record)
	}
}

func TestEraseLockAndSecondReadBranches(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), nil, &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}})
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	blocker := filepath.Join(t.TempDir(), "config-blocker")
	if err := os.WriteFile(blocker, []byte("file"), 0600); err != nil {
		t.Fatalf("write config blocker: %v", err)
	}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", blocker)
	input := "protocol=https\nhost=example.com\npath=/git/u/app.git\n\n"
	if err := manager.Erase(bytes.NewBufferString(input)); err == nil || !strings.Contains(err.Error(), "create Git credential lock dir") {
		t.Fatalf("Erase lock error = %v", err)
	}
}

func TestEraseSecondReadMissingReturnsNil(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	record := CredentialRecord{
		AppID:        "app_xxx",
		GitHTTPURL:   "https://example.com/git/u/app.git",
		Profile:      "default",
		ProfileAppID: "cli_xxx",
		UserOpenID:   "ou_xxx",
		Username:     "x-access-token",
		PATRef:       "ref",
		Status:       StatusConfirmed,
		ExpiresAt:    now.Add(24 * time.Hour).Unix(),
	}
	data, err := json.Marshal(CredentialFile{Version: CurrentCredentialVersion, CredentialRecord: record})
	if err != nil {
		t.Fatalf("marshal credential file: %v", err)
	}
	manager := NewManager(NewAppStore("app_xxx", &sequenceAppStorage{reads: [][]byte{data, nil}}), NewSecretStore(kc), nil, nil)
	manager.Now = func() time.Time { return now }
	input := "protocol=https\nhost=example.com\npath=/git/u/app.git\n\n"
	if err := manager.Erase(bytes.NewBufferString(input)); err != nil {
		t.Fatalf("Erase second read missing returned error: %v", err)
	}
}

func TestStoreCredentialDrainsStdin(t *testing.T) {
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(newFakeKeychain()), nil, nil)
	if err := manager.StoreCredential(bytes.NewBufferString("protocol=https\nhost=example.com\n\n")); err != nil {
		t.Fatalf("StoreCredential returned error: %v", err)
	}
}

func TestRemoveDeletesMetadataSecretAndGitConfig(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	gitConfig := &fakeGitConfig{}
	issuer := &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}}
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), gitConfig, issuer)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	result, err := manager.Remove(context.Background(), testProfile(), "app_xxx")
	if err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}
	if !result.Removed || len(result.Records) != 1 {
		t.Fatalf("remove result = %#v", result)
	}
	if got := kc.values[result.Records[0].PATRef]; got != "" {
		t.Fatalf("keychain PAT after remove = %q, want empty", got)
	}
	if len(gitConfig.unset) != 1 || gitConfig.unset[0] != "https://example.com/git/u/app.git" {
		t.Fatalf("git config unset = %#v", gitConfig.unset)
	}
	record, err := manager.Store.FindByURL("https://example.com/git/u/app.git")
	if err != nil {
		t.Fatalf("FindByURL returned error: %v", err)
	}
	if record != nil {
		t.Fatalf("record after remove = %#v, want nil", record)
	}
}

func TestInitWorksAfterRemove(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	gitConfig := &fakeGitConfig{}
	issuer := &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "first-pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}}
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), gitConfig, issuer)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("initial Init returned error: %v", err)
	}
	if _, err := manager.Remove(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Remove returned error: %v", err)
	}

	issuer.next = &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "second-pat",
		ExpiresAt:  now.Add(48 * time.Hour).Unix(),
	}
	result, err := manager.Init(context.Background(), testProfile(), "app_xxx")
	if err != nil {
		t.Fatalf("Init after Remove returned error: %v", err)
	}
	if result.Refreshed {
		t.Fatalf("Init after Remove marked refreshed, want fresh init")
	}
	record, err := manager.Store.FindByURL("https://example.com/git/u/app.git")
	if err != nil {
		t.Fatalf("FindByURL returned error: %v", err)
	}
	if record == nil || record.Status != StatusConfirmed {
		t.Fatalf("record after re-init = %#v, want confirmed", record)
	}
	if got := kc.values[record.PATRef]; got != "second-pat" {
		t.Fatalf("PAT after re-init = %q, want second-pat", got)
	}
	if len(gitConfig.set) != 2 {
		t.Fatalf("git config set calls = %#v, want initial and re-init", gitConfig.set)
	}
	if len(gitConfig.unset) != 1 {
		t.Fatalf("git config unset calls = %#v, want remove cleanup", gitConfig.unset)
	}
}

func TestRemoveIgnoresCurrentProfileMismatch(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	gitConfig := &fakeGitConfig{}
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), gitConfig, &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}})
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}

	other := ProfileContext{Profile: "work", ProfileAppID: "other_cli", UserOpenID: "ou_other"}
	result, err := manager.Remove(context.Background(), other, "app_xxx")
	if err != nil {
		t.Fatalf("Remove with profile mismatch returned error: %v", err)
	}
	if !result.Removed {
		t.Fatalf("Remove with profile mismatch did not remove: %#v", result)
	}
}

func TestRemoveWithoutRecordDoesNotTouchKeychainOrGitConfig(t *testing.T) {
	kc := newFakeKeychain()
	gitConfig := &fakeGitConfig{}
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), gitConfig, nil)

	result, err := manager.Remove(context.Background(), testProfile(), "app_xxx")
	if err != nil {
		t.Fatalf("Remove without record returned error: %v", err)
	}
	if result.Removed {
		t.Fatalf("Remove without record marked removed: %#v", result)
	}
	if len(kc.removed) != 0 {
		t.Fatalf("keychain removals = %#v, want none", kc.removed)
	}
	if len(gitConfig.unset) != 0 {
		t.Fatalf("git config unsets = %#v, want none", gitConfig.unset)
	}
}

func TestListReportsCredentialStatuses(t *testing.T) {
	now := time.Unix(1780000000, 0)
	for _, tc := range []struct {
		name    string
		mutate  func(*CredentialRecord, *fakeKeychain)
		want    string
		expired bool
	}{
		{
			name: "valid",
			want: ListStatusValid,
		},
		{
			name: "expired",
			mutate: func(record *CredentialRecord, kc *fakeKeychain) {
				record.ExpiresAt = now.Add(-time.Minute).Unix()
			},
			want:    ListStatusExpired,
			expired: true,
		},
		{
			name: "invalidated",
			mutate: func(record *CredentialRecord, kc *fakeKeychain) {
				record.InvalidatedAt = now.Unix()
			},
			want: ListStatusInvalidated,
		},
		{
			name: "missing-secret",
			mutate: func(record *CredentialRecord, kc *fakeKeychain) {
				delete(kc.values, record.PATRef)
			},
			want: ListStatusMissingSecret,
		},
		{
			name: "incomplete",
			mutate: func(record *CredentialRecord, kc *fakeKeychain) {
				record.Status = StatusPending
				record.PATRef = ""
			},
			want: ListStatusIncomplete,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			kc := newFakeKeychain()
			manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), nil, nil)
			manager.Now = func() time.Time { return now }
			record := CredentialRecord{
				AppID:        "app_xxx",
				GitHTTPURL:   "https://example.com/git/u/app.git",
				Profile:      "default",
				ProfileAppID: "cli_xxx",
				UserOpenID:   "ou_xxx",
				Username:     "x-access-token",
				PATRef:       "ref",
				Status:       StatusConfirmed,
				ExpiresAt:    now.Add(time.Hour).Unix(),
				UpdatedAt:    now.Unix(),
			}
			kc.values[record.PATRef] = "pat"
			if tc.mutate != nil {
				tc.mutate(&record, kc)
			}
			if err := manager.Store.Upsert(record); err != nil {
				t.Fatalf("Upsert returned error: %v", err)
			}

			result, err := manager.List()
			if err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if len(result.Records) != 1 {
				t.Fatalf("records = %#v, want one", result.Records)
			}
			got := result.Records[0]
			if got.Status != tc.want || got.Expired != tc.expired {
				t.Fatalf("list record = %#v, want status=%s expired=%v", got, tc.want, tc.expired)
			}
			if got.AppID != record.AppID || got.GitHTTPURL != record.GitHTTPURL || got.ProfileAppID != record.ProfileAppID || got.UserOpenID != record.UserOpenID {
				t.Fatalf("list record lost metadata: %#v", got)
			}
		})
	}
}

func TestListReturnsStoreError(t *testing.T) {
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(newFakeKeychain()), nil, nil)
	if err := os.WriteFile(manager.Store.Path(), []byte("{bad json"), 0600); err != nil {
		t.Fatalf("write invalid metadata: %v", err)
	}
	if _, err := manager.List(); err == nil || !strings.Contains(err.Error(), "invalid git.json") {
		t.Fatalf("List store error = %v", err)
	}
}

func TestGlobalGitConfigSetAndUnsetHelper(t *testing.T) {
	logPath := installFakeGit(t, 0)
	cfg := GlobalGitConfig{HelperCommand: "!custom-helper"}
	ctx := context.Background()

	if err := cfg.SetHelper(ctx, "https://example.com/git/u/app.git", "app_xxx"); err != nil {
		t.Fatalf("SetHelper returned error: %v", err)
	}
	if err := cfg.UnsetHelper(ctx, "https://example.com/git/u/app.git"); err != nil {
		t.Fatalf("UnsetHelper returned error: %v", err)
	}

	log := readFileString(t, logPath)
	for _, want := range []string{
		"config --global credential.https://example.com/git/u/app.git.helper !custom-helper",
		"config --global credential.https://example.com/git/u/app.git.useHttpPath true",
		"config --global --unset credential.https://example.com/git/u/app.git.helper",
		"config --global --unset credential.https://example.com/git/u/app.git.useHttpPath",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("git log missing %q in:\n%s", want, log)
		}
	}
}

func TestGlobalGitConfigNormalizesCredentialKeyURL(t *testing.T) {
	logPath := installFakeGit(t, 0)
	cfg := GlobalGitConfig{HelperCommand: "!custom-helper"}
	rawURL := "HTTPS://[2001:DB8::1]:443//repo.git?x=1"

	if err := cfg.SetHelper(context.Background(), rawURL, "app_xxx"); err != nil {
		t.Fatalf("SetHelper returned error: %v", err)
	}
	if err := cfg.UnsetHelper(context.Background(), rawURL); err != nil {
		t.Fatalf("UnsetHelper returned error: %v", err)
	}

	log := readFileString(t, logPath)
	for _, want := range []string{
		"config --global credential.https://[2001:db8::1]/repo.git.helper !custom-helper",
		"config --global credential.https://[2001:db8::1]/repo.git.useHttpPath true",
		"config --global --unset credential.https://[2001:db8::1]/repo.git.helper",
		"config --global --unset credential.https://[2001:db8::1]/repo.git.useHttpPath",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("git log missing normalized key %q in:\n%s", want, log)
		}
	}
}

func TestGlobalGitConfigRollsBackHelperWhenUseHttpPathFails(t *testing.T) {
	logPath := installFakeGit(t, 7)
	err := (GlobalGitConfig{}).SetHelper(context.Background(), "https://example.com/git/u/app.git", "app_xxx")
	if err == nil {
		t.Fatal("SetHelper returned nil error, want git failure")
	}
	log := readFileString(t, logPath)
	if !strings.Contains(log, "config --global --unset credential.https://example.com/git/u/app.git.helper") {
		t.Fatalf("git log missing rollback unset:\n%s", log)
	}
}

func TestGlobalGitConfigQuotesDefaultHelperAppID(t *testing.T) {
	logPath := installFakeGit(t, 0)
	appID := "app_xxx; touch /tmp/pwned"
	if err := (GlobalGitConfig{}).SetHelper(context.Background(), "https://example.com/git/u/app.git", appID); err != nil {
		t.Fatalf("SetHelper returned error: %v", err)
	}
	log := readFileString(t, logPath)
	want := "helper !lark-cli apps git-credential-helper --app-id 'app_xxx; touch /tmp/pwned'"
	if !strings.Contains(log, want) {
		t.Fatalf("git log missing quoted helper %q in:\n%s", want, log)
	}
}

func TestGlobalGitConfigReturnsFirstGitCommandError(t *testing.T) {
	installAlwaysFailingGit(t)
	err := (GlobalGitConfig{}).SetHelper(context.Background(), "https://example.com/git/u/app.git", "app_xxx")
	if err == nil {
		t.Fatal("SetHelper returned nil error, want first git command failure")
	}
}

func TestGlobalGitConfigUnsetReportsUnexpectedErrors(t *testing.T) {
	installAlwaysFailingGit(t)
	err := (GlobalGitConfig{}).UnsetHelper(context.Background(), "https://example.com/git/u/app.git")
	if err == nil || !strings.Contains(err.Error(), "get credential.https://example.com/git/u/app.git.helper") {
		t.Fatalf("UnsetHelper error = %v", err)
	}
}

func TestGlobalGitConfigDoesNotOverwriteOrUnsetNonLarkHelper(t *testing.T) {
	logPath := installFakeGitWithGet(t, "!other-helper")
	cfg := GlobalGitConfig{}
	err := cfg.SetHelper(context.Background(), "https://example.com/git/u/app.git", "app_xxx")
	if err == nil || !strings.Contains(err.Error(), "refusing to overwrite non-lark helper") {
		t.Fatalf("SetHelper error = %v", err)
	}
	if err := cfg.UnsetHelper(context.Background(), "https://example.com/git/u/app.git"); err != nil {
		t.Fatalf("UnsetHelper returned error: %v", err)
	}
	log := readFileString(t, logPath)
	for _, unwanted := range []string{
		"credential.https://example.com/git/u/app.git.helper !lark-cli",
		"--unset credential.https://example.com/git/u/app.git.helper",
		"--unset credential.https://example.com/git/u/app.git.useHttpPath",
	} {
		if strings.Contains(log, unwanted) {
			t.Fatalf("git log contains unwanted %q in:\n%s", unwanted, log)
		}
	}
}

func TestGlobalGitConfigUnsetIgnoresMissingManagedKeys(t *testing.T) {
	logPath := installFakeGitWithGetAndUnsetExit(t, "!lark-cli apps git-credential-helper --app-id app_xxx", 5)
	if err := (GlobalGitConfig{}).UnsetHelper(context.Background(), "https://example.com/git/u/app.git"); err != nil {
		t.Fatalf("UnsetHelper returned error: %v", err)
	}
	log := readFileString(t, logPath)
	for _, want := range []string{
		"config --global --unset credential.https://example.com/git/u/app.git.helper",
		"config --global --unset credential.https://example.com/git/u/app.git.useHttpPath",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("git log missing %q in:\n%s", want, log)
		}
	}
}

func TestGlobalGitConfigAdditionalBranches(t *testing.T) {
	if err := (GlobalGitConfig{}).SetHelper(context.Background(), "ssh://example.com/git/u/app.git", "app_xxx"); err == nil {
		t.Fatal("SetHelper invalid URL returned nil error")
	}
	if err := (GlobalGitConfig{}).UnsetHelper(context.Background(), "ssh://example.com/git/u/app.git"); err == nil {
		t.Fatal("UnsetHelper invalid URL returned nil error")
	}

	if err := (GlobalGitConfig{}).SetHelper(context.Background(), "https://example.com/git/u/app.git", "../bad"); err == nil {
		t.Fatal("SetHelper invalid appID returned nil error")
	}

	installFakeGitSetFails(t)
	if err := (GlobalGitConfig{}).SetHelper(context.Background(), "https://example.com/git/u/app.git", "app_xxx"); err == nil {
		t.Fatal("SetHelper set failure returned nil error")
	}

	logPath := installFakeGitWithGetAndUseHTTPPathFailure(t, "!lark-cli apps git-credential-helper --app-id old_app", 7)
	if err := (GlobalGitConfig{}).SetHelper(context.Background(), "https://example.com/git/u/app.git", "app_xxx"); err == nil {
		t.Fatal("SetHelper useHttpPath failure returned nil error")
	}
	log := readFileString(t, logPath)
	if !strings.Contains(log, "config --global credential.https://example.com/git/u/app.git.helper !lark-cli apps git-credential-helper --app-id old_app") {
		t.Fatalf("git log missing previous helper restore:\n%s", log)
	}

	installFakeGitWithGetAndUnsetExit(t, "!lark-cli apps git-credential-helper --app-id app_xxx", 9)
	if err := (GlobalGitConfig{}).UnsetHelper(context.Background(), "https://example.com/git/u/app.git"); err == nil || !strings.Contains(err.Error(), "unset credential.https://example.com/git/u/app.git.helper") {
		t.Fatalf("UnsetHelper helper unset error = %v", err)
	}

	logPath = installFakeGitWithGetAndSecondUnsetFails(t, "!lark-cli apps git-credential-helper --app-id app_xxx")
	if err := (GlobalGitConfig{}).UnsetHelper(context.Background(), "https://example.com/git/u/app.git"); err == nil || !strings.Contains(err.Error(), "unset credential.https://example.com/git/u/app.git.useHttpPath") {
		t.Fatalf("UnsetHelper useHttpPath unset error = %v", err)
	}
	if !strings.Contains(readFileString(t, logPath), "--unset credential.https://example.com/git/u/app.git.useHttpPath") {
		t.Fatalf("git log missing useHttpPath unset:\n%s", readFileString(t, logPath))
	}

	cfg := GlobalGitConfig{HelperCommand: "!custom-helper"}
	if !cfg.isManagedHelper(" !custom-helper ") {
		t.Fatal("custom helper should be managed")
	}
	if cfg.isManagedHelper("!other-helper") {
		t.Fatal("other helper should not be managed")
	}
	if isGitConfigUnsetMissing(errors.New("plain error")) {
		t.Fatal("plain error must not be treated as missing git config")
	}
}

func TestStoreLoadSaveAndQueryBranches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, MetadataFilename)
	store := NewStoreAt(path)
	if store.Path() != path {
		t.Fatalf("Path() = %q", store.Path())
	}
	empty, err := store.Load()
	if err != nil {
		t.Fatalf("Load missing file returned error: %v", err)
	}
	if empty.Version != CurrentCredentialVersion || empty.GitHTTPURL != "" {
		t.Fatalf("empty file = %#v", empty)
	}
	if err := store.Save(nil); err != nil {
		t.Fatalf("Save(nil) returned error: %v", err)
	}
	file, err := store.Load()
	if err != nil {
		t.Fatalf("Load after Save(nil) returned error: %v", err)
	}
	if file.Version != CurrentCredentialVersion {
		t.Fatalf("Version after Save(nil) = %d", file.Version)
	}
	if err := os.WriteFile(path, []byte{}, 0600); err != nil {
		t.Fatalf("write empty metadata: %v", err)
	}
	empty, err = store.Load()
	if err != nil {
		t.Fatalf("Load empty file returned error: %v", err)
	}
	if empty.Version != CurrentCredentialVersion {
		t.Fatalf("empty file version = %d", empty.Version)
	}
	emptyRecords, err := store.Records()
	if err != nil {
		t.Fatalf("Records empty file returned error: %v", err)
	}
	if len(emptyRecords) != 0 {
		t.Fatalf("empty records = %#v, want none", emptyRecords)
	}

	recordB := CredentialRecord{AppID: "app_a", GitHTTPURL: "https://example.com/git/a.git", Profile: "default", ProfileAppID: "cli", UserOpenID: "ou", Status: StatusConfirmed}
	recordC := CredentialRecord{AppID: "app_a", GitHTTPURL: "https://example.com/git/c.git", Profile: "default", ProfileAppID: "cli", UserOpenID: "ou", Status: StatusConfirmed}
	if err := store.Upsert(recordB); err != nil {
		t.Fatalf("Upsert B returned error: %v", err)
	}
	if err := store.Upsert(recordC); err != nil {
		t.Fatalf("Upsert C returned error: %v", err)
	}
	records, err := store.Records()
	if err != nil {
		t.Fatalf("Records returned error: %v", err)
	}
	if len(records) != 1 || records[0].GitHTTPURL != recordC.GitHTTPURL {
		t.Fatalf("records = %#v, want latest app-scoped record", records)
	}
	matches, err := store.FindByAppID("app_a", ProfileContext{Profile: "default", ProfileAppID: "cli", UserOpenID: "ou"})
	if err != nil {
		t.Fatalf("FindByAppID returned error: %v", err)
	}
	if len(matches) != 1 || matches[0].GitHTTPURL != recordC.GitHTTPURL {
		t.Fatalf("matches = %#v", matches)
	}
	matches, err = store.FindByAppID("app_a", ProfileContext{Profile: "work"})
	if err != nil {
		t.Fatalf("FindByAppID with profile mismatch returned error: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("profile mismatch matches = %#v, want empty", matches)
	}
	matches, err = store.FindByAppID("app_other", ProfileContext{})
	if err != nil {
		t.Fatalf("FindByAppID app mismatch returned error: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("app mismatch matches = %#v, want empty", matches)
	}
	for _, profile := range []ProfileContext{
		{Profile: "default", ProfileAppID: "other", UserOpenID: "ou"},
		{Profile: "default", ProfileAppID: "cli", UserOpenID: "other"},
	} {
		matches, err = store.FindByAppID("app_a", profile)
		if err != nil {
			t.Fatalf("FindByAppID mismatch returned error: %v", err)
		}
		if len(matches) != 0 {
			t.Fatalf("FindByAppID mismatch %#v returned %#v, want empty", profile, matches)
		}
	}
	deleted, err := store.DeleteByURL("https://example.com/git/missing.git")
	if err != nil || deleted != nil {
		t.Fatalf("DeleteByURL missing = %#v, %v; want nil, nil", deleted, err)
	}
	deleted, err = store.DeleteByURL(recordC.GitHTTPURL)
	if err != nil {
		t.Fatalf("DeleteByURL returned error: %v", err)
	}
	if deleted == nil || deleted.AppID != recordC.AppID {
		t.Fatalf("deleted = %#v", deleted)
	}
	if _, err := store.Records(); err != nil {
		t.Fatalf("Records after delete returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte("{bad json"), 0600); err != nil {
		t.Fatalf("write invalid metadata: %v", err)
	}
	if _, err := store.Records(); err == nil {
		t.Fatal("Records invalid metadata returned nil error")
	}
	if _, err := store.FindByAppID("app_a", ProfileContext{}); err == nil {
		t.Fatal("FindByAppID invalid metadata returned nil error")
	}
}

func TestAppStoreUsesAppScopedStorage(t *testing.T) {
	storage := newFakeAppStorage()
	store := NewAppStore("app_xxx", storage)
	if got := store.Path(); got != "apps:app_xxx/"+MetadataFilename {
		t.Fatalf("Path() = %q, want app-scoped path", got)
	}
	empty, err := store.Load()
	if err != nil {
		t.Fatalf("Load missing app storage returned error: %v", err)
	}
	if empty.Version != CurrentCredentialVersion {
		t.Fatalf("empty version = %d, want %d", empty.Version, CurrentCredentialVersion)
	}
	record := CredentialRecord{AppID: "app_xxx", GitHTTPURL: "https://example.com/git/u/app.git", Profile: "default", ProfileAppID: "cli_xxx", UserOpenID: "ou_xxx", Status: StatusConfirmed}
	if err := store.Upsert(record); err != nil {
		t.Fatalf("Upsert app storage returned error: %v", err)
	}
	if storage.values["app_xxx/"+MetadataFilename] == nil {
		t.Fatalf("app storage missing metadata key")
	}
	records, err := store.Records()
	if err != nil {
		t.Fatalf("Records app storage returned error: %v", err)
	}
	if len(records) != 1 || records[0].GitHTTPURL != record.GitHTTPURL {
		t.Fatalf("records = %#v, want stored record", records)
	}
	deleted, err := store.DeleteByURL(record.GitHTTPURL)
	if err != nil {
		t.Fatalf("DeleteByURL app storage returned error: %v", err)
	}
	if deleted == nil || deleted.GitHTTPURL != record.GitHTTPURL {
		t.Fatalf("deleted = %#v, want stored record", deleted)
	}
	if storage.values["app_xxx/"+MetadataFilename] != nil {
		t.Fatalf("app storage metadata still present after delete")
	}
}

func TestNewStoreUsesConfigDir(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", configDir)
	if got := NewStore().Path(); got != filepath.Join(configDir, MetadataFilename) {
		t.Fatalf("NewStore path = %q", got)
	}
}

func TestStoreLoadRejectsInvalidAndNewerVersions(t *testing.T) {
	path := filepath.Join(t.TempDir(), MetadataFilename)
	if err := os.WriteFile(path, []byte("{bad json"), 0600); err != nil {
		t.Fatalf("write invalid json: %v", err)
	}
	if _, err := NewStoreAt(path).Load(); err == nil {
		t.Fatal("Load invalid json returned nil error")
	} else {
		var cfgErr *errs.ConfigError
		if !errors.As(err, &cfgErr) {
			t.Fatalf("Load invalid json error = %T %v, want ConfigError", err, err)
		}
	}
	if err := os.WriteFile(path, []byte(`{"version":99,"credentials":{}}`), 0600); err != nil {
		t.Fatalf("write newer version: %v", err)
	}
	if _, err := NewStoreAt(path).Load(); err == nil {
		t.Fatal("Load newer version returned nil error")
	}
	if err := os.WriteFile(path, []byte(`{"credentials":null}`), 0600); err != nil {
		t.Fatalf("write version 0: %v", err)
	}
	file, err := NewStoreAt(path).Load()
	if err != nil {
		t.Fatalf("Load version 0 returned error: %v", err)
	}
	if file.Version != CurrentCredentialVersion {
		t.Fatalf("version 0 upgrade = %#v", file)
	}
	if _, err := NewStoreAt(t.TempDir()).Load(); err == nil {
		t.Fatal("Load directory path returned nil error")
	}
	if err := NewStoreAt(path).Upsert(CredentialRecord{GitHTTPURL: "https://example.com/repo.git"}); err != nil {
		t.Fatalf("Upsert after version 0 returned error: %v", err)
	}
	if err := os.WriteFile(path, []byte("{bad json"), 0600); err != nil {
		t.Fatalf("rewrite invalid json: %v", err)
	}
	if err := NewStoreAt(path).Upsert(CredentialRecord{GitHTTPURL: "https://example.com/repo.git"}); err == nil {
		t.Fatal("Upsert invalid json returned nil error")
	}
	if _, err := NewStoreAt(path).DeleteByURL("https://example.com/repo.git"); err == nil {
		t.Fatal("DeleteByURL invalid json returned nil error")
	}
}

func TestStoreSaveReturnsMkdirError(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "blocker")
	if err := os.WriteFile(blocker, []byte("file"), 0600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	store := NewStoreAt(filepath.Join(blocker, MetadataFilename))
	if err := store.Save(&CredentialFile{}); err == nil {
		t.Fatal("Save returned nil error, want mkdir error")
	}
}

func TestNormalizeGitHTTPURLBranches(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "empty", raw: " ", wantErr: true},
		{name: "bad parse", raw: "https://%zz", wantErr: true},
		{name: "unsupported", raw: "ssh://example.com/repo.git", wantErr: true},
		{name: "empty host", raw: "https:///repo.git", wantErr: true},
		{name: "http default port", raw: "http://EXAMPLE.com:80/repo.git/", want: "http://example.com/repo.git"},
		{name: "custom port", raw: "https://Example.com:8443//repo.git?x=1", want: "https://example.com:8443/repo.git"},
		{name: "ipv6 default port", raw: "HTTPS://[2001:DB8::1]:443//repo.git", want: "https://[2001:db8::1]/repo.git"},
		{name: "ipv6 custom port", raw: "https://[2001:db8::1]:8443/repo.git", want: "https://[2001:db8::1]:8443/repo.git"},
		{name: "root path", raw: "https://Example.com", want: "https://example.com/"},
	}
	for _, tt := range tests {
		got, err := NormalizeGitHTTPURL(tt.raw)
		if tt.wantErr {
			if err == nil {
				t.Fatalf("%s: NormalizeGitHTTPURL returned nil error", tt.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: NormalizeGitHTTPURL returned error: %v", tt.name, err)
		}
		if got != tt.want {
			t.Fatalf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
	if got := cleanURLPath("relative/path"); got != "/relative/path" {
		t.Fatalf("cleanURLPath(relative/path) = %q", got)
	}
	if got := cleanURLPath("/%zz"); got != "/%zz" {
		t.Fatalf("cleanURLPath(/%%zz) = %q", got)
	}
	if got := normalizeHostname("[example.com]"); got != "[example.com]" {
		t.Fatalf("normalizeHostname([example.com]) = %q", got)
	}
	if got := normalizeHostname("[2001:db8::1]"); got != "[2001:db8::1]" {
		t.Fatalf("normalizeHostname([2001:db8::1]) = %q", got)
	}
	got, err := normalizeParsedURL(&url.URL{Scheme: "https", Host: "example.com", Path: ".."})
	if err != nil {
		t.Fatalf("normalizeParsedURL dot path returned error: %v", err)
	}
	if got != "https://example.com/" {
		t.Fatalf("normalizeParsedURL dot path = %q", got)
	}
}

func TestNormalizeCredentialInputRequiresProtocolAndHost(t *testing.T) {
	if _, err := NormalizeCredentialInput(CredentialInput{Protocol: "https"}); err == nil {
		t.Fatal("NormalizeCredentialInput returned nil error for missing host")
	}
}

func TestSecretStoreBranches(t *testing.T) {
	if got, err := (*SecretStore)(nil).Get("ref"); err != nil || got != "" {
		t.Fatalf("nil SecretStore Get = %q, %v", got, err)
	}
	if err := (*SecretStore)(nil).Remove("ref"); err != nil {
		t.Fatalf("nil SecretStore Remove returned error: %v", err)
	}
	kc := newFakeKeychain()
	if got, err := NewSecretStore(kc).Get(""); err != nil || got != "" {
		t.Fatalf("empty SecretStore Get = %q, %v", got, err)
	}
	if err := NewSecretStore(kc).Remove(""); err != nil {
		t.Fatalf("empty SecretStore Remove returned error: %v", err)
	}
	if len(kc.removed) != 0 {
		t.Fatalf("keychain removals for empty ref = %#v, want none", kc.removed)
	}
	if err := NewSecretStore(nil).Remove("ref"); err == nil {
		t.Fatal("nil keychain SecretStore Remove returned nil error")
	}
	if got, err := NewSecretStore(nil).Get("ref"); err != nil || got != "" {
		t.Fatalf("nil keychain SecretStore Get = %q, %v", got, err)
	}
	if err := NewSecretStore(newFakeKeychain()).Set("", "pat"); err == nil {
		t.Fatal("SecretStore.Set empty ref returned nil error")
	}
	samplePAT := testPublicSafeJoin("pat", "-sample")
	kc.setErr = errors.New("keychain set failed with " + testCredentialAssignment("token", samplePAT))
	var setCfgErr *errs.ConfigError
	setErr := NewSecretStore(kc).Set("ref", samplePAT)
	if setErr == nil || !errors.As(setErr, &setCfgErr) {
		t.Fatalf("SecretStore.Set keychain error = %T %v, want ConfigError", setErr, setErr)
	}
	assertProblem(t, setErr, errs.CategoryConfig, errs.SubtypeInvalidConfig)
	if setCfgErr.Message != "save local Git credential PAT to keychain failed" {
		t.Fatalf("ConfigError message = %q, want static keychain failure", setCfgErr.Message)
	}
	if strings.Contains(setCfgErr.Message, samplePAT) {
		t.Fatalf("ConfigError message leaks credential value: %q", setCfgErr.Message)
	}
	if !errors.Is(setCfgErr, kc.setErr) {
		t.Fatalf("ConfigError does not preserve keychain cause")
	}
	kc.setErr = nil
	kc.removeErr = errors.New("keychain remove failed")
	var cfgErr *errs.ConfigError
	removeErr := NewSecretStore(kc).Remove("ref")
	if removeErr == nil || !errors.As(removeErr, &cfgErr) {
		t.Fatalf("SecretStore.Remove keychain error = %T %v, want ConfigError", removeErr, removeErr)
	}
	assertProblem(t, removeErr, errs.CategoryConfig, errs.SubtypeInvalidConfig)
	if cfgErr.Message != "remove local Git credential PAT from keychain failed" {
		t.Fatalf("ConfigError message = %q, want static keychain failure", cfgErr.Message)
	}
	if !errors.Is(cfgErr, kc.removeErr) {
		t.Fatalf("ConfigError does not preserve keychain cause")
	}
}

func TestManagerInitValidationAndIssuerErrors(t *testing.T) {
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(newFakeKeychain()), nil, nil)
	if _, err := manager.Init(context.Background(), testProfile(), " "); err == nil {
		t.Fatal("Init empty appID returned nil error")
	}
	if _, err := manager.Init(context.Background(), testProfile(), "../bad"); err == nil {
		t.Fatal("Init invalid appID returned nil error")
	}
	if _, err := manager.Init(context.Background(), ProfileContext{}, "app_xxx"); err == nil {
		t.Fatal("Init without login returned nil error")
	}
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err == nil {
		t.Fatal("Init without issuer returned nil error")
	}

	issuer := &fakeIssuer{err: errors.New("api down")}
	manager.Issuer = issuer
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err == nil {
		t.Fatal("Init with issuer error returned nil error")
	}
	issuer.err = nil
	issuer.next = &IssuedCredential{GitHTTPURL: "ssh://example.com/repo.git", PAT: "pat"}
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err == nil {
		t.Fatal("Init invalid URL returned nil error")
	}
	issuer.next = &IssuedCredential{AppID: "app_other", GitHTTPURL: "https://example.com/repo.git", PAT: "pat", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err == nil {
		t.Fatal("Init mismatched response app_id returned nil error")
	}
	issuer.next = &IssuedCredential{GitHTTPURL: "https://example.com/repo.git", PAT: "pat", ExpiresAt: time.Now().Add(-time.Hour).Unix()}
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err == nil {
		t.Fatal("Init expired credential response returned nil error")
	}
	issuer.next = &IssuedCredential{GitHTTPURL: "https://example.com/repo.git", ExpiresAt: time.Now().Add(time.Hour).Unix()}
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err == nil || !strings.Contains(err.Error(), "response missing token") {
		t.Fatalf("Init empty PAT response error = %v", err)
	}
	if err := validateIssuedCredential("app_xxx", "", &IssuedCredential{GitHTTPURL: "https://example.com/repo.git", PAT: "pat", ExpiresAt: time.Now().Add(time.Hour).Unix()}, time.Now().Unix()); err == nil {
		t.Fatal("validateIssuedCredential missing normalized URL returned nil")
	}
	if err := validateIssuedCredential("app_xxx", "https://example.com/repo.git", nil, time.Now().Unix()); err == nil {
		t.Fatal("validateIssuedCredential nil issued returned nil")
	}
}

func TestManagerInitAndRemoveLockFailures(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "config-blocker")
	if err := os.WriteFile(blocker, []byte("file"), 0600); err != nil {
		t.Fatalf("write config blocker: %v", err)
	}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", blocker)
	now := time.Unix(1780000000, 0)
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(newFakeKeychain()), nil, &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}})
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err == nil || !strings.Contains(err.Error(), "create Git credential lock dir") {
		t.Fatalf("Init lock error = %v", err)
	}
	if _, err := manager.Remove(context.Background(), testProfile(), "app_xxx"); err == nil || !strings.Contains(err.Error(), "create Git credential lock dir") {
		t.Fatalf("Remove lock error = %v", err)
	}
}

func TestLockAppHeldTimesOut(t *testing.T) {
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", t.TempDir())
	unlock, err := lockApp("held/app")
	if err != nil {
		t.Fatalf("initial lockApp returned error: %v", err)
	}
	defer unlock()
	if _, err := lockApp("held/app"); err == nil {
		t.Fatal("second lockApp returned nil error, want held lock timeout")
	}
}

func TestManagerGetPreservesTypedLockAppError(t *testing.T) {
	now := time.Unix(1780000000, 0)
	store := NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename))
	kc := newFakeKeychain()
	record := CredentialRecord{
		AppID:        "app_xxx",
		GitHTTPURL:   "https://example.com/git/u/app.git",
		Profile:      testProfile().Profile,
		ProfileAppID: testProfile().ProfileAppID,
		UserOpenID:   testProfile().UserOpenID,
		Username:     "x-access-token",
		PATRef:       "ref",
		Status:       StatusConfirmed,
		ExpiresAt:    now.Add(-time.Minute).Unix(),
		UpdatedAt:    now.Unix(),
	}
	if err := store.Upsert(record); err != nil {
		t.Fatalf("Upsert returned error: %v", err)
	}
	kc.values[record.PATRef] = "old-pat"

	blocker := filepath.Join(t.TempDir(), "config-blocker")
	if err := os.WriteFile(blocker, []byte("file"), 0600); err != nil {
		t.Fatalf("write config blocker: %v", err)
	}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", blocker)

	manager := NewManager(store, NewSecretStore(kc), nil, &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: record.GitHTTPURL,
		PAT:        "new-pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}})
	manager.Now = func() time.Time { return now }
	var out bytes.Buffer
	var errOut bytes.Buffer
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &out, &errOut); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", out.String())
	}
	stderr := errOut.String()
	if !strings.Contains(stderr, "create Git credential lock dir") {
		t.Fatalf("stderr = %q, want typed lock-dir setup error", stderr)
	}
	if strings.Contains(stderr, "acquire Git credential lock") {
		t.Fatalf("stderr rewrapped typed lock error: %q", stderr)
	}
}

func TestManagerInitStoreAndSecretReadErrors(t *testing.T) {
	now := time.Unix(1780000000, 0)
	path := filepath.Join(t.TempDir(), MetadataFilename)
	if err := os.WriteFile(path, []byte("{bad json"), 0600); err != nil {
		t.Fatalf("write invalid metadata: %v", err)
	}
	manager := NewManager(NewStoreAt(path), NewSecretStore(newFakeKeychain()), nil, &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}})
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err == nil {
		t.Fatal("Init with unreadable metadata returned nil error")
	}

	kc := newFakeKeychain()
	manager = NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), nil, &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "old-pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}})
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("initial Init returned error: %v", err)
	}
	kc.getErr = errors.New("keychain get failed")
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init should repair existing credential when old secret cannot be read, got %v", err)
	}
}

func TestManagerInitPendingWriteError(t *testing.T) {
	now := time.Unix(1780000000, 0)
	dir := t.TempDir()
	path := filepath.Join(dir, MetadataFilename)
	if err := NewStoreAt(path).Save(&CredentialFile{}); err != nil {
		t.Fatalf("Save seed metadata returned error: %v", err)
	}
	makeDirReadOnly(t, dir)
	manager := NewManager(NewStoreAt(path), NewSecretStore(newFakeKeychain()), nil, &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}})
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err == nil {
		t.Fatal("Init with pending metadata write error returned nil")
	}
}

func TestManagerInitConfirmedWriteErrorRollsBackSecret(t *testing.T) {
	now := time.Unix(1780000000, 0)
	dir := t.TempDir()
	path := filepath.Join(dir, MetadataFilename)
	kc := newFakeKeychain()
	kc.onSet = func(account, value string) {
		if value == "new-pat" {
			makeDirReadOnly(t, dir)
		}
	}
	manager := NewManager(NewStoreAt(path), NewSecretStore(kc), nil, &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "new-pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}})
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err == nil {
		t.Fatal("Init with confirmed metadata write error returned nil")
	}
	if len(kc.removed) != 1 {
		t.Fatalf("removed PAT refs = %#v, want rollback removal", kc.removed)
	}
}

func TestManagerInitConfirmedWriteErrorRestoresExistingSecret(t *testing.T) {
	now := time.Unix(1780000000, 0)
	dir := t.TempDir()
	path := filepath.Join(dir, MetadataFilename)
	kc := newFakeKeychain()
	issuer := &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "old-pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}}
	manager := NewManager(NewStoreAt(path), NewSecretStore(kc), nil, issuer)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("initial Init returned error: %v", err)
	}
	record, err := manager.Store.FindByURL("https://example.com/git/u/app.git")
	if err != nil {
		t.Fatalf("FindByURL returned error: %v", err)
	}
	kc.onSet = func(account, value string) {
		if value == "new-pat" {
			makeDirReadOnly(t, dir)
		}
	}
	issuer.next = &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "new-pat",
		ExpiresAt:  now.Add(48 * time.Hour).Unix(),
	}
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err == nil {
		t.Fatal("refresh Init with confirmed metadata write error returned nil")
	}
	if got := kc.values[record.PATRef]; got != "old-pat" {
		t.Fatalf("restored PAT = %q, want old-pat", got)
	}
}

func TestManagerRemoveValidationNoMatchAndErrors(t *testing.T) {
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(newFakeKeychain()), nil, nil)
	if _, err := manager.Remove(context.Background(), testProfile(), " "); err == nil {
		t.Fatal("Remove empty appID returned nil error")
	}
	if _, err := manager.Remove(context.Background(), testProfile(), "../bad"); err == nil {
		t.Fatal("Remove invalid appID returned nil error")
	}
	result, err := manager.Remove(context.Background(), testProfile(), "app_missing")
	if err != nil {
		t.Fatalf("Remove missing returned error: %v", err)
	}
	if result.Removed || len(result.Records) != 0 {
		t.Fatalf("Remove missing result = %#v", result)
	}

	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	gitConfig := &fakeGitConfig{err: errors.New("git config locked")}
	manager = NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), gitConfig, &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}})
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	result, err = manager.Remove(context.Background(), testProfile(), "app_xxx")
	if err != nil {
		t.Fatalf("Remove with git config warning returned error: %v", err)
	}
	if result == nil || !result.Removed || !strings.Contains(result.ConfigWarning, "git config locked") {
		t.Fatalf("Remove result = %#v, want removed with config warning", result)
	}
	record, err := manager.Store.Current()
	if err != nil {
		t.Fatalf("Current after remove with config warning returned error: %v", err)
	}
	if record != nil {
		t.Fatalf("metadata should be removed despite git config cleanup warning, got %#v", record)
	}

	kc = newFakeKeychain()
	kc.removeErr = errors.New("keychain remove failed")
	manager = NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), nil, &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}})
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	if _, err := manager.Remove(context.Background(), testProfile(), "app_xxx"); err == nil {
		t.Fatal("Remove with keychain error returned nil error")
	}
	record, err = manager.Store.Current()
	if err != nil {
		t.Fatalf("Current after keychain remove error returned error: %v", err)
	}
	if record == nil {
		t.Fatalf("metadata should stay after keychain remove error")
	}
}

func TestManagerRemoveStoreErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), MetadataFilename)
	if err := os.WriteFile(path, []byte("{bad json"), 0600); err != nil {
		t.Fatalf("write invalid metadata: %v", err)
	}
	manager := NewManager(NewStoreAt(path), NewSecretStore(newFakeKeychain()), nil, nil)
	if _, err := manager.Remove(context.Background(), testProfile(), "app_xxx"); err == nil {
		t.Fatal("Remove with invalid metadata returned nil error")
	}

	now := time.Unix(1780000000, 0)
	dir := t.TempDir()
	path = filepath.Join(dir, MetadataFilename)
	manager = NewManager(NewStoreAt(path), NewSecretStore(newFakeKeychain()), nil, &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}})
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	makeDirReadOnly(t, dir)
	if _, err := manager.Remove(context.Background(), testProfile(), "app_xxx"); err == nil {
		t.Fatal("Remove with delete save error returned nil error")
	}
}

func TestManagerGetBranches(t *testing.T) {
	now := time.Unix(1780000000, 0)
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(newFakeKeychain()), nil, nil)
	manager.Now = func() time.Time { return now }

	var out bytes.Buffer
	var errOut bytes.Buffer
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https"}, testProfile(), &out, &errOut); err != nil {
		t.Fatalf("Get invalid input returned error: %v", err)
	}
	if !strings.Contains(errOut.String(), "protocol and host") {
		t.Fatalf("stderr = %q, want protocol/host validation", errOut.String())
	}

	out.Reset()
	errOut.Reset()
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/missing.git"}, testProfile(), &out, &errOut); err != nil {
		t.Fatalf("Get missing record returned error: %v", err)
	}
	if out.Len() != 0 || errOut.Len() != 0 {
		t.Fatalf("missing record stdout=%q stderr=%q, want both empty", out.String(), errOut.String())
	}

	kc := newFakeKeychain()
	issuer := &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "old-pat",
		ExpiresAt:  now.Add(5 * time.Minute).Unix(),
	}}
	manager = NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), nil, issuer)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	manager.Issuer = nil
	errOut.Reset()
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &out, &errOut); err != nil {
		t.Fatalf("Get without issuer returned error: %v", err)
	}
	if !strings.Contains(errOut.String(), "issuer is not configured") {
		t.Fatalf("stderr = %q, want issuer error", errOut.String())
	}

	issuer.next = &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "older-pat",
		ExpiresAt:  now.Add(time.Minute).Unix(),
	}
	manager.Issuer = issuer
	out.Reset()
	errOut.Reset()
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &out, &errOut); err != nil {
		t.Fatalf("Get stale refresh returned error: %v", err)
	}
	if got := out.String(); got != "" {
		t.Fatalf("stale refresh output = %q, want empty", got)
	}

	kc.setErr = errors.New("keychain locked")
	issuer.next = &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "new-pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}
	out.Reset()
	errOut.Reset()
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &out, &errOut); err != nil {
		t.Fatalf("Get keychain set error returned error: %v", err)
	}
	stderr := errOut.String()
	if !strings.Contains(stderr, "save local Git credential PAT to keychain failed") {
		t.Fatalf("stderr = %q, want static keychain error", stderr)
	}
	if !strings.Contains(stderr, "lark-cli apps +git-credential-init") {
		t.Fatalf("stderr = %q, want init retry hint", stderr)
	}
	if strings.Contains(stderr, "keychain locked") {
		t.Fatalf("stderr leaks keychain cause: %q", stderr)
	}

	kc.setErr = nil
	issuer.next = &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/other.git",
		PAT:        "other-pat",
		ExpiresAt:  now.Add(48 * time.Hour).Unix(),
	}
	out.Reset()
	errOut.Reset()
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &out, &errOut); err != nil {
		t.Fatalf("Get URL mismatch returned error: %v", err)
	}
	if !strings.Contains(errOut.String(), "does not match initialized URL") {
		t.Fatalf("stderr = %q, want URL mismatch", errOut.String())
	}

	issuer.next = &IssuedCredential{
		GitHTTPURL: "ssh://example.com/git/u/app.git",
		PAT:        "pat",
		ExpiresAt:  now.Add(48 * time.Hour).Unix(),
	}
	out.Reset()
	errOut.Reset()
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &out, &errOut); err != nil {
		t.Fatalf("Get invalid issued URL returned error: %v", err)
	}
	if !strings.Contains(errOut.String(), "only supports http/https") {
		t.Fatalf("stderr = %q, want invalid issued URL", errOut.String())
	}

	issuer.next = &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		ExpiresAt:  now.Add(48 * time.Hour).Unix(),
	}
	out.Reset()
	errOut.Reset()
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &out, &errOut); err != nil {
		t.Fatalf("Get invalid issued credential returned error: %v", err)
	}
	if !strings.Contains(errOut.String(), "response missing token") {
		t.Fatalf("stderr = %q, want missing token", errOut.String())
	}

	kc = newFakeKeychain()
	issuer = &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "old-pat",
		ExpiresAt:  now.Add(5 * time.Minute).Unix(),
	}}
	manager = NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), nil, issuer)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init for lock failure returned error: %v", err)
	}
	blocker := filepath.Join(t.TempDir(), "config-blocker")
	if err := os.WriteFile(blocker, []byte("file"), 0600); err != nil {
		t.Fatalf("write config blocker: %v", err)
	}
	t.Setenv("LARKSUITE_CLI_CONFIG_DIR", blocker)
	errOut.Reset()
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &bytes.Buffer{}, &errOut); err != nil {
		t.Fatalf("Get lock failure returned error: %v", err)
	}
	if !strings.Contains(errOut.String(), "create Git credential lock dir") {
		t.Fatalf("stderr = %q, want lock error", errOut.String())
	}
}

func TestManagerGetSecondReadBranches(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	issuer := &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "old-pat",
		ExpiresAt:  now.Add(5 * time.Minute).Unix(),
	}}
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), nil, issuer)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	var calls int
	kc.onGet = func(account string) {
		calls++
		if calls == 1 {
			kc.values[account] = ""
			record, err := manager.Store.FindByURL("https://example.com/git/u/app.git")
			if err != nil {
				t.Fatalf("FindByURL in onGet returned error: %v", err)
			}
			record.ExpiresAt = now.Add(24 * time.Hour).Unix()
			if err := manager.Store.Upsert(*record); err != nil {
				t.Fatalf("Upsert in onGet returned error: %v", err)
			}
			return
		}
		kc.values[account] = "restored-pat"
	}
	var out bytes.Buffer
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("Get second usable branch returned error: %v", err)
	}
	if got := out.String(); got != "username=x-access-token\npassword=restored-pat\n\n" {
		t.Fatalf("second usable output = %q", got)
	}

	kc = newFakeKeychain()
	dir := t.TempDir()
	manager = NewManager(NewStoreAt(filepath.Join(dir, MetadataFilename)), NewSecretStore(kc), nil, issuer)
	manager.Now = func() time.Time { return now }
	issuer.next = &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "old-pat",
		ExpiresAt:  now.Add(5 * time.Minute).Unix(),
	}
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	deleted := false
	kc.onGet = func(account string) {
		if !deleted {
			deleted = true
			if err := os.Remove(filepath.Join(dir, MetadataFilename)); err != nil {
				t.Fatalf("remove metadata: %v", err)
			}
		}
	}
	out.Reset()
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("Get second read missing returned error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("second read missing stdout = %q, want empty", out.String())
	}

	kc = newFakeKeychain()
	dir = t.TempDir()
	path := filepath.Join(dir, MetadataFilename)
	manager = NewManager(NewStoreAt(path), NewSecretStore(kc), nil, issuer)
	manager.Now = func() time.Time { return now }
	issuer.next = &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "old-pat",
		ExpiresAt:  now.Add(5 * time.Minute).Unix(),
	}
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	wroteBadMetadata := false
	kc.onGet = func(account string) {
		if !wroteBadMetadata {
			wroteBadMetadata = true
			kc.values[account] = ""
			if err := os.WriteFile(path, []byte("{bad json"), 0600); err != nil {
				t.Fatalf("write invalid metadata: %v", err)
			}
		}
	}
	var errOut bytes.Buffer
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &bytes.Buffer{}, &errOut); err != nil {
		t.Fatalf("Get second read error returned error: %v", err)
	}
	if !strings.Contains(errOut.String(), "invalid git.json") {
		t.Fatalf("stderr = %q, want second read error", errOut.String())
	}
}

func TestManagerGetRefreshReadAndWriteErrors(t *testing.T) {
	now := time.Unix(1780000000, 0)
	dir := t.TempDir()
	path := filepath.Join(dir, MetadataFilename)
	kc := newFakeKeychain()
	issuer := &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "old-pat",
		ExpiresAt:  now.Add(5 * time.Minute).Unix(),
	}}
	manager := NewManager(NewStoreAt(path), NewSecretStore(kc), nil, issuer)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	issuer.next = &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "older-pat",
		ExpiresAt:  now.Add(time.Minute).Unix(),
	}
	issuer.onIssue = func() {
		if err := os.WriteFile(path, []byte("{bad json"), 0600); err != nil {
			t.Fatalf("write invalid metadata: %v", err)
		}
	}
	var errOut bytes.Buffer
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &bytes.Buffer{}, &errOut); err != nil {
		t.Fatalf("Get refresh read error returned error: %v", err)
	}
	if !strings.Contains(errOut.String(), "invalid git.json") {
		t.Fatalf("stderr = %q, want read error", errOut.String())
	}

	dir = t.TempDir()
	path = filepath.Join(dir, MetadataFilename)
	kc = newFakeKeychain()
	issuer = &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "old-pat",
		ExpiresAt:  now.Add(5 * time.Minute).Unix(),
	}}
	manager = NewManager(NewStoreAt(path), NewSecretStore(kc), nil, issuer)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	issuer.next = &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "new-pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}
	issuer.onIssue = func() { makeDirReadOnly(t, dir) }
	errOut.Reset()
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &bytes.Buffer{}, &errOut); err != nil {
		t.Fatalf("Get refresh write error returned error: %v", err)
	}
	if !strings.Contains(errOut.String(), "Git credential refresh failed") {
		t.Fatalf("stderr = %q, want refresh write error", errOut.String())
	}
	record, err := manager.Store.FindByURL("https://example.com/git/u/app.git")
	if err != nil {
		t.Fatalf("FindByURL returned error: %v", err)
	}
	if got := kc.values[record.PATRef]; got != "old-pat" {
		t.Fatalf("PAT after failed refresh = %q, want old-pat", got)
	}

	dir = t.TempDir()
	path = filepath.Join(dir, MetadataFilename)
	kc = newFakeKeychain()
	issuer = &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "old-pat",
		ExpiresAt:  now.Add(5 * time.Minute).Unix(),
	}}
	manager = NewManager(NewStoreAt(path), NewSecretStore(kc), nil, issuer)
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	issuer.next = &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "older-pat",
		ExpiresAt:  now.Add(time.Minute).Unix(),
	}
	issuer.onIssue = func() {
		if err := os.Remove(path); err != nil {
			t.Fatalf("remove metadata: %v", err)
		}
	}
	errOut.Reset()
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &bytes.Buffer{}, &errOut); err != nil {
		t.Fatalf("Get stale refresh missing record returned error: %v", err)
	}
	if errOut.Len() != 0 {
		t.Fatalf("stderr = %q, want empty on stale missing record", errOut.String())
	}
}

func TestManagerGetReadErrorsStayOnStderr(t *testing.T) {
	path := filepath.Join(t.TempDir(), MetadataFilename)
	if err := os.WriteFile(path, []byte("{bad json"), 0600); err != nil {
		t.Fatalf("write invalid metadata: %v", err)
	}
	manager := NewManager(NewStoreAt(path), NewSecretStore(newFakeKeychain()), nil, nil)
	var errOut bytes.Buffer
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/repo.git"}, testProfile(), &bytes.Buffer{}, &errOut); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if !strings.Contains(errOut.String(), "invalid git.json") {
		t.Fatalf("stderr = %q, want config parse error", errOut.String())
	}
}

func TestManagerGetSecretReadErrorStaysOnStderr(t *testing.T) {
	now := time.Unix(1780000000, 0)
	kc := newFakeKeychain()
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(kc), nil, &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}})
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	kc.getErr = errors.New("keychain read failed")
	manager.Issuer = nil
	var errOut bytes.Buffer
	if err := manager.Get(context.Background(), CredentialInput{Protocol: "https", Host: "example.com", Path: "/git/u/app.git"}, testProfile(), &bytes.Buffer{}, &errOut); err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if !strings.Contains(errOut.String(), "issuer is not configured") {
		t.Fatalf("stderr = %q, want refresh path after secret read failure", errOut.String())
	}
}

func TestEraseBranches(t *testing.T) {
	manager := NewManager(NewStoreAt(filepath.Join(t.TempDir(), MetadataFilename)), NewSecretStore(newFakeKeychain()), nil, nil)
	if err := manager.Erase(errorReader{}); err == nil {
		t.Fatal("Erase reader error returned nil error")
	}
	if err := manager.Erase(bytes.NewBufferString("protocol=ssh\nhost=example.com\n\n")); err == nil {
		t.Fatal("Erase invalid URL returned nil error")
	}
	if err := manager.Erase(bytes.NewBufferString("protocol=https\nhost=example.com\npath=/missing.git\n\n")); err != nil {
		t.Fatalf("Erase missing record returned error: %v", err)
	}
	path := filepath.Join(t.TempDir(), MetadataFilename)
	if err := os.WriteFile(path, []byte("{bad json"), 0600); err != nil {
		t.Fatalf("write invalid metadata: %v", err)
	}
	manager = NewManager(NewStoreAt(path), NewSecretStore(newFakeKeychain()), nil, nil)
	if err := manager.Erase(bytes.NewBufferString("protocol=https\nhost=example.com\npath=/repo.git\n\n")); err == nil {
		t.Fatal("Erase invalid store returned nil error")
	}

	now := time.Unix(1780000000, 0)
	dir := t.TempDir()
	path = filepath.Join(dir, MetadataFilename)
	manager = NewManager(NewStoreAt(path), NewSecretStore(newFakeKeychain()), nil, &fakeIssuer{next: &IssuedCredential{
		GitHTTPURL: "https://example.com/git/u/app.git",
		PAT:        "pat",
		ExpiresAt:  now.Add(24 * time.Hour).Unix(),
	}})
	manager.Now = func() time.Time { return now }
	if _, err := manager.Init(context.Background(), testProfile(), "app_xxx"); err != nil {
		t.Fatalf("Init returned error: %v", err)
	}
	makeDirReadOnly(t, dir)
	if err := manager.Erase(bytes.NewBufferString("protocol=https\nhost=example.com\npath=/git/u/app.git\n\n")); err == nil {
		t.Fatal("Erase with metadata write error returned nil")
	}
}

func TestParseCredentialInputURLAndErrors(t *testing.T) {
	if _, err := ParseCredentialInput(bytes.NewBufferString("ignored-line\nprotocol=https\nhost=example.com\n\n")); err != nil {
		t.Fatalf("ParseCredentialInput ignored line returned error: %v", err)
	}
	input, err := ParseCredentialInput(bytes.NewBufferString("url=https://example.com/git/u/app.git?x=1\n\n"))
	if err != nil {
		t.Fatalf("ParseCredentialInput returned error: %v", err)
	}
	if input.Protocol != "https" || input.Host != "example.com" || input.Path != "/git/u/app.git" {
		t.Fatalf("input = %#v", input)
	}
	input, err = parseNormalizedForInput("https://example.com")
	if err != nil {
		t.Fatalf("parseNormalizedForInput no slash returned error: %v", err)
	}
	if input.Path != "/" {
		t.Fatalf("no slash path = %q, want /", input.Path)
	}
	if _, err := parseNormalizedForInput("not-a-url"); err == nil {
		t.Fatal("parseNormalizedForInput invalid returned nil error")
	}
	if _, err := ParseCredentialInput(errorReader{}); err == nil {
		t.Fatal("ParseCredentialInput reader error returned nil error")
	}
}

func TestWriteGitCredentialBranches(t *testing.T) {
	var out bytes.Buffer
	if err := writeGitCredential(&out, "", "pat"); err != nil {
		t.Fatalf("writeGitCredential empty username returned error: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("empty username output = %q", out.String())
	}
	for _, failAt := range []int{1, 2, 3} {
		err := writeGitCredential(&failWriter{failAt: failAt}, "user", "pat")
		if err == nil {
			t.Fatalf("writeGitCredential failAt=%d returned nil error", failAt)
		}
	}
}

func TestNilManagerUsesTimeNow(t *testing.T) {
	var manager *Manager
	if manager.now().IsZero() {
		t.Fatal("nil manager now() returned zero time")
	}
}

// TestRedactCredentialText focuses on the redaction regex, asserting it
// covers credential shapes across forms and does not over-match concatenated
// words. JSON-quoted forms are common in server-provided error bodies and must
// be covered; concatenated words like mytoken must not be treated as token.
func TestRedactCredentialText(t *testing.T) {
	samplePAT := testPublicSafeJoin("pat", "-sample")
	samplePassword := "sample-password"
	sampleSecret := "sample-secret"
	githubLikeToken := testPublicSafeJoin("gh", "p_") + strings.Repeat("x", 20)
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "bare assignment",
			in: "permission denied: " +
				testCredentialAssignment("token", samplePAT) + " " +
				testCredentialAssignment("password", samplePassword),
			want: "permission denied: " +
				testRedactedAssignment("token") + " " +
				testRedactedAssignment("password"),
		},
		{
			name: "json double-quoted value",
			in: "body={" +
				testDoubleQuotedAssignment("password", samplePassword) + "," +
				testDoubleQuotedAssignment("token", samplePAT) +
				"}",
			want: "body={" +
				testDoubleQuotedRedactedAssignment("password") + "," +
				testDoubleQuotedRedactedAssignment("token") +
				"}",
		},
		{
			name: "json single-quoted value",
			in:   "body={" + testSingleQuotedAssignment("secret", sampleSecret) + "}",
			want: "body={" + testSingleQuotedRedactedAssignment("secret") + "}",
		},
		{
			name: "colon separator with quoted value",
			in:   testCredentialColon("token", `"`+samplePAT+`"`),
			want: testRedactedColon("token"),
		},
		{
			name: "url userinfo",
			in:   "clone " + testCredentialURLWithUserInfo("example.com/repo.git", samplePAT),
			want: "clone https://***@example.com/repo.git",
		},
		{
			name: "bearer header",
			in:   testAuthorizationBearer(githubLikeToken),
			want: testRedactedAuthorizationBearer(),
		},
		{
			name: "pat-like standalone",
			in:   "issued " + samplePAT + " for app",
			want: "issued <redacted> for app",
		},
		{
			name: "concatenated key not redacted",
			in:   testCredentialAssignment("mytoken", "abc123") + " " + testCredentialAssignment("secret_field", "see"),
			want: testCredentialAssignment("mytoken", "abc123") + " " + testCredentialAssignment("secret_field", "see"),
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := RedactCredentialText(tc.in); got != tc.want {
				t.Fatalf("RedactCredentialText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSafeCredentialErrorMessageFallbacks(t *testing.T) {
	if got := safeCredentialErrorMessage(nil); got != "" {
		t.Fatalf("safeCredentialErrorMessage(nil) = %q, want empty", got)
	}

	if got := safeCredentialErrorMessage(&errs.ConfigError{Problem: errs.Problem{
		Category: errs.CategoryConfig,
		Subtype:  errs.SubtypeInvalidConfig,
	}}); got != "config/invalid_config" {
		t.Fatalf("safeCredentialErrorMessage typed fallback = %q, want config/invalid_config", got)
	}

	samplePAT := testPublicSafeJoin("pat", "-sample")
	got := safeCredentialErrorMessage(errors.New("transport failed with " + testCredentialAssignment("token", samplePAT)))
	if strings.Contains(got, samplePAT) {
		t.Fatalf("safeCredentialErrorMessage leaks credential value: %q", got)
	}
	if want := testRedactedAssignment("token"); !strings.Contains(got, want) {
		t.Fatalf("safeCredentialErrorMessage missing %q after redaction: %q", want, got)
	}
}

func TestWriteCredentialErrorRedactsTypedMessageHint(t *testing.T) {
	samplePAT := testPublicSafeJoin("pat", "-sample")
	err := errs.NewInternalError(errs.SubtypeStorage, "save failed with %s", testCredentialAssignment("token", samplePAT)).
		WithHint("retry without %s", testCredentialAssignment("password", samplePAT)).
		WithLogID("log_x")

	var buf bytes.Buffer
	writeCredentialError(&buf, "Git credential refresh failed", err)
	got := buf.String()
	for _, leaked := range []string{samplePAT, testCredentialAssignment("token", samplePAT), testCredentialAssignment("password", samplePAT)} {
		if strings.Contains(got, leaked) {
			t.Fatalf("writeCredentialError leaks credential value %q in %q", leaked, got)
		}
	}
	for _, want := range []string{
		"Git credential refresh failed: save failed with " + testRedactedAssignment("token"),
		"log_id=log_x",
		"hint: retry without " + testRedactedAssignment("password"),
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("writeCredentialError output missing %q: %q", want, got)
		}
	}

	writeCredentialError(nil, "ignored", err)
	writeCredentialError(&buf, "ignored", nil)
}

func assertProblem(t *testing.T, err error, wantCategory errs.Category, wantSubtype errs.Subtype) {
	t.Helper()
	p, ok := errs.ProblemOf(err)
	if !ok {
		t.Fatalf("expected typed errs.Problem, got %T: %v", err, err)
	}
	if p.Category != wantCategory || p.Subtype != wantSubtype {
		t.Fatalf("problem metadata = %s/%s, want %s/%s", p.Category, p.Subtype, wantCategory, wantSubtype)
	}
}

func testPublicSafeJoin(parts ...string) string {
	return strings.Join(parts, "")
}

func testCredentialAssignment(key, value string) string {
	return key + "=" + value
}

func testRedactedAssignment(key string) string {
	return key + "=<redacted>"
}

func testCredentialColon(key, value string) string {
	return key + ": " + value
}

func testRedactedColon(key string) string {
	return key + ": <redacted>"
}

func testDoubleQuotedAssignment(key, value string) string {
	return `"` + key + `"` + ":" + `"` + value + `"`
}

func testDoubleQuotedRedactedAssignment(key string) string {
	return `"` + key + `"` + ":<redacted>"
}

func testSingleQuotedAssignment(key, value string) string {
	return `'` + key + `'` + ":" + `'` + value + `'`
}

func testSingleQuotedRedactedAssignment(key string) string {
	return `'` + key + `'` + ":<redacted>"
}

func testCredentialURLWithUserInfo(hostPath, credential string) string {
	return "https://" + "user:" + credential + "@" + hostPath
}

func testAuthorizationBearer(value string) string {
	return "Authorization" + ": " + "Bearer " + value
}

func testRedactedAuthorizationBearer() string {
	return "Authorization" + ": " + "Bearer <redacted>"
}

func testProfile() ProfileContext {
	return ProfileContext{Profile: "default", ProfileAppID: "cli_xxx", UserOpenID: "ou_xxx"}
}

type errorReader struct{}

func (errorReader) Read(p []byte) (int, error) {
	return 0, errors.New("read failed")
}

type failWriter struct {
	failAt int
	writes int
}

func (w *failWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes >= w.failAt {
		return 0, fmt.Errorf("write %d failed", w.writes)
	}
	return len(p), nil
}

func installFakeGit(t *testing.T, failUseHTTPPathExit int) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "git.log")
	gitPath := filepath.Join(dir, "git")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> "$GIT_FAKE_LOG"
case "$*" in
  *"--get"*) exit 1 ;;
esac
case "$*" in
  *useHttpPath*) exit %d ;;
esac
exit 0
`, failUseHTTPPathExit)
	if failUseHTTPPathExit == 0 {
		script = `#!/bin/sh
printf '%s\n' "$*" >> "$GIT_FAKE_LOG"
case "$*" in
  *"--get"*) exit 1 ;;
esac
exit 0
`
	}
	if err := os.WriteFile(gitPath, []byte(script), 0700); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("GIT_FAKE_LOG", logPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func installFakeGitWithGet(t *testing.T, value string) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "git.log")
	gitPath := filepath.Join(dir, "git")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> "$GIT_FAKE_LOG"
case "$*" in
  *"--get"*) printf '%%s\n' %s; exit 0 ;;
esac
exit 0
`, shellQuoteArg(value))
	if err := os.WriteFile(gitPath, []byte(script), 0700); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("GIT_FAKE_LOG", logPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func installFakeGitWithGetAndUnsetExit(t *testing.T, value string, unsetExit int) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "git.log")
	gitPath := filepath.Join(dir, "git")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> "$GIT_FAKE_LOG"
case "$*" in
  *"--get"*) printf '%%s\n' %s; exit 0 ;;
  *"--unset"*) exit %d ;;
esac
exit 0
`, shellQuoteArg(value), unsetExit)
	if err := os.WriteFile(gitPath, []byte(script), 0700); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("GIT_FAKE_LOG", logPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func installFakeGitSetFails(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "git.log")
	gitPath := filepath.Join(dir, "git")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$GIT_FAKE_LOG"
case "$*" in
  *"--get"*) exit 1 ;;
  *".helper "*) exit 8 ;;
esac
exit 0
`
	if err := os.WriteFile(gitPath, []byte(script), 0700); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("GIT_FAKE_LOG", logPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func installFakeGitWithGetAndUseHTTPPathFailure(t *testing.T, value string, useHTTPPathExit int) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "git.log")
	gitPath := filepath.Join(dir, "git")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> "$GIT_FAKE_LOG"
case "$*" in
  *"--get"*) printf '%%s\n' %s; exit 0 ;;
  *"useHttpPath true"*) exit %d ;;
esac
exit 0
`, shellQuoteArg(value), useHTTPPathExit)
	if err := os.WriteFile(gitPath, []byte(script), 0700); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("GIT_FAKE_LOG", logPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func installFakeGitWithGetAndSecondUnsetFails(t *testing.T, value string) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "git.log")
	gitPath := filepath.Join(dir, "git")
	script := fmt.Sprintf(`#!/bin/sh
printf '%%s\n' "$*" >> "$GIT_FAKE_LOG"
case "$*" in
  *"--get"*) printf '%%s\n' %s; exit 0 ;;
  *"--unset"*"useHttpPath"*) exit 9 ;;
  *"--unset"*) exit 0 ;;
esac
exit 0
`, shellQuoteArg(value))
	if err := os.WriteFile(gitPath, []byte(script), 0700); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("GIT_FAKE_LOG", logPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func installAlwaysFailingGit(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	logPath := filepath.Join(dir, "git.log")
	gitPath := filepath.Join(dir, "git")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$GIT_FAKE_LOG"
exit 9
`
	if err := os.WriteFile(gitPath, []byte(script), 0700); err != nil {
		t.Fatalf("write fake git: %v", err)
	}
	t.Setenv("GIT_FAKE_LOG", logPath)
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return logPath
}

func makeDirReadOnly(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatalf("chmod readonly %s: %v", dir, err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(dir, 0700)
	})
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

var _ io.Reader = errorReader{}

func mustReadMetadata(t *testing.T, manager *Manager) []byte {
	t.Helper()
	data, err := manager.Store.Load()
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	raw, err := jsonMarshal(data)
	if err != nil {
		t.Fatalf("jsonMarshal returned error: %v", err)
	}
	return raw
}

func jsonMarshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}
