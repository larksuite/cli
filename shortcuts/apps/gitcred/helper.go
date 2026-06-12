// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package gitcred

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
)

type Issuer interface {
	Issue(ctx context.Context, appID string, profile ProfileContext) (*IssuedCredential, error)
}

type Manager struct {
	Store     *Store
	Secrets   *SecretStore
	GitConfig GitConfig
	Issuer    Issuer
	Now       func() time.Time
}

func NewManager(store *Store, secrets *SecretStore, gitConfig GitConfig, issuer Issuer) *Manager {
	return &Manager{
		Store:     store,
		Secrets:   secrets,
		GitConfig: gitConfig,
		Issuer:    issuer,
		Now:       time.Now,
	}
}

func (m *Manager) Init(ctx context.Context, profile ProfileContext, appID string) (*InitResult, error) {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--app-id is required").WithParam("--app-id")
	}
	if err := validate.ResourceName(appID, "--app-id"); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%v", err).WithParam("--app-id").WithCause(err)
	}
	if profile.UserOpenID == "" {
		return nil, errs.NewAuthenticationError(errs.SubtypeTokenMissing, "not logged in").WithHint("run `lark-cli auth login --scope \"spark:app:read\"`")
	}
	unlockApp, err := lockApp(appID)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeStorage, "acquire Git credential lock for %s: %v", appID, err).WithCause(err)
	}
	defer unlockApp()
	if m.Issuer == nil {
		return nil, errs.NewInternalError(errs.SubtypeUnknown, "git credential issuer is not configured")
	}
	issued, err := m.Issuer.Issue(ctx, appID, profile)
	if err != nil {
		return nil, err
	}
	url, err := NormalizeGitHTTPURL(issued.GitHTTPURL)
	if err != nil {
		return nil, err
	}
	now := m.nowUnix()
	if err := validateIssuedCredential(appID, url, issued, now); err != nil {
		return nil, err
	}
	ref := BuildPATRef(profile, appID)
	previous, err := m.currentAppRecord(appID)
	if err != nil {
		return nil, err
	}
	var previousPAT string
	if previous != nil {
		previousPAT, _ = m.Secrets.Get(previous.PATRef)
	}
	record := CredentialRecord{
		AppID:        appID,
		GitHTTPURL:   url,
		Profile:      profile.Profile,
		ProfileAppID: profile.ProfileAppID,
		UserOpenID:   profile.UserOpenID,
		Username:     defaultUsername(issued.Username),
		PATRef:       ref,
		Status:       StatusPending,
		ExpiresAt:    issued.ExpiresAt,
		UpdatedAt:    now,
	}
	if err := m.Store.Upsert(record); err != nil {
		return nil, err
	}
	if err := m.Secrets.Set(ref, issued.PAT); err != nil {
		m.restoreAfterInitFailure(appID, previous, previousPAT)
		return nil, err
	}
	record.Status = StatusConfirmed
	if err := m.Store.Upsert(record); err != nil {
		if previous != nil && previous.PATRef == ref && previousPAT != "" {
			_ = m.Secrets.Set(ref, previousPAT)
		} else {
			_ = m.Secrets.Remove(ref)
		}
		m.restoreAfterInitFailure(appID, previous, previousPAT)
		return nil, err
	}
	if previous != nil && previous.PATRef != "" && previous.PATRef != ref {
		_ = m.Secrets.Remove(previous.PATRef)
	}
	result := &InitResult{AppID: appID, GitHTTPURL: url, Refreshed: previous != nil}
	if m.GitConfig != nil {
		if err := m.GitConfig.SetHelper(ctx, url, appID); err != nil {
			result.ConfigWarning = err.Error()
		} else if previous != nil && previous.GitHTTPURL != "" && previous.GitHTTPURL != url {
			if err := m.GitConfig.UnsetHelper(ctx, previous.GitHTTPURL); err != nil {
				result.ConfigWarning = err.Error()
			}
		}
	}
	return result, nil
}

