package store

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
)

type SpaceAccess struct {
	ID         uuid.UUID
	Key        string
	Visibility domain.Visibility
	ArchivedAt *time.Time
	DeletedAt  *time.Time
	MemberRole *domain.Role
}

func scanSpaceAccess(row pgx.Row) (SpaceAccess, error) {
	var a SpaceAccess
	var vis string
	var role *string
	err := row.Scan(&a.ID, &a.Key, &vis, &a.ArchivedAt, &a.DeletedAt, &role)
	if err != nil {
		return a, notFoundIf(err)
	}
	a.Visibility = domain.Visibility(vis)
	if role != nil {
		r := domain.Role(*role)
		a.MemberRole = &r
	}
	return a, nil
}

func (s *Store) GetSpaceAccessByKey(ctx context.Context, key string, userID uuid.UUID) (SpaceAccess, error) {
	return scanSpaceAccess(s.Pool.QueryRow(ctx,
		`SELECT s.id, s.key, s.visibility, s.archived_at, s.deleted_at, m.role
		 FROM spaces s
		 LEFT JOIN space_members m ON m.space_id = s.id AND m.user_id = $2
		 WHERE s.key = $1`, key, userID))
}

func (s *Store) GetSpaceAccessByID(ctx context.Context, spaceID, userID uuid.UUID) (SpaceAccess, error) {
	return scanSpaceAccess(s.Pool.QueryRow(ctx,
		`SELECT s.id, s.key, s.visibility, s.archived_at, s.deleted_at, m.role
		 FROM spaces s
		 LEFT JOIN space_members m ON m.space_id = s.id AND m.user_id = $2
		 WHERE s.id = $1`, spaceID, userID))
}

func (s *Store) GetSpaceAccessByPage(ctx context.Context, pageID, userID uuid.UUID) (SpaceAccess, error) {
	return scanSpaceAccess(s.Pool.QueryRow(ctx,
		`SELECT s.id, s.key, s.visibility, s.archived_at, s.deleted_at, m.role
		 FROM pages p
		 JOIN spaces s ON s.id = p.space_id
		 LEFT JOIN space_members m ON m.space_id = s.id AND m.user_id = $2
		 WHERE p.id = $1`, pageID, userID))
}

func (s *Store) GetPageSpaceID(ctx context.Context, pageID uuid.UUID) (uuid.UUID, error) {
	var id uuid.UUID
	err := s.Pool.QueryRow(ctx, `SELECT space_id FROM pages WHERE id = $1`, pageID).Scan(&id)
	return id, notFoundIf(err)
}
