package core

import (
	"database/sql"
	"fmt"
	"sync"
	"time"
)

type SQLiteAssetStore struct {
	db    *sql.DB
	clock func() time.Time
	mu    sync.Mutex
}

func NewSQLiteAssetStore(db *sql.DB) *SQLiteAssetStore {
	return &SQLiteAssetStore{
		db:    db,
		clock: time.Now,
	}
}

func (s *SQLiteAssetStore) Save(asset Asset) (string, error) {
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

	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO assets (key, bytes, created_at, expires_at) VALUES (?, ?, ?, ?)",
		key, asset.Bytes, asset.CreatedAt.Unix(), asset.ExpiresAt.Unix(),
	)
	if err != nil {
		return "", fmt.Errorf("保存 asset 失败: %w", err)
	}
	return key, nil
}

func (s *SQLiteAssetStore) Get(key string) (Asset, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRow("SELECT key, bytes, created_at, expires_at FROM assets WHERE key = ?", key)
	var a Asset
	var createdUnix, expiresUnix int64
	err := row.Scan(&a.Key, &a.Bytes, &createdUnix, &expiresUnix)
	if err != nil {
		return Asset{}, false
	}
	a.CreatedAt = time.Unix(createdUnix, 0)
	a.ExpiresAt = time.Unix(expiresUnix, 0)

	if !a.ExpiresAt.IsZero() && s.clock().After(a.ExpiresAt) {
		s.db.Exec("DELETE FROM assets WHERE key = ?", key)
		return Asset{}, false
	}
	return a, true
}

func (s *SQLiteAssetStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec("DELETE FROM assets WHERE key = ?", key)
	return err
}
