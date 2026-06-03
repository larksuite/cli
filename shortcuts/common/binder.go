// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/larksuite/cli/errs"
	"github.com/larksuite/cli/internal/cmdutil"
)

// fieldSpec is the binder's intermediate representation of one Args field.
// It captures both reflection metadata and the parsed tag values so later
// binder stages don't repeat the parsing work.
type fieldSpec struct {
	GoFieldName  string
	FlagName     string
	Description  string
	DefaultValue string
	EnumValues   []string
	Input        []string
	Required     bool
	Hidden       bool
	NoSplit      bool
	IsOneOfBkt   bool
	IsGroup      bool
	IsPtr        bool
	FieldType    reflect.Type
	StructType   reflect.Type
}

// walkArgs reflects an Args struct (must be *T where T is struct) and
// returns one fieldSpec per top-level field. Duplicate flag tags inside
// the same Args struct panic at Mount time — cross-shortcut duplicates are
// not checked (cobra's own per-command Add check covers that).
func walkArgs(t reflect.Type) ([]fieldSpec, error) {
	if t.Kind() != reflect.Ptr || t.Elem().Kind() != reflect.Struct {
		return nil, fmt.Errorf("Args must be *struct, got %s", t)
	}
	st := t.Elem()
	specs := make([]fieldSpec, 0, st.NumField())
	seen := map[string]string{}
	for i := 0; i < st.NumField(); i++ {
		f := st.Field(i)
		spec, err := parseFieldSpec(f)
		if err != nil {
			return nil, err
		}
		if spec.FlagName != "" {
			if owner, dup := seen[spec.FlagName]; dup {
				panic(fmt.Sprintf("duplicate flag tag %q in Args struct: fields %s and %s",
					spec.FlagName, owner, f.Name))
			}
			seen[spec.FlagName] = f.Name
		}
		specs = append(specs, spec)
	}
	return specs, nil
}

// parseFieldSpec extracts the relevant tag values from one struct field.
// Sub-struct (OneOf bucket / group) detection is delegated to caller stages;
// here we only set flags about the field shape.
func parseFieldSpec(f reflect.StructField) (fieldSpec, error) {
	spec := fieldSpec{
		GoFieldName:  f.Name,
		FlagName:     f.Tag.Get("flag"),
		Description:  f.Tag.Get("desc"),
		DefaultValue: f.Tag.Get("default"),
	}
	if enum := f.Tag.Get("enum"); enum != "" {
		spec.EnumValues = strings.Split(enum, ",")
	}
	if _, has := f.Tag.Lookup("required"); has {
		spec.Required = true
	}
	if input := f.Tag.Get("input"); input != "" {
		for _, src := range strings.Split(input, ",") {
			switch src = strings.TrimSpace(src); src {
			case "":
				// tolerate stray commas
			case File, Stdin:
				spec.Input = append(spec.Input, src)
			default:
				return spec, fmt.Errorf("field %s: unknown input source %q (allowed: %q, %q)", f.Name, src, File, Stdin)
			}
		}
	}
	if _, has := f.Tag.Lookup("hidden"); has {
		spec.Hidden = true
	}
	switch split := strings.TrimSpace(f.Tag.Get("split")); split {
	case "", "comma":
		// default: cobra StringSlice (comma-separated, also repeatable)
	case "none":
		spec.NoSplit = true // cobra StringArray: repeatable, no comma split
	default:
		return spec, fmt.Errorf("field %s: unknown split mode %q (allowed: comma, none)", f.Name, split)
	}
	ft := f.Type
	spec.FieldType = ft
	if ft.Kind() == reflect.Ptr {
		spec.IsPtr = true
		ft = ft.Elem()
	}
	if ft.Kind() == reflect.Struct {
		spec.StructType = ft
		ptr := reflect.PointerTo(ft)
		marker := reflect.TypeOf((*OneOfMarker)(nil)).Elem()
		if ft.Implements(marker) || ptr.Implements(marker) {
			spec.IsOneOfBkt = true
		} else {
			spec.IsGroup = true
		}
		return spec, nil
	}
	// Leaf field validation. Multi-value flags are []string (cobra
	// StringSlice / StringArray); any other slice element type is unsupported.
	// enum / input only make sense on plain string leaves (string or a
	// string-alias like ChatID); split only on []string. Reject mismatches at
	// Mount time instead of silently skipping or panicking at runtime.
	isStringSlice := ft.Kind() == reflect.Slice && ft.Elem().Kind() == reflect.String
	if ft.Kind() == reflect.Slice && !isStringSlice {
		return spec, fmt.Errorf("field %s: only []string slices are supported, got %s", f.Name, ft)
	}
	if spec.NoSplit && !isStringSlice {
		return spec, fmt.Errorf("field %s: split tag is only supported on []string fields", f.Name)
	}
	if ft.Kind() != reflect.String {
		if len(spec.EnumValues) > 0 {
			return spec, fmt.Errorf("field %s: enum tag is only supported on string fields", f.Name)
		}
		if len(spec.Input) > 0 {
			return spec, fmt.Errorf("field %s: input tag is only supported on string fields", f.Name)
		}
	}
	return spec, nil
}

