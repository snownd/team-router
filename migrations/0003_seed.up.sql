-- 0003: 种子数据（根部门 + 常用模型默认定价；admin 用户由 Go seeder 创建以生成 bcrypt 哈希）

-- 根部门（单组织树根）
INSERT INTO departments (id, parent_id, name, status)
VALUES (1, NULL, '总部', 1)
ON CONFLICT (id) DO NOTHING;
ALTER SEQUENCE departments_id_seq RESTART WITH 100;
SELECT setval('departments_id_seq', 100, false);

INSERT INTO department_closure (ancestor_id, descendant_id, depth)
VALUES (1, 1, 0)
ON CONFLICT DO NOTHING;

-- 常用模型默认定价（微美元 / 每百万 token）
INSERT INTO model_prices (model_name, provider_type, input_per_m, output_per_m, cache_read_per_m, cache_write_per_m, currency)
VALUES
    ('gpt-4o',        '',          2_500_000, 10_000_000, 1_250_000, 0, 'USD'),
    ('gpt-4o-mini',   '',            150_000,    600_000,    75_000, 0, 'USD'),
    ('gpt-4.1',       '',          2_000_000,  8_000_000,    500_000, 0, 'USD'),
    ('gpt-4.1-mini',  '',            400_000,  1_600_000,    100_000, 0, 'USD'),
    ('o3-mini',       '',          1_100_000,  4_400_000,    550_000, 0, 'USD'),
    ('claude-sonnet-4-5', 'anthropic', 3_000_000, 15_000_000, 300_000, 3_750_000, 'USD'),
    ('claude-3-7-sonnet', 'anthropic', 3_000_000, 15_000_000, 300_000, 3_750_000, 'USD'),
    ('claude-3-5-haiku',  'anthropic',    800_000,  4_000_000,   80_000, 1_000_000, 'USD')
ON CONFLICT (model_name, provider_type) DO NOTHING;
