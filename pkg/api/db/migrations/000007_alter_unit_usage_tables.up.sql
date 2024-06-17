DROP INDEX IF EXISTS uq_rm_uuid_start;
ALTER TABLE units ADD COLUMN "cluster_id" text;
CREATE UNIQUE INDEX IF NOT EXISTS uq_rm_id_uuid_start ON units (cluster_id,uuid,started_at);
DROP INDEX IF EXISTS uq_rm_project_usr;
ALTER TABLE usage ADD COLUMN "cluster_id" text;
CREATE UNIQUE INDEX IF NOT EXISTS uq_rm_id_project_usr ON usage (cluster_id,usr,project);
ALTER TABLE units ADD COLUMN "last_updated_at" text;
