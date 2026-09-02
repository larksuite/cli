// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package gitcred

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs" //nolint:depguard // resolving git's own global config path (home/XDG) is CLI-internal path discovery, matching store.go/lock.go.
)

type configValue struct {
	Origin string
	Value  string
}

type credentialConfigState struct {
	Helpers        []configValue
	UseHTTPPath    []configValue
	WritableOrigin string
}

type managedKind uint8

const (
	managedNone managedKind = iota
	managedAbsent
	managedLegacy
	managedCurrent
	// managedForeign: the URL-scoped helper list contains only non-lark-cli
	// helpers (all from the writable origin). lark-cli neither owns nor may
	// overwrite it; UnsetHelper leaves it untouched.
	managedForeign
	// managedPartial: a lark-cli-owned residue that is neither the exact legacy
	// nor current shape (e.g. the reset+helper pair written without useHttpPath,
	// or a useHttpPath deleted out from under the helper). It is safe to
	// re-normalize (SetHelper) or fully clean up (UnsetHelper).
	managedPartial
	// managedMixed: the URL-scoped helper list mixes lark-cli-owned values with
	// foreign ones. SetHelper refuses it; UnsetHelper removes only the lark-cli
	// values and reports that a foreign helper remains.
	managedMixed
)

type GitConfig interface {
	SetHelper(ctx context.Context, gitHTTPURL, appID string) error
	UnsetHelper(ctx context.Context, gitHTTPURL, appID string) error
}

type GlobalGitConfig struct {
	HelperCommand string
}

func (g GlobalGitConfig) SetHelper(ctx context.Context, gitHTTPURL, appID string) error {
	normalizedURL, err := NormalizeGitHTTPURL(gitHTTPURL)
	if err != nil {
		return err
	}
	appID = strings.TrimSpace(appID)
	if err := validate.ResourceName(appID, "appID"); err != nil {
		return err
	}
	canonical := g.helperCommand(appID)
	helperKey := gitCredentialKey(normalizedURL, "helper")
	useHTTPPathKey := gitCredentialKey(normalizedURL, "useHttpPath")

	writableOrigin, err := writableGlobalOrigin()
	if err != nil {
		return err
	}
	unlock, err := lockGlobalConfig(writableOrigin)
	if err != nil {
		return err
	}
	defer unlock()

	snapshot, err := readCredentialConfig(ctx, normalizedURL)
	if err != nil {
		return err
	}
	if !setHelperAcceptsState(classifyManagedState(snapshot, canonical)) {
		return gitConfigNotOwnedError(normalizedURL)
	}
	current, err := readCredentialConfig(ctx, normalizedURL)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, snapshot) {
		return gitConfigChangedError(normalizedURL)
	}

	known := cloneCredentialConfigState(snapshot)
	if err := gitConfigRewriteHelpersScoped(ctx, helperKey, configValuesOf(snapshot.Helpers), []string{"", canonical}, &known.Helpers, known.WritableOrigin); err != nil {
		return g.finishSetFailure(ctx, normalizedURL, snapshot, known, err)
	}
	if err := gitConfigSet(ctx, useHTTPPathKey, "true"); err != nil {
		return g.finishSetFailure(ctx, normalizedURL, snapshot, known, err)
	}
	known.UseHTTPPath = []configValue{{Origin: known.WritableOrigin, Value: "true"}}

	// The empty-helper reset only clears helpers that parse BEFORE the lark-cli
	// section. A generic credential.helper (or one from a later [include]) that
	// parses AFTER our section still participates in get/store/erase for the
	// URL. Detect that and relocate our section to the end of the writable file
	// so the reset defeats every earlier helper; fail closed if the competing
	// helper lives somewhere we must not edit.
	safe, err := larkResetDefeatsLaterHelpers(ctx, normalizedURL, canonical)
	if err != nil {
		return g.finishSetFailure(ctx, normalizedURL, snapshot, known, err)
	}
	if !safe {
		if err := repositionLarkSection(ctx, normalizedURL, canonical); err != nil {
			return g.finishSetFailure(ctx, normalizedURL, snapshot, known, err)
		}
		safe, err = larkResetDefeatsLaterHelpers(ctx, normalizedURL, canonical)
		if err != nil {
			return g.finishSetFailure(ctx, normalizedURL, snapshot, known, err)
		}
		if !safe {
			return g.finishSetFailure(ctx, normalizedURL, snapshot, known, gitConfigHelperOrderUnsafeError(normalizedURL))
		}
	}

	readback, err := readCredentialConfig(ctx, normalizedURL)
	if err != nil {
		return g.finishSetFailure(ctx, normalizedURL, snapshot, known, err)
	}
	if !reflect.DeepEqual(readback, known) || classifyManagedState(readback, canonical) != managedCurrent {
		return g.finishSetFailure(ctx, normalizedURL, snapshot, known, gitConfigChangedError(normalizedURL))
	}
	return nil
}

