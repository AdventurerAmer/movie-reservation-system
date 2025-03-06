CREATE TABLE IF NOT EXISTS checkout_sessions (
    user_id bigint NOT NULL UNIQUE REFERENCES users(id),
    session_id text NOT NULL UNIQUE,
    expires_at TIMESTAMP(0) with time zone NOT NULL DEFAULT NOW() + interval '10 minutes',
    PRIMARY KEY (user_id, session_id)
);