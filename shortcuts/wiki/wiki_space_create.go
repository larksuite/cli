// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package wiki

import (
	"context"
	"fmt"
	"strings"

	"github.com/larksuite/cli/internal/output"
	"github.com/larksuite/cli/shortcuts/common"
)

// WikiSpaceCreate wraps wiki.spaces.create. The raw API only takes two
// optional string fields, so the shortcut's value is flag ergonomics
// (no hand-written --params JSON), output flattening (data.space.* lifted
// to the top level), and a dry-run preview.
//
// The API only accepts a user access token (no tenant/bot), so AuthTypes is
// user-only — the framework's CheckIdentity rejects --as bot for us.
var WikiSpaceCreate = common.Shortcut{
	Service:     "wiki",
	Command:     "+space-create",
	Description: "Create a wiki space",
	Risk:        "write",
	// The API accepts wiki:wiki or wiki:space:write_only. The framework's
	// scope preflight does exact-string matching (see +space-list), so
	// declare the narrowest form the API takes to avoid false-rejecting
	// tokens that only carry wiki:space:write_only.
	Scopes:    []string{"wiki:space:write_only"},
	AuthTypes: []string{"user"},
	Flags: []common.Flag{
		{Name: "name", Desc: "wiki space name", Required: true},
		{Name: "description", Desc: "wiki space description"},
	},
	Tips: []string{
		"Only --as user is supported; the create API does not accept a tenant/bot token.",
		"The underlying spaces.create API is flagged danger in the schema browser; a space is recoverable via `wiki +delete-space` if created by mistake.",
		"--name is required: an unnamed space is almost always an accident and is hard to find later.",
	},
	Validate: func(ctx context.Context, runtime *common.RuntimeContext) error {
		_, err := readWikiSpaceCreateSpec(runtime)
		return err
	},
	DryRun: func(ctx context.Context, runtime *common.RuntimeContext) *common.DryRunAPI {
		spec, err := readWikiSpaceCreateSpec(runtime)
		if err != nil {
			return common.NewDryRunAPI().Set("error", err.Error())
		}
		return common.NewDryRunAPI().
			POST(wikiSpacesAPIPath).
			Body(spec.RequestBody())
	},
	Execute: func(ctx context.Context, runtime *common.RuntimeContext) error {
		spec, err := readWikiSpaceCreateSpec(runtime)
		if err != nil {
			return err
		}

		fmt.Fprintf(runtime.IO().ErrOut, "Creating wiki space %q...\n", spec.Name)

		data, err := runtime.CallAPI("POST", wikiSpacesAPIPath, nil, spec.RequestBody())
		if err != nil {
			return err
		}

		raw := common.GetMap(data, "space")
		if raw == nil {
			return output.Errorf(output.ExitAPI, "api_error", "wiki space create returned no space")
		}

		out := wikiSpaceCreateOutput(raw)
		fmt.Fprintf(runtime.IO().ErrOut, "Created wiki space %s\n", common.MaskToken(common.GetString(out, "space_id")))
		runtime.Out(out, nil)
		return nil
	},
}

// wikiSpaceCreateSpec is the normalized CLI input.
type wikiSpaceCreateSpec struct {
	Name        string
	Description string
}

// RequestBody converts the normalized input into the OpenAPI payload. Both
// fields are optional per the API, but Validate enforces a non-empty name,
// so name is always present here.
func (spec wikiSpaceCreateSpec) RequestBody() map[string]interface{} {
	body := map[string]interface{}{"name": spec.Name}
	if spec.Description != "" {
		body["description"] = spec.Description
	}
	return body
}

func readWikiSpaceCreateSpec(runtime *common.RuntimeContext) (wikiSpaceCreateSpec, error) {
	spec := wikiSpaceCreateSpec{
		Name:        strings.TrimSpace(runtime.Str("name")),
		Description: strings.TrimSpace(runtime.Str("description")),
	}
	if spec.Name == "" {
		return wikiSpaceCreateSpec{}, output.ErrValidation("--name is required and cannot be blank")
	}
	return spec, nil
}

// wikiSpaceCreateOutput flattens data.space into the top-level envelope. It
// reads the raw map (rather than parseWikiSpaceRecord) so the description
// the caller just set round-trips back in the output.
func wikiSpaceCreateOutput(raw map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"space_id":     common.GetString(raw, "space_id"),
		"name":         common.GetString(raw, "name"),
		"description":  common.GetString(raw, "description"),
		"space_type":   common.GetString(raw, "space_type"),
		"visibility":   common.GetString(raw, "visibility"),
		"open_sharing": common.GetString(raw, "open_sharing"),
	}
}
