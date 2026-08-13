// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package externalcredential

import (
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/runtimeplan"
)

func newRuntimePlan(*core.AppConfig, *Config) *runtimeplan.Plan {
	return runtimeplan.Failed(extendedEditionRequired(), runtimeplan.MetadataEmbeddedOnly)
}
