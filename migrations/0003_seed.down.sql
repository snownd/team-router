DELETE FROM department_closure WHERE ancestor_id = 1 AND descendant_id = 1 AND depth = 0;
DELETE FROM model_prices WHERE model_name IN (
    'gpt-4o','gpt-4o-mini','gpt-4.1','gpt-4.1-mini','o3-mini',
    'claude-sonnet-4-5','claude-3-7-sonnet','claude-3-5-haiku'
);
