// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package vc

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/larksuite/cli/internal/core"
	clie2e "github.com/larksuite/cli/tests/cli_e2e"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

const (
	vcMeetingManagementPermissionDeniedCode     = 10005
	vcMeetingManagementLarkUserType             = 1
	vcMeetingManagementDestructiveLiveEnv       = "LARK_CLI_E2E_VC_MEETING_MANAGEMENT_DESTRUCTIVE"
	vcMeetingManagementProvisionerEnv           = "LARK_CLI_E2E_VC_MEETING_MANAGEMENT_PROVISIONER"
	vcMeetingManagementExpectedEnvironmentEnv   = "LARK_CLI_E2E_VC_MEETING_MANAGEMENT_ENVIRONMENT"
	vcMeetingManagementManifestSchemaVersion    = 2
	vcMeetingManagementCreatedBy                = "vc-meeting-management-provisioner"
	vcMeetingManagementSubjectIDSource          = "provisioner_verified_user_token_subject"
	vcMeetingManagementProvisionerCreateTimeout = 2 * time.Minute
	vcMeetingManagementProvisionerCleanupTimout = 90 * time.Second
	vcMeetingManagementReadbackTimeout          = 45 * time.Second
	vcMeetingManagementMaxCreatedAge            = 10 * time.Minute
	vcMeetingManagementMaxFutureSkew            = 2 * time.Minute
	vcMeetingManagementMaxTTL                   = 2 * time.Hour
	vcMeetingManagementOnlineEnvironment        = "online"
	vcMeetingManagementFeishuOpenAPIHost        = "https://open.feishu.cn"
	vcMeetingManagementLarkOpenAPIHost          = "https://open.larksuite.com"
	vcMeetingManagementMaxUserTokenLength       = 4096
	vcMeetingManagementMinCleanupSuffixLength   = 16
	vcMeetingManagementMaxCleanupTokenLength    = 512
	vcMeetingManagementMaxActorOpenIDLength     = 256
	vcMeetingManagementCleanupFailureProbeEnv   = "LARK_CLI_E2E_VC_MEETING_MANAGEMENT_CLEANUP_FAILURE_PROBE"
)

var (
	vcMeetingManagementCleanupSuffixPattern  = regexp.MustCompile(`^[A-Za-z0-9._~-]+$`)
	vcMeetingManagementActorOpenIDPattern    = regexp.MustCompile(`^ou_[A-Za-z0-9_-]+$`)
	vcMeetingManagementNumericSubjectPattern = regexp.MustCompile(`^[1-9][0-9]*$`)
)

type vcParticipantTuple struct {
	ID       string `json:"id"`
	UserType int64  `json:"user_type"`
}

type vcMeetingManagementManifestCredential struct {
	UserAccessToken      string `json:"user_access_token"`
	TokenSubjectID       string `json:"token_subject_id"`
	TokenSubjectUserType int64  `json:"token_subject_user_type"`
	TokenSubjectOpenID   string `json:"token_subject_open_id"`
	TokenSubjectSource   string `json:"token_subject_source"`
}

type vcMeetingManagementManifestCredentials struct {
	Host   vcMeetingManagementManifestCredential `json:"host"`
	Cohost vcMeetingManagementManifestCredential `json:"cohost"`
	Normal vcMeetingManagementManifestCredential `json:"normal"`
}

type vcMeetingEndProvisionCase struct {
	MeetingID string             `json:"meeting_id"`
	HostUser  vcParticipantTuple `json:"host_user"`
	Actor     vcParticipantTuple `json:"actor"`
}

type vcMeetingEndProvisionManifest struct {
	Host   vcMeetingEndProvisionCase `json:"host"`
	Cohost vcMeetingEndProvisionCase `json:"cohost"`
	Normal vcMeetingEndProvisionCase `json:"normal"`
}

type vcMeetingKickoutProvisionCase struct {
	MeetingID string             `json:"meeting_id"`
	HostUser  vcParticipantTuple `json:"host_user"`
	Actor     vcParticipantTuple `json:"actor"`
	Target    vcParticipantTuple `json:"target"`
}

type vcMeetingKickoutProvisionPartialCase struct {
	MeetingID        string             `json:"meeting_id"`
	HostUser         vcParticipantTuple `json:"host_user"`
	Actor            vcParticipantTuple `json:"actor"`
	SuccessTarget    vcParticipantTuple `json:"success_target"`
	FailedTarget     vcParticipantTuple `json:"failed_target"`
	FailedTargetMode string             `json:"failed_target_mode"`
}

type vcMeetingKickoutProvisionManifest struct {
	Host    vcMeetingKickoutProvisionCase        `json:"host"`
	Cohost  vcMeetingKickoutProvisionCase        `json:"cohost"`
	Normal  vcMeetingKickoutProvisionCase        `json:"normal"`
	Partial vcMeetingKickoutProvisionPartialCase `json:"partial"`
}

type vcMeetingManagementProvisionManifest struct {
	Schema       int                                    `json:"schema"`
	Suite        string                                 `json:"suite,omitempty"`
	RunID        string                                 `json:"run_id"`
	CreatedBy    string                                 `json:"created_by"`
	Environment  string                                 `json:"environment"`
	CreatedAt    string                                 `json:"created_at"`
	ExpiresAt    string                                 `json:"expires_at"`
	Dedicated    bool                                   `json:"dedicated"`
	Disposable   bool                                   `json:"disposable"`
	Ownership    string                                 `json:"ownership"`
	CleanupToken string                                 `json:"cleanup_token"`
	AppID        string                                 `json:"app_id"`
	Brand        string                                 `json:"brand"`
	OpenAPIHost  string                                 `json:"effective_openapi_host"`
	Credentials  vcMeetingManagementManifestCredentials `json:"credentials"`
	End          *vcMeetingEndProvisionManifest         `json:"end,omitempty"`
	Kickout      *vcMeetingKickoutProvisionManifest     `json:"kickout,omitempty"`
}

type vcMeetingManagementRuntime struct {
	appID             string
	brand             string
	isolatedConfigDir string
}

type vcExpectedParticipantRole struct {
	Tuple    vcParticipantTuple
	IsHost   bool
	IsCohost bool
}