// registerFlags registers cobra flags for the given specs. Sub-struct fields
// (OneOf bucket / group) recurse into their inner fields.
func registerFlags(cmd *cobra.Command, specs []fieldSpec) error {
	for _, s := range specs {
		if s.IsOneOfBkt || s.IsGroup {
			inner, err := walkArgs(reflect.PointerTo(s.StructType))
			if err != nil {
				return err
			}
			if err := registerFlags(cmd, inner); err != nil {
				return err
			}
			continue
		}
		if err := registerLeaf(cmd, s, s.FieldType); err != nil {
			return err
		}
	}
	return nil
}

// registerLeaf registers a single primitive flag based on the underlying type.
func registerLeaf(cmd *cobra.Command, s fieldSpec, t reflect.Type) error {
	if s.FlagName == "" {
		return nil
	}
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.Bool:
		def := s.DefaultValue == "true"
		cmd.Flags().Bool(s.FlagName, def, s.Description)
	case reflect.Int, reflect.Int64:
		def := 0
		if s.DefaultValue != "" {
			def, _ = strconv.Atoi(s.DefaultValue)
		}
		cmd.Flags().Int(s.FlagName, def, s.Description)
	case reflect.Slice:
		// []string (validated at parse time). NoSplit → StringArray
		// (repeatable, literal); default → StringSlice (comma-separated).
		if s.NoSplit {
			cmd.Flags().StringArray(s.FlagName, nil, s.Description)
		} else {
			cmd.Flags().StringSlice(s.FlagName, nil, s.Description)
		}
	default:
		cmd.Flags().String(s.FlagName, s.DefaultValue, s.Description)
	}
	if s.Required {
		_ = cmd.MarkFlagRequired(s.FlagName)
	}
	if s.Hidden {
		_ = cmd.Flags().MarkHidden(s.FlagName)
	}
	if len(s.EnumValues) > 0 {
		vals := s.EnumValues
		cmdutil.RegisterFlagCompletion(cmd, s.FlagName, func(_ *cobra.Command, _ []string, _ string) ([]string, cobra.ShellCompDirective) {
			return vals, cobra.ShellCompDirectiveNoFileComp
		})
	}
	return nil
}

// resolveTypedInputs applies @file / stdin resolution to every leaf flag that
// declared an `input:"file,stdin"` tag, recursing into OneOf buckets and groups
// (cobra flags are flat, so a nested variant's flag resolves the same way). It
// runs before bindFlags so the file/stdin content is read back into the cobra
// flag and then bound into the Args struct. This is the typed counterpart of
// runShortcut's resolveInputFlags, which only sees the legacy shell's (empty)
// Flags slice — both ultimately call resolveInputForFlag.
func resolveTypedInputs(rctx *RuntimeContext, specs []fieldSpec) error {
	stdinUsed := false
	return resolveTypedInputsRec(rctx, specs, &stdinUsed)
}

