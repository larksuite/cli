// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"fmt"
	"reflect"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/spf13/cobra"
)

// TypedShortcut is the generic counterpart of the legacy Shortcut struct.
// Args must be a pointer to a struct (e.g. *MyArgs). Mounting reflects the
// struct, registers cobra flags, then delegates to a synthesized Shortcut
// shell so the existing run pipeline (identity / scopes / @file / stdin /
// jq / dry-run / high-risk gate) is reused verbatim.
type TypedShortcut[T any] struct {
	Service, Command, Description string
	Risk                          string
	Scopes, UserScopes, BotScopes []string
	ConditionalScopes             []string
	ConditionalUserScopes         []string
	ConditionalBotScopes          []string
	AuthTypes                     []string
	HasFormat                     bool
	Tips                          []string
	Hidden                        bool

	Examples []HelpExample

	DryRun   func(ctx context.Context, args T, rt *RuntimeContext) *DryRunAPI
	Validate func(ctx context.Context, args T, rt *RuntimeContext) error
	Execute  func(ctx context.Context, args T, rt *RuntimeContext) error

	PostMount func(cmd *cobra.Command)
}

func (s TypedShortcut[T]) GetService() string     { return s.Service }
func (s TypedShortcut[T]) GetCommand() string     { return s.Command }
func (s TypedShortcut[T]) GetDescription() string { return s.Description }
func (s TypedShortcut[T]) GetAuthTypes() []string { return s.AuthTypes }
func (s TypedShortcut[T]) GetRisk() string        { return s.Risk }

func (s TypedShortcut[T]) ScopesForIdentity(identity string) []string {
	switch identity {
	case "user":
		if len(s.UserScopes) > 0 {
			return s.UserScopes
		}
	case "bot":
		if len(s.BotScopes) > 0 {
			return s.BotScopes
		}
	}
	return s.Scopes
}

func (s TypedShortcut[T]) ConditionalScopesForIdentity(identity string) []string {
	switch identity {
	case "user":
		if len(s.ConditionalUserScopes) > 0 {
			return s.ConditionalUserScopes
		}
	case "bot":
		if len(s.ConditionalBotScopes) > 0 {
			return s.ConditionalBotScopes
		}
	}
	return s.ConditionalScopes
}

