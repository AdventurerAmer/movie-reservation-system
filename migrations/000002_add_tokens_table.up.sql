CREATE TABLE IF NOT EXISTS token_scopes (
    id smallint PRIMARY KEY,
    scope text NOT NULL UNIQUE
);

INSERT INTO token_scopes (id, scope)
VALUES
    (0, 'activation'),
    (1, 'authentication'),
    (2, 'password-reset')
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS tokens (
    id bigserial PRIMARY KEY,
    user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    scope_id smallint NOT NULL REFERENCES token_scopes(id),
    hash bytea NOT NULL,
    expires_at timestamp(0) with time zone NOT NULL
);