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
	return TemplateContent{}, ErrNotFound
}