func requireVCMeetingManagementProvisionManifest(t *testing.T, suite string) vcMeetingManagementProvisionManifest {
	t.Helper()

	executable := requireVCMeetingManagementProvisioner(t)

	expectedEnvironment := os.Getenv(vcMeetingManagementExpectedEnvironmentEnv)
	if expectedEnvironment == "" {
		t.Fatalf(
			"REQUIRED-LANE BLOCKER: set %s so the provisioned fixture manifest can prove which environment it belongs to",
			vcMeetingManagementExpectedEnvironmentEnv,
		)
	}
	if strings.TrimSpace(expectedEnvironment) != expectedEnvironment {
		t.Fatalf("REQUIRED-LANE BLOCKER: %s must not contain surrounding whitespace", vcMeetingManagementExpectedEnvironmentEnv)
	}
	if expectedEnvironment != vcMeetingManagementOnlineEnvironment {
		t.Fatalf("REQUIRED-LANE BLOCKER: unsupported VC fixture environment %q; only %q has a fixed OpenAPI host mapping", expectedEnvironment, vcMeetingManagementOnlineEnvironment)
	}

	runID := newVCMeetingManagementRunID(t, suite)
	provisionerEnv := vcMeetingManagementProvisionerEnvironment(expectedEnvironment)
	cleanupToken := newVCMeetingManagementCleanupToken(t, suite, runID)

	// Arm cleanup before create because a provisioner may create only part of a
	// fixture and then exit non-zero. Cleanup must therefore be idempotent for
	// this known suite/run_id/cleanup_token tuple, including when nothing was
	// created.
	t.Cleanup(func() {
		runVCMeetingManagementProvisionerCleanup(t, executable, provisionerEnv, suite, runID, cleanupToken)
	})

	rawManifest := runVCMeetingManagementProvisionerCreate(t, executable, provisionerEnv, suite, runID, cleanupToken)

	manifest, err := decodeVCMeetingManagementManifest(rawManifest)
	if err != nil {
		t.Fatalf("VC fixture provisioner create returned an invalid manifest for suite %s run_id=%s", suite, runID)
	}
	validateVCMeetingManagementManifestBase(t, manifest, suite, runID, expectedEnvironment, cleanupToken)
	return manifest
}

func TestVCMeetingManagementProvisionerCleanupRunsAfterPartialCreateFailure(t *testing.T) {
	if os.Getenv(vcMeetingManagementCleanupFailureProbeEnv) == "1" {
		requireVCMeetingManagementProvisionManifest(t, "end")
		t.Fatal("fake provisioner create unexpectedly succeeded")
	}

	tempDir := t.TempDir()
	provisionerPath := filepath.Join(tempDir, "fake-vc-meeting-management-provisioner")
	const provisioner = `#!/bin/sh
set -eu
case "${1:-}" in
create)
  shift
  printf '%s\n' "$@" > "$0.create.args"
  cat > "$0.create.stdin"
  : > "$0.resource"
  exit 23
  ;;
cleanup)
  shift
  printf '%s\n' "$@" > "$0.cleanup.args"
  cat > "$0.cleanup.stdin"
  rm -f "$0.resource"
  : > "$0.cleanup.marker"
  ;;
*)
  exit 64
  ;;
esac
`
	require.NoError(t, os.WriteFile(provisionerPath, []byte(provisioner), 0o700))

	t.Setenv(vcMeetingManagementCleanupFailureProbeEnv, "1")
	t.Setenv(vcMeetingManagementProvisionerEnv, provisionerPath)
	t.Setenv(vcMeetingManagementExpectedEnvironmentEnv, vcMeetingManagementOnlineEnvironment)

	cmd := exec.Command(os.Args[0], "-test.run=^TestVCMeetingManagementProvisionerCleanupRunsAfterPartialCreateFailure$")
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	require.Error(t, err, "failure probe must observe the injected create failure")
	require.Contains(t, string(output), "VC fixture provisioner create failed for suite end")

	readArtifact := func(suffix string) []byte {
		t.Helper()
		content, readErr := os.ReadFile(provisionerPath + suffix)
		require.NoError(t, readErr, "missing fake provisioner artifact %s", suffix)
		return content
	}

	createArgs := readArtifact(".create.args")
	cleanupArgs := readArtifact(".cleanup.args")
	require.Equal(t, createArgs, cleanupArgs, "cleanup must target the same suite and run_id as create")
	args := strings.Split(strings.TrimSpace(string(createArgs)), "\n")
	require.Len(t, args, 4)
	require.Equal(t, []string{"--suite", "end", "--run-id"}, args[:3])
	require.True(t, strings.HasPrefix(args[3], "end-"), "unexpected run_id %q", args[3])

	createPayload := readArtifact(".create.stdin")
	cleanupPayload := readArtifact(".cleanup.stdin")
	require.JSONEq(t, string(createPayload), string(cleanupPayload), "cleanup must use the token supplied to create")
	var payload struct {
		CleanupToken string `json:"cleanup_token"`
	}
	require.NoError(t, json.Unmarshal(cleanupPayload, &payload))
	require.True(t, strings.HasPrefix(payload.CleanupToken, vcMeetingManagementCleanupTokenPrefix("end", args[3])))

	_, statErr := os.Stat(provisionerPath + ".resource")
	require.ErrorIs(t, statErr, os.ErrNotExist, "cleanup must remove the resource marker left before create failed")
	readArtifact(".cleanup.marker")
}

func requireVCMeetingManagementDestructiveOptIn(t *testing.T) {
	t.Helper()

	value, present := os.LookupEnv(vcMeetingManagementDestructiveLiveEnv)
	if !present || value == "" {
		t.Skipf("set %s=1 in the dedicated VC management lane to run destructive live E2E tests", vcMeetingManagementDestructiveLiveEnv)
	}
	if value != "1" {
		t.Fatalf("%s must be exactly 1 when destructive VC management live E2E tests are enabled", vcMeetingManagementDestructiveLiveEnv)
	}
}

