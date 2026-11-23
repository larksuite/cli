// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"errors"
	"fmt"
	"strings"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/flagalias"
	"github.com/spf13/pflag"
)

// attributeAliasValidationError translates canonical error parameters back to
// the spelling that supplied their effective values. It belongs at the command
// boundary: business validators name only canonical flags and do not depend on
// alias metadata.
func attributeAliasValidationError(ctx *RuntimeContext, err error) error {
	if err == nil || ctx == nil || ctx.Cmd == nil {
		return err
	}
	var validationErr *errs.ValidationError
	if !errors.As(err, &validationErr) {
		return err
	}

	attribute := func(param string) string {
		flag := exactCanonicalFlag(ctx.Cmd.Flags(), param)
		if flag == nil || len(flagalias.Aliases(flag)) == 0 {
			return param
		}
		actual := callerFlagParam(flag)
		if actual != "--"+flag.Name {
			aliasHint := fmt.Sprintf("%s maps to canonical flag --%s", actual, flag.Name)
			if validationErr.Hint == "" {
				validationErr.Hint = aliasHint
			} else if !strings.Contains(validationErr.Hint, aliasHint) {
				validationErr.Hint += "\n" + aliasHint
			}
		}
		return actual
	}

	validationErr.Param = attribute(validationErr.Param)
	for i := range validationErr.Params {
		validationErr.Params[i].Name = attribute(validationErr.Params[i].Name)
	}
	return err
}

// exactCanonicalFlag deliberately avoids FlagSet.Lookup: alias normalization
// makes Lookup(alias) return the canonical flag, while error attribution must
// only translate parameters authored canonically by business code.
func exactCanonicalFlag(flags *pflag.FlagSet, param string) *pflag.Flag {
	name, ok := strings.CutPrefix(param, "--")
	if !ok || name == "" {
		return nil
	}
	var found *pflag.Flag
	flags.VisitAll(func(flag *pflag.Flag) {
		if flag.Name == name {
			found = flag
		}
	})
	return found
}

func callerFlagParam(flag *pflag.Flag) string {
	source := flagalias.Source(flag)
	if source == "" {
		source = flag.Name
	}
	return "--" + source
}
