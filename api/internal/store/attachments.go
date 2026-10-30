package store

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/hayward-solutions/knowledge/api/internal/domain"
)

const attachmentCols = `id, page_id, space_id, uploader_id, filename, content_type, size, s3_key, created_at, deleted_at`

type AttachmentWithUploader struct {
	domain.Attachment
	Uploader domain.UserRef `json:"uploader"`
}

type AttachmentPageMeta struct {
	SpaceID   uuid.UUID
	Status    domain.PageStatus
	CreatedBy uuid.UUID
}

func scanAttachment(row pgx.Row) (domain.Attachment, error) {
	var a domain.Attachment
	err := row.Scan(&a.ID, &a.PageID, &a.SpaceID, &a.UploaderID, &a.Filename, &a.ContentType,
		&a.Size, &a.S3Key, &a.CreatedAt, &a.DeletedAt)
	return a, notFoundIf(err)
}

func (s *Store) CreateAttachment(ctx context.Context, a *domain.Attachment) error {
	return s.Pool.QueryRow(ctx,
		`INSERT INTO attachments (id, page_id, space_id, uploader_id, filename, content_type, size, s3_key)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 RETURNING created_at`,
		a.ID, a.PageID, a.SpaceID, a.UploaderID, a.Filename, a.ContentType, a.Size, a.S3Key).
		Scan(&a.CreatedAt)
}

func (s *Store) GetAttachment(ctx context.Context, id uuid.UUID) (domain.Attachment, error) {
	return scanAttachment(s.Pool.QueryRow(ctx,
		`SELECT `+attachmentCols+` FROM attachments WHERE id = $1 AND deleted_at IS NULL`, id))
}

func (s *Store) GetAttachmentSpaceID(ctx context.Context, id uuid.UUID) (uuid.UUID, error) {
	var spaceID uuid.UUID
	err := s.Pool.QueryRow(ctx,
		`SELECT space_id FROM attachments WHERE id = $1 AND deleted_at IS NULL`, id).Scan(&spaceID)
	return spaceID, notFoundIf(err)
}

func (s *Store) GetAttachmentPageMeta(ctx context.Context, pageID uuid.UUID) (AttachmentPageMeta, error) {
	var m AttachmentPageMeta
	var status string
	err := s.Pool.QueryRow(ctx,
		`SELECT space_id, status, created_by FROM pages WHERE id = $1`, pageID).
		Scan(&m.SpaceID, &status, &m.CreatedBy)
	m.Status = domain.PageStatus(status)
	return m, notFoundIf(err)
}

func (s *Store) ListPageAttachments(ctx context.Context, pageID uuid.UUID) ([]AttachmentWithUploader, error) {
	rows, err := s.Pool.Query(ctx,
		`SELECT a.id, a.page_id, a.space_id, a.uploader_id, a.filename, a.content_type, a.size, a.s3_key,
		        a.created_at, a.deleted_at, u.id, u.display_name, u.email
		 FROM attachments a
		 JOIN users u ON u.id = a.uploader_id
		 WHERE a.page_id = $1 AND a.deleted_at IS NULL
		 ORDER BY a.created_at DESC, a.id DESC`, pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AttachmentWithUploader{}
	for rows.Next() {
		var a AttachmentWithUploader
		if err := rows.Scan(&a.ID, &a.PageID, &a.SpaceID, &a.UploaderID, &a.Filename, &a.ContentType,
			&a.Size, &a.S3Key, &a.CreatedAt, &a.DeletedAt,
			&a.Uploader.ID, &a.Uploader.DisplayName, &a.Uploader.Email); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

func (s *Store) CountPageAttachments(ctx context.Context, pageID uuid.UUID) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx,
		`SELECT count(*) FROM attachments WHERE page_id = $1 AND deleted_at IS NULL`, pageID).Scan(&n)
	return n, err
}

func (s *Store) SoftDeleteAttachment(ctx context.Context, id uuid.UUID) (domain.Attachment, error) {
	return scanAttachment(s.Pool.QueryRow(ctx,
		`UPDATE attachments SET deleted_at = now()
		 WHERE id = $1 AND deleted_at IS NULL
		 RETURNING `+attachmentCols, id))
}

func (s *Store) ListAttachmentObjectKeysUnderPage(ctx context.Context, pageID uuid.UUID) ([]string, error) {
	rows, err := s.Pool.Query(ctx,
		`WITH RECURSIVE tree(id, depth) AS (
		     SELECT id, 0 FROM pages WHERE id = $1
		     UNION
		     SELECT p.id, t.depth + 1 FROM pages p JOIN tree t ON p.parent_id = t.id WHERE t.depth < 1000
		 )
		 SELECT a.s3_key
		 FROM attachments a
		 WHERE a.page_id IN (SELECT id FROM tree) AND a.deleted_at IS NULL
		 ORDER BY a.created_at, a.id`, pageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
