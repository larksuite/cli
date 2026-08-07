// Copyright (c) 2026 Lark Technologies Pte. Ltd.
// SPDX-License-Identifier: MIT

package gitcred

import (
	"sort"
	"time"

	"github.com/larksuite/cli/internal/keychain"
	"github.com/larksuite/cli/internal/sparkstore"
)

// NewAppManager builds a Git credential manager backed by Spark app storage.
func NewAppManager(appID string, kc keychain.KeychainAccess, issuer Issuer) *Manager {
	storage := sparkstore.AppStorage{}
	return NewManager(NewAppStore(appID, storage), NewSecretStore(kc), GlobalGitConfig{}, issuer)
}

// ListCredentialRecords returns local Git credential records for all stored apps.
func ListCredentialRecords(kc keychain.KeychainAccess, now func() time.Time) ([]ListRecord, error) {
	storage := sparkstore.AppStorage{}
	appIDs, err := storage.ListAppIDs()
	if err != nil {
		return nil, err
	}
	records := make([]ListRecord, 0, len(appIDs))
	for _, appID := range appIDs {
		manager := NewAppManager(appID, kc, nil)
		manager.Now = now
		result, err := manager.List()
		if err != nil {
			return nil, err
		}
		records = append(records, result.Records...)
	}
	sort.Slice(records, func(i, j int) bool {
		if records[i].AppID == records[j].AppID {
			return records[i].GitHTTPURL < records[j].GitHTTPURL
		}
		return records[i].AppID < records[j].AppID
	})
	return records, nil
}
