// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package sec

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

// NewCmdSecConfig is the parent for `lark-cli sec config <verb>`. Currently
// it only carries `init`; future verbs (e.g. `show`, `reset`) plug in here.
func NewCmdSecConfig(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage lark-sec-cli daemon configuration",
	}
	cmd.AddCommand(NewCmdSecConfigInit(f, nil))
	return cmd
}

// ConfigInitOptions holds inputs for `lark-cli sec config init`.
type ConfigInitOptions struct {
	Factory   *cmdutil.Factory
	AppID     string
	AppSecret string
	Brand     string
	Yes       bool // skip the interactive form when all required values are provided
}

// NewCmdSecConfigInit collects App ID / App Secret / Brand from the user and
// registers them with the running lark-sec-cli daemon's admin endpoint. The
// daemon stashes the secret in the OS keychain and switches into sidecar mode
// for SEC_AUTH credential isolation.
func NewCmdSecConfigInit(f *cmdutil.Factory, runF func(*ConfigInitOptions) error) *cobra.Command {
	opts := &ConfigInitOptions{Factory: f}
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Register a Lark App with the running lark-sec-cli daemon",
		Long: `Register an App ID / App Secret with the lark-sec-cli daemon.

The daemon must already be running (start it with "lark-cli sec run"). The
registration POSTs to /_sec/api/v1/register-app on the local proxy port,
HMAC-signed with the daemon's proxy.key.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if runF != nil {
				return runF(opts)
			}
			return runConfigInit(cmd, opts)
		},
	}
	cmd.Flags().StringVar(&opts.AppID, "app-id", "", "App ID (skips the prompt when set)")
	cmd.Flags().StringVar(&opts.AppSecret, "app-secret", "", "App Secret (skips the prompt when set)")
	cmd.Flags().StringVar(&opts.Brand, "brand", "feishu", "feishu or lark")
	cmd.Flags().BoolVarP(&opts.Yes, "yes", "y", false, "skip the interactive form when all required values are provided")
	return cmd
}

// secBridge mirrors what the daemon writes to ~/.lark-cli/sec_config.json.
// It's the single contract between lark-cli and lark-sec-cli at runtime —
// we don't reach into lark-sec-cli internals, only what it chooses to publish.
type secBridge struct {
	Enable bool   `json:"LARKSUITE_CLI_SEC_ENABLE"`
	Proxy  string `json:"LARKSUITE_CLI_SEC_PROXY"`
	CA     string `json:"LARKSUITE_CLI_SEC_CA"`
	Auth   bool   `json:"LARKSUITE_CLI_SEC_AUTH"`
}

func runConfigInit(cmd *cobra.Command, opts *ConfigInitOptions) error {
	errOut := opts.Factory.IOStreams.ErrOut
	trace := verboseOut(cmd, errOut)

	tracef(trace, "sec config init", "loading daemon bridge from %s/sec_config.json", core.GetConfigDir())
	bridge, err := loadBridge()
	if err != nil {
		return output.ErrWithHint(output.ExitValidation, "sec_bridge_missing",
			fmt.Sprintf("daemon bridge file unreadable: %v", err),
			"Start the daemon first: `lark-cli sec run`.")
	}
	tracef(trace, "sec config init", "bridge: enable=%t proxy=%s ca=%s auth=%t", bridge.Enable, bridge.Proxy, bridge.CA, bridge.Auth)
	if !bridge.Enable || bridge.Proxy == "" {
		return output.ErrWithHint(output.ExitValidation, "sec_not_running",
			"lark-sec-cli is not advertising an active proxy",
			"Run `lark-cli sec run` to start it.")
	}

	// The HMAC key sits next to the CA in the daemon's config dir. Deriving
	// from the bridge's SEC_CA path keeps lark-cli decoupled from the daemon's
	// install location — if the daemon ever moves, the bridge follows and we
	// follow with it.
	tracef(trace, "sec config init", "reading daemon HMAC key beside %s", bridge.CA)
	hmacKey, err := readHMACKey(bridge.CA)
	if err != nil {
		return output.Errorf(output.ExitInternal, "sec_hmac_key", "read daemon HMAC key: %v", err)
	}

	if err := promptForMissing(opts); err != nil {
		return err
	}

	tracef(trace, "sec config init", "POST %s/_sec/api/v1/register-app app_id=%s brand=%s", bridge.Proxy, opts.AppID, opts.Brand)
	if err := registerApp(cmd.Context(), bridge.Proxy, hmacKey, opts.AppID, opts.AppSecret, opts.Brand); err != nil {
		return output.Errorf(output.ExitAPI, "sec_register_app", "register-app: %v", err)
	}

	output.PrintSuccess(errOut,
		fmt.Sprintf("registered app %s with lark-sec-cli (%s)", opts.AppID, opts.Brand))
	return nil
}

// loadBridge reads the daemon-written sec_config.json from lark-cli's config dir.
func loadBridge() (*secBridge, error) {
	path := filepath.Join(core.GetConfigDir(), "sec_config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var b secBridge
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	return &b, nil
}

// readHMACKey returns the daemon's proxy.key bytes. The daemon writes the key
// hex-encoded (64 ASCII chars); we hex-decode here. If the file is a raw
// 32-byte blob (older daemon variants), we use it as-is.
func readHMACKey(caPath string) ([]byte, error) {
	if caPath == "" {
		return nil, errors.New("sec_config.json has no LARKSUITE_CLI_SEC_CA — can't locate proxy.key")
	}
	keyPath := filepath.Join(filepath.Dir(caPath), "proxy.key")
	raw, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 64 {
		if decoded, err := hex.DecodeString(string(raw)); err == nil {
			return decoded, nil
		}
	}
	return raw, nil
}

// promptForMissing fills in any of AppID / AppSecret / Brand the user didn't
// provide via flags. --yes refuses to prompt; that's caller error if any are
// still missing at that point.
func promptForMissing(opts *ConfigInitOptions) error {
	if opts.AppID != "" && opts.AppSecret != "" && opts.Brand != "" {
		return nil
	}
	if opts.Yes {
		return output.ErrValidation("--yes set but missing one of --app-id / --app-secret / --brand")
	}

	groups := []*huh.Group{}
	if opts.AppID == "" {
		groups = append(groups, huh.NewGroup(
			huh.NewInput().Title("App ID").Placeholder("cli_xxxx").Value(&opts.AppID),
		))
	}
	if opts.AppSecret == "" {
		groups = append(groups, huh.NewGroup(
			huh.NewInput().Title("App Secret").EchoMode(huh.EchoModePassword).Value(&opts.AppSecret),
		))
	}
	if opts.Brand == "" {
		opts.Brand = "feishu"
		groups = append(groups, huh.NewGroup(
			huh.NewSelect[string]().Title("Brand").Options(
				huh.NewOption("Feishu (cn)", "feishu"),
				huh.NewOption("Lark (intl)", "lark"),
			).Value(&opts.Brand),
		))
	}
	if len(groups) == 0 {
		return nil
	}
	form := huh.NewForm(groups...).WithTheme(cmdutil.ThemeFeishu())
	if err := form.Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return output.ErrBare(1)
		}
		return err
	}
	return nil
}

// registerApp POSTs to /_sec/api/v1/register-app with the daemon's HMAC scheme.
// Canonical signing input is "method\npath\nsha256hex(body)\ntimestamp", per
// lark-sec-cli/internal/proxy/admin_handler.go's verifyHMAC.
func registerApp(ctx context.Context, proxyURL string, hmacKey []byte, appID, appSecret, brand string) error {
	const path = "/_sec/api/v1/register-app"

	body, err := json.Marshal(map[string]string{
		"app_id":     appID,
		"app_secret": appSecret,
		"brand":      brand,
	})
	if err != nil {
		return err
	}

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	bodyHash := sha256.Sum256(body)
	canonical := http.MethodPost + "\n" + path + "\n" + hex.EncodeToString(bodyHash[:]) + "\n" + ts
	mac := hmac.New(sha256.New, hmacKey)
	mac.Write([]byte(canonical))
	sig := hex.EncodeToString(mac.Sum(nil))

	reqCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, proxyURL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Lark-Admin-Signature", sig)
	req.Header.Set("X-Lark-Admin-Timestamp", ts)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	return fmt.Errorf("status %d: %s", resp.StatusCode, string(respBody))
}
