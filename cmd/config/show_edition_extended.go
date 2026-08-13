// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build extended

package config

import (
	"context"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/output"
)

type editionConfigShowResult struct {
	Source                 string  `json:"source"`
	CredentialProvider     string  `json:"credentialProvider"`
	Manageable             bool    `json:"manageable"`
	Workspace              string  `json:"workspace"`
	AppID                  string  `json:"appId"`
	Brand                  string  `json:"brand"`
	DefaultAs              string  `json:"defaultAs"`
	Profile                *string `json:"profile,omitempty"`
	ExternalCredentialMode *string `json:"externalCredentialMode,omitempty"`
	RemoteEndpoint         *string `json:"remoteEndpoint,omitempty"`
}

func showEditionConfig(f *cmdutil.Factory) (bool, error) {
	if f == nil || f.Credential == nil {
		return false, nil
	}
	source, err := f.Credential.InspectSource(context.Background())
	if err != nil {
		return true, typedEditionProviderError("determine the active credential provider", err)
	}
	if source == nil || !source.Managed {
		return false, nil
	}
	if source.AppID == "" {
		return true, errs.NewInternalError(errs.SubtypeInvalidResponse,
			"external credential provider %q returned no account", source.Name)
	}
	result := editionConfigShowResult{
		Source:             "external",
		CredentialProvider: source.Name,
		Manageable:         false,
		Workspace:          core.CurrentWorkspace().Display(),
		AppID:              source.AppID,
		Brand:              string(source.Brand),
		DefaultAs:          string(source.DefaultAs),
	}
	description := f.RuntimeDescription()
	if source.ProfileName != "" {
		result.Profile = &source.ProfileName
	}
	if description.Managed && description.Variant != "" {
		result.ExternalCredentialMode = &description.Variant
		if description.ProxiesRequests {
			result.RemoteEndpoint = &description.DataPlaneEndpoint
		}
	}
	output.PrintJson(f.IOStreams.Out, result)
	return true, nil
}

func typedEditionProviderError(action string, err error) error {
	if _, ok := errs.ProblemOf(err); ok {
		return err
	}
	return errs.NewInternalError(errs.SubtypeUnknown, "failed to %s: %v", action, err).WithCause(err)
}