func resolveTypedInputsRec(rctx *RuntimeContext, specs []fieldSpec, stdinUsed *bool) error {
	for _, s := range specs {
		if s.IsOneOfBkt || s.IsGroup {
			inner, err := walkArgs(reflect.PointerTo(s.StructType))
			if err != nil {
				return err
			}
			if err := resolveTypedInputsRec(rctx, inner, stdinUsed); err != nil {
				return err
			}
			continue
		}
		if s.FlagName == "" || len(s.Input) == 0 {
			continue
		}
		if err := resolveInputForFlag(rctx, s.FlagName, s.Input, stdinUsed); err != nil {
			return err
		}
	}
	return nil
}

// bindFlags writes cobra-parsed values back into the Args struct. argsVal
// is a reflect.Value of the struct (not pointer).
func bindFlags(cmd *cobra.Command, argsVal reflect.Value, specs []fieldSpec) error {
	for _, s := range specs {
		if s.IsOneOfBkt || s.IsGroup {
			continue
		}
		if s.FlagName == "" {
			continue
		}
		if err := bindLeaf(cmd, argsVal, s); err != nil {
			return err
		}
	}
	return nil
}

func bindLeaf(cmd *cobra.Command, argsVal reflect.Value, s fieldSpec) error {
	fv := argsVal.FieldByName(s.GoFieldName)
	if !fv.CanSet() {
		return nil
	}
	// Pointer leaves preserve "nil = not given" semantics: only allocate when
	// the user explicitly set the flag. This mirrors the OneOf bucket
	// convention (`Chat *ChatID` — nil means the variant wasn't selected) and
	// lets typed shortcuts express tri-state bool / int / string flags using
	// plain Go pointers instead of a separate Maybe[T] wrapper type.
	if s.IsPtr && !cmd.Flags().Changed(s.FlagName) {
		return nil
	}
	leafType := s.FieldType
	if leafType.Kind() == reflect.Ptr {
		leafType = leafType.Elem()
	}
	switch leafType.Kind() {
	case reflect.Bool:
		v, _ := cmd.Flags().GetBool(s.FlagName)
		setLeaf(fv, reflect.ValueOf(v))
	case reflect.Int, reflect.Int64:
		v, _ := cmd.Flags().GetInt(s.FlagName)
		setLeaf(fv, reflect.ValueOf(int64(v)).Convert(leafType))
	case reflect.Slice:
		var vals []string
		if s.NoSplit {
			vals, _ = cmd.Flags().GetStringArray(s.FlagName)
		} else {
			vals, _ = cmd.Flags().GetStringSlice(s.FlagName)
		}
		setLeaf(fv, stringsToSlice(vals, leafType))
	default:
		v, _ := cmd.Flags().GetString(s.FlagName)
		setLeaf(fv, reflect.ValueOf(v).Convert(leafType))
	}
	return nil
}

// stringsToSlice builds a slice value of type t (kind Slice with string-kinded
// elements) from raw strings, element-by-element via SetString. This handles a
// plain []string, a named slice type (type IDs []string), AND a named element
// type (type ID string → []ID) — reflect.Convert would panic on the last case
// because []string is not convertible to []ID.
func stringsToSlice(vals []string, t reflect.Type) reflect.Value {
	out := reflect.MakeSlice(t, len(vals), len(vals))
	for i, v := range vals {
		out.Index(i).SetString(v)
	}
	return out
}

func setLeaf(dst reflect.Value, src reflect.Value) {
	if dst.Kind() == reflect.Ptr {
		ptr := reflect.New(dst.Type().Elem())
		ptr.Elem().Set(src.Convert(dst.Type().Elem()))
		dst.Set(ptr)
		return
	}
	dst.Set(src.Convert(dst.Type()))
}