// setHelperAcceptsState reports whether SetHelper may (re)write the URL-scoped
// configuration for a state. Absent/Legacy/Current/Partial are lark-cli-owned
// (or empty) and safe to normalize; Foreign/Mixed/None belong to the user or a
// third party and must not be overwritten.
func setHelperAcceptsState(kind managedKind) bool {
	switch kind {
	case managedAbsent, managedLegacy, managedCurrent, managedPartial:
		return true
	default:
		return false
	}
}

func (g GlobalGitConfig) UnsetHelper(ctx context.Context, gitHTTPURL, appID string) error {
	normalizedURL, err := NormalizeGitHTTPURL(gitHTTPURL)
	if err != nil {
		return err
	}
	appID = strings.TrimSpace(appID)
	if err := validate.ResourceName(appID, "appID"); err != nil {
		return err
	}
	canonical := g.helperCommand(appID)
	helperKey := gitCredentialKey(normalizedURL, "helper")
	useHTTPPathKey := gitCredentialKey(normalizedURL, "useHttpPath")

	writableOrigin, err := writableGlobalOrigin()
	if err != nil {
		return err
	}
	unlock, err := lockGlobalConfig(writableOrigin)
	if err != nil {
		return err
	}
	defer unlock()

	state, err := readCredentialConfig(ctx, normalizedURL)
	if err != nil {
		return err
	}
	switch classifyManagedState(state, canonical) {
	case managedLegacy, managedCurrent, managedPartial:
		return unsetOwnedHelper(ctx, helperKey, useHTTPPathKey, state)
	case managedMixed:
		return unsetMixedHelper(ctx, normalizedURL, helperKey, useHTTPPathKey, canonical, state)
	default:
		// Absent, Foreign, or unclassifiable: nothing lark-cli owns to remove.
		return nil
	}
}

// unsetOwnedHelper removes a fully lark-cli-owned URL-scoped configuration. It
// deletes useHttpPath FIRST, then the helper list, so that if the second delete
// fails the residue is still a lark-cli-recoverable state (helper present, no
// useHttpPath = managedPartial) rather than a useHttpPath-only orphan that
// SetHelper would refuse to re-init. On helper-delete failure it restores
// useHttpPath best-effort and annotates the error.
func unsetOwnedHelper(ctx context.Context, helperKey, useHTTPPathKey string, state credentialConfigState) error {
	hadUseHTTPPath := len(state.UseHTTPPath) == 1
	if hadUseHTTPPath {
		if err := gitConfigUnsetAll(ctx, useHTTPPathKey); err != nil {
			return err
		}
	}
	if err := gitConfigUnsetAll(ctx, helperKey); err != nil {
		if hadUseHTTPPath {
			if restoreErr := gitConfigAdd(ctx, useHTTPPathKey, "true"); restoreErr != nil {
				return withRollbackFailureHint(err)
			}
		}
		return err
	}
	return nil
}

// unsetMixedHelper removes only the lark-cli-owned values (the empty reset and
// the canonical helper) from a helper list that also contains foreign helpers,
// deletes the lark-cli useHttpPath, and returns a non-nil warning that a
// foreign helper remains so callers surface it instead of reporting a clean
// removal.
func unsetMixedHelper(ctx context.Context, gitHTTPURL, helperKey, useHTTPPathKey, canonical string, state credentialConfigState) error {
	if err := unsetHelperValueScoped(ctx, helperKey, ""); err != nil {
		return err
	}
	if err := unsetHelperValueScoped(ctx, helperKey, canonical); err != nil {
		return err
	}
	if len(state.UseHTTPPath) == 1 {
		if err := gitConfigUnsetAll(ctx, useHTTPPathKey); err != nil {
			return err
		}
	}
	return gitConfigForeignRemainsWarning(gitHTTPURL)
}

