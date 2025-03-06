CREATE TABLE IF NOT EXISTS users(
    id bigserial PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    name text NOT NULL,
    email citext UNIQUE NOT NULL,
    password_hash bytea NOT NULL,
    is_activated boolean NOT NULL DEFAULT false,
    version int NOT NULL DEFAULT 1 
);

CREATE INDEX users_email_index ON users(email);