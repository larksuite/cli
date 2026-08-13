// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package externalcredential

import (
	"encoding/json"
	"errors"
	"os"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/runtimeplan"
	"github.com/larksuite/cli/internal/vfs"
)

// ProfileSelection is the immutable configuration snapshot and invocation plan
// used to wire one Factory.
type ProfileSelection struct {
	Config              *core.MultiAppConfig
	Plan                *runtimeplan.Plan
	SystemConfigPresent bool
	DisableRemoteMeta   bool
}

// SelectProfile resolves the active configuration once for a Factory. A
// syntactically damaged legacy config is left to environment providers so
// existing environment-only integrations keep their historical behavior.
// Reserved externalCredential fields in a Profile always fail closed.
func SelectProfile(profileOverride string) (*ProfileSelection, error) {
	systemConfig, systemConfigPresent, err := loadSystemConfig()
	selection := &ProfileSelection{
		Plan:                runtimeplan.Default(),
		SystemConfigPresent: systemConfigPresent,
		// Until a present configuration has been parsed and validated, do not
		// allow startup to make an unproxied metadata request.
		DisableRemoteMeta: systemConfigPresent,
	}
	if err != nil {
		return selection, err
	}
	data, readErr := vfs.ReadFile(core.GetConfigPath())
	if readErr != nil {
		if errors.Is(readErr, os.ErrNotExist) {
			if systemConfig != nil {
				return selection, errs.NewConfigError(errs.SubtypeNotConfigured,
					"system external credential mode requires a Profile").
					WithHint("create a Profile containing only appId, brand, and defaultAs")
			}
			return selection, nil
		}
		if systemConfig == nil {
			// Preserve the established provider chain when no administrator
			// runtime is active. In particular, an environment provider may be
			// the complete credential source and must not be disabled merely
			// because an unrelated local Profile cannot be read.
			return selection, nil
		}
		return selection, errs.NewConfigError(errs.SubtypeInvalidConfig,
			"cannot read profile configuration while selecting external credentials: %v", readErr).WithCause(readErr)
	}

	reservedFieldErr := rejectReservedProfileFields(data)
	if reservedFieldErr != nil && errors.Is(reservedFieldErr, errUnknownConfigField) {
		return selection, errs.NewConfigError(errs.SubtypeInvalidConfig,
			"Profile contains a reserved external credential configuration field: %v", reservedFieldErr).
			WithCause(reservedFieldErr)
	}

	var multi core.MultiAppConfig
	if err := json.Unmarshal(data, &multi); err != nil {
		if systemConfig != nil {
			return selection, errs.NewConfigError(errs.SubtypeInvalidConfig, "cannot load Profile while system external credentials are enabled: %v", err).WithCause(err)
		}
		return selection, nil
	}
	selection.Config = &multi
	app := multi.CurrentAppConfig(profileOverride)
	if app == nil {
		if systemConfig != nil {
			return selection, errs.NewConfigError(errs.SubtypeInvalidConfig,
				"cannot select a Profile for the system external credential configuration").
				WithHint("check currentApp or the --profile value")
		}
		return selection, nil
	}
	if systemConfig == nil {
		return selection, nil
	}
	if err := validateConfig(app, systemConfig); err != nil {
		return selection, err
	}
	if err := requireExtendedEdition(); err != nil {
		return selection, err
	}
	if err := validateTrustedSystemConfig(systemConfig); err != nil {
		return selection, err
	}
	selection.DisableRemoteMeta = systemConfig.Mode.IsProxy()
	selection.Plan = newRuntimePlan(app, systemConfig)
	return selection, nil
}
