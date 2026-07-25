package core

import (
	"fmt"
	"sync"
	"time"
)

type MemoryAssetStore struct {
	mu     sync.Mutex
	assets map[string]Asset
	clock  func() time.Time
}

func NewMemoryAssetStore() *MemoryAssetStore {
	return &MemoryAssetStore{
		assets: make(map[string]Asset),
		clock:  time.Now,
	}
}

func (s *MemoryAssetStore) Save(asset Asset) (string, error) {
	if len(asset.Bytes) == 0 {
		return "", fmt.Errorf("asset bytes 为空")
	}
	if asset.ExpiresAt.IsZero() {
		return "", fmt.Errorf("asset expiresAt 为空")
	}
	key := asset.Key
	if key == "" {
		key = randomKey(16)
		if key == "" {
			return "", fmt.Errorf("生成 asset key 失败")
		}
	}
	asset.Key = key
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assets[key] = asset
	return key, nil
}

func (s *MemoryAssetStore) Get(key string) (Asset, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	asset, ok := s.assets[key]
	if !ok {
		return Asset{}, false
	}
	if !asset.ExpiresAt.IsZero() && s.clock().After(asset.ExpiresAt) {
		delete(s.assets, key)
		return Asset{}, false
	}
	return asset, true
}

func (s *MemoryAssetStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.assets, key)
	return nil
}

