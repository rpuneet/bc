-- mycel unified database schema — relational + time-series in one database
-- Uses TimescaleDB extension for hypertables (time-series metrics)
-- All tables use standard SQL compatible with both SQLite and Postgres

-- ============================================================================
-- TimescaleDB extension (no-op if already enabled)
-- ============================================================================
CREATE EXTENSION IF NOT EXISTS timescaledb;

-- ============================================================================
-- Relational tables (agents, channels, costs, events, cron, etc.)
-- ============================================================================

CREATE TABLE IF NOT EXISTS agents (
    name          TEXT PRIMARY KEY,
    role          TEXT NOT NULL,
    state         TEXT NOT NULL DEFAULT 'idle',
    tool          TEXT,
    parent_id     TEXT,
    team          TEXT,
    task          TEXT,
    session       TEXT,
    workspace     TEXT NOT NULL,
    worktree_dir  TEXT,
    log_file      TEXT,
    hooked_work   TEXT,
    children      TEXT,
    is_root       INTEGER NOT NULL DEFAULT 0,
    crash_count   INTEGER NOT NULL DEFAULT 0,
    last_crash_time TEXT,
    recovered_from  TEXT,
    runtime_backend TEXT,
    session_id    TEXT,
    created_at    TEXT,
    stopped_at    TEXT,
    started_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS channels (
    id            SERIAL PRIMARY KEY,
    name          TEXT NOT NULL UNIQUE,
    type          TEXT DEFAULT 'group',
    description   TEXT DEFAULT '',
    created_at    TEXT NOT NULL DEFAULT TO_CHAR(NOW(), 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
    updated_at    TEXT NOT NULL DEFAULT TO_CHAR(NOW(), 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
);

CREATE TABLE IF NOT EXISTS channel_members (
    channel_id    INTEGER REFERENCES channels(id),
    agent_id      TEXT NOT NULL,
    joined_at     TEXT NOT NULL,
    last_read_msg_id INTEGER DEFAULT 0,
    PRIMARY KEY (channel_id, agent_id)
);

CREATE TABLE IF NOT EXISTS messages (
    id            SERIAL PRIMARY KEY,
    channel_id    INTEGER REFERENCES channels(id),
    sender        TEXT NOT NULL,
    content       TEXT NOT NULL,
    type          TEXT DEFAULT 'text',
    metadata      TEXT DEFAULT '',
    created_at    TEXT NOT NULL DEFAULT TO_CHAR(NOW(), 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
);

CREATE TABLE IF NOT EXISTS cost_records (
    id            SERIAL PRIMARY KEY,
    agent_id      TEXT,
    team_id       TEXT,
    model         TEXT,
    input_tokens  INTEGER DEFAULT 0,
    output_tokens INTEGER DEFAULT 0,
    total_tokens  INTEGER DEFAULT 0,
    cost_usd      REAL DEFAULT 0,
    session_id    TEXT,
    cache_creation_tokens INTEGER DEFAULT 0,
    cache_read_tokens     INTEGER DEFAULT 0,
    timestamp     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS events (
    id            SERIAL PRIMARY KEY,
    type          TEXT NOT NULL,
    agent         TEXT,
    message       TEXT,
    data          TEXT,
    timestamp     TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS cron_jobs (
    name          TEXT PRIMARY KEY,
    schedule      TEXT NOT NULL,
    agent_name    TEXT,
    prompt        TEXT,
    command       TEXT,
    enabled       INTEGER NOT NULL DEFAULT 1,
    last_run      TEXT,
    next_run      TEXT,
    run_count     INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL DEFAULT TO_CHAR(NOW(), 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
);

CREATE TABLE IF NOT EXISTS mcp_servers (
    name          TEXT PRIMARY KEY,
    transport     TEXT NOT NULL DEFAULT 'stdio',
    command       TEXT,
    args          TEXT,
    url           TEXT,
    env           TEXT,
    enabled       INTEGER NOT NULL DEFAULT 1,
    created_at    TEXT NOT NULL DEFAULT TO_CHAR(NOW(), 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
);

CREATE TABLE IF NOT EXISTS secrets (
    name          TEXT PRIMARY KEY,
    value         TEXT NOT NULL,
    description   TEXT DEFAULT '',
    created_at    TEXT NOT NULL,
    updated_at    TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tools (
    name          TEXT PRIMARY KEY,
    command       TEXT NOT NULL DEFAULT '',
    install_cmd   TEXT,
    upgrade_cmd   TEXT,
    slash_cmds    TEXT,
    mcp_servers   TEXT,
    config        TEXT,
    builtin       INTEGER NOT NULL DEFAULT 0,
    enabled       INTEGER NOT NULL DEFAULT 1,
    created_at    TEXT NOT NULL DEFAULT TO_CHAR(NOW(), 'YYYY-MM-DD"T"HH24:MI:SS"Z"')
);

CREATE TABLE IF NOT EXISTS daemons (
    name          TEXT PRIMARY KEY,
    runtime       TEXT NOT NULL,
    cmd           TEXT,
    image         TEXT,
    status        TEXT NOT NULL DEFAULT 'stopped',
    pid           INTEGER,
    container_id  TEXT,
    ports         TEXT,
    env           TEXT,
    restart       TEXT DEFAULT 'no',
    created_at    TEXT NOT NULL,
    started_at    TEXT,
    stopped_at    TEXT
);

-- Seed default channels
INSERT INTO channels (name, type, description) VALUES
    ('general',     'group', 'General discussion for all agents'),
    ('engineering', 'group', 'Engineering team coordination'),
    ('all',         'group', 'Broadcast channel for announcements')
ON CONFLICT (name) DO NOTHING;

-- Relational indexes
CREATE INDEX IF NOT EXISTS idx_agents_state ON agents(state);
CREATE INDEX IF NOT EXISTS idx_agents_role ON agents(role);
CREATE INDEX IF NOT EXISTS idx_messages_channel ON messages(channel_id);
CREATE INDEX IF NOT EXISTS idx_messages_sender ON messages(sender);
CREATE INDEX IF NOT EXISTS idx_cost_records_agent ON cost_records(agent_id);
CREATE INDEX IF NOT EXISTS idx_cost_records_model ON cost_records(model);
CREATE INDEX IF NOT EXISTS idx_cost_records_timestamp ON cost_records(timestamp);
CREATE INDEX IF NOT EXISTS idx_events_agent ON events(agent);
CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp);

-- ============================================================================
-- Time-series tables (TimescaleDB hypertables)
-- ============================================================================

-- System Metrics — mycel-daemon, mycel-db containers
CREATE TABLE IF NOT EXISTS system_metrics (
    time             TIMESTAMPTZ NOT NULL,
    system_name      TEXT NOT NULL,
    cpu_percent      DOUBLE PRECISION NOT NULL DEFAULT 0,
    mem_used_bytes   BIGINT NOT NULL DEFAULT 0,
    mem_limit_bytes  BIGINT NOT NULL DEFAULT 0,
    mem_percent      DOUBLE PRECISION NOT NULL DEFAULT 0,
    net_rx_bytes     BIGINT NOT NULL DEFAULT 0,
    net_tx_bytes     BIGINT NOT NULL DEFAULT 0,
    disk_read_bytes  BIGINT NOT NULL DEFAULT 0,
    disk_write_bytes BIGINT NOT NULL DEFAULT 0
);
SELECT create_hypertable('system_metrics', 'time', if_not_exists => TRUE);

-- Agent Metrics — per-agent container stats
CREATE TABLE IF NOT EXISTS agent_metrics (
    time             TIMESTAMPTZ NOT NULL,
    agent_name       TEXT NOT NULL,
    role             TEXT NOT NULL DEFAULT '',
    tool             TEXT NOT NULL DEFAULT '',
    runtime          TEXT NOT NULL DEFAULT 'docker',
    state            TEXT NOT NULL DEFAULT '',
    cpu_percent      DOUBLE PRECISION NOT NULL DEFAULT 0,
    mem_used_bytes   BIGINT NOT NULL DEFAULT 0,
    mem_limit_bytes  BIGINT NOT NULL DEFAULT 0,
    mem_percent      DOUBLE PRECISION NOT NULL DEFAULT 0,
    net_rx_bytes     BIGINT NOT NULL DEFAULT 0,
    net_tx_bytes     BIGINT NOT NULL DEFAULT 0,
    disk_read_bytes  BIGINT NOT NULL DEFAULT 0,
    disk_write_bytes BIGINT NOT NULL DEFAULT 0
);
SELECT create_hypertable('agent_metrics', 'time', if_not_exists => TRUE);

-- Token Metrics — per-agent token consumption from JSONL
CREATE TABLE IF NOT EXISTS token_metrics (
    time          TIMESTAMPTZ NOT NULL,
    agent_name    TEXT NOT NULL DEFAULT '',
    model         TEXT NOT NULL DEFAULT '',
    input_tokens  BIGINT NOT NULL DEFAULT 0,
    output_tokens BIGINT NOT NULL DEFAULT 0,
    cache_read    BIGINT NOT NULL DEFAULT 0,
    cache_create  BIGINT NOT NULL DEFAULT 0,
    cost_usd      DOUBLE PRECISION NOT NULL DEFAULT 0,
    UNIQUE (time, agent_name, model)
);
SELECT create_hypertable('token_metrics', 'time', if_not_exists => TRUE);

-- Channel Metrics — message/member/reaction counts
CREATE TABLE IF NOT EXISTS channel_metrics (
    time           TIMESTAMPTZ NOT NULL,
    channel_name   TEXT NOT NULL,
    message_count  BIGINT NOT NULL DEFAULT 0,
    member_count   INT NOT NULL DEFAULT 0,
    reaction_count BIGINT NOT NULL DEFAULT 0
);
SELECT create_hypertable('channel_metrics', 'time', if_not_exists => TRUE);

-- Retention policies
SELECT add_retention_policy('system_metrics',  INTERVAL '7 days',  if_not_exists => TRUE);
SELECT add_retention_policy('agent_metrics',   INTERVAL '7 days',  if_not_exists => TRUE);
SELECT add_retention_policy('token_metrics',   INTERVAL '30 days', if_not_exists => TRUE);
SELECT add_retention_policy('channel_metrics', INTERVAL '30 days', if_not_exists => TRUE);