func (s TypedShortcut[T]) DeclaredScopesForIdentity(identity string) []string {
	base := s.ScopesForIdentity(identity)
	extra := s.ConditionalScopesForIdentity(identity)
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	out := make([]string, 0, len(base)+len(extra))
	seen := map[string]struct{}{}
	for _, scope := range append(base, extra...) {
		if scope == "" {
			continue
		}
		if _, ok := seen[scope]; ok {
			continue
		}
		seen[scope] = struct{}{}
		out = append(out, scope)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// Mount registers the typed shortcut on a parent command, mirroring the legacy
// Shortcut.Mount API so migrating common.Shortcut → common.TypedShortcut[T]
// does not force existing callers (or their tests) to also rewrite the Mount
// call site. Delegates to MountWithContext with a background context.
func (s TypedShortcut[T]) Mount(parent *cobra.Command, f *cmdutil.Factory) {
	s.MountWithContext(context.Background(), parent, f)
}

// MountWithContext is implemented in Task 16's adapter section.
func (s TypedShortcut[T]) MountWithContext(ctx context.Context, parent *cobra.Command, f *cmdutil.Factory) {
	mountTyped[T](ctx, parent, f, s)
}

// mountTyped synthesizes a legacy *Shortcut shell that delegates back to the
// typed hooks. The shell's Validate/DryRun/Execute closures read or write
// the per-run typed args via RuntimeContext.{TypedArgs,SetTypedArgs}.
//
// Pipeline order inside the shell (matches runShortcut at runner.go:748):
//
//  1. identity / scopes / runtime    — handled by Shortcut.runShortcut
//  2. validateEnumFlags              — Shortcut machinery
//  3. resolveInputFlags              — @file / stdin (legacy shell only; the
//     synthesized shell's Flags slice is empty, so this is a no-op for typed —
//     typed flags declare inputs via the `input` tag, resolved in step 5)
//  4. ValidateJqFlags                — --jq
//  5. shell.Validate                 — resolveTypedInputs (@file / stdin for
//     `input`-tagged flags), binds T, runs Normalize / ValidateValue /
//     framework rules / ArgsValidator / user-typed Validate
//  6. --dry-run gate                 — shell.DryRun reads typed args from rt
//  7. high-risk-write confirmation   — when Risk == "high-risk-write"
//  8. shell.Execute                  — reads typed args from rt
func mountTyped[T any](ctx context.Context, parent *cobra.Command, f *cmdutil.Factory, s TypedShortcut[T]) {
	// Mirror legacy Shortcut.MountWithContext: a shortcut with no Execute is
	// not a runnable command, so it is not mounted at all (rather than mounted
	// and erroring at invocation time). Keeps the command tree identical to
	// legacy after migration.
	if s.Execute == nil {
		return
	}
	var zero T
	argsType := reflect.TypeOf(zero)
	if argsType == nil || argsType.Kind() != reflect.Ptr {
		panic("TypedShortcut[T]: T must be a pointer to a struct, got " + fmt.Sprintf("%T", zero))
	}
	specs, err := walkArgs(argsType)
	if err != nil {
		panic("TypedShortcut[T].Mount: " + err.Error())
	}

	shell := Shortcut{
		Service:               s.Service,
		Command:               s.Command,
		Description:           s.Description,
		Risk:                  s.Risk,
		Scopes:                s.Scopes,
		UserScopes:            s.UserScopes,
		BotScopes:             s.BotScopes,
		ConditionalScopes:     s.ConditionalScopes,
		ConditionalUserScopes: s.ConditionalUserScopes,
		ConditionalBotScopes:  s.ConditionalBotScopes,
		AuthTypes:             s.AuthTypes,
		HasFormat:             s.HasFormat,
		Tips:                  s.Tips,
		Hidden:                s.Hidden,
		PostMount: func(cmd *cobra.Command) {
			if err := registerFlags(cmd, specs); err != nil {
				panic("TypedShortcut[T] registerFlags: " + err.Error())
			}
			cmd.SetHelpFunc(buildTypedHelp(specs, s.Examples))
			if s.PostMount != nil {
				s.PostMount(cmd)
			}
		},
		Validate: func(c context.Context, rt *RuntimeContext) error {
			// @file / stdin resolution for flags that declared an `input` tag.
			// Runs before bindFlags so the resolved file/stdin content is what
			// gets bound into the Args struct.
			if err := resolveTypedInputs(rt, specs); err != nil {
				return err
			}
			args := reflect.New(argsType.Elem()).Interface().(T)
			argsVal := reflect.ValueOf(args).Elem()
			if err := bindFlags(rt.Cmd, argsVal, specs); err != nil {
				return err
			}
			if err := bindBuckets(rt.Cmd, argsVal, specs); err != nil {
				return err
			}
			if err := bindGroups(rt.Cmd, argsVal, specs); err != nil {
				return err
			}
			if err := runNormalize(c, rt, argsVal, specs); err != nil {
				return err
			}
			if err := runValidateValue(rt, argsVal, specs); err != nil {
				return err
			}
			if err := runFrameworkRules(rt.Cmd, argsVal, specs); err != nil {
				return err
			}
			if av, ok := any(args).(ArgsValidator); ok {
				if err := av.Validate(c, rt); err != nil {
					return err
				}
			}
			rt.SetTypedArgs(args)
			if s.Validate != nil {
				return s.Validate(c, args, rt)
			}
			return nil
		},
		Execute: func(c context.Context, rt *RuntimeContext) error {
			if s.Execute == nil {
				return &errs.InternalError{
					Problem: errs.Problem{
						Category: errs.CategoryInternal,
						Message:  "shortcut " + s.Service + " " + s.Command + " has no Execute handler",
					},
				}
			}
			args, _ := rt.TypedArgs().(T)
			return s.Execute(c, args, rt)
		},
	}
	if s.DryRun != nil {
		shell.DryRun = func(c context.Context, rt *RuntimeContext) *DryRunAPI {
			args, _ := rt.TypedArgs().(T)
			return s.DryRun(c, args, rt)
		}
	}
	shell.MountWithContext(ctx, parent, f)
}