func requireVCMeetingManagementProvisioner(t *testing.T) string {
	t.Helper()

	executable := os.Getenv(vcMeetingManagementProvisionerEnv)
	if executable == "" {
		t.Fatalf(
			"REQUIRED-LANE BLOCKER: set %s to an executable that supports `create --suite <suite> --run-id <id>` and `cleanup --suite <suite> --run-id <id>` with a cleanup_token JSON request for dedicated disposable VC fixtures",
			vcMeetingManagementProvisionerEnv,
		)
	}
	if strings.TrimSpace(executable) != executable {
		t.Fatalf("REQUIRED-LANE BLOCKER: %s must not contain surrounding whitespace", vcMeetingManagementProvisionerEnv)
	}
	if !filepath.IsAbs(executable) || filepath.Clean(executable) != executable {
		t.Fatalf("REQUIRED-LANE BLOCKER: %s must be a clean absolute path", vcMeetingManagementProvisionerEnv)
	}
	resolvedExecutable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatalf("REQUIRED-LANE BLOCKER: cannot resolve %s: %v", vcMeetingManagementProvisionerEnv, err)
	}
	if !filepath.IsAbs(resolvedExecutable) || filepath.Clean(resolvedExecutable) != resolvedExecutable {
		t.Fatalf("REQUIRED-LANE BLOCKER: %s must resolve to a clean absolute path", vcMeetingManagementProvisionerEnv)
	}
	info, err := os.Stat(resolvedExecutable)
	if err != nil {
		t.Fatalf("REQUIRED-LANE BLOCKER: cannot inspect resolved %s: %v", vcMeetingManagementProvisionerEnv, err)
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("REQUIRED-LANE BLOCKER: %s must resolve to a regular executable file", vcMeetingManagementProvisionerEnv)
	}
	return resolvedExecutable
}

func vcMeetingManagementProvisionerEnvironment(expectedEnvironment string) []string {
	return []string{
		"PATH=/usr/bin:/bin",
		"LANG=C",
		"LC_ALL=C",
		vcMeetingManagementExpectedEnvironmentEnv + "=" + expectedEnvironment,
	}
}

func runVCMeetingManagementProvisionerCreate(t *testing.T, executable string, environment []string, suite, runID, cleanupToken string) []byte {
	t.Helper()
	payload, err := json.Marshal(struct {
		CleanupToken string `json:"cleanup_token"`
	}{
		CleanupToken: cleanupToken,
	})
	if err != nil {
		t.Fatalf("VC fixture create payload failed for suite %s run_id=%s", suite, runID)
	}

	ctx, cancel := context.WithTimeout(context.Background(), vcMeetingManagementProvisionerCreateTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, executable, "create", "--suite", suite, "--run-id", runID)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = append([]string(nil), environment...)

	var stdout bytes.Buffer
	cmd.Stdout = &stdout

	if err := cmd.Run(); err != nil {
		t.Fatalf("VC fixture provisioner create failed for suite %s run_id=%s (exit=%d)", suite, runID, execCommandExitCode(err))
	}
	return bytes.TrimSpace(stdout.Bytes())
}

func runVCMeetingManagementProvisionerCleanup(t *testing.T, executable string, environment []string, suite, runID, cleanupToken string) {
	t.Helper()
	payload, err := json.Marshal(struct {
		CleanupToken string `json:"cleanup_token"`
	}{
		CleanupToken: cleanupToken,
	})
	if err != nil {
		t.Errorf("VC fixture cleanup payload failed for suite %s run_id=%s", suite, runID)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), vcMeetingManagementProvisionerCleanupTimout)
	defer cancel()

	cmd := exec.CommandContext(ctx, executable, "cleanup", "--suite", suite, "--run-id", runID)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Env = append([]string(nil), environment...)

	if err := cmd.Run(); err != nil {
		t.Errorf("VC fixture cleanup failed for suite %s run_id=%s (exit=%d)", suite, runID, execCommandExitCode(err))
	}
}

func decodeVCMeetingManagementManifest(raw []byte) (vcMeetingManagementProvisionManifest, error) {
	var manifest vcMeetingManagementProvisionManifest

	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return manifest, errors.New("empty manifest")
	}

	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return manifest, err
	}

	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return manifest, errors.New("manifest must contain exactly one JSON value")
		}
		return manifest, err
	}

	return manifest, nil
}

func validateVCMeetingManagementManifestBase(t *testing.T, manifest vcMeetingManagementProvisionManifest, suite, runID, expectedEnvironment, cleanupToken string) {
	t.Helper()

	if manifest.Schema != vcMeetingManagementManifestSchemaVersion {
		t.Fatalf("invalid VC fixture manifest schema %d: want %d", manifest.Schema, vcMeetingManagementManifestSchemaVersion)
	}
	if manifest.Suite != suite {
		t.Fatalf("invalid VC fixture manifest suite %q: want %q", manifest.Suite, suite)
	}
	if manifest.RunID != runID {
		t.Fatalf("invalid VC fixture manifest run_id %q: want %q", manifest.RunID, runID)
	}
	if manifest.CreatedBy != vcMeetingManagementCreatedBy {
		t.Fatalf("invalid VC fixture manifest created_by %q: want %q", manifest.CreatedBy, vcMeetingManagementCreatedBy)
	}
	if manifest.Environment != expectedEnvironment {
		t.Fatalf("invalid VC fixture manifest environment %q: want %q", manifest.Environment, expectedEnvironment)
	}

	createdAt, err := time.Parse(time.RFC3339, manifest.CreatedAt)
	if err != nil {
		t.Fatalf("invalid VC fixture manifest created_at %q: want RFC3339", manifest.CreatedAt)
	}
	expiresAt, err := time.Parse(time.RFC3339, manifest.ExpiresAt)
	if err != nil {
		t.Fatalf("invalid VC fixture manifest expires_at %q: want RFC3339", manifest.ExpiresAt)
	}
	now := time.Now()
	if createdAt.After(now.Add(vcMeetingManagementMaxFutureSkew)) {
		t.Fatalf("invalid VC fixture manifest: created_at %s is too far in the future", manifest.CreatedAt)
	}
	if createdAt.Before(now.Add(-vcMeetingManagementMaxCreatedAge)) {
		t.Fatalf("invalid VC fixture manifest: created_at %s is too old", manifest.CreatedAt)
	}
	if !createdAt.Before(expiresAt) {
		t.Fatalf("invalid VC fixture manifest: expires_at %s must be after created_at %s", manifest.ExpiresAt, manifest.CreatedAt)
	}
	if !now.Before(expiresAt) {
		t.Fatalf("invalid VC fixture manifest: expires_at %s is not in the future", manifest.ExpiresAt)
	}
	if expiresAt.Sub(createdAt) > vcMeetingManagementMaxTTL {
		t.Fatalf("invalid VC fixture manifest: ttl %s exceeds max %s", expiresAt.Sub(createdAt), vcMeetingManagementMaxTTL)
	}

	if !manifest.Dedicated {
		t.Fatal("invalid VC fixture manifest: dedicated must be true")
	}
	if !manifest.Disposable {
		t.Fatal("invalid VC fixture manifest: disposable must be true")
	}
	if manifest.Ownership != vcMeetingManagementOwnership(suite, runID) {
		t.Fatalf("invalid VC fixture manifest ownership %q: want %q", manifest.Ownership, vcMeetingManagementOwnership(suite, runID))
	}
	validateVCMeetingManagementCleanupToken(t, manifest.CleanupToken, suite, runID)
	if manifest.CleanupToken != cleanupToken {
		t.Fatal("invalid VC fixture manifest: cleanup_token must echo the credential supplied to create")
	}
	if strings.TrimSpace(manifest.AppID) == "" {
		t.Fatal("invalid VC fixture manifest: app_id must be non-empty")
	}
	switch manifest.Brand {
	case "feishu", "lark":
	default:
		t.Fatalf("invalid VC fixture manifest brand %q: want feishu|lark", manifest.Brand)
	}

	expectedOpenAPIHost, ok := vcMeetingManagementOpenAPIHost(manifest.Environment, manifest.Brand)
	if !ok {
		t.Fatalf("invalid VC fixture manifest: unsupported environment/brand binding %q/%q", manifest.Environment, manifest.Brand)
	}
	if manifest.OpenAPIHost != expectedOpenAPIHost {
		t.Fatalf("invalid VC fixture manifest effective_openapi_host %q: want %q for environment/brand %q/%q", manifest.OpenAPIHost, expectedOpenAPIHost, manifest.Environment, manifest.Brand)
	}
	if resolvedHost := core.ResolveOpenBaseURL(core.LarkBrand(manifest.Brand)); resolvedHost != manifest.OpenAPIHost {
		t.Fatalf("invalid VC fixture manifest: CLI resolves brand %q to %q, not effective_openapi_host %q", manifest.Brand, resolvedHost, manifest.OpenAPIHost)
	}

	validateVCMeetingManagementCredential(t, "credentials.host", manifest.Credentials.Host)
	validateVCMeetingManagementCredential(t, "credentials.cohost", manifest.Credentials.Cohost)
	validateVCMeetingManagementCredential(t, "credentials.normal", manifest.Credentials.Normal)
}

