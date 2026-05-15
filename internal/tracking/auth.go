// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package tracking

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/larksuite/cli/internal/build"
	"github.com/larksuite/cli/internal/vfs"
)

type authEventKind string

const (
	authEventResponse authEventKind = "auth_response"
	authEventError    authEventKind = "auth_error"
)

type AuthComponent string

const (
	AuthComponentAuth     AuthComponent = "auth"
	AuthComponentKeychain AuthComponent = "keychain"
)

type AuthOp string

const (
	AuthOpSecurityPolicy    AuthOp = "security_policy"
	AuthOpPermissionDenied  AuthOp = "permission_denied"
	AuthOpAuthError         AuthOp = "auth_error"
	AuthOpNeedAuthorization AuthOp = "need_authorization"
	AuthOpTokenUnavailable  AuthOp = "token_unavailable"
	AuthOpKeychainGet       AuthOp = "Get"
	AuthOpKeychainSet       AuthOp = "Set"
	AuthOpKeychainRemove    AuthOp = "Remove"
)

type authEvent struct {
	Kind             authEventKind
	Time             time.Time
	Cmdline          string
	Path             string
	Status           int
	LogID            string
	Component        AuthComponent
	Op               AuthOp
	Error            string
	HasOfflineAccess bool
}

var (
	authResponseLogger     *log.Logger
	authResponseLoggerOnce = &sync.Once{}

	authResponseLogNow  = time.Now
	authResponseLogArgs = func() []string { return os.Args }
)

func initAuthLogger() {
	authResponseLoggerOnce.Do(func() {
		if authResponseLogger != nil {
			return
		}

		dir := logDir()
		now := authResponseLogNow()
		if err := vfs.MkdirAll(dir, 0700); err != nil {
			return
		}

		logName := fmt.Sprintf("auth-%s.log", now.Format("2006-01-02"))
		logPath := filepath.Join(dir, logName)
		if f, err := vfs.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600); err == nil {
			authResponseLogger = log.New(f, "", 0)
			cleanupOldAuthLogs(dir, now)
		}
	})
}
func LogAuthResponse(path string, status int, logID string) {
	emitAuthEvent(authEvent{
		Kind:             authEventResponse,
		Time:             authResponseLogNow(),
		Cmdline:          formatCmdline(authResponseLogArgs()),
		Path:             path,
		Status:           status,
		LogID:            logID,
		HasOfflineAccess: checkOfflineAccess(),
	})
}

func LogAuthError(component AuthComponent, op AuthOp, err error) {
	if err == nil {
		return
	}
	emitAuthEvent(authEvent{
		Kind:             authEventError,
		Time:             authResponseLogNow(),
		Cmdline:          formatCmdline(authResponseLogArgs()),
		Component:        component,
		Op:               op,
		Error:            err.Error(),
		HasOfflineAccess: checkOfflineAccessForError(component),
	})
}

func checkOfflineAccess() bool {
	if OfflineAccessChecker == nil {
		return false
	}
	defer func() { recover() }()
	return OfflineAccessChecker()
}

func checkOfflineAccessForError(component AuthComponent) bool {
	if component == AuthComponentKeychain {
		return false
	}
	return checkOfflineAccess()
}

func emitAuthEvent(event authEvent) {
	emitLocalAuthEvent(event)
	emitRemoteAuthEvent(event)
}

func emitLocalAuthEvent(event authEvent) {
	initAuthLogger()
	if authResponseLogger == nil {
		return
	}

	switch event.Kind {
	case authEventResponse:
		authResponseLogger.Printf(
			"[lark-cli] auth-response: time=%s path=%s status=%d x-tt-logid=%s cmdline=%s",
			event.Time.Format(time.RFC3339Nano),
			event.Path,
			event.Status,
			event.LogID,
			event.Cmdline,
		)
	case authEventError:
		authResponseLogger.Printf(
			"[lark-cli] auth-error: time=%s component=%s op=%s error=%q cmdline=%s",
			event.Time.Format(time.RFC3339Nano),
			event.Component,
			event.Op,
			event.Error,
			event.Cmdline,
		)
	}
}

func emitRemoteAuthEvent(event authEvent) {
	endpoint := resolveTelemetryEndpoint(brand)
	if !remoteEnabled || endpoint == "" {
		return
	}

	defer func() {
		_ = recover()
	}()

	_ = postRemoteAuthEvent(event, endpoint)
}

