package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
)

var (
	ErrInvalidParent = errors.New("parent must be a non-trashed page in the same space")
	ErrPageCycle     = errors.New("a page cannot be moved under itself or one of its descendants")
	ErrInvalidAnchor = errors.New("after_id must be a sibling under the target parent")
	ErrHomePage      = errors.New("the space home page cannot be trashed")
	ErrPageTrashed   = errors.New("page is trashed")
	ErrNotTrashed    = errors.New("page is not trashed")
)

type VersionConflictError struct {
	Current int
}

func (e *VersionConflictError) Error() string { return "page was updated by someone else" }

const PageEmptyDocJSON = `{"type":"doc","content":[{"type":"paragraph"}]}`

const pageCols = `id, space_id, parent_id, title, slug, position, status, version, content_json, content_html, content_text,
	created_by, updated_by, created_at, updated_at, published_at, trashed_at, trashed_by`

const pageSummaryCols = `p.id, p.title, p.slug, p.parent_id, p.position, p.status,
	EXISTS (SELECT 1 FROM pages c WHERE c.parent_id = p.id AND c.status <> 'trashed')`

type PageSummary struct {
	ID          uuid.UUID         `json:"id"`
	Title       string            `json:"title"`
	Slug        string            `json:"slug"`
	ParentID    *uuid.UUID        `json:"parent_id"`
	Position    int               `json:"position"`
	Status      domain.PageStatus `json:"status"`
	HasChildren bool              `json:"has_children"`
}

type DraftRef struct {
	UpdatedAt   time.Time `json:"updated_at"`
	BaseVersion int       `json:"base_version"`
}

func scanPage(row pgx.Row) (domain.Page, error) {
	var p domain.Page
	var status string
	err := row.Scan(&p.ID, &p.SpaceID, &p.ParentID, &p.Title, &p.Slug, &p.Position, &status, &p.Version,
		&p.ContentJSON, &p.ContentHTML, &p.ContentText, &p.CreatedBy, &p.UpdatedBy, &p.CreatedAt, &p.UpdatedAt,
		&p.PublishedAt, &p.TrashedAt, &p.TrashedBy)
	if err != nil {
		return p, notFoundIf(err)
	}
	p.Status = domain.PageStatus(status)
	return p, nil
}

func scanPageSummary(row pgx.Row) (PageSummary, error) {
	var p PageSummary
	var status string
	err := row.Scan(&p.ID, &p.Title, &p.Slug, &p.ParentID, &p.Position, &status, &p.HasChildren)
	if err != nil {
		return p, notFoundIf(err)
	}
	p.Status = domain.PageStatus(status)
	return p, nil
}

