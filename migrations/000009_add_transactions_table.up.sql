CREATE TABLE IF NOT EXISTS transactions (
    id bigserial PRIMARY KEY,
    ticket_id bigint NOT NULL REFERENCES tickets(id),
    user_id bigint NOT NULL REFERENCES users(id)
);