CREATE TABLE IF NOT EXISTS movies (
    id bigserial PRIMARY KEY,
    created_at timestamp(0) WITH TIME ZONE NOT NULL DEFAULT NOW(),
    title text NOT NULL,
    runtime int NOT NULL,
    year int NOT NULL,
    genres text[] NOT NULL,
    version int NOT NULL DEFAULT 1
);

INSERT INTO permissions(code)
VALUES
('movies:read'),
('movies:create'),
('movies:update'),
('movies:delete')
ON CONFLICT DO NOTHING;

CREATE INDEX IF NOT EXISTS movies_title_idx ON movies USING GIN (to_tsvector('simple', title));
CREATE INDEX IF NOT EXISTS movies_genres_idx ON movies USING GIN (genres);