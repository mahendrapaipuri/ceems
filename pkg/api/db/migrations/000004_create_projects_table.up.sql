CREATE TABLE IF NOT EXISTS projects (
 "id" integer not null primary key,
 "uid" text,
 "cluster_id" text,
 "resource_manager" text,
 "name" text,
 "users" text default '[]',
 "tags" text default '[]',
 "last_updated_at" text
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_cluster_id_project ON projects (cluster_id,name);
