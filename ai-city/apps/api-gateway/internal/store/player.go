// Package store 封装数据访问层
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Player struct {
	ID           string
	Username     string
	Email        string
	PasswordHash string
	DisplayName  string
	AvatarURL    string
	CreatedAt    time.Time
	LastLoginAt  *time.Time
	IsActive     bool
}

var ErrPlayerNotFound = errors.New("player not found")
var ErrInvalidCredentials = errors.New("invalid credentials")

type PlayerStore struct {
	pool *pgxpool.Pool
}

func NewPlayerStore(pool *pgxpool.Pool) *PlayerStore {
	return &PlayerStore{pool: pool}
}

// FindByUsername 查询玩家（按用户名）
func (s *PlayerStore) FindByUsername(ctx context.Context, username string) (*Player, error) {
	const q = `
		SELECT id, username, email, password_hash, display_name,
		       COALESCE(avatar_url, '') AS avatar_url,
		       created_at, last_login_at, is_active
		FROM player
		WHERE username = $1 AND deleted_at IS NULL
		LIMIT 1
	`
	var p Player
	err := s.pool.QueryRow(ctx, q, username).Scan(
		&p.ID, &p.Username, &p.Email, &p.PasswordHash, &p.DisplayName,
		&p.AvatarURL, &p.CreatedAt, &p.LastLoginAt, &p.IsActive,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPlayerNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// FindByID 按 ID 查询
func (s *PlayerStore) FindByID(ctx context.Context, id string) (*Player, error) {
	const q = `
		SELECT id, username, email, password_hash, display_name,
		       COALESCE(avatar_url, '') AS avatar_url,
		       created_at, last_login_at, is_active
		FROM player
		WHERE id = $1 AND deleted_at IS NULL
		LIMIT 1
	`
	var p Player
	err := s.pool.QueryRow(ctx, q, id).Scan(
		&p.ID, &p.Username, &p.Email, &p.PasswordHash, &p.DisplayName,
		&p.AvatarURL, &p.CreatedAt, &p.LastLoginAt, &p.IsActive,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrPlayerNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// UpdateLastLogin 更新最后登录时间
func (s *PlayerStore) UpdateLastLogin(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE player SET last_login_at = NOW() WHERE id = $1`, id)
	return err
}

// CreatePosition 初始化玩家位置（首次登录时）
func (s *PlayerStore) CreatePosition(ctx context.Context, playerID string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO player_position (player_id, tile_id, x, y)
		VALUES ($1, 'tile_0_0', 0, 0)
		ON CONFLICT (player_id) DO NOTHING
	`, playerID)
	return err
}

// UpsertPosition 由 Redis 订阅者调用：把 world-engine 发布的最新位置写入 PG。
// player_id 必须是 PG `player` 表里已存在的 UUID；否则 FK 报错（外键约束）。
func (s *PlayerStore) UpsertPosition(ctx context.Context, playerID, tileID string, x, y float32) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO player_position (player_id, tile_id, x, y, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (player_id) DO UPDATE
		SET tile_id    = EXCLUDED.tile_id,
		    x          = EXCLUDED.x,
		    y          = EXCLUDED.y,
		    updated_at = NOW()
	`, playerID, tileID, x, y)
	return err
}