func requireVCMeetingEndProvisionFixture(t *testing.T) vcMeetingEndLiveFixture {
	t.Helper()

	manifest := requireVCMeetingManagementProvisionManifest(t, "end")
	if manifest.End == nil {
		t.Fatal("invalid VC fixture manifest: end suite payload must be present")
	}

	for _, item := range []struct {
		name       string
		data       vcMeetingEndProvisionCase
		credential vcMeetingManagementManifestCredential
	}{
		{name: "end.host", data: manifest.End.Host, credential: manifest.Credentials.Host},
		{name: "end.cohost", data: manifest.End.Cohost, credential: manifest.Credentials.Cohost},
		{name: "end.normal", data: manifest.End.Normal, credential: manifest.Credentials.Normal},
	} {
		requirePositiveVCMeetingID(t, item.data.MeetingID)
		validateVCParticipantTuple(t, item.name+".host_user", item.data.HostUser)
		validateVCActorCredentialBinding(t, item.name+".actor", item.data.Actor, item.credential)
	}

	runtime := newVCMeetingManagementRuntime(manifest, t.TempDir())
	fixture := vcMeetingEndLiveFixture{
		appID:             manifest.AppID,
		brand:             manifest.Brand,
		hostToken:         manifest.Credentials.Host.UserAccessToken,
		cohostToken:       manifest.Credentials.Cohost.UserAccessToken,
		normalToken:       manifest.Credentials.Normal.UserAccessToken,
		hostMeetingID:     manifest.End.Host.MeetingID,
		cohostMeetingID:   manifest.End.Cohost.MeetingID,
		normalMeetingID:   manifest.End.Normal.MeetingID,
		isolatedConfigDir: runtime.isolatedConfigDir,
	}

	requireDistinctVCFixtureValues(t, "role tokens", fixture.hostToken, fixture.cohostToken, fixture.normalToken)
	requireDistinctVCFixtureValues(t, "role token subject IDs", manifest.Credentials.Host.TokenSubjectID, manifest.Credentials.Cohost.TokenSubjectID, manifest.Credentials.Normal.TokenSubjectID)
	requireDistinctVCFixtureValues(t, "role token subject OpenIDs", manifest.Credentials.Host.TokenSubjectOpenID, manifest.Credentials.Cohost.TokenSubjectOpenID, manifest.Credentials.Normal.TokenSubjectOpenID)
	requireDistinctVCFixtureValues(t, "meeting IDs", fixture.hostMeetingID, fixture.cohostMeetingID, fixture.normalMeetingID)

	requireVCAuthStatusCredentialBinding(t, runtime, manifest.Credentials.Host)
	requireVCAuthStatusCredentialBinding(t, runtime, manifest.Credentials.Cohost)
	requireVCAuthStatusCredentialBinding(t, runtime, manifest.Credentials.Normal)

	assertVCActiveMeetingSnapshot(
		t,
		requireVCMeetingReadback(t, runtime, fixture.hostToken, fixture.hostMeetingID),
		fixture.hostMeetingID,
		manifest.End.Host.HostUser,
		vcExpectedParticipantRole{Tuple: manifest.End.Host.HostUser, IsHost: true},
		vcExpectedParticipantRole{Tuple: manifest.End.Host.Actor, IsHost: true},
	)
	assertVCActiveMeetingSnapshot(
		t,
		requireVCMeetingReadback(t, runtime, fixture.cohostToken, fixture.cohostMeetingID),
		fixture.cohostMeetingID,
		manifest.End.Cohost.HostUser,
		vcExpectedParticipantRole{Tuple: manifest.End.Cohost.HostUser, IsHost: true},
		vcExpectedParticipantRole{Tuple: manifest.End.Cohost.Actor, IsCohost: true},
	)
	assertVCActiveMeetingSnapshot(
		t,
		requireVCMeetingReadback(t, runtime, fixture.normalToken, fixture.normalMeetingID),
		fixture.normalMeetingID,
		manifest.End.Normal.HostUser,
		vcExpectedParticipantRole{Tuple: manifest.End.Normal.HostUser, IsHost: true},
		vcExpectedParticipantRole{Tuple: manifest.End.Normal.Actor},
	)

	return fixture
}

