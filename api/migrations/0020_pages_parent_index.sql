-- +goose Up
CREATE INDEX idx_pages_parent ON pages(parent_id);

-- +goose Down
DROP INDEX idx_pages_parent;