func (g GlobalGitConfig) finishSetFailure(ctx context.Context, gitHTTPURL string, snapshot, known credentialConfigState, first error) error {
	if rollbackErr := rollbackIfUnchanged(ctx, gitHTTPURL, snapshot, known); rollbackErr != nil {
		return withRollbackFailureHint(first)
	}
	return first
}

func rollbackIfUnchanged(ctx context.Context, gitHTTPURL string, snapshot, known credentialConfigState) error {
	current, err := readCredentialConfig(ctx, gitHTTPURL)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(current, known) {
		return nil
	}
	if err := gitConfigReplaceAll(ctx, gitCredentialKey(gitHTTPURL, "helper"), configValuesOf(snapshot.Helpers)); err != nil {
		return err
	}
	return gitConfigReplaceAll(ctx, gitCredentialKey(gitHTTPURL, "useHttpPath"), configValuesOf(snapshot.UseHTTPPath))
}

func cloneCredentialConfigState(state credentialConfigState) credentialConfigState {
	state.Helpers = append([]configValue(nil), state.Helpers...)
	state.UseHTTPPath = append([]configValue(nil), state.UseHTTPPath...)
	return state
}

func configValuesOf(values []configValue) []string {
	result := make([]string, len(values))
	for i := range values {
		result[i] = values[i].Value
	}
	return result
}

func gitConfigNotOwnedError(gitHTTPURL string) error {
	return errs.NewValidationError(
		errs.SubtypeFailedPrecondition,
		"git credential configuration for %s is not owned by lark-cli; refusing to overwrite it",
		gitHTTPURL,
	).WithHint("inspect the URL-scoped global Git credential.helper and credential.useHttpPath values, including included config files")
}

func gitConfigChangedError(gitHTTPURL string) error {
	return errs.NewValidationError(
		errs.SubtypeFailedPrecondition,
		"git credential configuration for %s changed while it was being updated",
		gitHTTPURL,
	).WithHint("inspect the URL-scoped global Git credential settings and retry after concurrent changes stop")
}

func gitConfigHelperOrderUnsafeError(gitHTTPURL string) error {
	return errs.NewValidationError(
		errs.SubtypeFailedPrecondition,
		"a generic Git credential.helper is applied after the lark-cli helper for %s, so the credential-helper reset cannot isolate it",
		gitHTTPURL,
	).WithHint("a global credential.helper (possibly from a later included config file) still participates for this URL; move or remove it, then retry")
}

func gitConfigSectionHasExtraKeysError(gitHTTPURL string) error {
	return errs.NewValidationError(
		errs.SubtypeFailedPrecondition,
		"the URL-scoped git credential section for %s holds keys other than helper and useHttpPath, which cannot be safely relocated",
		gitHTTPURL,
	).WithHint("the credential-helper reset needs to move this section to the end of the global config, but that would discard the extra keys; remove them (e.g. credential.<url>.username) or move the lark-cli section manually, then retry")
}

func gitConfigForeignRemainsWarning(gitHTTPURL string) error {
	return errs.NewValidationError(
		errs.SubtypeFailedPrecondition,
		"removed the lark-cli credential helper for %s but a third-party credential.helper remains",
		gitHTTPURL,
	).WithHint("the URL-scoped configuration also contained a non-lark-cli helper, which was left in place; remove it manually if it is no longer needed")
}

const rollbackFailureHint = "automatic rollback could not restore the previous Git credential configuration; inspect the URL-scoped global Git credential settings before retrying"

func withRollbackFailureHint(err error) error {
	var internalErr *errs.InternalError
	if errors.As(err, &internalErr) {
		return internalErr.WithHint(rollbackFailureHint)
	}
	var validationErr *errs.ValidationError
	if errors.As(err, &validationErr) {
		return validationErr.WithHint(rollbackFailureHint)
	}
	return err
}

func (g GlobalGitConfig) helperCommand(appID string) string {
	if g.HelperCommand != "" {
		return g.HelperCommand
	}
	return "!lark-cli apps git-credential-helper --app-id " + shellQuoteArg(appID)
}

func gitCredentialKey(gitHTTPURL, name string) string {
	return "credential." + gitHTTPURL + "." + name
}