func requireVCKickoutProvisionFixture(t *testing.T) vcMeetingParticipantKickoutLiveFixture {
	t.Helper()

	manifest := requireVCMeetingManagementProvisionManifest(t, "kickout")
	if manifest.Kickout == nil {
		t.Fatal("invalid VC fixture manifest: kickout suite payload must be present")
	}

	for _, item := range []struct {
		name       string
		data       vcMeetingKickoutProvisionCase
		credential vcMeetingManagementManifestCredential
	}{
		{name: "kickout.host", data: manifest.Kickout.Host, credential: manifest.Credentials.Host},
		{name: "kickout.cohost", data: manifest.Kickout.Cohost, credential: manifest.Credentials.Cohost},
		{name: "kickout.normal", data: manifest.Kickout.Normal, credential: manifest.Credentials.Normal},
	} {
		requirePositiveVCMeetingID(t, item.data.MeetingID)
		validateVCParticipantTuple(t, item.name+".host_user", item.data.HostUser)
		validateVCActorCredentialBinding(t, item.name+".actor", item.data.Actor, item.credential)
		validateVCParticipantTuple(t, item.name+".target", item.data.Target)
	}
	requirePositiveVCMeetingID(t, manifest.Kickout.Partial.MeetingID)
	validateVCParticipantTuple(t, "kickout.partial.host_user", manifest.Kickout.Partial.HostUser)
	validateVCActorCredentialBinding(t, "kickout.partial.actor", manifest.Kickout.Partial.Actor, manifest.Credentials.Host)
	validateVCParticipantTuple(t, "kickout.partial.success_target", manifest.Kickout.Partial.SuccessTarget)
	validateVCParticipantTuple(t, "kickout.partial.failed_target", manifest.Kickout.Partial.FailedTarget)
	validateVCFailedTargetMode(t, manifest.Kickout.Partial.FailedTargetMode)

	runtime := newVCMeetingManagementRuntime(manifest, t.TempDir())
	fixture := vcMeetingParticipantKickoutLiveFixture{
		appID:             manifest.AppID,
		brand:             manifest.Brand,
		hostToken:         manifest.Credentials.Host.UserAccessToken,
		cohostToken:       manifest.Credentials.Cohost.UserAccessToken,
		normalToken:       manifest.Credentials.Normal.UserAccessToken,
		hostMeetingID:     manifest.Kickout.Host.MeetingID,
		hostTarget:        vcKickoutParticipantFixture{tuple: manifest.Kickout.Host.Target.TupleString(), id: manifest.Kickout.Host.Target.ID, userType: manifest.Kickout.Host.Target.UserType},
		cohostMeetingID:   manifest.Kickout.Cohost.MeetingID,
		cohostTarget:      vcKickoutParticipantFixture{tuple: manifest.Kickout.Cohost.Target.TupleString(), id: manifest.Kickout.Cohost.Target.ID, userType: manifest.Kickout.Cohost.Target.UserType},
		normalMeetingID:   manifest.Kickout.Normal.MeetingID,
		normalTarget:      vcKickoutParticipantFixture{tuple: manifest.Kickout.Normal.Target.TupleString(), id: manifest.Kickout.Normal.Target.ID, userType: manifest.Kickout.Normal.Target.UserType},
		partialMeetingID:  manifest.Kickout.Partial.MeetingID,
		partialSuccess:    vcKickoutParticipantFixture{tuple: manifest.Kickout.Partial.SuccessTarget.TupleString(), id: manifest.Kickout.Partial.SuccessTarget.ID, userType: manifest.Kickout.Partial.SuccessTarget.UserType},
		partialFailure:    vcKickoutParticipantFixture{tuple: manifest.Kickout.Partial.FailedTarget.TupleString(), id: manifest.Kickout.Partial.FailedTarget.ID, userType: manifest.Kickout.Partial.FailedTarget.UserType},
		isolatedConfigDir: runtime.isolatedConfigDir,
	}

	requireDistinctVCFixtureValues(t, "role tokens", fixture.hostToken, fixture.cohostToken, fixture.normalToken)
	requireDistinctVCFixtureValues(t, "role token subject IDs", manifest.Credentials.Host.TokenSubjectID, manifest.Credentials.Cohost.TokenSubjectID, manifest.Credentials.Normal.TokenSubjectID)
	requireDistinctVCFixtureValues(t, "role token subject OpenIDs", manifest.Credentials.Host.TokenSubjectOpenID, manifest.Credentials.Cohost.TokenSubjectOpenID, manifest.Credentials.Normal.TokenSubjectOpenID)
	requireDistinctVCFixtureValues(t, "meeting IDs", fixture.hostMeetingID, fixture.cohostMeetingID, fixture.normalMeetingID, fixture.partialMeetingID)
	if fixture.partialSuccess.tuple == fixture.partialFailure.tuple {
		t.Fatal("invalid VC fixture manifest: partial success and failure tuples must be distinct")
	}

	requireVCAuthStatusCredentialBinding(t, runtime, manifest.Credentials.Host)
	requireVCAuthStatusCredentialBinding(t, runtime, manifest.Credentials.Cohost)
	requireVCAuthStatusCredentialBinding(t, runtime, manifest.Credentials.Normal)

	assertVCActiveMeetingSnapshot(
		t,
		requireVCMeetingReadback(t, runtime, fixture.hostToken, fixture.hostMeetingID),
		fixture.hostMeetingID,
		manifest.Kickout.Host.HostUser,
		vcExpectedParticipantRole{Tuple: manifest.Kickout.Host.HostUser, IsHost: true},
		vcExpectedParticipantRole{Tuple: manifest.Kickout.Host.Actor, IsHost: true},
		vcExpectedParticipantRole{Tuple: manifest.Kickout.Host.Target},
	)
	assertVCActiveMeetingSnapshot(
		t,
		requireVCMeetingReadback(t, runtime, fixture.cohostToken, fixture.cohostMeetingID),
		fixture.cohostMeetingID,
		manifest.Kickout.Cohost.HostUser,
		vcExpectedParticipantRole{Tuple: manifest.Kickout.Cohost.HostUser, IsHost: true},
		vcExpectedParticipantRole{Tuple: manifest.Kickout.Cohost.Actor, IsCohost: true},
		vcExpectedParticipantRole{Tuple: manifest.Kickout.Cohost.Target},
	)
	assertVCActiveMeetingSnapshot(
		t,
		requireVCMeetingReadback(t, runtime, fixture.normalToken, fixture.normalMeetingID),
		fixture.normalMeetingID,
		manifest.Kickout.Normal.HostUser,
		vcExpectedParticipantRole{Tuple: manifest.Kickout.Normal.HostUser, IsHost: true},
		vcExpectedParticipantRole{Tuple: manifest.Kickout.Normal.Actor},
		vcExpectedParticipantRole{Tuple: manifest.Kickout.Normal.Target},
	)
	assertVCActiveMeetingSnapshot(
		t,
		requireVCMeetingReadback(t, runtime, fixture.hostToken, fixture.partialMeetingID),
		fixture.partialMeetingID,
		manifest.Kickout.Partial.HostUser,
		vcExpectedParticipantRole{Tuple: manifest.Kickout.Partial.HostUser, IsHost: true},
		vcExpectedParticipantRole{Tuple: manifest.Kickout.Partial.Actor, IsHost: true},
		vcExpectedParticipantRole{Tuple: manifest.Kickout.Partial.SuccessTarget},
	)
	assertVCFailedTargetReadbackState(
		t,
		requireVCMeetingReadback(t, runtime, fixture.hostToken, fixture.partialMeetingID),
		manifest.Kickout.Partial.FailedTargetMode,
		manifest.Kickout.Partial.FailedTarget,
	)

	return fixture
}

