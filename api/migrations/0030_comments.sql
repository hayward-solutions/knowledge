-- +goose Up
CREATE TABLE comments (
    id uuid PRIMARY KEY,
    page_id uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
    parent_id uuid REFERENCES comments(id) ON DELETE CASCADE,
    author_id uuid NOT NULL REFERENCES users(id),
    body_json jsonb,
    body_html text NOT NULL DEFAULT '',
    inline_anchor jsonb,
    resolved_at timestamptz,
    resolved_by uuid REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    deleted_at timestamptz
);
CREATE INDEX idx_comments_page ON comments(page_id, created_at);
CREATE INDEX idx_comments_parent ON comments(parent_id);

-- +goose Down
DROP TABLE comments;
