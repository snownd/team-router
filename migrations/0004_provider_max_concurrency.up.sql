-- 0004: providers 增加每渠道最大并发限制（0 = 不限制）
ALTER TABLE providers ADD COLUMN IF NOT EXISTS max_concurrency INT NOT NULL DEFAULT 0;
