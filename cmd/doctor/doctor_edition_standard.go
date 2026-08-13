// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

//go:build !extended

package doctor

import "github.com/larksuite/cli/errs"

func runEditionDoctor(opts *DoctorOptions, checks []checkResult) (bool, error) {
	if opts == nil || opts.Factory == nil {
		return false, nil
	}
	startupErr := opts.Factory.RuntimeStartupError()
	if startupErr == nil {
		return false, nil
	}
	hint := ""
	if problem, ok := errs.ProblemOf(startupErr); ok {
		hint = problem.Hint
	}
	checks = append(checks, fail("credential_source", startupErr.Error(), hint))
	return true, finishDoctor(opts.Factory, checks)
}
