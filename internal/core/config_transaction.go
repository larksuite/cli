package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
	"github.com/larksuite/cli/internal/validate"
)

func withConfigLock(fn func() error) error {
	path, err := filepath.Abs(GetConfigPath())
	if err != nil {
		return err
	}
	parent := filepath.Dir(path)
	if err = os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	if canonical, e := filepath.EvalSymlinks(parent); e == nil {
		path = filepath.Join(canonical, filepath.Base(path))
	}
	lock := flock.New(path + ".lock")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	ok, err := lock.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("config_lock_timeout")
	}
	defer lock.Unlock()
	return fn()
}

// MergeAppUser merges only the explicitly selected app/user while preserving unknown fields.
func MergeAppUser(appID string, user AppUser) error {
	if appID == "" || user.UserOpenId == "" {
		return fmt.Errorf("invalid account identity")
	}
	return mutateConfigUsers(appID, func(users []AppUser) []AppUser {
		for i := range users {
			if users[i].UserOpenId == user.UserOpenId {
				users[i] = user
				return users
			}
		}
		return append(users, user)
	})
}

// RemoveAppUsers removes only accounts actually logged out, not concurrently added users.
func RemoveAppUsers(appID string, userIDs []string) error {
	return mutateConfigUsers(appID, func(users []AppUser) []AppUser {
		remove := map[string]bool{}
		for _, id := range userIDs {
			remove[id] = true
		}
		out := []AppUser{}
		for _, u := range users {
			if !remove[u.UserOpenId] {
				out = append(out, u)
			}
		}
		return out
	})
}
func mutateConfigUsers(appID string, mutate func([]AppUser) []AppUser) error {
	return withConfigLock(func() error {
		raw, err := os.ReadFile(GetConfigPath())
		if err != nil {
			return err
		}
		var config map[string]json.RawMessage
		if json.Unmarshal(raw, &config) != nil {
			return fmt.Errorf("invalid config format")
		}
		var apps []map[string]json.RawMessage
		if json.Unmarshal(config["apps"], &apps) != nil {
			return fmt.Errorf("invalid apps format")
		}
		found := false
		for _, app := range apps {
			var id string
			_ = json.Unmarshal(app["appId"], &id)
			if id != appID {
				continue
			}
			found = true
			var users []AppUser
			if len(app["users"]) > 0 && string(app["users"]) != "null" {
				if json.Unmarshal(app["users"], &users) != nil {
					return fmt.Errorf("invalid users format")
				}
			}
			var rawUsers []map[string]json.RawMessage
			if len(app["users"]) > 0 {
				_ = json.Unmarshal(app["users"], &rawUsers)
			}
			previous := map[string]map[string]json.RawMessage{}
			for _, record := range rawUsers {
				var id string
				_ = json.Unmarshal(record["userOpenId"], &id)
				previous[id] = record
			}
			updated := []map[string]json.RawMessage{}
			for _, user := range mutate(users) {
				record := previous[user.UserOpenId]
				if record == nil {
					record = map[string]json.RawMessage{}
				}
				record["userOpenId"], _ = json.Marshal(user.UserOpenId)
				record["userName"], _ = json.Marshal(user.UserName)
				updated = append(updated, record)
			}
			app["users"], err = json.Marshal(updated)
			if err != nil {
				return err
			}
			break
		}
		if !found {
			return fmt.Errorf("selected app not found")
		}
		config["apps"], err = json.Marshal(apps)
		if err != nil {
			return err
		}
		raw, err = json.MarshalIndent(config, "", "  ")
		if err != nil {
			return err
		}
		return validate.AtomicWrite(GetConfigPath(), append(raw, '\n'), 0600)
	})
}
