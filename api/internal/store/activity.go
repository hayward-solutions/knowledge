package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
)

type ActivityView struct {
	domain.Activity
	PageTitle *string         `json:"page_title"`
	SpaceKey  *string         `json:"space_key"`
	Actor     *domain.UserRef `json:"actor"`
}

func (s *Store) InsertActivity(ctx context.Context, a domain.Activity) error {
	payload := a.Payload
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO activity (id, space_id, page_id, actor_id, type, payload, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, COALESCE($7, now()))`,
		a.ID, a.SpaceID, a.PageID, a.ActorID, a.Type, []byte(payload), zeroTimeNil(a.CreatedAt))
	return err
}

func (s *Store) ListSpaceActivity(ctx context.Context, spaceID, viewerID uuid.UUID, viewerIsAdmin bool, before *time.Time, limit int) ([]ActivityView, error) {
	if limit <= 0 {
		limit = 30
	}
	var cursor any
	if before != nil {
		cursor = *before
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT a.id, a.space_id, a.page_id, a.actor_id, a.type, a.payload, a.created_at,
		        COALESCE(a.payload->>'page_title', a.payload->>'title'), sp.key,
		        u.id, u.display_name, u.email
		 FROM activity a
		 LEFT JOIN users u ON u.id = a.actor_id
		 LEFT JOIN spaces sp ON sp.id = a.space_id
		 LEFT JOIN pages p ON p.id = a.page_id
		 WHERE a.space_id = $1
		   AND ($2::timestamptz IS NULL OR a.created_at < $2::timestamptz)
		   AND (p.id IS NULL OR p.status <> 'draft' OR p.created_by = $3 OR $4::boolean)
		 ORDER BY a.created_at DESC, a.id DESC
		 LIMIT $5`, spaceID, cursor, viewerID, viewerIsAdmin, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ActivityView{}
	for rows.Next() {
		var v ActivityView
		var uid *uuid.UUID
		var name, email *string
		if err := rows.Scan(&v.ID, &v.SpaceID, &v.PageID, &v.ActorID, &v.Type, &v.Payload, &v.CreatedAt,
			&v.PageTitle, &v.SpaceKey, &uid, &name, &email); err != nil {
			return nil, err
		}
		v.CreatedAt = v.CreatedAt.UTC()
		if len(v.Payload) == 0 {
			v.Payload = json.RawMessage(`{}`)
		}
		if uid != nil && name != nil && email != nil {
			v.Actor = &domain.UserRef{ID: *uid, DisplayName: *name, Email: *email}
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