// bindBuckets allocates and populates OneOf bucket / group sub-struct fields
// from cobra flag state. The shared bindFlags() above only writes top-level
// leaves; this function is the framework's recursion into nested Args structs
// so future typed shortcuts don't each have to ship a bespoke binder helper.
//
// Conventions:
//   - Pointer-leaf in a bucket (e.g. *string, *argstype.ChatID): set iff
//     cobra reports the flag was explicitly provided. nil means "variant not
//     selected" — the framework's runFrameworkRules and runValidateValue
//     both honor this nil/non-nil split.
//   - Non-pointer leaf in a group (e.g. a typed-primitive field inside a
//     paired group struct): always copy the cobra flag value back. Empty
//     string is a valid "not provided" sentinel for group completeness checks.
//   - Pointer-to-group / pointer-to-bucket (a nested group/bucket pointer
//     inside an outer OneOf bucket): allocate iff at least one inner flag was
//     Changed, then recurse to bind the inner fields.
func bindBuckets(cmd *cobra.Command, argsVal reflect.Value, specs []fieldSpec) error {
	for _, s := range specs {
		if !s.IsOneOfBkt {
			continue
		}
		fv := argsVal.FieldByName(s.GoFieldName)
		if !fv.IsValid() || !fv.CanSet() {
			continue
		}
		target := fv
		if target.Kind() == reflect.Ptr {
			if target.IsNil() {
				target.Set(reflect.New(s.StructType))
			}
			target = target.Elem()
		}
		inner, err := walkArgs(reflect.PointerTo(s.StructType))
		if err != nil {
			return err
		}
		if err := bindBucketInner(cmd, target, inner); err != nil {
			return err
		}
	}
	return nil
}

// bindGroups is the top-level counterpart to bindBuckets for IsGroup fields
// (regular nested structs without an OneOf() marker). A group's inner flags
// are registered and validated for completeness, but bindFlags above skips
// the field; this function fills the binding gap so an Args struct can place
// a group directly at the top level (not just nested inside an OneOf bucket).
//
// Conventions mirror bindBuckets / bindBucketInner:
//   - Value-type group: always populated; inner fields receive cobra flag
//     values (including defaults).
//   - Pointer-type group: allocated iff at least one inner flag was Changed,
//     so a nil group still signals "user did not engage this group at all".
func bindGroups(cmd *cobra.Command, argsVal reflect.Value, specs []fieldSpec) error {
	for _, s := range specs {
		if !s.IsGroup {
			continue
		}
		fv := argsVal.FieldByName(s.GoFieldName)
		if !fv.IsValid() || !fv.CanSet() {
			continue
		}
		inner, err := walkArgs(reflect.PointerTo(s.StructType))
		if err != nil {
			return err
		}
		if fv.Kind() == reflect.Ptr {
			anyChanged := false
			for _, g := range inner {
				if g.FlagName != "" && cmd.Flags().Changed(g.FlagName) {
					anyChanged = true
					break
				}
			}
			if !anyChanged {
				continue
			}
			if fv.IsNil() {
				fv.Set(reflect.New(s.StructType))
			}
			if err := bindBucketInner(cmd, fv.Elem(), inner); err != nil {
				return err
			}
			continue
		}
		// Value-type group: populate inner fields directly.
		if err := bindBucketInner(cmd, fv, inner); err != nil {
			return err
		}
	}
	return nil
}

func bindBucketInner(cmd *cobra.Command, argsVal reflect.Value, specs []fieldSpec) error {
	for _, s := range specs {
		if s.IsOneOfBkt || s.IsGroup {
			grandSpecs, err := walkArgs(reflect.PointerTo(s.StructType))
			if err != nil {
				return err
			}
			anyChanged := false
			for _, g := range grandSpecs {
				if g.FlagName != "" && cmd.Flags().Changed(g.FlagName) {
					anyChanged = true
					break
				}
			}
			if !anyChanged {
				continue
			}
			fv := argsVal.FieldByName(s.GoFieldName)
			if !fv.IsValid() || !fv.CanSet() {
				continue
			}
			target := fv
			if fv.Kind() == reflect.Ptr {
				if fv.IsNil() {
					fv.Set(reflect.New(s.StructType))
				}
				target = fv.Elem()
			}
			if err := bindBucketInner(cmd, target, grandSpecs); err != nil {
				return err
			}
			continue
		}
		if s.FlagName == "" {
			continue
		}
		fv := argsVal.FieldByName(s.GoFieldName)
		if !fv.IsValid() || !fv.CanSet() {
			continue
		}
		if s.IsPtr {
			if !cmd.Flags().Changed(s.FlagName) {
				continue
			}
			elemType := fv.Type().Elem()
			ptr := reflect.New(elemType)
			ptr.Elem().Set(bucketLeafValue(cmd, s.FlagName, elemType, s.NoSplit))
			fv.Set(ptr)
			continue
		}
		fv.Set(bucketLeafValue(cmd, s.FlagName, fv.Type(), s.NoSplit))
	}
	return nil
}

