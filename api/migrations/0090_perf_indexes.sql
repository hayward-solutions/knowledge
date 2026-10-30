-- +goose Up
CREATE INDEX idx_page_views_page ON page_views(page_id);
CREATE INDEX idx_pages_recent ON pages(updated_at DESC, id DESC) WHERE status = 'published';
CREATE INDEX idx_pages_draft_owner ON pages(created_by) WHERE status = 'draft';
CREATE INDEX idx_notifications_user_created ON notifications(user_id, created_at DESC, id DESC);

-- +goose Down
DROP INDEX idx_notifications_user_created;
DROP INDEX idx_pages_draft_owner;
DROP INDEX idx_pages_recent;
DROP INDEX idx_page_views_page;
