// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

// Package migrate handles schema-version upgrades for lark-cli's
// on-disk state.

package migrate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	larkauth "github.com/larksuite/cli/internal/auth"
	"github.com/larksuite/cli/internal/core"
)

// flockTimeout bounds the cross-process "login" flock wait. Racing
// migrators are benign: the loser no-ops and the next invocation sees
// the bumped SchemaVersion.
const flockTimeout = 5 * time.Second

// noOp signals a declined run (no config, schema current, flock
// contended). Bootstrap treats it as success.
var noOp = errors.New("migrate: no-op")

// IsNoOp reports whether err is the noOp sentinel.
func IsNoOp(err error) bool { return errors.Is(err, noOp) }

// MaybeMigrate runs the legacy → multi-user migration if needed.
// Safe to call on every invocation; idempotent. Bootstrap callers
// should log-and-continue on error: legacy code paths remain
// back-compat with SchemaVersion=0 input.
//
// errOut receives best-effort warnings prefixed with
// "[lark-cli] [WARN] migrate: ...". Pass io.Discard to silence.
func MaybeMigrate(root larkauth.Root, errOut io.Writer) error {
	if errOut == nil {
		errOut = io.Discard
	}

	// Pre-lock peek: skip without grabbing the flock when the schema
	// is already current, so we don't contend with auth login / users
	// use on every invocation.
	multi, err := core.LoadMultiAppConfig()
	if err != nil || multi == nil {
		// No config yet — first `auth login` will stamp SchemaVersion.
		return noOp
	}
	if multi.SchemaVersion >= core.CurrentSchemaVersion {
		return noOp
	}

	// Same flock used by login / users use / users logout / auth logout,
	// so a migration in flight can't race with any of them.
	flockCtx, cancel := context.WithTimeout(context.Background(), flockTimeout)
	defer cancel()
	lk, err := root.Locks(larkauth.SingleUser()).Acquire(flockCtx, "login", flockTimeout)
	if err != nil {
		// Lost the race — another process is migrating.
		return noOp
	}
	defer lk.Release()

	// Reload under the flock: another migrator may have just finished.
	multi, err = core.LoadMultiAppConfig()
	if err != nil || multi == nil {
		return noOp
	}
	if multi.SchemaVersion >= core.CurrentSchemaVersion {
		return noOp
	}

	now := time.Now().UTC()

	for i := range multi.Apps {
		app := &multi.Apps[i]
		for j := range app.Users {
			u := &app.Users[j]
			if u.UserOpenId == "" {
				continue
			}
			if u.FirstAuthAt == nil {
				ts := now
				u.FirstAuthAt = &ts
			}
			ctx := larkauth.ForUser(app.AppId, u.UserOpenId)

			// Only write if missing, so a rerun doesn't clobber richer
			// data saved by a later login.
			if existing, perr := larkauth.LoadUserProfileFor(root, ctx); perr != nil || existing == nil {
				p := larkauth.UserProfile{
					UserOpenId:  u.UserOpenId,
					UnionId:     u.UnionId,
					UserName:    u.UserName,
					CachedAt:    now,
					FirstAuthAt: now,
				}
				if u.FirstAuthAt != nil && !u.FirstAuthAt.IsZero() {
					p.FirstAuthAt = u.FirstAuthAt.UTC()
				}
				if err := larkauth.SaveUserProfileFor(root, ctx, p); err != nil {
					fmt.Fprintf(errOut, "[lark-cli] [WARN] migrate: backfill sidecar profile for %s/%s: %v\n", app.AppId, u.UserOpenId, err)
				}
			}

			// RecordUserActivity is the same upsert login.go uses
			// post-mint, so row shape stays byte-identical. nil scopes
			// means "don't touch LastScopes".
			if err := larkauth.RecordUserActivity(root, ctx, nil); err != nil {
				fmt.Fprintf(errOut, "[lark-cli] [WARN] migrate: backfill index row for %s/%s: %v\n", app.AppId, u.UserOpenId, err)
			}
		}
	}

	// SaveMultiAppConfig forward-stamps SchemaVersion (bump-only-forward
	// policy lives there). Always save, even when only sidecar/index
	// backfills happened, otherwise SchemaVersion=0 stays on disk and
	// the next invocation redoes the walk.
	if err := core.SaveMultiAppConfig(multi); err != nil {
		return fmt.Errorf("migrate: stamp schema version: %w", err)
	}
	return nil
}