func pageSummaries(rows pgx.Rows) ([]PageSummary, error) {
	defer rows.Close()
	out := []PageSummary{}
	for rows.Next() {
		p, err := scanPageSummary(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

func pageJSONArg(raw json.RawMessage) any {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	return trimmed
}

func IsPageVisible(page domain.Page, userID uuid.UUID, role domain.Role) bool {
	creatorOrAdmin := page.CreatedBy == userID || role == domain.RoleAdmin
	switch page.Status {
	case domain.PageStatusDraft:
		return creatorOrAdmin
	case domain.PageStatusTrashed:
		if page.Version == 0 && page.PublishedAt == nil {
			return creatorOrAdmin
		}
		return role.AtLeast(domain.RoleEditor)
	}
	return true
}

func (s *Store) GetPage(ctx context.Context, id uuid.UUID) (domain.Page, error) {
	return scanPage(s.Pool.QueryRow(ctx, `SELECT `+pageCols+` FROM pages WHERE id = $1`, id))
}

func (s *Store) GetPageSummary(ctx context.Context, id uuid.UUID) (PageSummary, error) {
	return scanPageSummary(s.Pool.QueryRow(ctx, `SELECT `+pageSummaryCols+` FROM pages p WHERE p.id = $1`, id))
}

func (s *Store) ListAncestors(ctx context.Context, id uuid.UUID) ([]PageSummary, error) {
	return s.listAncestors(ctx, id, nil, domain.RoleAdmin)
}

func (s *Store) ListVisibleAncestors(ctx context.Context, id, viewerID uuid.UUID, role domain.Role) ([]PageSummary, error) {
	return s.listAncestors(ctx, id, &viewerID, role)
}

func (s *Store) listAncestors(ctx context.Context, id uuid.UUID, viewerID *uuid.UUID, role domain.Role) ([]PageSummary, error) {
	rows, err := s.Pool.Query(ctx,
		`WITH RECURSIVE anc AS (
		     SELECT parent_id AS id, 1 AS depth FROM pages WHERE id = $1 AND parent_id IS NOT NULL
		     UNION ALL
		     SELECT p.parent_id, a.depth + 1 FROM pages p JOIN anc a ON p.id = a.id
		     WHERE p.parent_id IS NOT NULL AND a.depth < 1000
		 )
		 SELECT `+pageSummaryCols+`
		 FROM anc JOIN pages p ON p.id = anc.id
		 WHERE $2::uuid IS NULL OR CASE
		     WHEN p.status = 'draft' THEN p.created_by = $2 OR $3
		     WHEN p.status = 'trashed' AND p.version = 0 AND p.published_at IS NULL THEN p.created_by = $2 OR $3
		     WHEN p.status = 'trashed' THEN $3 OR $4
		     ELSE true
		 END
		 ORDER BY anc.depth DESC`,
		id, viewerID, role == domain.RoleAdmin, role.AtLeast(domain.RoleEditor))
	if err != nil {
		return nil, err
	}
	return pageSummaries(rows)
}

func (s *Store) ListChildren(ctx context.Context, parentID, viewerID uuid.UUID, viewerIsAdmin bool) ([]PageSummary, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+pageSummaryCols+`
		 FROM pages p
		 WHERE p.parent_id = $1
		   AND p.status <> 'trashed'
		   AND (p.status <> 'draft' OR p.created_by = $2 OR $3)
		 ORDER BY p.position, p.created_at, p.id`, parentID, viewerID, viewerIsAdmin)
	if err != nil {
		return nil, err
	}
	return pageSummaries(rows)
}

func (s *Store) PageSpaceHomeID(ctx context.Context, spaceID uuid.UUID) (*uuid.UUID, error) {
	var id *uuid.UUID
	err := s.Pool.QueryRow(ctx, `SELECT home_page_id FROM spaces WHERE id = $1`, spaceID).Scan(&id)
	return id, notFoundIf(err)
}

func pageLockSpace(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text, 0))`, spaceID.String())
	return err
}

func pageCheckParent(ctx context.Context, tx pgx.Tx, spaceID, parentID uuid.UUID) error {
	var status string
	err := tx.QueryRow(ctx, `SELECT status FROM pages WHERE id = $1 AND space_id = $2`, parentID, spaceID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvalidParent
	}
	if err != nil {
		return err
	}
	if status == string(domain.PageStatusTrashed) {
		return ErrInvalidParent
	}
	return nil
}

func pageNextPosition(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID, parentID *uuid.UUID) (int, error) {
	var pos int
	err := tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(position), -1) + 1 FROM pages
		 WHERE space_id = $1 AND parent_id IS NOT DISTINCT FROM $2 AND status <> 'trashed'`,
		spaceID, parentID).Scan(&pos)
	return pos, err
}

func pageRenumber(ctx context.Context, tx pgx.Tx, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	positions := make([]int32, len(ids))
	for i := range ids {
		positions[i] = int32(i)
	}
	_, err := tx.Exec(ctx,
		`UPDATE pages p SET position = v.pos
		 FROM unnest($1::uuid[], $2::int[]) AS v(id, pos)
		 WHERE p.id = v.id AND p.position <> v.pos`, ids, positions)
	return err
}