func (m *Manager) Remove(ctx context.Context, profile ProfileContext, appID string) (*RemoveResult, error) {
	appID = strings.TrimSpace(appID)
	if appID == "" {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "--app-id is required").WithParam("--app-id")
	}
	if err := validate.ResourceName(appID, "--app-id"); err != nil {
		return nil, errs.NewValidationError(errs.SubtypeInvalidArgument, "%v", err).WithParam("--app-id").WithCause(err)
	}
	unlockApp, err := lockApp(appID)
	if err != nil {
		return nil, errs.NewInternalError(errs.SubtypeStorage, "acquire Git credential lock for %s: %v", appID, err).WithCause(err)
	}
	defer unlockApp()
	records, err := m.Store.FindByAppID(appID, ProfileContext{})
	if err != nil {
		return nil, err
	}
	result := &RemoveResult{AppID: appID, Records: records}
	for _, record := range records {
		if err := m.Secrets.Remove(record.PATRef); err != nil {
			return nil, err
		}
		if m.GitConfig != nil {
			if err := m.GitConfig.UnsetHelper(ctx, record.GitHTTPURL); err != nil {
				result.ConfigWarning = err.Error()
			}
		}
		if _, err := m.Store.DeleteByURL(record.GitHTTPURL); err != nil {
			return nil, err
		}
		result.Removed = true
	}
	return result, nil
}

func (m *Manager) List() (*ListResult, error) {
	records, err := m.Store.Records()
	if err != nil {
		return nil, err
	}
	out := make([]ListRecord, 0, len(records))
	for _, record := range records {
		out = append(out, m.listRecord(record))
	}
	return &ListResult{Records: out}, nil
}

func (m *Manager) Get(ctx context.Context, input CredentialInput, current ProfileContext, out, errOut io.Writer) error {
	url, err := NormalizeCredentialInput(input)
	if err != nil {
		fmt.Fprintf(errOut, "Git credential unavailable: %s\n", err)
		return nil
	}
	record, pat, ok, err := m.readConfirmed(url, current)
	if err != nil {
		fmt.Fprintf(errOut, "Git credential unavailable: %s\n", err)
		return nil
	}
	if !ok {
		return nil
	}
	if m.usable(record, pat) {
		return writeGitCredential(out, record.Username, pat)
	}

	unlock := lockURL(url)
	defer unlock()
	unlockApp, err := lockApp(record.AppID)
	if err != nil {
		fmt.Fprintf(errOut, "Git credential refresh failed: acquire lock for %s: %s\n", record.AppID, err)
		return nil
	}
	defer unlockApp()

	record, pat, ok, err = m.readConfirmed(url, current)
	if err != nil {
		fmt.Fprintf(errOut, "Git credential unavailable: %s\n", err)
		return nil
	}
	if !ok {
		return nil
	}
	if m.usable(record, pat) {
		return writeGitCredential(out, record.Username, pat)
	}
	if m.Issuer == nil {
		fmt.Fprintln(errOut, "Git credential refresh failed: issuer is not configured")
		return nil
	}
	issued, err := m.Issuer.Issue(ctx, record.AppID, current)
	if err != nil {
		fmt.Fprintf(errOut, "Git credential refresh failed: %s\nNext step: lark-cli apps +git-credential-init --app-id %s\n", err, record.AppID)
		return nil
	}
	issuedURL, urlErr := NormalizeGitHTTPURL(issued.GitHTTPURL)
	if urlErr != nil {
		fmt.Fprintf(errOut, "Git credential refresh failed: %s\n", urlErr)
		return nil
	}
	if err := validateIssuedCredential(record.AppID, issuedURL, issued, m.nowUnix()); err != nil {
		fmt.Fprintf(errOut, "Git credential refresh failed: %s\n", err)
		return nil
	}
	if issuedURL != url {
		fmt.Fprintf(errOut, "Git credential refresh failed: issued repository URL %q does not match initialized URL %q\n", issuedURL, url)
		return nil
	}
	if issued.ExpiresAt < record.ExpiresAt {
		latest, latestPAT, found, readErr := m.readConfirmed(url, current)
		if readErr != nil {
			fmt.Fprintf(errOut, "Git credential unavailable: %s\n", readErr)
			return nil
		}
		if found && m.usable(latest, latestPAT) {
			return writeGitCredential(out, latest.Username, latestPAT)
		}
		return nil
	}
	record.Username = defaultUsername(issued.Username)
	record.ExpiresAt = issued.ExpiresAt
	record.UpdatedAt = m.nowUnix()
	record.InvalidatedAt = 0
	record.Status = StatusConfirmed
	oldPAT := pat
	if err := m.Secrets.Set(record.PATRef, issued.PAT); err != nil {
		fmt.Fprintf(errOut, "Git credential refresh failed: %s\n", err)
		return nil
	}
	if err := m.Store.Upsert(record); err != nil {
		_ = m.Secrets.Set(record.PATRef, oldPAT)
		fmt.Fprintf(errOut, "Git credential refresh failed: %s\n", err)
		return nil
	}
	return writeGitCredential(out, record.Username, issued.PAT)
}

