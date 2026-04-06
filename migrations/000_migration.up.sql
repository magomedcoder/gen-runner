CREATE TABLE IF NOT EXISTS users
(
    id              SERIAL PRIMARY KEY,
    username        VARCHAR(255) UNIQUE NOT NULL,
    password        VARCHAR(255)        NOT NULL,
    name            VARCHAR(255)        NOT NULL,
    surname         VARCHAR(255)        NOT NULL,
    role            INTEGER             NOT NULL DEFAULT 0,
    created_at      TIMESTAMP           NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP           NOT NULL DEFAULT NOW(),
    last_visited_at TIMESTAMP           NULL,
    deleted_at      TIMESTAMP           NULL
);

CREATE TABLE IF NOT EXISTS user_sessions
(
    id         SERIAL PRIMARY KEY,
    user_id    INTEGER     NOT NULL REFERENCES users (id),
    token      TEXT        NOT NULL UNIQUE,
    type       VARCHAR(20) NOT NULL,
    expires_at TIMESTAMP   NOT NULL,
    created_at TIMESTAMP   NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP   NULL
);

CREATE TABLE IF NOT EXISTS chats
(
    id              BIGSERIAL PRIMARY KEY,
    user_id         INTEGER      NOT NULL REFERENCES users (id),
    title           VARCHAR(500) NOT NULL,
    model           VARCHAR(255) NOT NULL DEFAULT '',
    selected_runner VARCHAR(255) NOT NULL DEFAULT '',
    system_prompt   TEXT         NOT NULL DEFAULT '',
    stop_sequences  TEXT[]       NOT NULL DEFAULT '{}',
    timeout_seconds INTEGER      NOT NULL DEFAULT 0,
    temperature     REAL         NULL,
    top_k           INTEGER      NULL,
    top_p           REAL         NULL,
    json_mode       BOOLEAN      NOT NULL DEFAULT FALSE,
    json_schema     TEXT         NOT NULL DEFAULT '',
    tools_json      TEXT         NOT NULL DEFAULT '',
    profile         VARCHAR(64)  NOT NULL DEFAULT '',
    created_at      TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP    NOT NULL DEFAULT NOW(),
    deleted_at      TIMESTAMP    NULL
);

CREATE TABLE IF NOT EXISTS files
(
    id                            BIGSERIAL PRIMARY KEY,
    filename                      VARCHAR(255) NOT NULL,
    mime_type                     VARCHAR(100) NULL,
    size                          BIGINT       NOT NULL DEFAULT 0,
    storage_path                  TEXT         NOT NULL,
    chat_session_id               BIGINT       NULL REFERENCES chats (id) ON DELETE SET NULL,
    user_id                       INTEGER      NULL REFERENCES users (id) ON DELETE SET NULL,
    expires_at                    TIMESTAMP    NULL,
    kind                          VARCHAR(32)  NOT NULL DEFAULT '',
    created_at                    TIMESTAMP    NOT NULL DEFAULT NOW(),
    extracted_text                TEXT         NULL,
    extracted_text_content_sha256 VARCHAR(64)  NULL
);

CREATE TABLE IF NOT EXISTS messages
(
    id                 BIGSERIAL PRIMARY KEY,
    session_id         BIGINT      NOT NULL REFERENCES chats (id) ON DELETE CASCADE,
    content            TEXT        NOT NULL,
    role               VARCHAR(20) NOT NULL,
    attachment_file_id BIGINT      NULL REFERENCES files (id) ON DELETE SET NULL,
    tool_call_id       TEXT        NULL,
    tool_name          TEXT        NULL,
    tool_calls_json    TEXT        NULL,
    created_at         TIMESTAMP   NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMP   NOT NULL DEFAULT NOW(),
    deleted_at         TIMESTAMP   NULL
);

CREATE TABLE IF NOT EXISTS editor_text_history
(
    id         BIGSERIAL PRIMARY KEY,
    user_id    INTEGER      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    runner     VARCHAR(255) NOT NULL DEFAULT '',
    text       TEXT         NOT NULL,
    created_at TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS message_edits
(
    id                   BIGSERIAL PRIMARY KEY,
    session_id           BIGINT      NOT NULL REFERENCES chats (id) ON DELETE CASCADE,
    message_id           BIGINT      NOT NULL REFERENCES messages (id) ON DELETE CASCADE,
    editor_user_id       INTEGER     NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    kind                 VARCHAR(32) NOT NULL DEFAULT 'user_edit',
    old_content          TEXT        NOT NULL,
    new_content          TEXT        NOT NULL,
    soft_deleted_from_id BIGINT      NULL,
    soft_deleted_to_id   BIGINT      NULL,
    created_at           TIMESTAMP   NOT NULL DEFAULT NOW(),
    reverted_at          TIMESTAMP   NULL
);

CREATE TABLE IF NOT EXISTS user_runner_models
(
    user_id        INTEGER      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    runner_address VARCHAR(255) NOT NULL,
    model          VARCHAR(255) NOT NULL DEFAULT '',
    updated_at     TIMESTAMP    NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, runner_address)
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users (username);
CREATE INDEX IF NOT EXISTS idx_users_role ON users (role);
CREATE INDEX IF NOT EXISTS idx_users_deleted_at ON users (deleted_at);
CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions (user_id);
CREATE INDEX IF NOT EXISTS idx_user_sessions_token ON user_sessions (token);
CREATE INDEX IF NOT EXISTS idx_user_sessions_expires_at ON user_sessions (expires_at);
CREATE INDEX IF NOT EXISTS idx_user_sessions_deleted_at ON user_sessions (deleted_at);
CREATE INDEX IF NOT EXISTS idx_chats_user_id ON chats (user_id);
CREATE INDEX IF NOT EXISTS idx_chats_created_at ON chats (created_at);
CREATE INDEX IF NOT EXISTS idx_chats_deleted_at ON chats (deleted_at);
CREATE INDEX IF NOT EXISTS idx_files_created_at ON files (created_at);
CREATE INDEX IF NOT EXISTS idx_files_expires_at ON files (expires_at) WHERE expires_at IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_files_chat_session_kind ON files (chat_session_id, kind) WHERE chat_session_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_messages_session_id ON messages (session_id);
CREATE INDEX IF NOT EXISTS idx_messages_role ON messages (role);
CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages (created_at);
CREATE INDEX IF NOT EXISTS idx_messages_deleted_at ON messages (deleted_at);
CREATE INDEX IF NOT EXISTS idx_messages_attachment_file_id ON messages (attachment_file_id);
CREATE INDEX IF NOT EXISTS idx_messages_session_created_active ON messages (session_id, created_at) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_messages_session_id_active ON messages (session_id, id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_editor_text_history_user_id ON editor_text_history (user_id);
CREATE INDEX IF NOT EXISTS idx_editor_text_history_created_at ON editor_text_history (created_at);
CREATE INDEX IF NOT EXISTS idx_user_runner_models_user_id ON user_runner_models (user_id);
CREATE INDEX IF NOT EXISTS idx_message_edits_message_id_created_at ON message_edits (message_id, created_at);
CREATE INDEX IF NOT EXISTS idx_message_edits_session_id_created_at ON message_edits (session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_message_edits_kind_created_at ON message_edits (kind, created_at);
