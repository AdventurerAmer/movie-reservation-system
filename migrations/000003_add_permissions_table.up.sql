CREATE TABLE IF NOT EXISTS permissions (
    id serial PRIMARY KEY,
    code text NOT NULL UNIQUE
);

INSERT INTO permissions(code)
VALUES
('users:read'),
('users:create'),
('users:update'),
('users:delete');

CREATE TABLE IF NOT EXISTS users_permissions (
    user_id bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    permission_id integer NOT NULL REFERENCES permissions(id) ON DELETE CASCADE,
    PRIMARY KEY (user_id, permission_id)
)