func pageSiblingIDs(ctx context.Context, tx pgx.Tx, spaceID uuid.UUID, parentID *uuid.UUID, exclude uuid.UUID) ([]uuid.UUID, error) {
	rows, err := tx.Query(ctx,
		`SELECT id FROM pages
		 WHERE space_id = $1 AND parent_id IS NOT DISTINCT FROM $2 AND status <> 'trashed' AND id <> $3
		 ORDER BY position, created_at, id`, spaceID, parentID, exclude)
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

type NewPage struct {
	SpaceID     uuid.UUID
	ParentID    *uuid.UUID
	Title       string
	ContentJSON json.RawMessage
	ContentHTML string
	ContentText string
	CreatedBy   uuid.UUID
}

func (s *Store) CreatePage(ctx context.Context, in NewPage) (domain.Page, error) {
	id := uuid.Must(uuid.NewV7())
	var out domain.Page
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := pageLockSpace(ctx, tx, in.SpaceID); err != nil {
			return err
		}
		if in.ParentID != nil {
			if err := pageCheckParent(ctx, tx, in.SpaceID, *in.ParentID); err != nil {
				return err
			}
		}
		pos, err := pageNextPosition(ctx, tx, in.SpaceID, in.ParentID)
		if err != nil {
			return err
		}
		out, err = scanPage(tx.QueryRow(ctx,
			`INSERT INTO pages (id, space_id, parent_id, title, slug, position, status, version,
			                    content_json, content_html, content_text, created_by, updated_by)
			 VALUES ($1, $2, $3, $4, $5, $6, 'draft', 0, $7::jsonb, $8, $9, $10, $10)
			 RETURNING `+pageCols,
			id, in.SpaceID, in.ParentID, in.Title, SlugFromTitle(in.Title), pos,
			pageJSONArg(in.ContentJSON), in.ContentHTML, in.ContentText, in.CreatedBy))
		return err
	})
	return out, err
}

func (s *Store) UpdatePageTitle(ctx context.Context, id, actorID uuid.UUID, title string) (domain.Page, error) {
	p, err := scanPage(s.Pool.QueryRow(ctx,
		`UPDATE pages SET title = $2, slug = $3, updated_by = $4, updated_at = now()
		 WHERE id = $1 AND status <> 'trashed'
		 RETURNING `+pageCols, id, title, SlugFromTitle(title), actorID))
	if errors.Is(err, ErrNotFound) {
		if _, gerr := s.GetPage(ctx, id); gerr == nil {
			return p, ErrPageTrashed
		}
	}
	return p, err
}

type PublishParams struct {
	PageID      uuid.UUID
	ActorID     uuid.UUID
	Title       string
	ContentJSON json.RawMessage
	ContentHTML string
	ContentText string
	BaseVersion int
	Force       bool
	Message     *string
}

func (s *Store) PublishPage(ctx context.Context, p PublishParams) (domain.Page, error) {
	var out domain.Page
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		cur, err := scanPage(tx.QueryRow(ctx, `SELECT `+pageCols+` FROM pages WHERE id = $1 FOR UPDATE`, p.PageID))
		if err != nil {
			return err
		}
		if cur.Status == domain.PageStatusTrashed {
			return ErrPageTrashed
		}
		if cur.Version != p.BaseVersion && !p.Force {
			return &VersionConflictError{Current: cur.Version}
		}
		next := cur.Version + 1
		out, err = scanPage(tx.QueryRow(ctx,
			`UPDATE pages SET title = $2, slug = $3, version = $4, status = 'published',
			        content_json = $5::jsonb, content_html = $6, content_text = $7,
			        published_at = COALESCE(published_at, now()), updated_at = now(), updated_by = $8
			 WHERE id = $1
			 RETURNING `+pageCols,
			p.PageID, p.Title, SlugFromTitle(p.Title), next, pageJSONArg(p.ContentJSON), p.ContentHTML, p.ContentText, p.ActorID))
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`INSERT INTO page_versions (id, page_id, version, title, content_json, content_html, author_id, message)
			 VALUES ($1, $2, $3, $4, $5::jsonb, $6, $7, $8)`,
			uuid.Must(uuid.NewV7()), p.PageID, next, p.Title, pageJSONArg(p.ContentJSON), p.ContentHTML, p.ActorID, p.Message); err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `DELETE FROM page_drafts WHERE page_id = $1 AND user_id = $2`, p.PageID, p.ActorID)
		return err
	})
	return out, err
}

type MoveParams struct {
	PageID   uuid.UUID
	ParentID *uuid.UUID
	AfterID  *uuid.UUID
	Position *int
}

