DROP INDEX IF EXISTS uq_rm_project_usr;
ALTER TABLE usage DROP COLUMN resource_manager;
CREATE UNIQUE INDEX IF NOT EXISTS uq_project_usr ON usage (usr,project);
DROP INDEX IF EXISTS uq_rm_uuid_start;
ALTER TABLE units DROP COLUMN resource_manager;
CREATE UNIQUE INDEX IF NOT EXISTS uq_uuid_start ON units (uuid,started_at);