func readCredentialConfig(ctx context.Context, gitHTTPURL string) (credentialConfigState, error) {
	writableOrigin, err := writableGlobalOrigin()
	if err != nil {
		return credentialConfigState{}, err
	}
	helpers, err := gitConfigGetAllOriginValues(ctx, gitCredentialKey(gitHTTPURL, "helper"))
	if err != nil {
		return credentialConfigState{}, err
	}
	useHTTPPath, err := gitConfigGetAllOriginValues(ctx, gitCredentialKey(gitHTTPURL, "useHttpPath"))
	if err != nil {
		return credentialConfigState{}, err
	}
	return credentialConfigState{
		Helpers:        helpers,
		UseHTTPPath:    useHTTPPath,
		WritableOrigin: writableOrigin,
	}, nil
}

func gitConfigGetAllOriginValues(ctx context.Context, key string) ([]configValue, error) {
	out, err := exec.CommandContext(ctx, "git", "config", "--global", "--includes", "--null", "--show-origin", "--get-all", key).Output()
	if err == nil {
		return parseOriginValues(out)
	}
	if isGitConfigGetMissing(err) {
		return nil, nil
	}
	return nil, errs.NewInternalError(errs.SubtypeExternalTool, "git config get-all %s failed: %v", key, err).WithCause(err)
}

func parseOriginValues(raw []byte) ([]configValue, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if raw[len(raw)-1] != 0 {
		return nil, errs.NewInternalError(errs.SubtypeExternalTool, "git config --show-origin returned output without a NUL terminator")
	}
	tokens := bytes.Split(raw[:len(raw)-1], []byte{0})
	if len(tokens)%2 != 0 {
		return nil, errs.NewInternalError(errs.SubtypeExternalTool, "git config --show-origin returned an incomplete origin/value pair")
	}
	values := make([]configValue, 0, len(tokens)/2)
	for i := 0; i < len(tokens); i += 2 {
		if len(tokens[i]) == 0 {
			return nil, errs.NewInternalError(errs.SubtypeExternalTool, "git config --show-origin returned an empty origin")
		}
		values = append(values, configValue{Origin: canonicalOrigin(string(tokens[i])), Value: string(tokens[i+1])})
	}
	return values, nil
}

// larkResetDefeatsLaterHelpers reports whether the URL-scoped empty-helper
// reset actually isolates the credential chain for gitHTTPURL. Git's empty
// helper ("") clears only helpers that parse BEFORE it; a generic
// credential.helper — or one from a later [include] — that parses AFTER the
// lark-cli section still participates in get/store/erase. It returns true iff
// the lark-cli canonical helper is the LAST credential helper git would apply
// for the URL (i.e. no helper parses after it).
func larkResetDefeatsLaterHelpers(ctx context.Context, gitHTTPURL, canonical string) (bool, error) {
	order, err := readHelperFillOrder(ctx, gitHTTPURL)
	if err != nil {
		return false, err
	}
	lastLark := -1
	lastAny := -1
	for i, value := range order {
		lastAny = i
		if value == canonical {
			lastLark = i
		}
	}
	if lastLark < 0 {
		return false, nil
	}
	return lastLark == lastAny, nil
}

// readHelperFillOrder returns, in git's parse (fill) order, the values of every
// credential.helper that applies to gitHTTPURL: the generic credential.helper
// and the exact URL-scoped credential.<url>.helper. Order is derived from
// `git config --includes --show-origin -z --list`, which streams every entry in
// true parse order across included files — the faithful oracle for the order in
// which `git credential` invokes helpers (git config --get-urlmatch is not).
func readHelperFillOrder(ctx context.Context, gitHTTPURL string) ([]string, error) {
	out, err := exec.CommandContext(ctx, "git", "config", "--global", "--includes", "--show-origin", "-z", "--list").Output()
	if err != nil {
		if isGitConfigGetMissing(err) {
			return nil, nil
		}
		return nil, errs.NewInternalError(errs.SubtypeExternalTool, "git config --list failed: %v", err).WithCause(err)
	}
	genericKey := "credential.helper"
	// Git lowercases the section and the final key component but preserves the
	// subsection (the URL) verbatim, both when storing and when printing
	// --list. gitCredentialKey already emits a lowercase section/component, so
	// compare against it with the URL unchanged. Lowercasing the whole key would
	// drop entries for URLs with an uppercase path segment (NormalizeGitHTTPURL
	// preserves path case), wrongly making the reset look ineffective.
	scopedKey := gitCredentialKey(gitHTTPURL, "helper")
	var order []string
	// Records are NUL-terminated; within each record the origin and the
	// "key\nvalue" pair are separated by a NUL as well, so the stream is
	// origin\0key\nvalue\0 repeating. Git lowercases the section and final key
	// component but preserves the subsection (the URL) verbatim.
	for _, record := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if record == "" {
			continue
		}
		keyValue := record
		if idx := strings.IndexByte(record, 0); idx >= 0 {
			keyValue = record[idx+1:]
		}
		key, value, ok := strings.Cut(keyValue, "\n")
		if !ok {
			continue
		}
		if key == genericKey || key == scopedKey {
			order = append(order, value)
		}
	}
	return order, nil
}