func postRemoteAuthEvent(event authEvent, endpoint string) error {
	userUniqueID, err := userUniqueIDProvider()
	if err != nil || strings.TrimSpace(userUniqueID) == "" {
		fallbackID, fallbackErr := uuid.NewV7()
		if fallbackErr != nil {
			return fallbackErr
		}
		userUniqueID = fallbackID.String()
	}

	payload, err := buildRemoteAuthPayload(event, userUniqueID)
	if err != nil {
		return err
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), remoteTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := remoteClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("auth log remote sink returned status %d", resp.StatusCode)
	}
	return nil
}

func buildRemoteAuthPayload(event authEvent, userUniqueID string) ([]remoteRequestItem, error) {
	ts := event.Time.Unix()
	params, err := buildRemoteAuthParams(event, ts)
	if err != nil {
		return nil, err
	}

	item := remoteRequestItem{
		User: remoteUser{
			DeviceID:     "",
			UserID:       0,
			UserUniqueID: userUniqueID,
		},
		Header: remoteHeader{
			AppID:        remoteAppID,
			AppName:      "",
			AppVersion:   build.Version,
			AppChannel:   "",
			DeviceModel:  "",
			OSName:       osName(),
			ABSDKVersion: "",
			Custom:       map[string]any{},
		},
		Events: []remoteEvent{{
			Event:       authRemoteEventName(event),
			Params:      params,
			Time:        ts,
			LocalTimeMS: event.Time.UnixMilli(),
		}},
		Caller: remoteCaller,
	}
	return []remoteRequestItem{item}, nil
}

func buildRemoteAuthParams(event authEvent, ts int64) (string, error) {
	data := map[string]any{
		"cmdline":            event.Cmdline,
		"has_offline_access": event.HasOfflineAccess,
		"lark_cli_version":   build.Version,
		"op_client_id":       appID,
		"timestamp_s":        ts,
	}

	switch event.Kind {
	case authEventResponse:
		data["path"] = event.Path
		data["status"] = event.Status
		data["x_tt_logid"] = event.LogID
	case authEventError:
		data["component"] = event.Component
		data["op"] = event.Op
		data["error"] = event.Error
	}

	raw, err := json.Marshal(data)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

func authRemoteEventName(event authEvent) string {
	if event.Kind == authEventError {
		return "cli_auth_error"
	}
	return "cli_auth_response"
}

func SetAuthLogHooksForTest(logger *log.Logger, now func() time.Time, args func() []string) func() {
	prevLogger := authResponseLogger
	prevNow := authResponseLogNow
	prevArgs := authResponseLogArgs
	prevOnce := authResponseLoggerOnce
	prevRemoteEnabled := remoteEnabled

	authResponseLogger = logger
	authResponseLoggerOnce = &sync.Once{}
	remoteEnabled = false

	if now != nil {
		authResponseLogNow = now
	}
	if args != nil {
		authResponseLogArgs = args
	}

	return func() {
		authResponseLogger = prevLogger
		authResponseLogNow = prevNow
		authResponseLogArgs = prevArgs
		authResponseLoggerOnce = prevOnce
		remoteEnabled = prevRemoteEnabled
	}
}

func SetAuthLogRemoteHooksForTest(client *http.Client, b string, provider func() (string, error), enabled bool) func() {
	prevClient := remoteClient
	prevBrand := brand
	prevProvider := userUniqueIDProvider
	prevEnabled := remoteEnabled

	if client != nil {
		remoteClient = client
	}
	brand = b
	if provider != nil {
		userUniqueIDProvider = provider
	}
	remoteEnabled = enabled

	return func() {
		remoteClient = prevClient
		brand = prevBrand
		userUniqueIDProvider = prevProvider
		remoteEnabled = prevEnabled
	}
}

func cleanupOldAuthLogs(dir string, now time.Time) {
	defer func() { recover() }()

	entries, err := vfs.ReadDir(dir)
	if err != nil {
		return
	}

	now = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	cutoff := now.AddDate(0, 0, -7)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "auth-") || !strings.HasSuffix(entry.Name(), ".log") {
			continue
		}

		dateStr := strings.TrimPrefix(entry.Name(), "auth-")
		dateStr = strings.TrimSuffix(dateStr, ".log")

		logDate, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			continue
		}

		logDate = time.Date(logDate.Year(), logDate.Month(), logDate.Day(), 0, 0, 0, 0, now.Location())
		if logDate.Before(cutoff) {
			_ = vfs.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}
