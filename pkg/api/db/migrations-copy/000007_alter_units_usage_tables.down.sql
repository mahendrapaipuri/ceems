ALTER TABLE units DROP COLUMN "last_updated_at";
DROP INDEX IF EXISTS uq_rm_id_project_usr;
ALTER TABLE usage DROP COLUMN cluster_id;
CREATE UNIQUE INDEX IF NOT EXISTS uq_rm_project_usr ON usage (resource_manager,usr,project);
DROP INDEX IF EXISTS uq_rm_id_uuid_start;
ALTER TABLE units DROP COLUMN cluster_id;
CREATE UNIQUE INDEX IF NOT EXISTS uq_rm_uuid_start ON units (resource_manager,uuid,started_at);
