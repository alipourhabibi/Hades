INSERT INTO resources (id, resource_type)
SELECT id, 'module' FROM modules
ON CONFLICT DO NOTHING;

INSERT INTO resources (id, resource_type)
SELECT id, 'commit' FROM commits
ON CONFLICT DO NOTHING;