// bucketLeafValue reads a single cobra flag and returns it as a reflect.Value
// convertible to targetType. It dispatches on the underlying kind so nested
// bucket/group leaves typed as bool or int bind correctly instead of being
// force-read through GetString (which would panic on reflect conversion of a
// string into a numeric/bool type).
func bucketLeafValue(cmd *cobra.Command, flagName string, targetType reflect.Type, noSplit bool) reflect.Value {
	kind := targetType.Kind()
	if kind == reflect.Ptr {
		kind = targetType.Elem().Kind()
	}
	switch kind {
	case reflect.Bool:
		v, _ := cmd.Flags().GetBool(flagName)
		return reflect.ValueOf(v).Convert(targetType)
	case reflect.Int, reflect.Int64:
		v, _ := cmd.Flags().GetInt(flagName)
		return reflect.ValueOf(int64(v)).Convert(targetType)
	case reflect.Slice:
		var vals []string
		if noSplit {
			vals, _ = cmd.Flags().GetStringArray(flagName)
		} else {
			vals, _ = cmd.Flags().GetStringSlice(flagName)
		}
		return stringsToSlice(vals, targetType)
	default:
		v, _ := cmd.Flags().GetString(flagName)
		return reflect.ValueOf(v).Convert(targetType)
	}
}

// runNormalize invokes the Normalize method (via reflection) on every field
// whose type implements Normalizable[T]. The canonical value is written back.
// Plan's static type assertion can't work because Normalizable is generic —
// the method's return type is the typed T, not any — so we dispatch by
// reflection on method shape.
//
// Local-path normalization hints are dropped at the primitive layer; only
// non-path hints reach stderr (log safety).
func runNormalize(ctx context.Context, rt *RuntimeContext, argsVal reflect.Value, specs []fieldSpec) error {
	ctxType := reflect.TypeOf((*context.Context)(nil)).Elem()
	for _, s := range specs {
		fv := argsVal.FieldByName(s.GoFieldName)
		if !fv.IsValid() {
			continue
		}
		m := fv.MethodByName("Normalize")
		if !m.IsValid() {
			continue
		}
		mt := m.Type()
		if mt.NumIn() != 2 || mt.NumOut() != 3 {
			continue
		}
		if !ctxType.AssignableTo(mt.In(0)) || mt.In(1).Kind() != reflect.String {
			continue
		}
		raw := asString(fv)
		rets := m.Call([]reflect.Value{reflect.ValueOf(ctx), reflect.ValueOf(raw)})
		if errRet := rets[2]; !errRet.IsNil() {
			return errRet.Interface().(error)
		}
		canon := rets[0]
		if fv.CanSet() && canon.Type().AssignableTo(fv.Type()) {
			fv.Set(canon)
		}
		hints, _ := rets[1].Interface().([]string)
		if len(hints) > 0 && rt != nil && rt.Cmd != nil {
			for _, h := range hints {
				_, _ = rt.Cmd.ErrOrStderr().Write([]byte(h + "\n"))
			}
		}
	}
	return nil
}

func asString(fv reflect.Value) string {
	if fv.Kind() == reflect.String {
		return fv.String()
	}
	if fv.Kind() == reflect.Ptr && !fv.IsNil() {
		return asString(fv.Elem())
	}
	return ""
}

