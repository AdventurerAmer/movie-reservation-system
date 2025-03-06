CREATE TABLE IF NOT EXISTS schedules (
    id bigserial PRIMARY KEY,
    created_at TIMESTAMPTZ DEFAULT NOW(),
    movie_id bigint NOT NULL REFERENCES movies(id), 
    hall_id int NOT NULL REFERENCES halls(id),
    price decimal(6, 2) NOT NULL,
    starts_at TIMESTAMPTZ NOT NULL,
    ends_at TIMESTAMPTZ NOT NULL,
    version int NOT NULL DEFAULT 1,
    CONSTRAINT is_valid_schedule CHECK (starts_at >= NOW() AND ends_at >= starts_at)
);

INSERT INTO permissions(code)
VALUES
('schedules:read'),
('schedules:create'),
('schedules:update'),
('schedules:delete')
ON CONFLICT DO NOTHING;
