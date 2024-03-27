DROP INDEX IF EXISTS uq_uuid_start;
ALTER TABLE units ADD COLUMN "resource_manager" text default "";
CREATE UNIQUE INDEX IF NOT EXISTS uq_rm_uuid_start ON units (resource_manager,uuid,started_at);
DROP INDEX IF EXISTS uq_project_usr;
ALTER TABLE usage ADD COLUMN "resource_manager" text default "";
CREATE UNIQUE INDEX IF NOT EXISTS uq_rm_project_usr ON usage (resource_manager,usr,project);
