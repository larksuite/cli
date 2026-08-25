// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//nolint:forbidigo // PostMount guard errors are registration-time programmer diagnostics consumed by the mount panic boundary.
package common

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type typedMountSnapshot struct {
	use, short, long, example                          string
	aliases                                            []string
	argsFunc, preRunFunc, runFunc, usageFunc, helpFunc uintptr
	annotations                                        map[string]string
	flags                                              map[string]typedMountedFlag
}

type typedMountedFlag struct {
	valueType, defaultValue, usage, shorthand, noOptDefault, deprecated string
	hidden                                                              bool
	annotations                                                         map[string][]string
}

func captureTypedMountSnapshot(command *cobra.Command) typedMountSnapshot {
	snapshot := typedMountSnapshot{
		use: command.Use, short: command.Short, long: command.Long, example: command.Example,
		aliases: append([]string{}, command.Aliases...), annotations: make(map[string]string, len(command.Annotations)), flags: make(map[string]typedMountedFlag),
	}
	for key, value := range command.Annotations {
		snapshot.annotations[key] = value
	}
	if command.Args != nil {
		snapshot.argsFunc = reflect.ValueOf(command.Args).Pointer()
	}
	if command.PreRunE != nil {
		snapshot.preRunFunc = reflect.ValueOf(command.PreRunE).Pointer()
	}
	if command.RunE != nil {
		snapshot.runFunc = reflect.ValueOf(command.RunE).Pointer()
	}
	if usage := command.UsageFunc(); usage != nil {
		snapshot.usageFunc = reflect.ValueOf(usage).Pointer()
	}
	if help := command.HelpFunc(); help != nil {
		snapshot.helpFunc = reflect.ValueOf(help).Pointer()
	}
	command.LocalFlags().VisitAll(func(flag *pflag.Flag) {
		annotations := make(map[string][]string, len(flag.Annotations))
		for key, values := range flag.Annotations {
			annotations[key] = append([]string{}, values...)
		}
		snapshot.flags[flag.Name] = typedMountedFlag{
			valueType: flag.Value.Type(), defaultValue: flag.DefValue, usage: flag.Usage,
			shorthand: flag.Shorthand, noOptDefault: flag.NoOptDefVal, deprecated: flag.Deprecated,
			hidden: flag.Hidden, annotations: annotations,
		}
	})
	return snapshot
}

func validateTypedPostMount(command *cobra.Command, before typedMountSnapshot) error {
	after := captureTypedMountSnapshot(command)
	if before.use != after.use || before.short != after.short || before.long != after.long || before.example != after.example || !reflect.DeepEqual(before.aliases, after.aliases) {
		return fmt.Errorf("PostMount modified Typed command metadata or Help text")
	}
	if before.argsFunc != after.argsFunc || before.preRunFunc != after.preRunFunc || before.runFunc != after.runFunc || before.usageFunc != after.usageFunc || before.helpFunc != after.helpFunc {
		return fmt.Errorf("PostMount replaced Typed command validation, execution, or Help functions")
	}
	if !reflect.DeepEqual(before.annotations, after.annotations) {
		return fmt.Errorf("PostMount modified Typed command annotations")
	}
	names := make([]string, 0, len(before.flags)+len(after.flags))
	seen := make(map[string]struct{}, len(before.flags)+len(after.flags))
	for name := range before.flags {
		seen[name] = struct{}{}
		names = append(names, name)
	}
	for name := range after.flags {
		if _, ok := seen[name]; !ok {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		beforeFlag, existedBefore := before.flags[name]
		afterFlag, existsAfter := after.flags[name]
		switch {
		case !existedBefore:
			return fmt.Errorf("PostMount added undeclared Typed flag --%s", name)
		case !existsAfter:
			return fmt.Errorf("PostMount removed Typed flag --%s", name)
		case !reflect.DeepEqual(beforeFlag, afterFlag):
			return fmt.Errorf("PostMount modified Typed flag --%s or its input annotations", name)
		}
	}
	return nil
}