// repositionLarkSection moves the URL-scoped lark-cli credential section to the
// end of the writable global config file by removing and re-adding it. This
// makes the empty-helper reset parse AFTER any earlier generic helper (and
// after earlier [include] directives), so the reset defeats them. Only the
// writable file is touched; values that live in included files are unaffected.
//
// --remove-section drops every key in the section, but SetHelper only tracks
// (and can roll back) helper and useHttpPath. If the user set any other key on
// the same URL-scoped section (e.g. credential.<url>.username), repositioning
// would silently discard it with no way to restore it, so we refuse and return
// a typed validation error rather than destroying user config.
func repositionLarkSection(ctx context.Context, gitHTTPURL, canonical string) error {
	extra, err := sectionHasUntrackedKeys(ctx, gitHTTPURL)
	if err != nil {
		return err
	}
	if extra {
		return gitConfigSectionHasExtraKeysError(gitHTTPURL)
	}
	if err := gitConfigRemoveSection(ctx, "credential."+gitHTTPURL); err != nil {
		return err
	}
	helperKey := gitCredentialKey(gitHTTPURL, "helper")
	if err := gitConfigAdd(ctx, helperKey, ""); err != nil {
		return err
	}
	if err := gitConfigAdd(ctx, helperKey, canonical); err != nil {
		return err
	}
	return gitConfigAdd(ctx, gitCredentialKey(gitHTTPURL, "useHttpPath"), "true")
}

// sectionHasUntrackedKeys reports whether the URL-scoped credential section in
// the writable global config holds any key other than the ones SetHelper
// tracks (helper, useHttpPath). Git lowercases the final key component, so the
// comparison is against the lowercased names. A missing section (git exit 1) is
// not an error and reports no extra keys.
//
// The scan uses --no-includes so it sees only keys in the writable file itself:
// repositionLarkSection's --remove-section edits just that file, so a key that
// lives only in an included config would not be discarded and must not trip the
// "extra keys" refusal.
func sectionHasUntrackedKeys(ctx context.Context, gitHTTPURL string) (bool, error) {
	pattern := "^" + regexp.QuoteMeta("credential."+gitHTTPURL+".")
	out, err := exec.CommandContext(ctx, "git", "config", "--global", "--no-includes", "-z", "--name-only", "--get-regexp", pattern).Output()
	if err != nil {
		if isGitConfigGetMissing(err) {
			return false, nil
		}
		return false, errs.NewInternalError(errs.SubtypeExternalTool, "git config get-regexp %s failed: %v", pattern, err).WithCause(err)
	}
	prefix := "credential." + gitHTTPURL + "."
	for _, name := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if name == "" {
			continue
		}
		// Git lowercases the final key component but preserves the subsection
		// (URL) verbatim, so strip the verbatim prefix and compare the component.
		component := strings.ToLower(strings.TrimPrefix(name, prefix))
		if component != "helper" && component != "usehttppath" {
			return true, nil
		}
	}
	return false, nil
}

// canonicalOrigin normalizes a git config origin so origins produced by
// git's --show-origin (echoed verbatim from GIT_CONFIG_GLOBAL / $HOME /
// $XDG_CONFIG_HOME) compare equal to origins this package derives itself.
// Git does not clean these paths, so a trailing-slash $HOME or a path with
// embedded "./" or "//" yields e.g. "file:/home/u//.gitconfig" from git but
// "file:/home/u/.gitconfig" from filepath.Join here; cleaning both keeps
// ownership classification, readback verification, and rollback correct.
// Relative GIT_CONFIG_GLOBAL values are read from the same env on both sides,
// so cleaning (rather than absolutizing) is sufficient and avoids touching the
// filesystem. Non-file origins (command line, blob) are left unchanged so they
// never match a writable file origin.
func canonicalOrigin(origin string) string {
	path := strings.TrimPrefix(origin, "file:")
	if path == origin {
		return origin
	}
	return "file:" + filepath.Clean(path)
}

