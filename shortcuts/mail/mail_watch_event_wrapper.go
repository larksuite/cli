// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/client"
	"github.com/larksuite/cli/internal/core"
	eventlib "github.com/larksuite/cli/internal/event"
	"github.com/larksuite/cli/internal/event/consume"
	"github.com/larksuite/cli/internal/event/transport"
	"github.com/larksuite/cli/internal/validate"
	"github.com/larksuite/cli/internal/vfs"
	"github.com/larksuite/cli/shortcuts/common"
)

func runMailWatchViaEventConsume(ctx context.Context, runtime *common.RuntimeContext) error {
	ensureMailEventKeysRegistered()

	cfg := runtime.Config
	apiClient, err := runtime.Factory.NewAPIClientWithConfig(cfg)
	if err != nil {
		return err
	}

	outputDir := runtime.Str("output-dir")
	if outputDir != "" {
		if strings.HasPrefix(outputDir, "~") {
			return mailValidationParamError("--output-dir", "--output-dir does not support ~ expansion; use a relative path like ./output instead")
		}
		safePath, err := validate.SafeOutputPath(outputDir)
		if err != nil {
			return mailValidationParamError("--output-dir", "invalid --output-dir %q: %v", outputDir, err).WithCause(err)
		}
		outputDir = safePath
		if err := vfs.MkdirAll(outputDir, 0700); err != nil {
			return mailFileIOError("cannot create output directory %q: %v", err, outputDir, err)
		}
	}

	var timeout time.Duration
	if raw := runtime.Str("timeout"); raw != "" {
		timeout, err = time.ParseDuration(raw)
		if err != nil {
			return mailValidationParamError("--timeout", "invalid --timeout %q: %v", raw, err).WithCause(err)
		}
	}

	params := map[string]string{}
	addParam := func(name, value string) {
		if value != "" {
			params[name] = value
		}
	}
	addParam("mailbox", resolveMailboxID(runtime))
	addParam("labels", runtime.Str("labels"))
	addParam("folders", runtime.Str("folders"))
	addParam("label_ids", runtime.Str("label-ids"))
	addParam("folder_ids", runtime.Str("folder-ids"))
	addParam("msg_format", runtime.Str("msg-format"))
	addParam("legacy_format", runtime.Str("format"))
	addParam("legacy_identity", string(runtime.As()))

	domain := core.ResolveEndpoints(cfg.Brand).Open
	return consume.Run(ctx, transport.New(), cfg.AppID, cfg.ProfileName, domain, consume.Options{
		EventKey:        MailMessageReceivedEventKey,
		Params:          params,
		JQExpr:          runtime.JqExpr,
		OutputDir:       outputDir,
		Runtime:         &mailWatchConsumeRuntime{client: apiClient, accessIdentity: runtime.As()},
		RemoteAPIClient: &mailWatchConsumeRuntime{client: apiClient, accessIdentity: core.AsBot},
		Out:             runtime.IO().Out,
		ErrOut:          runtime.IO().ErrOut,
		MaxEvents:       runtime.Int("max-events"),
		Timeout:         timeout,
		IsTTY:           runtime.IO().IsTerminal,
	})
}

func mailWatchEventConsumeArgs(runtime *common.RuntimeContext) []string {
	args := []string{MailMessageReceivedEventKey, "--as", string(runtime.As())}
	addParam := func(name, value string) {
		if value == "" {
			return
		}
		args = append(args, "--param", name+"="+value)
	}

	addParam("mailbox", resolveMailboxID(runtime))
	addParam("labels", runtime.Str("labels"))
	addParam("folders", runtime.Str("folders"))
	addParam("label_ids", runtime.Str("label-ids"))
	addParam("folder_ids", runtime.Str("folder-ids"))
	addParam("msg_format", runtime.Str("msg-format"))
	addParam("legacy_format", runtime.Str("format"))
	addParam("legacy_identity", string(runtime.As()))

	if jq := runtime.JqExpr; jq != "" {
		args = append(args, "--jq", jq)
	}
	if outputDir := runtime.Str("output-dir"); outputDir != "" {
		args = append(args, "--output-dir", outputDir)
	}
	if maxEvents := runtime.Int("max-events"); maxEvents > 0 {
		args = append(args, "--max-events", fmt.Sprintf("%d", maxEvents))
	}
	if timeout := runtime.Str("timeout"); timeout != "" {
		args = append(args, "--timeout", timeout)
	}
	return args
}

func ensureMailEventKeysRegistered() {
	if _, ok := eventlib.Lookup(MailMessageReceivedEventKey); ok {
		return
	}
	for _, key := range EventKeys() {
		eventlib.RegisterKey(key)
	}
}

type mailWatchConsumeRuntime struct {
	client         *client.APIClient
	accessIdentity core.Identity
}

func (r *mailWatchConsumeRuntime) CallAPI(ctx context.Context, method, path string, body interface{}) (json.RawMessage, error) {
	resp, err := r.client.DoAPI(ctx, client.RawApiRequest{
		Method: method,
		URL:    path,
		Data:   body,
		As:     r.accessIdentity,
	})
	if err != nil {
		if _, ok := errs.ProblemOf(err); ok {
			return nil, err
		}
		return nil, errs.NewNetworkError(errs.SubtypeNetworkTransport,
			"api %s %s: %s", method, path, err).WithCause(err)
	}
	ct := resp.Header.Get("Content-Type")
	if resp.StatusCode >= 400 && !client.IsJSONContentType(ct) && ct != "" {
		body := string(resp.RawBody)
		const maxBodyEcho = 256
		if len(body) > maxBodyEcho {
			body = body[:maxBodyEcho] + "...(truncated)"
		}
		if resp.StatusCode >= 500 {
			return nil, errs.NewNetworkError(errs.SubtypeNetworkServer,
				"api %s %s returned %d: %s", method, path, resp.StatusCode, body).WithRetryable()
		}
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"api %s %s returned %d: %s", method, path, resp.StatusCode, body)
	}
	result, err := client.ParseJSONResponse(resp)
	if err != nil {
		if _, ok := errs.ProblemOf(err); ok {
			return nil, err
		}
		return nil, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"api %s %s: %s", method, path, err).WithCause(err)
	}
	if apiErr := r.client.CheckResponse(result, r.accessIdentity); apiErr != nil {
		return json.RawMessage(resp.RawBody), apiErr
	}
	return json.RawMessage(resp.RawBody), nil
}