type MoveResult struct {
	Page        domain.Page
	OldParentID *uuid.UUID
}

func (s *Store) MovePage(ctx context.Context, p MoveParams) (MoveResult, error) {
	var res MoveResult
	spaceID, err := s.GetPageSpaceID(ctx, p.PageID)
	if err != nil {
		return res, err
	}
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := pageLockSpace(ctx, tx, spaceID); err != nil {
			return err
		}
		cur, err := scanPage(tx.QueryRow(ctx, `SELECT `+pageCols+` FROM pages WHERE id = $1 FOR UPDATE`, p.PageID))
		if err != nil {
			return err
		}
		if cur.Status == domain.PageStatusTrashed {
			return ErrPageTrashed
		}
		res.OldParentID = cur.ParentID
		if p.ParentID != nil {
			if *p.ParentID == p.PageID {
				return ErrPageCycle
			}
			if err := pageCheckParent(ctx, tx, cur.SpaceID, *p.ParentID); err != nil {
				return err
			}
			var cycle bool
			if err := tx.QueryRow(ctx,
				`WITH RECURSIVE anc AS (
				     SELECT id, parent_id, 0 AS depth FROM pages WHERE id = $1
				     UNION ALL
				     SELECT p.id, p.parent_id, a.depth + 1 FROM pages p JOIN anc a ON p.id = a.parent_id
				     WHERE a.depth < 1000
				 )
				 SELECT EXISTS (SELECT 1 FROM anc WHERE id = $2)`, *p.ParentID, p.PageID).Scan(&cycle); err != nil {
				return err
			}
			if cycle {
				return ErrPageCycle
			}
		}
		siblings, err := pageSiblingIDs(ctx, tx, cur.SpaceID, p.ParentID, p.PageID)
		if err != nil {
			return err
		}
		idx := len(siblings)
		switch {
		case p.AfterID != nil:
			idx = -1
			for i, id := range siblings {
				if id == *p.AfterID {
					idx = i + 1
					break
				}
			}
			if idx < 0 {
				return ErrInvalidAnchor
			}
		case p.Position != nil:
			idx = *p.Position
			if idx < 0 {
				idx = 0
			}
			if idx > len(siblings) {
				idx = len(siblings)
			}
		}
		ordered := make([]uuid.UUID, 0, len(siblings)+1)
		ordered = append(ordered, siblings[:idx]...)
		ordered = append(ordered, p.PageID)
		ordered = append(ordered, siblings[idx:]...)
		if _, err := tx.Exec(ctx, `UPDATE pages SET parent_id = $2 WHERE id = $1`, p.PageID, p.ParentID); err != nil {
			return err
		}
		if err := pageRenumber(ctx, tx, ordered); err != nil {
			return err
		}
		if !pageSameParent(cur.ParentID, p.ParentID) {
			old, err := pageSiblingIDs(ctx, tx, cur.SpaceID, cur.ParentID, p.PageID)
			if err != nil {
				return err
			}
			if err := pageRenumber(ctx, tx, old); err != nil {
				return err
			}
		}
		res.Page, err = scanPage(tx.QueryRow(ctx, `SELECT `+pageCols+` FROM pages WHERE id = $1`, p.PageID))
		return err
	})
	return res, err
}

func pageSameParent(a, b *uuid.UUID) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func (s *Store) TrashPage(ctx context.Context, id, actorID uuid.UUID) error {
	spaceID, err := s.GetPageSpaceID(ctx, id)
	if err != nil {
		return err
	}
	return s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := pageLockSpace(ctx, tx, spaceID); err != nil {
			return err
		}
		cur, err := scanPage(tx.QueryRow(ctx, `SELECT `+pageCols+` FROM pages WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if cur.Status == domain.PageStatusTrashed {
			return ErrPageTrashed
		}
		var home *uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT home_page_id FROM spaces WHERE id = $1`, cur.SpaceID).Scan(&home); err != nil {
			return notFoundIf(err)
		}
		if home != nil && *home == id {
			return ErrHomePage
		}
		_, err = tx.Exec(ctx,
			`WITH RECURSIVE sub AS (
			     SELECT id FROM pages WHERE id = $1
			     UNION ALL
			     SELECT c.id FROM pages c JOIN sub ON c.parent_id = sub.id WHERE c.status <> 'trashed'
			 )
			 UPDATE pages SET status = 'trashed', trashed_at = now(), trashed_by = $2
			 WHERE id IN (SELECT id FROM sub)`, id, actorID)
		return err
	})
}

