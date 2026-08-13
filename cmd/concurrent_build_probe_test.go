// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package cmd

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/larksuite/cli/internal/apicatalog"
	"github.com/larksuite/cli/internal/cmdutil"
	"github.com/larksuite/cli/internal/core"
	"github.com/larksuite/cli/internal/runtimebootstrap"
	"github.com/larksuite/cli/internal/runtimeplan"
)

// TestConcurrentBuildsKeepStaticCatalogStateRaceFree guards the immutable
// metadata assets reached while two command trees are assembled. It is most
// valuable under `go test -race`, which is part of make unit-test on supported
// architectures.
func TestConcurrentBuildsKeepStaticCatalogStateRaceFree(t *testing.T) {
	profile := &core.MultiAppConfig{Apps: []core.AppConfig{{
		Name:      "default",
		AppId:     "cli_concurrent_build",
		AppSecret: core.PlainSecret("test-secret"),
		Brand:     core.BrandFeishu,
	}}}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for _, plan := range []*runtimeplan.Plan{
		runtimeplan.Default(),
		runtimeplan.New(runtimeplan.Options{Metadata: runtimeplan.MetadataEmbeddedOnly}),
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			buildInternal(
				context.Background(),
				cmdutil.InvocationContext{},
				WithIO(strings.NewReader(""), io.Discard, io.Discard),
				WithServiceCatalog(apicatalog.New(apicatalog.SourceEmbedded, nil)),
				withRuntimeBootstrap(&runtimebootstrap.Result{ProfileConfig: profile, Plan: plan}),
				WithoutPlugins(),
				WithoutStrictMode(),
			)
		}()
	}
	close(start)
	wg.Wait()
}
