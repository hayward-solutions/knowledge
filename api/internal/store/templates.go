package store

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
)

const (
	templateCols     = `t.id, t.space_id, s.key, t.name, t.description, t.content_json, t.content_html, t.created_by, t.created_at, t.updated_at`
	templateFrom     = `FROM templates t LEFT JOIN spaces s ON s.id = t.space_id`
	templateSeedLock = 6006
)

type TemplateView struct {
	domain.Template
	SpaceKey *string `json:"space_key"`
}

type NewTemplate struct {
	SpaceID     *uuid.UUID
	Name        string
	Description string
	ContentJSON json.RawMessage
	ContentHTML string
	CreatedBy   uuid.UUID
}

type TemplatePatch struct {
	Name           *string
	Description    *string
	ContentJSON    json.RawMessage
	SetContentJSON bool
	ContentHTML    *string
}

type TemplateSeed struct {
	Name        string
	Description string
	ContentJSON json.RawMessage
	ContentHTML string
}

func templateJSONArg(raw json.RawMessage) any {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	return trimmed
}

func templateScan(row pgx.Row) (TemplateView, error) {
	var v TemplateView
	err := row.Scan(&v.ID, &v.SpaceID, &v.SpaceKey, &v.Name, &v.Description, &v.ContentJSON, &v.ContentHTML,
		&v.CreatedBy, &v.CreatedAt, &v.UpdatedAt)
	if err != nil {
		return v, notFoundIf(err)
	}
	return v, nil
}

func templateScanAll(rows pgx.Rows) ([]TemplateView, error) {
	defer rows.Close()
	out := []TemplateView{}
	for rows.Next() {
		v, err := templateScan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) CreateTemplate(ctx context.Context, in NewTemplate) (TemplateView, error) {
	id := uuid.Must(uuid.NewV7())
	_, err := s.Pool.Exec(ctx,
		`INSERT INTO templates (id, space_id, name, description, content_json, content_html, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		id, in.SpaceID, in.Name, in.Description, templateJSONArg(in.ContentJSON), in.ContentHTML, in.CreatedBy)
	if err != nil {
		return TemplateView{}, err
	}
	return s.GetTemplate(ctx, id)
}

func (s *Store) GetTemplate(ctx context.Context, id uuid.UUID) (TemplateView, error) {
	return templateScan(s.Pool.QueryRow(ctx,
		`SELECT `+templateCols+` `+templateFrom+` WHERE t.id = $1`, id))
}

func (s *Store) UpdateTemplate(ctx context.Context, id uuid.UUID, p TemplatePatch) (TemplateView, error) {
	var jsonArg any
	if p.SetContentJSON {
		jsonArg = templateJSONArg(p.ContentJSON)
	}
	tag, err := s.Pool.Exec(ctx,
		`UPDATE templates SET
		     name = COALESCE($2, name),
		     description = COALESCE($3, description),
		     content_json = CASE WHEN $4::boolean THEN $5::jsonb ELSE content_json END,
		     content_html = COALESCE($6, content_html),
		     updated_at = now()
		 WHERE id = $1`,
		id, p.Name, p.Description, p.SetContentJSON, jsonArg, p.ContentHTML)
	if err != nil {
		return TemplateView{}, err
	}
	if tag.RowsAffected() == 0 {
		return TemplateView{}, ErrNotFound
	}
	return s.GetTemplate(ctx, id)
}

func (s *Store) DeleteTemplate(ctx context.Context, id uuid.UUID) error {
	tag, err := s.Pool.Exec(ctx, `DELETE FROM templates WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) ListSpaceTemplates(ctx context.Context, spaceID uuid.UUID) ([]TemplateView, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+templateCols+` `+templateFrom+` WHERE t.space_id = $1 ORDER BY lower(t.name), t.created_at`, spaceID)
	if err != nil {
		return nil, err
	}
	return templateScanAll(rows)
}

func (s *Store) ListVisibleTemplates(ctx context.Context, userID uuid.UUID, isSystemAdmin bool) ([]TemplateView, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT `+templateCols+` `+templateFrom+`
		 WHERE t.space_id IS NULL
		    OR (s.deleted_at IS NULL AND (
		         $2::boolean
		         OR EXISTS (SELECT 1 FROM space_members m
		                    WHERE m.space_id = t.space_id AND m.user_id = $1 AND m.role IN ('editor', 'admin'))))
		 ORDER BY (t.space_id IS NOT NULL), lower(t.name), t.created_at`, userID, isSystemAdmin)
	if err != nil {
		return nil, err
	}
	return templateScanAll(rows)
}

func (s *Store) SeedTemplatesIfEmpty(ctx context.Context, createdBy uuid.UUID, seeds []TemplateSeed) (bool, error) {
	var existing int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM templates`).Scan(&existing); err != nil {
		return false, err
	}
	if existing > 0 {
		return false, nil
	}
	seeded := false
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, templateSeedLock); err != nil {
			return err
		}
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM templates`).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			return nil
		}
		for _, sd := range seeds {
			if _, err := tx.Exec(ctx,
				`INSERT INTO templates (id, space_id, name, description, content_json, content_html, created_by)
				 VALUES ($1, NULL, $2, $3, $4, $5, $6)`,
				uuid.Must(uuid.NewV7()), sd.Name, sd.Description, templateJSONArg(sd.ContentJSON), sd.ContentHTML, createdBy); err != nil {
				return err
			}
		}
		seeded = true
		return nil
	})
	return seeded, err
}
