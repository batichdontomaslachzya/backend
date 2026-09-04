CREATE TABLE users (
    id UUID PRIMARY KEY,
    username TEXT NOT NULL UNIQUE,
    salt BYTEA NOT NULL CHECK (octet_length(salt) = 16),
    password_hash BYTEA NOT NULL CHECK (octet_length(password_hash) = 32),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tasks (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id),
    status TEXT NOT NULL CHECK (status IN ('in_progress', 'ready', 'failed')),
    media_type TEXT NOT NULL CHECK (media_type IN ('image/png', 'image/jpeg')),
    filter JSONB NOT NULL,
    input_path TEXT NOT NULL,
    result_path TEXT NOT NULL,
    error TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    completed_at TIMESTAMPTZ,
    published_at TIMESTAMPTZ
);

CREATE INDEX tasks_user_id_idx ON tasks(user_id);
-- Сама запись задачи служит outbox: её нельзя потерять до отправки в Kafka.
CREATE INDEX tasks_unpublished_idx ON tasks(created_at)
    WHERE published_at IS NULL AND status = 'in_progress';