func newVCMeetingManagementRuntime(manifest vcMeetingManagementProvisionManifest, configDir string) vcMeetingManagementRuntime {
	return vcMeetingManagementRuntime{
		appID:             manifest.AppID,
		brand:             manifest.Brand,
		isolatedConfigDir: configDir,
	}
}

func vcMeetingManagementUserEnv(appID, brand, configDir, token string) map[string]string {
	return map[string]string{
		"LARKSUITE_CLI_APP_ID":              appID,
		"LARKSUITE_CLI_APP_SECRET":          "",
		"LARKSUITE_CLI_USER_ACCESS_TOKEN":   token,
		"LARKSUITE_CLI_TENANT_ACCESS_TOKEN": "",
		"LARKSUITE_CLI_BRAND":               brand,
		"LARKSUITE_CLI_CONFIG_DIR":          configDir,
		"LARKSUITE_CLI_DATA_DIR":            filepath.Join(configDir, "data"),
		"LARKSUITE_CLI_LOG_DIR":             filepath.Join(configDir, "logs"),
		"LARKSUITE_CLI_DEFAULT_AS":          "user",
		"LARKSUITE_CLI_STRICT_MODE":         "user",
		"LARKSUITE_CLI_REMOTE_META":         "off",
		"LARKSUITE_CLI_NO_UPDATE_NOTIFIER":  "1",
		"LARKSUITE_CLI_NO_SKILLS_NOTIFIER":  "1",

		// The shared E2E runner inherits os.Environ. Explicitly neutralize
		// selectors that could replace the manifest credentials, route through
		// an ambient proxy, or switch to another workspace.
		"LARKSUITE_CLI_PROFILE":       "",
		"LARKSUITE_CLI_AUTH_PROXY":    "",
		"LARKSUITE_CLI_PROXY_KEY":     "",
		"LARKSUITE_CLI_PROXY_ENABLE":  "",
		"LARKSUITE_CLI_PROXY_ADDRESS": "",
		"LARKSUITE_CLI_CA_PATH":       "",
		"LARK_CLI_NO_PROXY":           "1",
		"HTTP_PROXY":                  "",
		"http_proxy":                  "",
		"HTTPS_PROXY":                 "",
		"https_proxy":                 "",
		"ALL_PROXY":                   "",
		"all_proxy":                   "",
		"OPENCLAW_CLI":                "",
		"OPENCLAW_HOME":               "",
		"OPENCLAW_STATE_DIR":          "",
		"OPENCLAW_CONFIG_PATH":        "",
		"OPENCLAW_SERVICE_MARKER":     "",
		"OPENCLAW_SERVICE_VERSION":    "",
		"OPENCLAW_GATEWAY_PORT":       "",
		"OPENCLAW_SHELL":              "",
		"HERMES_HOME":                 "",
		"HERMES_QUIET":                "",
		"HERMES_EXEC_ASK":             "",
		"HERMES_GATEWAY_TOKEN":        "",
		"HERMES_SESSION_KEY":          "",
		"LARK_CHANNEL":                "",
	}
}

func vcMeetingManagementNoRetry() clie2e.RetryOptions {
	return clie2e.RetryOptions{
		Attempts: 1,
		ShouldRetry: func(*clie2e.Result) bool {
			return false
		},
	}
}

func requireVCAuthStatusCredentialBinding(t *testing.T, runtime vcMeetingManagementRuntime, credential vcMeetingManagementManifestCredential) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), vcMeetingManagementReadbackTimeout)
	defer cancel()

	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"auth", "status", "--verify", "--json"},
		DefaultAs: "user",
		Env:       vcMeetingManagementUserEnv(runtime.appID, runtime.brand, runtime.isolatedConfigDir, credential.UserAccessToken),
	}, vcMeetingManagementNoRetry())
	if err != nil {
		t.Fatalf("auth status --verify --json failed while binding actor for fixture role")
	}
	result.AssertExitCode(t, 0)

	stdout := strings.TrimSpace(result.Stdout)
	if !gjson.Valid(stdout) {
		t.Fatal("auth status --verify --json returned non-JSON stdout for fixture role binding")
	}
	if gotAppID := gjson.Get(stdout, "appId").String(); gotAppID == "" || gotAppID != runtime.appID {
		t.Fatalf("fixture role app does not match manifest app_id: got %q want %q", gotAppID, runtime.appID)
	}
	if gotBrand := gjson.Get(stdout, "brand").String(); gotBrand == "" || gotBrand != runtime.brand {
		t.Fatalf("fixture role endpoint brand does not match manifest brand: got %q want %q", gotBrand, runtime.brand)
	}
	if verified := gjson.Get(stdout, "identities.user.verified"); !verified.Exists() || !verified.Bool() {
		t.Fatal("auth status --verify --json did not verify the user identity for fixture role binding")
	}
	if openID := gjson.Get(stdout, "identities.user.openId").String(); openID == "" || openID != credential.TokenSubjectOpenID {
		t.Fatalf("fixture role token OpenID does not match manifest token_subject_open_id: got %q want %q", openID, credential.TokenSubjectOpenID)
	}
}

