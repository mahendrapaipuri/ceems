CREATE TABLE IF NOT EXISTS users (
 "id" integer not null primary key,
 "uid" text,
 "cluster_id" text,
 "resource_manager" text,
 "name" text,
 "projects" text default '[]',
 "tags" text default '[]',
 "last_updated_at" text
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_cluster_id_user ON users (cluster_id,name);