func writableGlobalOrigin() (string, error) {
	if globalPath, ok := os.LookupEnv("GIT_CONFIG_GLOBAL"); ok && globalPath != "" {
		return gitConfigFileOrigin(globalPath)
	}

	home, err := vfs.UserHomeDir()
	if err != nil {
		return "", errs.NewInternalError(errs.SubtypeFileIO, "resolve home directory for global git config: %v", err).WithCause(err)
	}
	if home == "" {
		return "", errs.NewInternalError(errs.SubtypeFileIO, "resolve home directory for global git config: empty home directory")
	}
	homeConfig := filepath.Join(home, ".gitconfig")
	homeExists, err := configFileExists(homeConfig)
	if err != nil {
		return "", err
	}
	if homeExists {
		return gitConfigFileOrigin(homeConfig)
	}

	xdgHome, ok := os.LookupEnv("XDG_CONFIG_HOME")
	if !ok || xdgHome == "" {
		xdgHome = filepath.Join(home, ".config")
	}
	xdgConfig := filepath.Join(xdgHome, "git", "config")
	xdgExists, err := configFileExists(xdgConfig)
	if err != nil {
		return "", err
	}
	if xdgExists {
		return gitConfigFileOrigin(xdgConfig)
	}
	return gitConfigFileOrigin(homeConfig)
}

func configFileExists(path string) (bool, error) {
	if _, err := vfs.Stat(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, errs.NewInternalError(errs.SubtypeFileIO, "inspect global git config path %s: %v", path, err).WithCause(err)
	}
	return true, nil
}

func gitConfigFileOrigin(path string) (string, error) {
	return canonicalOrigin("file:" + path), nil
}

func classifyManagedState(state credentialConfigState, canonical string) managedKind {
	valuesFromWritableOrigin := func(values []configValue) bool {
		for _, value := range values {
			if value.Origin != state.WritableOrigin {
				return false
			}
		}
		return true
	}
	// Cross-origin / include-sourced values are never lark-cli-owned: we can
	// only safely rewrite values that live in the single writable global file.
	if !valuesFromWritableOrigin(state.Helpers) || !valuesFromWritableOrigin(state.UseHTTPPath) {
		return managedNone
	}
	// useHttpPath must be absent or exactly one "true"; any other shape
	// (false, duplicates, arbitrary value) is not a shape lark-cli writes.
	useHTTPPathOwned := len(state.UseHTTPPath) == 0 ||
		(len(state.UseHTTPPath) == 1 && strings.EqualFold(state.UseHTTPPath[0].Value, "true"))
	if !useHTTPPathOwned {
		return managedNone
	}

	if len(state.Helpers) == 0 {
		if len(state.UseHTTPPath) == 0 {
			return managedAbsent
		}
		// useHttpPath=true with no helper is a lark-cli residue (the helper was
		// deleted but useHttpPath was not yet, or vice versa). Recoverable.
		return managedPartial
	}

	var larkCanonical, larkReset, foreign int
	for _, h := range state.Helpers {
		switch h.Value {
		case canonical:
			larkCanonical++
		case "":
			larkReset++
		default:
			foreign++
		}
	}
	switch {
	case foreign > 0 && larkCanonical > 0:
		return managedMixed
	case foreign > 0:
		// Only foreign helpers (a bare reset marker without our canonical helper
		// is not treated as lark-owned): third-party configuration.
		return managedForeign
	case larkCanonical == 0:
		// Helpers present but only bare reset markers: not a shape lark-cli
		// writes on its own; leave it alone.
		return managedNone
	case len(state.Helpers) == 1 && larkReset == 0:
		return managedLegacy
	case len(state.Helpers) == 2 && state.Helpers[0].Value == "" && state.Helpers[1].Value == canonical && len(state.UseHTTPPath) == 1:
		return managedCurrent
	default:
		// Canonical helper present in some other lark-cli arrangement (e.g. the
		// reset+canonical pair written without useHttpPath): recoverable.
		return managedPartial
	}
}

func gitConfigReplaceAll(ctx context.Context, key string, values []string) error {
	known := []configValue(nil)
	return gitConfigReplaceAllTracked(ctx, key, values, &known, "")
}