func requireVCMeetingReadback(t *testing.T, runtime vcMeetingManagementRuntime, token, meetingID string) gjson.Result {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), vcMeetingManagementReadbackTimeout)
	defer cancel()

	result, err := clie2e.RunCmdWithRetry(ctx, clie2e.Request{
		Args:      []string{"vc", "meeting", "get"},
		DefaultAs: "user",
		Params: map[string]any{
			"meeting_id":        meetingID,
			"with_participants": true,
		},
		Env: vcMeetingManagementUserEnv(runtime.appID, runtime.brand, runtime.isolatedConfigDir, token),
	}, vcMeetingManagementNoRetry())
	if err != nil {
		t.Fatalf("vc meeting get readback failed for meeting_id=%s", meetingID)
	}
	result.AssertExitCode(t, 0)
	result.AssertStdoutStatus(t, true)

	meeting := gjson.Get(result.Stdout, "data.meeting")
	if !meeting.Exists() || !meeting.IsObject() {
		t.Fatalf("vc meeting get readback for meeting_id=%s did not return data.meeting", meetingID)
	}
	return meeting
}

func assertVCActiveMeetingSnapshot(t *testing.T, meeting gjson.Result, meetingID string, hostUser vcParticipantTuple, expectedParticipants ...vcExpectedParticipantRole) {
	t.Helper()

	require.Equal(t, meetingID, meeting.Get("id").String(), "meeting snapshot:\n%s", meeting.Raw)
	require.EqualValues(t, 2, meeting.Get("status").Int(), "meeting snapshot:\n%s", meeting.Raw)
	assertVCParticipantTuple(t, "meeting.host_user", hostUser, meeting.Get("host_user"))

	participants := meeting.Get("participants").Array()
	require.NotEmpty(t, participants, "meeting snapshot:\n%s", meeting.Raw)
	for _, expected := range expectedParticipants {
		participant := findVCParticipant(t, participants, expected.Tuple)
		require.EqualValues(t, 2, participant.Get("status").Int(), "expected participant must be active in meeting snapshot:\n%s", meeting.Raw)
		require.Equal(t, expected.IsHost, participant.Get("is_host").Bool(), "meeting snapshot:\n%s", meeting.Raw)
		require.Equal(t, expected.IsCohost, participant.Get("is_cohost").Bool(), "meeting snapshot:\n%s", meeting.Raw)
	}
}

func assertVCFailedTargetReadbackState(t *testing.T, meeting gjson.Result, mode string, target vcParticipantTuple) {
	t.Helper()
	participants := meeting.Get("participants").Array()
	switch mode {
	case "missing":
		assertVCParticipantAbsent(t, participants, target)
	case "inactive":
		participant := findVCParticipant(t, participants, target)
		require.True(t, participant.Get("status").Exists(), "inactive failed target must expose status in meeting snapshot:\n%s", meeting.Raw)
		require.NotEqualValues(t, 2, participant.Get("status").Int(), "failed target must not be active in partial fixture:\n%s", meeting.Raw)
	default:
		t.Fatalf("invalid VC fixture failed_target_mode %q", mode)
	}
}

func assertVCParticipantAbsent(t *testing.T, participants []gjson.Result, tuple vcParticipantTuple) {
	t.Helper()
	for _, participant := range participants {
		if participant.Get("id").String() == tuple.ID && participant.Get("user_type").Int() == tuple.UserType {
			t.Fatalf("meeting snapshot unexpectedly contains missing participant tuple %s", tuple.TupleString())
		}
	}
}

func findVCParticipant(t *testing.T, participants []gjson.Result, tuple vcParticipantTuple) gjson.Result {
	t.Helper()

	for _, participant := range participants {
		if participant.Get("id").String() == tuple.ID && participant.Get("user_type").Int() == tuple.UserType {
			return participant
		}
	}
	t.Fatalf("meeting snapshot missing participant tuple %s", tuple.TupleString())
	return gjson.Result{}
}

func assertVCParticipantTuple(t *testing.T, path string, want vcParticipantTuple, got gjson.Result) {
	t.Helper()
	require.True(t, got.Exists(), "%s must exist", path)
	require.Equal(t, want.ID, got.Get("id").String(), "%s.id", path)
	require.EqualValues(t, want.UserType, got.Get("user_type").Int(), "%s.user_type", path)
}

func requireDistinctVCFixtureValues(t *testing.T, kind string, values ...string) {
	t.Helper()
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			t.Fatalf("invalid VC fixture: %s must be pairwise distinct", kind)
		}
		seen[value] = struct{}{}
	}
}

func requirePositiveVCMeetingID(t *testing.T, meetingID string) {
	t.Helper()
	parsed, err := strconv.ParseInt(meetingID, 10, 64)
	if err != nil || parsed <= 0 {
		t.Fatalf("invalid VC fixture meeting ID %q: want positive base-10 int64", meetingID)
	}
}

func validateVCParticipantTuple(t *testing.T, field string, tuple vcParticipantTuple) {
	t.Helper()
	if tuple.ID == "" || strings.TrimSpace(tuple.ID) != tuple.ID {
		t.Fatalf("invalid VC fixture %s.id %q: must be non-empty and have no surrounding whitespace", field, tuple.ID)
	}
	parsedID, err := strconv.ParseInt(tuple.ID, 10, 64)
	if err != nil || parsedID <= 0 {
		t.Fatalf("invalid VC fixture %s.id %q: want positive base-10 int64", field, tuple.ID)
	}
	if tuple.UserType < 1 || tuple.UserType > 7 {
		t.Fatalf("invalid VC fixture %s.user_type %d: want 1..7", field, tuple.UserType)
	}
}

func validateVCActorCredentialBinding(t *testing.T, field string, actor vcParticipantTuple, credential vcMeetingManagementManifestCredential) {
	t.Helper()
	validateVCParticipantTuple(t, field, actor)
	if actor.UserType != vcMeetingManagementLarkUserType {
		t.Fatalf("invalid VC fixture %s.user_type %d: meeting-management actor must be LARK_USER=%d", field, actor.UserType, vcMeetingManagementLarkUserType)
	}
	if actor.ID != credential.TokenSubjectID || actor.UserType != credential.TokenSubjectUserType {
		t.Fatalf("invalid VC fixture %s: actor tuple %s must exactly match credential token subject %s=%d", field, actor.TupleString(), credential.TokenSubjectID, credential.TokenSubjectUserType)
	}
}

