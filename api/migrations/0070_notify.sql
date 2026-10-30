-- +goose Up
CREATE TABLE watches (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_type text NOT NULL CHECK (target_type IN ('page', 'space')),
    target_id uuid NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, target_type, target_id)
);
CREATE INDEX idx_watches_target ON watches(target_type, target_id);

CREATE TABLE notifications (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type text NOT NULL,
    actor_id uuid NOT NULL REFERENCES users(id),
    space_id uuid,
    page_id uuid,
    comment_id uuid,
    payload jsonb NOT NULL DEFAULT '{}',
    read_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_notifications_user ON notifications(user_id, read_at, created_at DESC);

CREATE TABLE activity (
    id uuid PRIMARY KEY,
    space_id uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    page_id uuid,
    actor_id uuid NOT NULL REFERENCES users(id),
    type text NOT NULL,
    payload jsonb NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_activity_space ON activity(space_id, created_at DESC);

-- +goose Down
DROP TABLE activity;
DROP TABLE notifications;
DROP TABLE watches;