func gitConfigReplaceAllTracked(ctx context.Context, key string, values []string, known *[]configValue, origin string) error {
	if err := gitConfigUnsetAll(ctx, key); err != nil {
		return err
	}
	*known = nil
	for _, value := range values {
		if err := gitConfigAdd(ctx, key, value); err != nil {
			return err
		}
		*known = append(*known, configValue{Origin: origin, Value: value})
	}
	return nil
}

// gitConfigRewriteHelpersScoped rewrites the URL-scoped helper list from the
// exact set of values observed in the ownership snapshot (oldValues) to the
// desired list (newValues), deleting ONLY the known old values by exact match
// rather than clearing the whole key.
//
// A whole-key --unset-all would also delete any helper a concurrent, non-lark
// process inserted into the same list AFTER the ownership read but BEFORE this
// rewrite — the subsequent re-add of the canonical pair would then leave a
// readback that matches `known`, so the deletion of the third party's value
// goes unnoticed and SetHelper returns success. Deleting only the snapshot's
// own values leaves such an interloping helper in place, so the readback
// diverges from `known` and SetHelper fails closed with the foreign value
// preserved. oldValues comes from a snapshot the ownership gate already accepted
// (Absent/Legacy/Current/Partial), so it contains only lark-cli-owned values
// ("" and the canonical helper); deleting them by exact value never removes a
// third party's helper.
func gitConfigRewriteHelpersScoped(ctx context.Context, key string, oldValues, newValues []string, known *[]configValue, origin string) error {
	for _, value := range dedupeStrings(oldValues) {
		if err := unsetHelperValueScoped(ctx, key, value); err != nil {
			return err
		}
	}
	*known = nil
	for _, value := range newValues {
		if err := gitConfigAdd(ctx, key, value); err != nil {
			return err
		}
		*known = append(*known, configValue{Origin: origin, Value: value})
	}
	return nil
}

func dedupeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func gitConfigSet(ctx context.Context, key, value string) error {
	return runGitConfig(ctx, "set", key, "--global", key, value)
}

func gitConfigAdd(ctx context.Context, key, value string) error {
	return runGitConfig(ctx, "add", key, "--global", "--add", key, value)
}

func gitConfigUnsetAll(ctx context.Context, key string) error {
	err := exec.CommandContext(ctx, "git", "config", "--global", "--unset-all", key).Run()
	if err == nil || isGitConfigUnsetMissing(err) {
		return nil
	}
	return errs.NewInternalError(errs.SubtypeExternalTool, "git config unset-all %s failed: %v", key, err).WithCause(err)
}

// unsetHelperValueScoped removes only the occurrences of key whose value equals
// value exactly, leaving other values (foreign helpers) in place. It anchors a
// QuoteMeta-escaped pattern so a value with regex metacharacters matches
// literally. A no-match (git exit 5) is not an error.
func unsetHelperValueScoped(ctx context.Context, key, value string) error {
	pattern := "^" + regexp.QuoteMeta(value) + "$"
	err := exec.CommandContext(ctx, "git", "config", "--global", "--unset-all", key, pattern).Run()
	if err == nil || isGitConfigUnsetMissing(err) {
		return nil
	}
	return errs.NewInternalError(errs.SubtypeExternalTool, "git config unset-all %s (value-scoped) failed: %v", key, err).WithCause(err)
}

// gitConfigRemoveSection removes an entire config section from the writable
// global file. It is only used to reposition a section that is known to exist,
// so any error is surfaced as an external-tool failure.
func gitConfigRemoveSection(ctx context.Context, section string) error {
	if err := exec.CommandContext(ctx, "git", "config", "--global", "--remove-section", section).Run(); err != nil {
		return errs.NewInternalError(errs.SubtypeExternalTool, "git config remove-section %s failed: %v", section, err).WithCause(err)
	}
	return nil
}

func runGitConfig(ctx context.Context, operation, key string, args ...string) error {
	commandArgs := append([]string{"config"}, args...)
	if err := exec.CommandContext(ctx, "git", commandArgs...).Run(); err != nil {
		return errs.NewInternalError(errs.SubtypeExternalTool, "git config %s %s failed: %v", operation, key, err).WithCause(err)
	}
	return nil
}

func isGitConfigGetMissing(err error) bool {
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
		return true
	}
	return false
}

func isGitConfigUnsetMissing(err error) bool {
	if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 5 {
		return true
	}
	return false
}

func shellQuoteArg(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
