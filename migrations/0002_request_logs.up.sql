-- 0002: request_logs 按月 RANGE 分区表
-- 注意：分区表上 identity 列在 PG17 之前不支持，因此使用独立 sequence 提供默认值。
-- 父表上的索引会自动应用到所有（含未来）分区（PG11+）。

CREATE SEQUENCE IF NOT EXISTS request_logs_id_seq;

CREATE TABLE IF NOT EXISTS request_logs (
    id                 BIGINT NOT NULL DEFAULT nextval('request_logs_id_seq'),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    request_id         TEXT NOT NULL DEFAULT '',
    user_id            BIGINT NOT NULL DEFAULT 0,
    username           TEXT NOT NULL DEFAULT '',
    api_key_id         BIGINT NOT NULL DEFAULT 0,
    provider_id        BIGINT NOT NULL DEFAULT 0,
    provider_name      TEXT NOT NULL DEFAULT '',
    upstream_type      TEXT NOT NULL DEFAULT '',
    protocol           TEXT NOT NULL DEFAULT '',
    model              TEXT NOT NULL DEFAULT '',
    upstream_model     TEXT NOT NULL DEFAULT '',
    stream             BOOLEAN NOT NULL DEFAULT FALSE,
    status_code        INT NOT NULL DEFAULT 0,
    is_error           BOOLEAN NOT NULL DEFAULT FALSE,
    err_msg            TEXT NOT NULL DEFAULT '',
    input_tokens       INT NOT NULL DEFAULT 0,
    output_tokens      INT NOT NULL DEFAULT 0,
    reasoning_tokens   INT NOT NULL DEFAULT 0,
    cache_read_tokens  INT NOT NULL DEFAULT 0,
    cache_write_tokens INT NOT NULL DEFAULT 0,
    ttft_ms            INT NOT NULL DEFAULT 0,
    total_ms           INT NOT NULL DEFAULT 0,
    retries            INT NOT NULL DEFAULT 0,
    quota_cost         BIGINT NOT NULL DEFAULT 0,
    billing_detail     JSONB,
    client_ip          TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

CREATE INDEX IF NOT EXISTS idx_request_logs_created ON request_logs (created_at);
CREATE INDEX IF NOT EXISTS idx_request_logs_user ON request_logs (user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_request_logs_api_key ON request_logs (api_key_id, created_at);
CREATE INDEX IF NOT EXISTS idx_request_logs_provider ON request_logs (provider_id, created_at);
CREATE INDEX IF NOT EXISTS idx_request_logs_model ON request_logs (model, created_at);

-- 兜底分区，避免分区管理器未及时创建时丢数据
CREATE TABLE IF NOT EXISTS request_logs_default PARTITION OF request_logs DEFAULT;