func (s *Store) RestorePage(ctx context.Context, id uuid.UUID) (domain.Page, error) {
	var out domain.Page
	spaceID, err := s.GetPageSpaceID(ctx, id)
	if err != nil {
		return out, err
	}
	err = s.WithTx(ctx, func(tx pgx.Tx) error {
		if err := pageLockSpace(ctx, tx, spaceID); err != nil {
			return err
		}
		cur, err := scanPage(tx.QueryRow(ctx, `SELECT `+pageCols+` FROM pages WHERE id = $1 FOR UPDATE`, id))
		if err != nil {
			return err
		}
		if cur.Status != domain.PageStatusTrashed {
			return ErrNotTrashed
		}
		parent := cur.ParentID
		if parent != nil {
			var status string
			err := tx.QueryRow(ctx, `SELECT status FROM pages WHERE id = $1`, *parent).Scan(&status)
			detached := false
			switch {
			case errors.Is(err, pgx.ErrNoRows):
				detached = true
			case err != nil:
				return err
			case status == string(domain.PageStatusTrashed):
				detached = true
			}
			if detached {
				parent = nil
				var home *uuid.UUID
				if err := tx.QueryRow(ctx, `SELECT home_page_id FROM spaces WHERE id = $1`, cur.SpaceID).Scan(&home); err != nil {
					return notFoundIf(err)
				}
				if home != nil && *home != id {
					err := tx.QueryRow(ctx, `SELECT status FROM pages WHERE id = $1`, *home).Scan(&status)
					switch {
					case errors.Is(err, pgx.ErrNoRows):
					case err != nil:
						return err
					case status != string(domain.PageStatusTrashed):
						parent = home
					}
				}
			}
		}
		pos, err := pageNextPosition(ctx, tx, cur.SpaceID, parent)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx,
			`WITH RECURSIVE sub AS (
			     SELECT id FROM pages WHERE id = $1
			     UNION ALL
			     SELECT c.id FROM pages c JOIN sub ON c.parent_id = sub.id
			     WHERE c.status = 'trashed' AND c.trashed_at = $2
			 )
			 UPDATE pages SET status = CASE WHEN version = 0 THEN 'draft' ELSE 'published' END,
			        trashed_at = NULL, trashed_by = NULL
			 WHERE id IN (SELECT id FROM sub)`, id, cur.TrashedAt); err != nil {
			return err
		}
		out, err = scanPage(tx.QueryRow(ctx,
			`UPDATE pages SET parent_id = $2, position = $3 WHERE id = $1 RETURNING `+pageCols, id, parent, pos))
		return err
	})
	return out, err
}

func (s *Store) PurgePage(ctx context.Context, id uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM pages WHERE id = $1 AND status = 'trashed'`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		if _, gerr := s.GetPage(ctx, id); gerr != nil {
			return gerr
		}
		return ErrNotTrashed
	}
	return nil
}

func (s *Store) RecordPageView(ctx context.Context, userID, pageID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO page_views (user_id, page_id, viewed_at) VALUES ($1, $2, now())
		 ON CONFLICT (user_id, page_id) DO UPDATE SET viewed_at = EXCLUDED.viewed_at`, userID, pageID)
	return err
}

func (s *Store) StarPage(ctx context.Context, userID, pageID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO page_stars (user_id, page_id) VALUES ($1, $2) ON CONFLICT (user_id, page_id) DO NOTHING`,
		userID, pageID)
	return err
}

func (s *Store) UnstarPage(ctx context.Context, userID, pageID uuid.UUID) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM page_stars WHERE user_id = $1 AND page_id = $2`, userID, pageID)
	return err
}

func (s *Store) IsPageStarred(ctx context.Context, userID, pageID uuid.UUID) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM page_stars WHERE user_id = $1 AND page_id = $2)`, userID, pageID).Scan(&ok)
	return ok, err
}
