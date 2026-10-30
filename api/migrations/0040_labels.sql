-- +goose Up
CREATE TABLE labels (
    id uuid PRIMARY KEY,
    name text NOT NULL UNIQUE CHECK (name = lower(name)),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE page_labels (
    page_id uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
    label_id uuid NOT NULL REFERENCES labels(id) ON DELETE CASCADE,
    PRIMARY KEY (page_id, label_id)
);
CREATE INDEX idx_page_labels_label ON page_labels(label_id);

-- +goose Down
DROP TABLE page_labels;
DROP TABLE labels;
