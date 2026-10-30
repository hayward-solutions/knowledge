-- +goose Up
CREATE TABLE pages (
    id uuid PRIMARY KEY,
    space_id uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    parent_id uuid REFERENCES pages(id) ON DELETE CASCADE,
    title text NOT NULL DEFAULT '',
    slug text NOT NULL DEFAULT '',
    position integer NOT NULL DEFAULT 0,
    status text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'published', 'trashed')),
    version integer NOT NULL DEFAULT 0,
    content_json jsonb,
    content_html text NOT NULL DEFAULT '',
    content_text text NOT NULL DEFAULT '',
    search_vector tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(content_text, '')), 'B')
    ) STORED,
    created_by uuid NOT NULL REFERENCES users(id),
    updated_by uuid NOT NULL REFERENCES users(id),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    trashed_at timestamptz,
    trashed_by uuid REFERENCES users(id)
);
CREATE INDEX idx_pages_tree ON pages(space_id, parent_id, position);
CREATE INDEX idx_pages_search ON pages USING gin(search_vector);
CREATE INDEX idx_pages_space_status ON pages(space_id, status);

CREATE TABLE page_versions (
    id uuid PRIMARY KEY,
    page_id uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
    version integer NOT NULL,
    title text NOT NULL,
    content_json jsonb,
    content_html text NOT NULL DEFAULT '',
    author_id uuid NOT NULL REFERENCES users(id),
    message text,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (page_id, version)
);

CREATE TABLE page_drafts (
    page_id uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title text NOT NULL DEFAULT '',
    content_json jsonb,
    content_html text NOT NULL DEFAULT '',
    base_version integer NOT NULL DEFAULT 0,
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (page_id, user_id)
);
CREATE INDEX idx_page_drafts_user ON page_drafts(user_id, updated_at DESC);

CREATE TABLE page_views (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    page_id uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
    viewed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, page_id)
);
CREATE INDEX idx_page_views_recent ON page_views(user_id, viewed_at DESC);

CREATE TABLE page_stars (
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    page_id uuid NOT NULL REFERENCES pages(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, page_id)
);
CREATE INDEX idx_page_stars_page ON page_stars(page_id);

ALTER TABLE spaces
    ADD CONSTRAINT spaces_home_page_fk FOREIGN KEY (home_page_id) REFERENCES pages(id) ON DELETE SET NULL;

-- +goose Down
ALTER TABLE spaces DROP CONSTRAINT spaces_home_page_fk;
DROP TABLE page_stars;
DROP TABLE page_views;
DROP TABLE page_drafts;
DROP TABLE page_versions;
DROP TABLE pages;
