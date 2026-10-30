package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
)

type ExportPage struct {
	ID          uuid.UUID
	ParentID    *uuid.UUID
	Title       string
	Slug        string
	Position    int
	ContentJSON json.RawMessage
	ContentHTML string
	UpdatedAt   time.Time
}

func (s *Store) ListSpacePagesForExport(ctx context.Context, spaceID uuid.UUID) ([]ExportPage, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT id, parent_id, title, slug, position, content_json, content_html, updated_at
		 FROM pages
		 WHERE space_id = $1 AND status = 'published'
		 ORDER BY position, created_at, id`, spaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ExportPage{}
	for rows.Next() {
		var p ExportPage
		if err := rows.Scan(&p.ID, &p.ParentID, &p.Title, &p.Slug, &p.Position, &p.ContentJSON, &p.ContentHTML, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

type ImportPage struct {
	Parent      int
	Title       string
	ContentHTML string
	ContentText string
	Message     string
}

func (s *Store) ImportPages(ctx context.Context, spaceID uuid.UUID, baseParent *uuid.UUID, actorID uuid.UUID, in []ImportPage) ([]domain.Page, error) {
	out := make([]domain.Page, 0, len(in))
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		out = out[:0]
		if err := pageLockSpace(ctx, tx, spaceID); err != nil {
			return err
		}
		if baseParent != nil {
			if err := pageCheckParent(ctx, tx, spaceID, *baseParent); err != nil {
				return err
			}
		}
		for _, ip := range in {
			parent := baseParent
			if ip.Parent >= 0 && ip.Parent < len(out) {
				id := out[ip.Parent].ID
				parent = &id
			}
			pos, err := pageNextPosition(ctx, tx, spaceID, parent)
			if err != nil {
				return err
			}
			id := uuid.Must(uuid.NewV7())
			page, err := scanPage(tx.QueryRow(ctx,
				`INSERT INTO pages (id, space_id, parent_id, title, slug, position, status, version,
				                    content_json, content_html, content_text, created_by, updated_by, published_at)
				 VALUES ($1, $2, $3, $4, $5, $6, 'published', 1, NULL, $7, $8, $9, $9, now())
				 RETURNING `+pageCols,
				id, spaceID, parent, ip.Title, SlugFromTitle(ip.Title), pos, ip.ContentHTML, ip.ContentText, actorID))
			if err != nil {
				return err
			}
			if _, err := tx.Exec(ctx,
				`INSERT INTO page_versions (id, page_id, version, title, content_json, content_html, author_id, message)
				 VALUES ($1, $2, 1, $3, NULL, $4, $5, $6)`,
				uuid.Must(uuid.NewV7()), id, ip.Title, ip.ContentHTML, actorID, ip.Message); err != nil {
				return err
			}
			out = append(out, page)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
