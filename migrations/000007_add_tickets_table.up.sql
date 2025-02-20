CREATE TABLE IF NOT EXISTS ticket_states (
    id smallint PRIMARY KEY,
    state text NOT NULL UNIQUE
);

INSERT INTO ticket_states(id, state)
VALUES (0, 'unsold'),
       (1, 'locked'),
       (2, 'sold')
ON CONFLICT DO NOTHING;

CREATE TABLE IF NOT EXISTS tickets (
    id bigserial PRIMARY KEY,
    created_at timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    schedule_id bigint NOT NULL REFERENCES schedules(id),
    seat_id int NOT NULL REFERENCES seats(id),
    price decimal(6, 2) NOT NULL,
    state_id smallint NOT NULL DEFAULT 0 REFERENCES ticket_states(id),
    state_changed_at timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    version int NOT NULL DEFAULT 1,
    CONSTRAINT unique_ticket UNIQUE (schedule_id, seat_id)
);

CREATE TABLE IF NOT EXISTS tickets_users (
    ticket_id bigint NOT NULL REFERENCES tickets(id),
    user_id bigint NOT NULL REFERENCES users(id),
    expires_at timestamp(0) with time zone NOT NULL DEFAULT NOW() + interval '10 minutes', 
    PRIMARY KEY (ticket_id, user_id)
);