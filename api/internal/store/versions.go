package store

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
)

type PageVersionSummary struct {
	ID        uuid.UUID       `json:"id"`
	PageID    uuid.UUID       `json:"page_id"`
	Version   int             `json:"version"`
	Title     string          `json:"title"`
	AuthorID  uuid.UUID       `json:"author_id"`
	Author    *domain.UserRef `json:"author"`
	Message   *string         `json:"message"`
	CreatedAt time.Time       `json:"created_at"`
}

type PageVersionView struct {
	domain.PageVersion
	Author *domain.UserRef `json:"author"`
}

func (s *Store) ListPageVersions(ctx context.Context, pageID uuid.UUID) ([]PageVersionSummary, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT v.id, v.page_id, v.version, v.title, v.author_id, u.id, u.display_name, u.email, v.message, v.created_at
		 FROM page_versions v
		 JOIN users u ON u.id = v.author_id
		 WHERE v.page_id = $1
		 ORDER BY v.version DESC`, pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PageVersionSummary{}
	for rows.Next() {
		var v PageVersionSummary
		var a domain.UserRef
		if err := rows.Scan(&v.ID, &v.PageID, &v.Version, &v.Title, &v.AuthorID, &a.ID, &a.DisplayName, &a.Email, &v.Message, &v.CreatedAt); err != nil {
			return nil, err
		}
		v.Author = &a
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) GetPageVersion(ctx context.Context, pageID uuid.UUID, version int) (PageVersionView, error) {
	var v PageVersionView
	var a domain.UserRef
	err := s.Pool.QueryRow(ctx,
		`SELECT v.id, v.page_id, v.version, v.title, v.content_json, v.content_html, v.author_id, v.message, v.created_at,
		        u.id, u.display_name, u.email
		 FROM page_versions v
		 JOIN users u ON u.id = v.author_id
		 WHERE v.page_id = $1 AND v.version = $2`, pageID, version).
		Scan(&v.ID, &v.PageID, &v.Version, &v.Title, &v.ContentJSON, &v.ContentHTML, &v.AuthorID, &v.Message, &v.CreatedAt,
			&a.ID, &a.DisplayName, &a.Email)
	if err != nil {
		return v, notFoundIf(err)
	}
	v.Author = &a
	return v, nil
}
