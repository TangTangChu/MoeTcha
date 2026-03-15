package core

import "time"

type Asset struct {
	Key       string
	Bytes     []byte
	CreatedAt time.Time
	ExpiresAt time.Time
}

type AssetStore interface {
	Save(asset Asset) (string, error)
	Get(key string) (Asset, bool)
	Delete(key string) error
}
