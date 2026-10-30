package store

import (
	"context"

	"github.com/google/uuid"
)

type TemplateContent struct {
	SpaceID     uuid.UUID
	Title       string
	ContentJSON []byte
	ContentHTML string
}

func (s *Store) GetTemplateContent(ctx context.Context, id uuid.UUID) (TemplateContent, error) {
	var spaceID *uuid.UUID
	var c TemplateContent
	err := s.Pool.QueryRow(ctx,
		`SELECT space_id, name, content_json, content_html FROM templates WHERE id = $1`, id).
		Scan(&spaceID, &c.Title, &c.ContentJSON, &c.ContentHTML)
	if err != nil {
		return TemplateContent{}, notFoundIf(err)
	}
	if spaceID != nil {
		c.SpaceID = *spaceID
	}
	return c, nil
}
