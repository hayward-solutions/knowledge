-- +goose Up
CREATE TABLE templates (
    id uuid PRIMARY KEY,
    space_id uuid REFERENCES spaces(id) ON DELETE CASCADE,
    name text NOT NULL,
    description text NOT NULL DEFAULT '',
    content_json jsonb,
    content_html text NOT NULL DEFAULT '',
    created_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_templates_space_name ON templates(space_id, name);

-- +goose Down
DROP TABLE templates;
