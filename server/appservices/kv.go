package appservices

import (
	"encoding/json"

	"github.com/pkg/errors"

	"github.com/mattermost/mattermost-plugin-apps/server/incoming"
	"github.com/mattermost/mattermost-plugin-apps/utils"
)

func (a *AppServices) KVSet(r *incoming.Request, prefix, id string, data []byte) (bool, error) {
	if err := r.Check(
		r.RequireActingUser,
		r.RequireFromApp,
	); err != nil {
		return false, err
	}
	if !json.Valid(data) {
		return false, utils.NewInvalidError("payload is not valid json")
	}

	return a.store.AppKV.Set(r, prefix, id, data)
}

// KVGet returns the stored KV data for a given user and app.
// If err != nil, the returned data is always valid JSON.
func (a *AppServices) KVGet(r *incoming.Request, prefix, id string) ([]byte, error) {
	if err := r.Check(
		r.RequireActingUser,
		r.RequireFromApp,
	); err != nil {
		return nil, err
	}
	data, err := a.store.AppKV.Get(r, prefix, id)
	if err != nil && !errors.Is(err, utils.ErrNotFound) {
		return nil, err
	}

	if len(data) == 0 {
		// Ensure valid json is returned even if no data is set yet
		data = []byte(string("{}"))
	}

	return data, nil
}

func (a *AppServices) KVDelete(r *incoming.Request, prefix, id string) error {
	if err := r.Check(
		r.RequireActingUser,
		r.RequireFromApp,
	); err != nil {
		return err
	}

	return a.store.AppKV.Delete(r, prefix, id)
}

func (a *AppServices) KVList(r *incoming.Request, prefix string, processf func(key string) error) error {
	if err := r.Check(
		r.RequireActingUser,
		r.RequireFromApp,
	); err != nil {
		return err
	}

	return a.store.AppKV.List(r, prefix, processf)
}
