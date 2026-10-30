package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
)

func (s *Store) SetWatch(ctx context.Context, userID uuid.UUID, target domain.WatchTarget, targetID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO watches (user_id, target_type, target_id) VALUES ($1, $2, $3)
		 ON CONFLICT DO NOTHING`, userID, string(target), targetID)
	return err
}

func (s *Store) RemoveWatch(ctx context.Context, userID uuid.UUID, target domain.WatchTarget, targetID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx,
		`DELETE FROM watches WHERE user_id = $1 AND target_type = $2 AND target_id = $3`,
		userID, string(target), targetID)
	return err
}

func (s *Store) IsWatching(ctx context.Context, userID uuid.UUID, target domain.WatchTarget, targetID uuid.UUID) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM watches WHERE user_id = $1 AND target_type = $2 AND target_id = $3)`,
		userID, string(target), targetID).Scan(&ok)
	return ok, err
}

func (s *Store) ListWatches(ctx context.Context, userID uuid.UUID) ([]domain.Watch, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT user_id, target_type, target_id, created_at FROM watches
		 WHERE user_id = $1 ORDER BY created_at DESC, target_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Watch{}
	for rows.Next() {
		var w domain.Watch
		var t string
		if err := rows.Scan(&w.UserID, &t, &w.TargetID, &w.CreatedAt); err != nil {
			return nil, err
		}
		w.TargetType = domain.WatchTarget(t)
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) ListPageWatcherIDs(ctx context.Context, pageID uuid.UUID) ([]uuid.UUID, error) {
	return s.watcherIDs(ctx,
		`SELECT DISTINCT user_id FROM watches
		 WHERE target_type = 'page' AND target_id = $1
		 ORDER BY user_id`, pageID)
}

func (s *Store) ListPublishWatcherIDs(ctx context.Context, pageID, spaceID uuid.UUID) ([]uuid.UUID, error) {
	return s.watcherIDs(ctx,
		`WITH RECURSIVE chain(id, depth) AS (
			SELECT id, 0 FROM pages WHERE id = $1
			UNION
			SELECT p.parent_id, chain.depth + 1 FROM pages p JOIN chain ON p.id = chain.id
			WHERE p.parent_id IS NOT NULL AND chain.depth < 1000
		)
		SELECT DISTINCT user_id FROM watches
		WHERE (target_type = 'page' AND target_id IN (SELECT id FROM chain))
		   OR (target_type = 'space' AND target_id = $2)
		ORDER BY user_id`, pageID, spaceID)
}

func (s *Store) watcherIDs(ctx context.Context, sql string, args ...any) ([]uuid.UUID, error) {
	rows, err := s.Pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (s *Store) FilterNotifiable(ctx context.Context, spaceID uuid.UUID, pageID *uuid.UUID, userIDs []uuid.UUID) ([]uuid.UUID, error) {
	if len(userIDs) == 0 {
		return []uuid.UUID{}, nil
	}
	var page any
	if pageID != nil {
		page = *pageID
	}
	return s.watcherIDs(ctx,
		`SELECT u.id FROM users u
		 JOIN spaces s ON s.id = $1 AND s.deleted_at IS NULL
		 LEFT JOIN space_members m ON m.space_id = s.id AND m.user_id = u.id
		 LEFT JOIN pages p ON p.id = $3::uuid
		 WHERE u.id = ANY($2::uuid[])
		   AND u.disabled_at IS NULL
		   AND (u.is_system_admin OR m.user_id IS NOT NULL OR s.visibility = 'internal')
		   AND (p.id IS NULL OR p.status <> 'draft' OR u.id = p.created_by OR u.is_system_admin OR m.role = 'admin')
		 ORDER BY u.id`, spaceID, userIDs, page)
}

func deletePageSubtreeWatches(ctx context.Context, tx pgx.Tx, pageID uuid.UUID) error {
	_, err := tx.Exec(ctx,
		`WITH RECURSIVE sub(id, depth) AS (
		     SELECT id, 0 FROM pages WHERE id = $1
		     UNION
		     SELECT c.id, sub.depth + 1 FROM pages c JOIN sub ON c.parent_id = sub.id WHERE sub.depth < 1000
		 )
		 DELETE FROM watches WHERE target_type = 'page' AND target_id IN (SELECT id FROM sub)`, pageID)
	return err
}