func (m *Manager) currentAppRecord(appID string) (*CredentialRecord, error) {
	records, err := m.Store.FindByAppID(appID, ProfileContext{})
	if err != nil || len(records) == 0 {
		return nil, err
	}
	return &records[0], nil
}

func (m *Manager) restoreAfterInitFailure(appID string, existing *CredentialRecord, existingPAT string) {
	if existing == nil {
		records, err := m.Store.FindByAppID(appID, ProfileContext{})
		if err == nil {
			for _, record := range records {
				_, _ = m.Store.DeleteByURL(record.GitHTTPURL)
			}
		}
		return
	}
	_ = m.Store.Upsert(*existing)
	if existingPAT != "" {
		_ = m.Secrets.Set(existing.PATRef, existingPAT)
	}
}

func (m *Manager) listRecord(record CredentialRecord) ListRecord {
	now := m.nowUnix()
	status := ListStatusValid
	expired := record.ExpiresAt <= now
	switch {
	case record.Status != StatusConfirmed || record.GitHTTPURL == "" || record.PATRef == "":
		status = ListStatusIncomplete
	case record.InvalidatedAt > 0:
		status = ListStatusInvalidated
	case !m.hasSecret(record.PATRef):
		status = ListStatusMissingSecret
	case expired:
		status = ListStatusExpired
	}
	return ListRecord{
		AppID:         record.AppID,
		GitHTTPURL:    record.GitHTTPURL,
		Status:        status,
		ExpiresAt:     record.ExpiresAt,
		UpdatedAt:     record.UpdatedAt,
		Profile:       record.Profile,
		ProfileAppID:  record.ProfileAppID,
		UserOpenID:    record.UserOpenID,
		Expired:       expired,
		InvalidatedAt: record.InvalidatedAt,
	}
}

func (m *Manager) hasSecret(ref string) bool {
	pat, err := m.Secrets.Get(ref)
	return err == nil && pat != ""
}

func (m *Manager) StoreCredential(r io.Reader) error {
	_, err := io.Copy(io.Discard, r)
	return err
}

func (m *Manager) Erase(r io.Reader) error {
	input, err := ParseCredentialInput(r)
	if err != nil {
		return err
	}
	url, err := NormalizeCredentialInput(input)
	if err != nil {
		return err
	}
	record, err := m.Store.FindByURL(url)
	if err != nil || record == nil {
		return err
	}
	unlockApp, err := lockApp(record.AppID)
	if err != nil {
		return errs.NewInternalError(errs.SubtypeStorage, "acquire Git credential lock for %s: %v", record.AppID, err).WithCause(err)
	}
	defer unlockApp()
	record, err = m.Store.FindByURL(url)
	if err != nil || record == nil {
		return err
	}
	now := m.nowUnix()
	if record.LastEraseAt > 0 && now-record.LastEraseAt < int64(eraseCooldown.Seconds()) {
		return nil
	}
	record.InvalidatedAt = now
	record.LastEraseAt = now
	if err := m.Store.Upsert(*record); err != nil {
		return err
	}
	return m.Secrets.Remove(record.PATRef)
}

