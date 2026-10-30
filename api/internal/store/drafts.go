package store

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
)

const draftCols = `page_id, user_id, title, content_json, content_html, base_version, updated_at`

func scanDraft(row pgx.Row) (domain.PageDraft, error) {
	var d domain.PageDraft
	err := row.Scan(&d.PageID, &d.UserID, &d.Title, &d.ContentJSON, &d.ContentHTML, &d.BaseVersion, &d.UpdatedAt)
	return d, notFoundIf(err)
}

type DraftParams struct {
	PageID      uuid.UUID
	UserID      uuid.UUID
	Title       string
	ContentJSON json.RawMessage
	ContentHTML string
	BaseVersion int
}

func (s *Store) UpsertDraft(ctx context.Context, p DraftParams) (domain.PageDraft, error) {
	return scanDraft(s.Pool.QueryRow(ctx,
		`INSERT INTO page_drafts (page_id, user_id, title, content_json, content_html, base_version, updated_at)
		 VALUES ($1, $2, $3, $4::jsonb, $5, $6, now())
		 ON CONFLICT (page_id, user_id) DO UPDATE SET
		     title = EXCLUDED.title,
		     content_json = EXCLUDED.content_json,
		     content_html = EXCLUDED.content_html,
		     base_version = EXCLUDED.base_version,
		     updated_at = EXCLUDED.updated_at
		 RETURNING `+draftCols,
		p.PageID, p.UserID, p.Title, pageJSONArg(p.ContentJSON), p.ContentHTML, p.BaseVersion))
}

func (s *Store) GetDraft(ctx context.Context, pageID, userID uuid.UUID) (domain.PageDraft, error) {
	return scanDraft(s.Pool.QueryRow(ctx,
		`SELECT `+draftCols+` FROM page_drafts WHERE page_id = $1 AND user_id = $2`, pageID, userID))
}

func (s *Store) GetDraftRef(ctx context.Context, pageID, userID uuid.UUID) (*DraftRef, error) {
	var ref DraftRef
	err := s.Pool.QueryRow(ctx,
		`SELECT updated_at, base_version FROM page_drafts WHERE page_id = $1 AND user_id = $2`, pageID, userID).
		Scan(&ref.UpdatedAt, &ref.BaseVersion)
	if err != nil {
		if notFoundIf(err) == ErrNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &ref, nil
}

func (s *Store) DeleteDraft(ctx context.Context, pageID, userID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM page_drafts WHERE page_id = $1 AND user_id = $2`, pageID, userID)
	return err
}
