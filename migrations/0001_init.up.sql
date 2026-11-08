-- 0001: 基础表（用户 / 部门闭包表 / API Key / Provider / 定价）
-- 第一期全局单组织：departments 树根 parent_id = NULL，无组织表。

CREATE TABLE IF NOT EXISTS users (
    id              BIGSERIAL PRIMARY KEY,
    username        TEXT NOT NULL UNIQUE,
    display_name    TEXT NOT NULL DEFAULT '',
    email           TEXT NOT NULL DEFAULT '',
    password_hash   TEXT NOT NULL DEFAULT '',
    role            TEXT NOT NULL DEFAULT 'user',
    status          INT  NOT NULL DEFAULT 1,
    quota           BIGINT NOT NULL DEFAULT 0,
    used_quota      BIGINT NOT NULL DEFAULT 0,
    sso_provider    TEXT NOT NULL DEFAULT '',
    sso_external_id TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_users_sso ON users (sso_provider, sso_external_id) WHERE sso_provider <> '';

-- 部门（闭包表实现层级）
CREATE TABLE IF NOT EXISTS departments (
    id         BIGSERIAL PRIMARY KEY,
    parent_id  BIGINT REFERENCES departments (id) ON DELETE CASCADE,
    name       TEXT NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    status     INT NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_departments_parent_name UNIQUE (parent_id, name)
);

-- 闭包表：每个节点对自身一行（depth=0），对每个祖先一行
CREATE TABLE IF NOT EXISTS department_closure (
    ancestor_id   BIGINT NOT NULL REFERENCES departments (id) ON DELETE CASCADE,
    descendant_id BIGINT NOT NULL REFERENCES departments (id) ON DELETE CASCADE,
    depth         INT NOT NULL,
    PRIMARY KEY (ancestor_id, descendant_id)
);

CREATE INDEX IF NOT EXISTS idx_closure_descendant ON department_closure (descendant_id);

-- 用户多部门关联
CREATE TABLE IF NOT EXISTS user_departments (
    user_id    BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    dept_id    BIGINT NOT NULL REFERENCES departments (id) ON DELETE CASCADE,
    is_primary BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, dept_id)
);

CREATE INDEX IF NOT EXISTS idx_user_departments_dept ON user_departments (dept_id);

-- API Key
CREATE TABLE IF NOT EXISTS api_keys (
    id             BIGSERIAL PRIMARY KEY,
    user_id        BIGINT NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    name           TEXT NOT NULL DEFAULT '',
    key_prefix     TEXT NOT NULL,
    key_hash       TEXT NOT NULL UNIQUE,
    status         INT NOT NULL DEFAULT 1,
    quota          BIGINT NOT NULL DEFAULT 0,   -- 0 表示不限额（受用户总额度约束）
    used_quota     BIGINT NOT NULL DEFAULT 0,
    allowed_models JSONB,
    expires_at     TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys (user_id);

-- Provider（渠道）
CREATE TABLE IF NOT EXISTS providers (
    id             BIGSERIAL PRIMARY KEY,
    name           TEXT NOT NULL UNIQUE,
    type           TEXT NOT NULL,               -- openai | anthropic | custom_openai ...
    base_url       TEXT NOT NULL,
    api_key        TEXT NOT NULL DEFAULT '',     -- AES-GCM 加密后的上游密钥
    models         JSONB NOT NULL DEFAULT '[]',
    model_mapping  JSONB NOT NULL DEFAULT '{}',
    priority       INT NOT NULL DEFAULT 0,
    weight         INT NOT NULL DEFAULT 1,
    status         INT NOT NULL DEFAULT 1,
    proxy_url      TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_providers_status ON providers (status);

-- 模型定价：每百万 token 单价（微美元）
CREATE TABLE IF NOT EXISTS model_prices (
    id               BIGSERIAL PRIMARY KEY,
    model_name       TEXT NOT NULL,
    provider_type    TEXT NOT NULL DEFAULT '',   -- 空 = 通配
    input_per_m      BIGINT NOT NULL DEFAULT 0,
    output_per_m     BIGINT NOT NULL DEFAULT 0,
    cache_read_per_m BIGINT NOT NULL DEFAULT 0,
    cache_write_per_m BIGINT NOT NULL DEFAULT 0,
    currency         TEXT NOT NULL DEFAULT 'USD',
    status           INT NOT NULL DEFAULT 1,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT uq_model_prices UNIQUE (model_name, provider_type)
);
