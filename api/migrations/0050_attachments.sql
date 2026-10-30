-- +goose Up
CREATE TABLE attachments (
    id uuid PRIMARY KEY,
    page_id uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
    space_id uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    uploader_id uuid NOT NULL REFERENCES users(id),
    filename text NOT NULL,
    content_type text NOT NULL DEFAULT 'application/octet-stream',
    size bigint NOT NULL CHECK (size >= 0),
    s3_key text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE INDEX idx_attachments_page ON attachments(page_id, created_at);

-- +goose Down
DROP TABLE attachments;
