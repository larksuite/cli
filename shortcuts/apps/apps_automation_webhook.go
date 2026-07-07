// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package apps

import (
	"fmt"
	"io"
	"strings"

	"github.com/larksuite/cli/shortcuts/common"
)

const webhookTokenType = "bearerToken"

// runWebhookURLReset handles --reset-url --app-env <preview|runtime>. Rotates the
// hookKey for the given env; old URL invalidated immediately. New URL shown once.
func runWebhookURLReset(rctx *common.RuntimeContext) error {
	appID, err := requireAppID(rctx.Str("app-id"))
	if err != nil {
		return err
	}
	name := strings.TrimSpace(rctx.Str("name"))
	appEnv := strings.TrimSpace(rctx.Str("app-env"))
	if appEnv == "" {
		return appsValidationParamError("--app-env", "--reset-url requires --app-env preview|runtime")
	}
	if appEnv != "preview" && appEnv != "runtime" {
		return appsValidationParamError("--app-env", "--app-env must be preview or runtime, got %q", appEnv)
	}
	body := map[string]interface{}{"app_env": appEnv}
	data, err := rctx.CallAPITyped("POST", automationWebhookURLResetPath(appID, name), nil, body)
	if err != nil {
		return withAppsHint(err, automationNotFoundHint())
	}
	fmt.Fprintln(rctx.IO().ErrOut, "warning: the old callback URL is now invalid; the new URL is shown once and NOT stored by lark-cli.")
	rctx.OutFormat(data, nil, func(w io.Writer) {
		fmt.Fprintf(w, "new %s URL: %v  (shown once)\n", appEnv, firstNonEmpty(
			common.GetString(data, appEnv+"_url"), common.GetString(data, "url")))
	})
	return nil
}

// runWebhookTokenStatus handles --enable-token / --disable-token. Both map to the
// same token/status endpoint. enable surfaces the plaintext token once.
func runWebhookTokenStatus(rctx *common.RuntimeContext, enable bool) error {
	appID, err := requireAppID(rctx.Str("app-id"))
	if err != nil {
		return err
	}
	name := strings.TrimSpace(rctx.Str("name"))
	status := "disabled"
	if enable {
		status = "enabled"
	}
	body := map[string]interface{}{"status": status, "token_type": webhookTokenType}
	data, err := rctx.CallAPITyped("POST", automationWebhookTokenStatusPath(appID, name), nil, body)
	if err != nil {
		return withAppsHint(err, automationNotFoundHint())
	}
	if enable {
		return outputIssuedWebhookToken(rctx, data)
	}
	rctx.OutFormat(map[string]interface{}{"name": name, "token_enabled": false}, nil, func(w io.Writer) {
		fmt.Fprintf(w, "trigger %s: bearer token disabled (irreversible; callbacks no longer require a token)\n", name)
	})
	return nil
}

// runWebhookTokenReset handles --reset-token. Rotates the token; old token invalidated.
func runWebhookTokenReset(rctx *common.RuntimeContext) error {
	appID, err := requireAppID(rctx.Str("app-id"))
	if err != nil {
		return err
	}
	name := strings.TrimSpace(rctx.Str("name"))
	body := map[string]interface{}{"token_type": webhookTokenType}
	data, err := rctx.CallAPITyped("POST", automationWebhookTokenResetPath(appID, name), nil, body)
	if err != nil {
		return withAppsHint(err, automationNotFoundHint())
	}
	return outputIssuedWebhookToken(rctx, data)
}

// outputIssuedWebhookToken emits the plaintext bearer token ONCE with a one-time
// stderr warning; never persisted (mirrors outputIssuedKey in apps_openapi_key_create.go).
func outputIssuedWebhookToken(rctx *common.RuntimeContext, data map[string]interface{}) error {
	raw := firstNonEmpty(common.GetString(data, "token_value"), common.GetString(data, "token"))
	fmt.Fprintln(rctx.IO().ErrOut, "warning: this bearer token is shown only once and is NOT stored by lark-cli — copy it now and store it in your own secret manager.")
	out := map[string]interface{}{"token_value": raw, "token_enabled": true}
	rctx.OutFormat(out, nil, func(w io.Writer) {
		fmt.Fprintf(w, "bearer token: %v  (shown once)\n", raw)
	})
	return nil
}
