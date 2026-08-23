-- Admin AI assistant: restaurant-scoped chat with proposed actions.
-- One (lazily created) thread per restaurant for v1.

CREATE TABLE assistant_threads (
    id            uuid PRIMARY KEY,
    restaurant_id uuid NOT NULL REFERENCES restaurants (id) ON DELETE CASCADE,
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX assistant_threads_restaurant_idx ON assistant_threads (restaurant_id);

CREATE TABLE assistant_messages (
    id            uuid PRIMARY KEY,
    thread_id     uuid NOT NULL REFERENCES assistant_threads (id) ON DELETE CASCADE,
    role          text NOT NULL, -- 'user' | 'assistant'
    text          text NOT NULL DEFAULT '',
    attachments   jsonb NOT NULL DEFAULT '[]'::jsonb, -- [{name, url, mime}]
    actions       jsonb NOT NULL DEFAULT '[]'::jsonb,
    action_status text, -- NULL | 'applied' | 'discarded'
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX assistant_messages_thread_idx ON assistant_messages (thread_id, created_at);
