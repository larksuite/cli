package auth

import (
	"encoding/json"
	"reflect"
	"strings"
)

// Preserve opaque fields from newer credential schemas during local mutations.
func (t *StoredUAToken) UnmarshalJSON(data []byte) error {
	type record StoredUAToken
	var decoded record
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(data, &all); err != nil {
		return err
	}
	typ := reflect.TypeOf(decoded)
	for i := 0; i < typ.NumField(); i++ {
		tag := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if tag != "" && tag != "-" {
			delete(all, tag)
		}
	}
	*t = StoredUAToken(decoded)
	t.UnknownFields = all
	return nil
}
func (t StoredUAToken) MarshalJSON() ([]byte, error) {
	type record StoredUAToken
	data, err := json.Marshal(record(t))
	if err != nil {
		return nil, err
	}
	var all map[string]json.RawMessage
	if err = json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	for key, value := range t.UnknownFields {
		if _, exists := all[key]; !exists {
			all[key] = value
		}
	}
	return json.Marshal(all)
}