func validateVCActorOpenID(t *testing.T, field, openID string) {
	t.Helper()
	if len(openID) > vcMeetingManagementMaxActorOpenIDLength || !vcMeetingManagementActorOpenIDPattern.MatchString(openID) {
		t.Fatalf("invalid VC fixture %s: want an ou_-prefixed OpenID of at most %d ASCII characters", field, vcMeetingManagementMaxActorOpenIDLength)
	}
}

func validateVCMeetingManagementUserToken(t *testing.T, field, token string) {
	t.Helper()
	if len(token) < len("u-")+1 || len(token) > vcMeetingManagementMaxUserTokenLength || !strings.HasPrefix(token, "u-") {
		t.Fatalf("invalid VC fixture manifest %s: want a non-empty u- prefixed bearer token of at most %d ASCII bytes", field, vcMeetingManagementMaxUserTokenLength)
	}
	for _, char := range []byte(token[len("u-"):]) {
		if char < 0x21 || char > 0x7e {
			t.Fatalf("invalid VC fixture manifest %s: bearer token must contain only printable, non-whitespace ASCII bytes", field)
		}
	}
}

func validateVCMeetingManagementCredential(t *testing.T, field string, credential vcMeetingManagementManifestCredential) {
	t.Helper()
	validateVCMeetingManagementUserToken(t, field+".user_access_token", credential.UserAccessToken)
	if !vcMeetingManagementNumericSubjectPattern.MatchString(credential.TokenSubjectID) {
		t.Fatalf("invalid VC fixture manifest %s.token_subject_id %q: want canonical positive base-10 int64", field, credential.TokenSubjectID)
	}
	if parsed, err := strconv.ParseInt(credential.TokenSubjectID, 10, 64); err != nil || parsed <= 0 {
		t.Fatalf("invalid VC fixture manifest %s.token_subject_id %q: want canonical positive base-10 int64", field, credential.TokenSubjectID)
	}
	if credential.TokenSubjectUserType != vcMeetingManagementLarkUserType {
		t.Fatalf("invalid VC fixture manifest %s.token_subject_user_type %d: want LARK_USER=%d", field, credential.TokenSubjectUserType, vcMeetingManagementLarkUserType)
	}
	validateVCActorOpenID(t, field+".token_subject_open_id", credential.TokenSubjectOpenID)
	if credential.TokenSubjectSource != vcMeetingManagementSubjectIDSource {
		t.Fatalf("invalid VC fixture manifest %s.token_subject_source %q: want %q", field, credential.TokenSubjectSource, vcMeetingManagementSubjectIDSource)
	}
}

func validateVCMeetingManagementCleanupToken(t *testing.T, token, suite, runID string) {
	t.Helper()
	prefix := vcMeetingManagementCleanupTokenPrefix(suite, runID)
	if strings.TrimSpace(token) != token || len(token) > vcMeetingManagementMaxCleanupTokenLength || !strings.HasPrefix(token, prefix) {
		t.Fatal("invalid VC fixture manifest: cleanup_token must be bounded and bind suite and run_id")
	}
	suffix := strings.TrimPrefix(token, prefix)
	if len(suffix) < vcMeetingManagementMinCleanupSuffixLength || !vcMeetingManagementCleanupSuffixPattern.MatchString(suffix) {
		t.Fatalf("invalid VC fixture manifest: cleanup_token suffix must contain at least %d safe ASCII characters", vcMeetingManagementMinCleanupSuffixLength)
	}
}

func vcMeetingManagementOpenAPIHost(environment, brand string) (string, bool) {
	if environment != vcMeetingManagementOnlineEnvironment {
		return "", false
	}
	switch brand {
	case "feishu":
		return vcMeetingManagementFeishuOpenAPIHost, true
	case "lark":
		return vcMeetingManagementLarkOpenAPIHost, true
	default:
		return "", false
	}
}

func validateVCFailedTargetMode(t *testing.T, mode string) {
	switch mode {
	case "missing", "inactive":
		return
	default:
		t.Fatalf("invalid VC fixture failed_target_mode %q: want missing|inactive", mode)
	}
}

func assertVCMeetingManagementDenied(t *testing.T, result *clie2e.Result) {
	t.Helper()
	require.NotNil(t, result)
	require.NotZero(t, result.ExitCode, "destructive VC management unexpectedly succeeded\nstdout:\n%s\nstderr:\n%s", result.Stdout, result.Stderr)
	require.Empty(t, result.Stdout)
	require.True(t, gjson.Valid(result.Stderr), "stderr must be a structured error envelope:\n%s", result.Stderr)
	require.EqualValues(t, vcMeetingManagementPermissionDeniedCode, gjson.Get(result.Stderr, "error.code").Int(), "stderr:\n%s", result.Stderr)
	require.NotEmpty(t, gjson.Get(result.Stderr, "error.type").String(), "stderr:\n%s", result.Stderr)
	require.NotEmpty(t, gjson.Get(result.Stderr, "error.message").String(), "stderr:\n%s", result.Stderr)
	require.Equal(t, "user", gjson.Get(result.Stderr, "identity").String(), "stderr:\n%s", result.Stderr)
	require.False(t, gjson.Get(result.Stderr, "error.retryable").Bool(), "stderr:\n%s", result.Stderr)
}

func newVCMeetingManagementRunID(t *testing.T, suite string) string {
	t.Helper()
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("failed to generate VC fixture run_id for suite %s", suite)
	}
	return suite + "-" + hex.EncodeToString(buf)
}

func newVCMeetingManagementCleanupToken(t *testing.T, suite, runID string) string {
	t.Helper()
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("failed to generate VC fixture cleanup token for suite %s run_id=%s", suite, runID)
	}
	return vcMeetingManagementCleanupTokenPrefix(suite, runID) + hex.EncodeToString(buf)
}

func vcMeetingManagementOwnership(suite, runID string) string {
	return fmt.Sprintf("vc-meeting-management/%s/%s", suite, runID)
}

func vcMeetingManagementCleanupTokenPrefix(suite, runID string) string {
	return fmt.Sprintf("vc-meeting-management:%s:%s:", suite, runID)
}

func execCommandExitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func (t vcParticipantTuple) TupleString() string {
	return fmt.Sprintf("%s=%d", t.ID, t.UserType)
}
