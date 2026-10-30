-- +goose Up
CREATE TABLE spaces (
    id uuid PRIMARY KEY,
    key text NOT NULL UNIQUE CHECK (key = upper(key)),
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    icon text,
    visibility text NOT NULL DEFAULT 'internal' CHECK (visibility IN ('private', 'internal')),
    home_page_id uuid,
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    deleted_at timestamptz
);

CREATE TABLE space_members (
    space_id uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role text NOT NULL CHECK (role IN ('admin', 'editor', 'viewer')),
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (space_id, user_id)
);
CREATE INDEX idx_space_members_user ON space_members(user_id);

-- +goose Down
DROP TABLE space_members;
DROP TABLE spaces;
