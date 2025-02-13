CREATE TABLE IF NOT EXISTS users(
    id bigserial PRIMARY KEY,
    created_at timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    name varchar(50) NOT NULL,
    email citext UNIQUE NOT NULL,
    password_hash bytea NOT NULL,
    is_activated boolean NOT NULL DEFAULT false,
    version integer NOT NULL DEFAULT 1 
);

CREATE INDEX users_email_index ON users(email);