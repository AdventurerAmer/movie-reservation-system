CREATE TABLE IF NOT EXISTS cinemas (
    id serial PRIMARY KEY,
    name text NOT NULL,
    location text NOT NULL,
    owner_id bigint NOT NULL REFERENCES users(id),
    version int NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS halls (
    id serial PRIMARY KEY,
    name text NOT NULL,
    cinema_id int NOT NULL REFERENCES cinemas(id),
    seat_arrangement text NOT NULL,
    seat_price decimal(6, 2) NOT NULL,
    version int NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS seats (
    id serial PRIMARY KEY,
    coordinates text NOT NULL,
    hall_id int NOT NULL REFERENCES halls(id),
    version int NOT NULL DEFAULT 1
);

CREATE INDEX IF NOT EXISTS cinemas_name_idx ON cinemas USING GIN (to_tsvector('simple', name));
CREATE INDEX IF NOT EXISTS cinemas_location_idx ON cinemas USING GIN (to_tsvector('simple', location));

INSERT INTO permissions(code)
VALUES
('cinemas:read'),
('cinemas:create'),
('cinemas:update'),
('cinemas:delete')
ON CONFLICT DO NOTHING;