// runValidateValue calls ValidateValue on every Validatable field, recursing
// into OneOf bucket / group sub-structs so typed-primitive leaves inside
// nested Args structs (e.g. a typed ID primitive inside a OneOf bucket) still
// get their format check. Returns the first error to keep error envelopes
// deterministic.
func runValidateValue(rt *RuntimeContext, argsVal reflect.Value, specs []fieldSpec) error {
	for _, s := range specs {
		fv := argsVal.FieldByName(s.GoFieldName)
		if !fv.IsValid() {
			continue
		}
		if s.IsOneOfBkt || s.IsGroup {
			structVal := fv
			if structVal.Kind() == reflect.Ptr {
				if structVal.IsNil() {
					continue
				}
				structVal = structVal.Elem()
			}
			// Some buckets/groups implement Validatable themselves (e.g. a
			// raw-JSON variant that checks JSON validity in its ValidateValue).
			// Call the struct-level check BEFORE recursing into inner fields so
			// the cross-field rule fires even when none of the inner leaves are
			// individually Validatable.
			if fv.CanInterface() {
				if val, ok := fv.Interface().(Validatable); ok {
					if err := val.ValidateValue(rt, s.FlagName); err != nil {
						return err
					}
				}
			}
			inner, err := walkArgs(reflect.PointerTo(s.StructType))
			if err != nil {
				return err
			}
			if err := runValidateValue(rt, structVal, inner); err != nil {
				return err
			}
			continue
		}
		// Leaf field. Skip nil pointers (variant not selected).
		if fv.Kind() == reflect.Ptr && fv.IsNil() {
			continue
		}
		if !fv.CanInterface() {
			continue
		}
		v := fv.Interface()
		if val, ok := v.(Validatable); ok {
			if err := val.ValidateValue(rt, s.FlagName); err != nil {
				return err
			}
			continue
		}
		// Pointer leaf: dereference and re-check (for value-receiver
		// ValidateValue methods on the pointee type).
		if fv.Kind() == reflect.Ptr {
			if val, ok := fv.Elem().Interface().(Validatable); ok {
				if err := val.ValidateValue(rt, s.FlagName); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// runFrameworkRules enforces OneOf / group / required / enum invariants and
// returns a *errs.ValidationError on the first violation. Each rule's
// stderr-facing param is the Args field name (not the inner struct type name),
// so OneOf bucket errors mention the user-visible field (e.g. "Target") rather
// than the implementation-detail type name behind it.
//
// Recurses into OneOf bucket sub-structs so a nested group inside a bucket
// still gets its checkGroup fire automatically.
func runFrameworkRules(cmd *cobra.Command, argsVal reflect.Value, specs []fieldSpec) error {
	for _, s := range specs {
		fv := argsVal.FieldByName(s.GoFieldName)
		switch {
		case s.IsOneOfBkt:
			if err := checkOneOf(cmd, fv, s); err != nil {
				return err
			}
			structVal := fv
			if structVal.Kind() == reflect.Ptr {
				if structVal.IsNil() {
					continue
				}
				structVal = structVal.Elem()
			}
			inner, err := walkArgs(reflect.PointerTo(s.StructType))
			if err != nil {
				return err
			}
			if err := runFrameworkRules(cmd, structVal, inner); err != nil {
				return err
			}
		case s.IsGroup:
			if err := checkGroup(cmd, fv, s); err != nil {
				return err
			}
		default:
			if err := checkEnumAndRequired(cmd, fv, s); err != nil {
				return err
			}
		}
	}
	return nil
}

// checkOneOf counts how many variants the user attempted inside the OneOf
// bucket; exactly one must be attempted. A variant counts as "attempted" if:
//   - it's a simple pointer leaf (e.g. *string, *ChatID) and its own flag was
//     explicitly provided, or
//   - it's a nested group / bucket and ANY of its inner flags was explicitly
//     provided. No "trigger" field is required — supplying any flag of a
//     group is enough to mark the variant as attempted, and a follow-up
//     checkGroup catches the partial-fill case with shortcut_group_incomplete.
func checkOneOf(cmd *cobra.Command, _ reflect.Value, s fieldSpec) error {
	inner, err := walkArgs(reflect.PointerTo(s.StructType))
	if err != nil {
		return err
	}
	var triggered []string
	for _, child := range inner {
		// Simple pointer leaf variant: its own flag is the signal.
		if child.IsPtr && !child.IsOneOfBkt && !child.IsGroup {
			if child.FlagName != "" && cmd.Flags().Changed(child.FlagName) {
				triggered = append(triggered, "--"+child.FlagName)
			}
			continue
		}
		// Nested group / bucket variant: any inner flag Changed counts.
		if child.IsGroup || child.IsOneOfBkt {
			grand, _ := walkArgs(reflect.PointerTo(child.StructType))
			for _, g := range grand {
				if g.FlagName != "" && cmd.Flags().Changed(g.FlagName) {
					triggered = append(triggered, "--"+g.FlagName)
					break
				}
			}
		}
	}
	switch len(triggered) {
	case 0:
		return &errs.ValidationError{
			Problem: errs.Problem{
				Category: errs.CategoryValidation,
				Subtype:  errs.SubtypeShortcutOneOfMissing,
				Message:  "exactly one " + s.GoFieldName + " variant must be provided",
			},
			Param: s.GoFieldName,
		}
	case 1:
		return nil
	default:
		return &errs.ValidationError{
			Problem: errs.Problem{
				Category: errs.CategoryValidation,
				Subtype:  errs.SubtypeShortcutOneOfMultiple,
				Message:  "choose only one of " + strings.Join(triggered, ", "),
			},
			Param: s.GoFieldName,
		}
	}
}

// checkGroup ensures all fields of a group sub-struct were provided when
// the group's trigger (or first field) was set.
func checkGroup(cmd *cobra.Command, _ reflect.Value, s fieldSpec) error {
	inner, err := walkArgs(reflect.PointerTo(s.StructType))
	if err != nil {
		return err
	}
	anySet := false
	var missing []string
	for _, child := range inner {
		if child.FlagName == "" {
			continue
		}
		if cmd.Flags().Changed(child.FlagName) {
			anySet = true
			continue
		}
		// Flags with a default value are never "missing" — the default is a
		// valid implicit value (e.g. an enum flag that defaults to a value).
		// Only flags without defaults need explicit user input when the
		// group is partially populated.
		if child.DefaultValue != "" {
			continue
		}
		missing = append(missing, "--"+child.FlagName)
	}
	if anySet && len(missing) > 0 {
		// Group errors use the inner struct TYPE name (the group struct's own
		// name), not the Args field name. This matches the spec's
		// "shortcut_group_incomplete" envelope contract — callers identify
		// the *kind* of group that is incomplete, which is the type name.
		// OneOf bucket errors use the field name instead (see checkOneOf).
		return &errs.ValidationError{
			Problem: errs.Problem{
				Category: errs.CategoryValidation,
				Subtype:  errs.SubtypeShortcutGroupIncomplete,
				Message:  s.StructType.Name() + " requires " + strings.Join(missing, ", "),
			},
			Param: s.StructType.Name(),
		}
	}
	return nil
}

// checkEnumAndRequired enforces enum membership. Required-presence is already
// enforced at cobra level via MarkFlagRequired, so this only adds the enum
// check.
func checkEnumAndRequired(cmd *cobra.Command, _ reflect.Value, s fieldSpec) error {
	if len(s.EnumValues) == 0 {
		return nil
	}
	v, _ := cmd.Flags().GetString(s.FlagName)
	if v == "" && s.DefaultValue == "" {
		return nil
	}
	for _, allowed := range s.EnumValues {
		if v == allowed {
			return nil
		}
	}
	return &errs.ValidationError{
		Problem: errs.Problem{
			Category: errs.CategoryValidation,
			Subtype:  errs.SubtypeInvalidArgument,
			Message:  "--" + s.FlagName + ": value must be one of " + strings.Join(s.EnumValues, "|"),
		},
		Param: s.FlagName,
	}
}
