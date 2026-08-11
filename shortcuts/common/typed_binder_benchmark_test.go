// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package common

import (
	"reflect"
	"testing"
)

type binderBenchmarkArgs struct {
	Value Provided[int]
}

func BenchmarkTypedBinderIndexedAssignment(b *testing.B) {
	field := compiledInputField{index: []int{0}, valueIndex: []int{0}, provided: true, valueType: reflect.TypeFor[int](), goName: "Value", name: "value"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		args := binderBenchmarkArgs{}
		if err := assignCompiledField(reflect.ValueOf(&args).Elem(), field, 42, true); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkTypedBinderDirectAssignment(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		args := binderBenchmarkArgs{}
		args.Value = Provided[int]{Value: 42, Set: true}
		_ = args
	}
}
