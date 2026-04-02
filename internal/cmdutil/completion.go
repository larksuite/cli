// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmdutil

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// NoFileCompletion disables file completion for a command.
// Use this on leaf commands that only take flags, not positional args.
func NoFileCompletion(cmd *cobra.Command) {
	cmd.ValidArgsFunction = func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
}

// RegisterEnumFlag registers a string flag that only allows values listed in options,
// with automatic shell completion. Invalid values are rejected at flag parse time.
func RegisterEnumFlag(cmd *cobra.Command, p *string, name, shorthand, defaultVal string, options []string, usage string) {
	*p = defaultVal
	val := &enumValue{str: p, options: options}
	cmd.Flags().VarPF(val, name, shorthand, fmt.Sprintf("%s: {%s}", usage, strings.Join(options, "|")))
	_ = cmd.RegisterFlagCompletionFunc(name, func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
		return options, cobra.ShellCompDirectiveNoFileComp
	})
}

// enumValue implements pflag.Value with validation against a fixed set of options.
type enumValue struct {
	str     *string
	options []string
}

func (e *enumValue) Set(value string) error {
	for _, opt := range e.options {
		if value == opt {
			*e.str = value
			return nil
		}
	}
	return fmt.Errorf("invalid argument %q for this flag: valid values are {%s}", value, strings.Join(e.options, "|"))
}

func (e *enumValue) String() string {
	return *e.str
}

func (e *enumValue) Type() string {
	return "string"
}
