// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package tracking

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/internal/vfs/localfileio"
)

const (
	remoteCaller  = "lark-cli"
	remoteAppID   = 1011422
	remoteTimeout = 3 * time.Second
)

type remoteRequestItem struct {
	User   remoteUser    `json:"user"`
	Header remoteHeader  `json:"header"`
	Events []remoteEvent `json:"events"`
	Caller string        `json:"caller"`
}

type remoteUser struct {
	DeviceID     string `json:"device_id"`
	UserID       int64  `json:"user_id"`
	UserUniqueID string `json:"user_unique_id"`
}

type remoteHeader struct {
	AppID        int64          `json:"app_id"`
	AppName      string         `json:"app_name"`
	AppVersion   string         `json:"app_version"`
	AppChannel   string         `json:"app_channel"`
	DeviceModel  string         `json:"device_model"`
	OSName       string         `json:"os_name"`
	ABSDKVersion string         `json:"ab_sdk_version"`
	Custom       map[string]any `json:"custom"`
}

type remoteEvent struct {
	Event       string `json:"event"`
	Params      string `json:"params"`
	Time        int64  `json:"time"`
	LocalTimeMS int64  `json:"local_time_ms"`
}

var RuntimeDirFunc = defaultRuntimeDir

var userUniqueIDProvider = loadOrCreateUserUniqueID

var OfflineAccessChecker func() bool

var brand string

var appID string

func SetTrackingFromConfig(b string, id string) {
	brand = b
	appID = id
}

var userUniqueIDMu sync.Mutex

func loadOrCreateUserUniqueID() (string, error) {
	userUniqueIDMu.Lock()
	defer userUniqueIDMu.Unlock()

	dir := logDir()
	idFile := filepath.Join(dir, "user_uniq_id")

	if data, err := vfs.ReadFile(idFile); err == nil {
		if id := strings.TrimSpace(string(data)); id != "" {
			return id, nil
		}
	}

	newID, err := uuid.NewV7()
	if err != nil {
		return "", fmt.Errorf("generate user unique id: %w", err)
	}
	id := newID.String()

	if err := vfs.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}
	if err := localfileio.AtomicWrite(idFile, []byte(id+"\n"), 0600); err != nil {
		return "", fmt.Errorf("write user unique id: %w", err)
	}
	return id, nil
}

func defaultRuntimeDir() string {
	if dir := os.Getenv("LARKSUITE_CLI_CONFIG_DIR"); dir != "" {
		return dir
	}
	home, err := vfs.UserHomeDir()
	if err != nil || home == "" {
		home = ""
	}
	return filepath.Join(home, ".lark-cli")
}

var (
	remoteEnabled = true
	remoteClient  = &http.Client{Timeout: remoteTimeout}
)

func logDir() string {
	if dir := os.Getenv("LARKSUITE_CLI_LOG_DIR"); dir != "" {
		safeDir, err := validate.SafeEnvDirPath(dir, "LARKSUITE_CLI_LOG_DIR")
		if err == nil {
			return safeDir
		}
	}

	return filepath.Join(RuntimeDirFunc(), "logs")
}

func formatCmdline(args []string) string {
	if len(args) == 0 {
		return ""
	}

	if len(args) <= 3 {
		return strings.Join(args, " ")
	}

	return strings.Join(args[:3], " ") + " ..."
}

func osName() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "macOS"
	case "linux":
		return "Linux"
	default:
		return runtime.GOOS
	}
}

const telemetryEndpointFeishu = "https://mcs-bd.feishu.cn/v1/list"

func resolveTelemetryEndpoint(brand string) string {
	if brand == "feishu" {
		return telemetryEndpointFeishu
	}
	return ""
}
