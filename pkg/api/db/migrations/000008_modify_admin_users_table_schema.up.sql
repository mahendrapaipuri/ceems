ALTER TABLE admin_users RENAME TO tmp_admin_users;
CREATE TABLE IF NOT EXISTS admin_users (
 "id" integer not null primary key,
 "uid" text default '',
 "cluster_id" text default 'all',
 "resource_manager" text default '',
 "name" text,
 "projects" text default '[]',
 "tags" text default '[]',
 "last_updated_at" text
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_cluster_id_admin_user ON admin_users (cluster_id,name);
INSERT INTO admin_users (name,tags,last_updated_at) SELECT value, json_array(source),last_updated_at FROM tmp_admin_users, json_each(users) WHERE value != '';
DROP TABLE IF EXISTS tmp_admin_users;
