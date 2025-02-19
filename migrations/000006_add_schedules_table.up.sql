CREATE TABLE IF NOT EXISTS schedules (
    id bigserial PRIMARY KEY,
    created_at timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    movie_id bigint NOT NULL REFERENCES movies(id), 
    hall_id int NOT NULL REFERENCES halls(id),
    price decimal(6, 2) NOT NULL,
    starts_at timestamp(0) with time zone NOT NULL,
    ends_at timestamp(0) with time zone NOT NULL,
    version int NOT NULL DEFAULT 1,
    CONSTRAINT is_valid_schedule CHECK (starts_at >= NOW() AND ends_at >= starts_at)
);