func (m *Manager) readConfirmed(url string, current ProfileContext) (CredentialRecord, string, bool, error) {
	record, err := m.Store.FindByURL(url)
	if err != nil || record == nil {
		return CredentialRecord{}, "", false, err
	}
	if record.ProfileAppID != current.ProfileAppID || record.UserOpenID != current.UserOpenID {
		return CredentialRecord{}, "", false, errs.NewValidationError(errs.SubtypeFailedPrecondition, "current login does not match initialized credential").
			WithHint(fmt.Sprintf("run `lark-cli apps +git-credential-init --app-id %s` with the current login or switch back to the original account", record.AppID))
	}
	pat, err := m.Secrets.Get(record.PATRef)
	if err != nil {
		pat = ""
	}
	return *record, pat, true, nil
}

func (m *Manager) usable(record CredentialRecord, pat string) bool {
	if record.Status != StatusConfirmed || pat == "" || record.InvalidatedAt > 0 {
		return false
	}
	return record.ExpiresAt-m.nowUnix() > int64(refreshBeforeExpiry.Seconds())
}

func (m *Manager) now() time.Time {
	if m != nil && m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Manager) nowUnix() int64 {
	return m.now().Unix()
}

func ParseCredentialInput(r io.Reader) (CredentialInput, error) {
	scanner := bufio.NewScanner(r)
	var input CredentialInput
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			break
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "protocol":
			input.Protocol = value
		case "host":
			input.Host = value
		case "path":
			input.Path = value
		case "url":
			u, err := NormalizeGitHTTPURL(value)
			if err == nil {
				parsed, _ := parseNormalizedForInput(u)
				input = parsed
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return input, err
	}
	return input, nil
}

func parseNormalizedForInput(raw string) (CredentialInput, error) {
	parts := strings.SplitN(raw, "://", 2)
	if len(parts) != 2 {
		return CredentialInput{}, errs.NewValidationError(errs.SubtypeInvalidArgument, "invalid credential URL")
	}
	hostPath := parts[1]
	idx := strings.Index(hostPath, "/")
	if idx < 0 {
		return CredentialInput{Protocol: parts[0], Host: hostPath, Path: "/"}, nil
	}
	return CredentialInput{Protocol: parts[0], Host: hostPath[:idx], Path: hostPath[idx:]}, nil
}

func writeGitCredential(w io.Writer, username, pat string) error {
	if username == "" || pat == "" {
		return nil
	}
	if _, err := fmt.Fprintf(w, "username=%s\n", username); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "password=%s\n", pat); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w)
	return err
}

func defaultUsername(username string) string {
	username = strings.TrimSpace(username)
	if username == "" {
		return "x-access-token"
	}
	return username
}

func validateIssuedCredential(appID, normalizedURL string, issued *IssuedCredential, now int64) error {
	if issued == nil {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "Issue Miaoda Git credential: empty credential")
	}
	if issued.AppID != "" && issued.AppID != appID {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "Issue Miaoda Git credential: response app_id %q does not match requested app_id %q", issued.AppID, appID)
	}
	if normalizedURL == "" {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "Issue Miaoda Git credential: response missing gitURL")
	}
	if strings.TrimSpace(issued.PAT) == "" {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "Issue Miaoda Git credential: response missing token")
	}
	if issued.ExpiresAt <= now {
		return errs.NewInternalError(errs.SubtypeInvalidResponse, "Issue Miaoda Git credential: response expiredTime must be in the future")
	}
	return nil
}
