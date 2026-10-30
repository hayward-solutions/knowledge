package store

import (
	"context"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
	"github.com/hayward-solutions/knowledge/api/internal/search"
)

type LabelWithCount struct {
	domain.Label
	PageCount int `json:"page_count"`
}

type LabelPageSummary struct {
	ID          uuid.UUID         `json:"id"`
	Title       string            `json:"title"`
	Slug        string            `json:"slug"`
	ParentID    *uuid.UUID        `json:"parent_id"`
	Position    int               `json:"position"`
	Status      domain.PageStatus `json:"status"`
	HasChildren bool              `json:"has_children"`
}

func scanLabelsWithCount(rows pgx.Rows) ([]LabelWithCount, error) {
	defer rows.Close()
	out := []LabelWithCount{}
	for rows.Next() {
		var l LabelWithCount
		if err := rows.Scan(&l.ID, &l.Name, &l.CreatedAt, &l.PageCount); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) ListLabels(ctx context.Context, userID uuid.UUID, isAdmin bool, prefix string, limit int) ([]LabelWithCount, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	pattern := search.EscapeLike(strings.ToLower(strings.TrimSpace(prefix))) + "%"
	rows, err := s.Pool.Query(ctx,
		`SELECT id, name, created_at, page_count FROM (
		     SELECT l.id, l.name, l.created_at,
		            (SELECT count(*) FROM page_labels pl
		             JOIN pages p ON p.id = pl.page_id
		             JOIN spaces s ON s.id = p.space_id
		             WHERE pl.label_id = l.id AND p.status = 'published' AND `+search.AccessClause("s", "$1", "$2")+`)::int AS page_count
		     FROM labels l
		     WHERE l.name LIKE $3
		 ) t
		 WHERE page_count > 0
		 ORDER BY page_count DESC, name
		 LIMIT $4`, userID, isAdmin, pattern, limit)
	if err != nil {
		return nil, err
	}
	return scanLabelsWithCount(rows)
}

func (s *Store) ListSpaceLabels(ctx context.Context, spaceID uuid.UUID) ([]LabelWithCount, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT l.id, l.name, l.created_at, count(*)::int AS page_count
		 FROM labels l
		 JOIN page_labels pl ON pl.label_id = l.id
		 JOIN pages p ON p.id = pl.page_id
		 WHERE p.space_id = $1 AND p.status = 'published'
		 GROUP BY l.id
		 ORDER BY page_count DESC, l.name`, spaceID)
	if err != nil {
		return nil, err
	}
	return scanLabelsWithCount(rows)
}

func (s *Store) ListSpaceLabelPages(ctx context.Context, spaceID uuid.UUID, name string) ([]LabelPageSummary, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT p.id, p.title, p.slug, p.parent_id, p.position, p.status,
		        EXISTS (SELECT 1 FROM pages c WHERE c.parent_id = p.id AND c.status = 'published')
		 FROM pages p
		 JOIN page_labels pl ON pl.page_id = p.id
		 JOIN labels l ON l.id = pl.label_id
		 WHERE p.space_id = $1 AND p.status = 'published' AND l.name = $2
		 ORDER BY lower(p.title), p.title, p.id`, spaceID, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []LabelPageSummary{}
	for rows.Next() {
		var p LabelPageSummary
		var status string
		if err := rows.Scan(&p.ID, &p.Title, &p.Slug, &p.ParentID, &p.Position, &status, &p.HasChildren); err != nil {
			return nil, err
		}
		p.Status = domain.PageStatus(status)
		out = append(out, p)
	}
	return out, rows.Err()
}

func (s *Store) ListPageLabels(ctx context.Context, pageID uuid.UUID) ([]domain.Label, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT l.id, l.name, l.created_at
		 FROM page_labels pl JOIN labels l ON l.id = pl.label_id
		 WHERE pl.page_id = $1
		 ORDER BY l.name`, pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Label{}
	for rows.Next() {
		var l domain.Label
		if err := rows.Scan(&l.ID, &l.Name, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

func (s *Store) ListLabelsForPages(ctx context.Context, pageIDs []uuid.UUID) (map[uuid.UUID][]domain.Label, error) {
	out := map[uuid.UUID][]domain.Label{}
	if len(pageIDs) == 0 {
		return out, nil
	}
	rows, err := s.Pool.Query(ctx,
		`SELECT pl.page_id, l.id, l.name, l.created_at
		 FROM page_labels pl JOIN labels l ON l.id = pl.label_id
		 WHERE pl.page_id = ANY($1::uuid[])
		 ORDER BY l.name`, pageIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var pageID uuid.UUID
		var l domain.Label
		if err := rows.Scan(&pageID, &l.ID, &l.Name, &l.CreatedAt); err != nil {
			return nil, err
		}
		out[pageID] = append(out[pageID], l)
	}
	return out, rows.Err()
}

func (s *Store) GetPageLabelGate(ctx context.Context, pageID uuid.UUID) (domain.PageStatus, uuid.UUID, error) {
	var status string
	var createdBy uuid.UUID
	err := s.Pool.QueryRow(ctx, `SELECT status, created_by FROM pages WHERE id = $1`, pageID).Scan(&status, &createdBy)
	if err != nil {
		return "", uuid.Nil, notFoundIf(err)
	}
	return domain.PageStatus(status), createdBy, nil
}

func (s *Store) SetPageLabels(ctx context.Context, pageID uuid.UUID, names []string) ([]domain.Label, error) {
	sorted := append([]string(nil), names...)
	sort.Strings(sorted)
	var out []domain.Label
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		out = make([]domain.Label, 0, len(sorted))
		ids := make([]uuid.UUID, 0, len(sorted))
		for _, name := range sorted {
			var l domain.Label
			err := tx.QueryRow(ctx,
				`INSERT INTO labels (id, name) VALUES ($1, $2)
				 ON CONFLICT (name) DO UPDATE SET name = EXCLUDED.name
				 RETURNING id, name, created_at`, uuid.Must(uuid.NewV7()), name).
				Scan(&l.ID, &l.Name, &l.CreatedAt)
			if err != nil {
				return err
			}
			out = append(out, l)
			ids = append(ids, l.ID)
		}
		if _, err := tx.Exec(ctx,
			`DELETE FROM page_labels WHERE page_id = $1 AND label_id <> ALL($2::uuid[])`, pageID, ids); err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		_, err := tx.Exec(ctx,
			`INSERT INTO page_labels (page_id, label_id)
			 SELECT $1, unnest($2::uuid[])
			 ON CONFLICT DO NOTHING`, pageID, ids)
		return err
	})
	if IsForeignKeyViolation(err) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return out, nil
}
